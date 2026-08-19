// Package migrations embeds the goose migration files.
//
// Embedded rather than read from disk so the binary is self-contained: the
// container ships one file, and a deploy cannot be half-applied because the
// migrations directory was not copied or was mounted from the wrong place.
//
// The .go file lives inside the migrations directory because go:embed cannot
// reach up out of its own package. goose ignores it.
package migrations

import "embed"

// FS holds every .sql migration, in filename order.
//
//go:embed *.sql
var FS embed.FS
