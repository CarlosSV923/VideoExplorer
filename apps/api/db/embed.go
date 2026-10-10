// Package db embeds the SQL migrations (goose format), which are also the
// schema sqlc reads to generate the typed queries.
package db

import "embed"

// Migrations contains migrations/*.sql.
//
//go:embed migrations/*.sql
var Migrations embed.FS
