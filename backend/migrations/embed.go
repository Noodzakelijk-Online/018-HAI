// Package migrations embeds the versioned SQL schema migrations so they ship
// inside the compiled binary and can be applied deterministically at startup or
// via the `migrate` subcommand.
//
// Layout (see docs/migrations.md):
//
//	migrations/pre/   run before Gorm AutoMigrate (extensions the models rely on)
//	migrations/post/  run after AutoMigrate (indexes, constraints, backfills)
//
// Each change is a pair: NNNN_name.up.sql applies it, NNNN_name.down.sql reverses
// it. Never edit a migration that has shipped — add a new one.
package migrations

import "embed"

//go:embed pre/*.sql post/*.sql
var Files embed.FS
