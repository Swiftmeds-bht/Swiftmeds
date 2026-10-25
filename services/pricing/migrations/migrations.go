// Package migrations embeds the pricing service's SQL migrations so the binary
// can apply them at startup with pkg/migrate.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
