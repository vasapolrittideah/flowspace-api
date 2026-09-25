package migrations

import "embed"

// Files contains the Identity database migrations.
//
//go:embed *.sql
var Files embed.FS
