package migrations

import "embed"

// FS contains the notification service schema migrations.
//
//go:embed *.sql
var FS embed.FS
