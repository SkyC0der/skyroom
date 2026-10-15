// Package migrations embeds the ordered PostgreSQL schema migrations so the
// API binary can apply them without the source tree.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
