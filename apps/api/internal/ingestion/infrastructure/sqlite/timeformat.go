// Package sqlite implements the ingestion repositories on the shared SQLite
// database (spec RF-10a: the states survive a restart). The typed queries come
// from queries/ through sqlc (`task gen:sql`).
package sqlite

import "time"

// timeLayout is fixed-width UTC, so string comparison in SQL equals time order.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }
