// Package migrations holds the SQL files for freshness_db, embedded in the
// binary so the service can run them when it starts.
package migrations

import "embed"

// FS contains every *.up.sql file in this folder.
//
//go:embed *.up.sql
var FS embed.FS
