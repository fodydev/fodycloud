// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/service"
	"github.com/fodydev/fodycloud/internal/sse"
)

// Live build logs as server-sent events. Events:
//
//	event: log     id: <offset>  data: <new output>
//	event: status  data: <deployment status>
//	event: end     data: <final status>   (the deployment is finished)
//
// Each connection lasts at most StreamWindow, below the reverse proxy's
// write timeout; clients reconnect with Last-Event-ID (EventSource does it
// by itself) and resume from that offset.
const (
	StreamWindow    = 50 * time.Second
	streamHeartbeat = 15 * time.Second
)

var (
	stopOnce    sync.Once
	stopStreams = make(chan struct{})
)

// StopStreams ends every open event stream (on server shutdown, which
// otherwise waits for them); clients reconnect to the next server.
func StopStreams() { stopOnce.Do(func() { close(stopStreams) }) }

// errStreamed tells respond that the handler already wrote the response.
var errStreamed = errors.New("response streamed")

// WantsStream reports whether the client asked for an event stream.
func WantsStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// StreamFrom is the offset a stream resumes from: Last-Event-ID, else
// ?from=, else 0.
func StreamFrom(r *http.Request) int64 {
	v := r.Header.Get("Last-Event-ID")
	if v == "" {
		v = r.URL.Query().Get("from")
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// StreamBuildLog follows a deployment's log and writes it as events. Errors
// before the stream starts (not found, too many streams) are returned for
// the caller to report; after that, they end the stream.
func StreamBuildLog(w http.ResponseWriter, r *http.Request, svc *service.Service, u *auth.User, name string, id int64) error {
	ctx, cancel := context.WithTimeout(r.Context(), StreamWindow)
	defer cancel()
	events, err := svc.FollowBuildLog(ctx, u, name, id, StreamFrom(r))
	if err != nil {
		return err
	}
	st, err := sse.Start(w, 1000)
	if err != nil {
		return err
	}
	beat := time.NewTicker(streamHeartbeat)
	defer beat.Stop()
	for {
		select {
		case <-stopStreams:
			return nil
		case <-beat.C:
			if st.Comment("keep-alive") != nil {
				return nil
			}
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			var werr error
			switch {
			case ev.Done:
				return st.Event("end", "", ev.Status)
			case ev.Text != "":
				werr = st.Event("log", strconv.FormatInt(ev.End, 10), service.StripANSI(ev.Text))
			case ev.Status != "":
				werr = st.Event("status", "", ev.Status)
			}
			if werr != nil {
				return nil
			}
		}
	}
}
