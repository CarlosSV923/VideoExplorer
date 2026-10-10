//go:build unix

package diskspace

import "syscall"

// Free reports the bytes available to unprivileged users on path's filesystem.
func Free(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil //nolint:gosec,unconvert // Bsize is positive; its type varies by OS
}
