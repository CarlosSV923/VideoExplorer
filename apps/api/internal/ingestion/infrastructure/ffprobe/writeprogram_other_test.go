//go:build !unix

package ffprobe

import "os"

// writeProgram writes an executable file. Only the unix variant needs the fork lock
// (ETXTBSY does not exist elsewhere), and the fake program only runs on unix.
func writeProgram(path string, data []byte) error {
	return os.WriteFile(path, data, 0o700) //nolint:gosec // a test program that must run
}
