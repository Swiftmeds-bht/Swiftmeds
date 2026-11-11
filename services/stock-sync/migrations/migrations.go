// Package migrations embeds Stock Sync's SQL migrations for startup.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
