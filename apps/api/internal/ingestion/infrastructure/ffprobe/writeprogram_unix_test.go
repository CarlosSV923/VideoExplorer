//go:build unix

package ffprobe

import (
	"os"
	"syscall"
)

// writeProgram writes an executable file. A fork that starts while the file is still open
// for writing hands its copy of the descriptor to the child, and exec of the program then
// fails with ETXTBSY ("text file busy") until that child execs. No fork can start while
// syscall.ForkLock is held, so the descriptor is closed before another process can copy it.
func writeProgram(path string, data []byte) error {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()

	return os.WriteFile(path, data, 0o700) //nolint:gosec // a test program that must run
}
