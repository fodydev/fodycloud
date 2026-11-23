// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/service"
)

// storageNames are the dashboard's names of each storage kind.
var storageNames = map[string]string{"volume": "Persistent volume", "postgres": "PostgreSQL database", "valkey": "Valkey cache"}

// storageRow is one line of the app page's Storage and databases card: a
// service the app has, or one its plan offers.
type storageRow struct {
	Kind, Name, Hint string
	Included         bool                // in the plan
	Limit            string              // the plan's limit
	Service          *service.AppService // nil = not added
}

var storageHints = map[string]string{
	"volume":   "Files your app writes in /data are kept across deployments and restarts.",
	"postgres": "A database for your app. Its address is in the DATABASE_URL variable.",
	"valkey":   "An in-memory cache (Redis-compatible), emptied when it restarts. Its address is in REDIS_URL.",
}

func storageRows(have []service.AppService, p plans.Plan) []storageRow {
	rows := make([]storageRow, 0, len(service.ServiceKinds))
	for _, kind := range service.ServiceKinds {
		r := storageRow{Kind: kind, Name: storageNames[kind], Hint: storageHints[kind], Included: p.Has(kind)}
		switch kind {
		case "volume":
			r.Limit = p.VolumeSize
		case "postgres":
			r.Limit = p.DBSize
		default:
			r.Limit = p.CacheMemory
		}
		for i := range have {
			if have[i].Kind == kind {
				r.Service = &have[i]
			}
		}
		rows = append(rows, r)
	}
	return rows
}
