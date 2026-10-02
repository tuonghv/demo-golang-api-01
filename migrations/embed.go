// Package migrations embeds the SQL migration files.
package migrations

import "embed"

// FS holds the *.sql migrations.
//
//go:embed *.sql
var FS embed.FS
