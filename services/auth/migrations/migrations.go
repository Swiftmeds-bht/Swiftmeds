// Package migrations embeds the auth service's SQL migrations so the binary
// can apply them at startup with pkg/migrate (no files needed in the image).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
