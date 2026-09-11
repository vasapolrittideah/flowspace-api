package migrations

import "embed"

// Files contains the Workspace database migrations.
//
//go:embed *.sql
var Files embed.FS
