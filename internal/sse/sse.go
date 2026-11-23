// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sse writes server-sent events (text/event-stream), used to push
// build logs to the dashboard and API clients as they are written.
package sse

import (
	"fmt"
	"net/http"
	"strings"
)

// Stream is an open event stream.
type Stream struct {
	w http.ResponseWriter
	f http.Flusher
}

// Start sends the stream's headers and asks the client to reconnect after
// retryMS milliseconds when the connection ends. It fails if the response
// can't be flushed.
func Start(w http.ResponseWriter, retryMS int) (*Stream, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // no buffering by proxies
	w.WriteHeader(http.StatusOK)
	s := &Stream{w: w, f: f}
	fmt.Fprintf(w, "retry: %d\n\n", retryMS)
	f.Flush()
	return s, nil
}

// Event sends one event. The data may span several lines; line breaks of
// any kind (\n, \r\n, \r) arrive as \n. An empty id is left out.
func (s *Stream) Event(name, id, data string) error {
	_, err := s.w.Write([]byte(Encode(name, id, data)))
	s.f.Flush()
	return err
}

// Comment sends a comment line, which keeps idle connections open.
func (s *Stream) Comment(text string) error {
	_, err := fmt.Fprintf(s.w, ": %s\n\n", text)
	s.f.Flush()
	return err
}

// Encode formats one event.
func Encode(name, id, data string) string {
	var b strings.Builder
	if name != "" {
		b.WriteString("event: " + name + "\n")
	}
	if id != "" {
		b.WriteString("id: " + id + "\n")
	}
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	for _, line := range strings.Split(data, "\n") {
		b.WriteString("data: " + line + "\n")
	}
	b.WriteString("\n")
	return b.String()
}
