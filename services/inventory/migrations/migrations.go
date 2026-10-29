// Package migrations embeds Inventory's SQL migrations for startup.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
