//go:build !unix

package diskspace

import "math"

// Free is not measured on non-Unix systems (the app runs on Linux; this
// only keeps the package compiling elsewhere).
func Free(string) (uint64, error) { return math.MaxUint64, nil }
