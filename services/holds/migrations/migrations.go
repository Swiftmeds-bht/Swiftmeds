// Package migrations embeds the holds service's SQL migrations so the binary
// can apply them at startup with pkg/migrate.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
