//go:build !embed

package static

import "net/http"

// Handler returns nil when built without the embed tag (development mode).
// The main server skips registering the static catch-all in this case.
func Handler() http.Handler { return nil }
