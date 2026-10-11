//go:build unix

// Package fileid reads the identity of a file from the file system (RF-24 cache). On
// unix the device and the inode name the file, and they survive a rename inside one file
// system; on other platforms there is no identity and the probes are not cached.
package fileid

import (
	"fmt"
	"os"
	"syscall"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Identifier implements application.Identifier.
type Identifier struct{}

var _ application.Identifier = Identifier{}

// Identify implements application.Identifier: the device and inode from stat, with the
// size and modification time of the content.
func (Identifier) Identify(path string) (domain.FileIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return domain.FileIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return domain.FileIdentity{}, fmt.Errorf("fileid: not a regular file: %s", info.Mode().Type())
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return domain.FileIdentity{}, application.ErrNoFileIdentity
	}
	return domain.FileIdentity{
		Device:  uint64(st.Dev), //nolint:unconvert // the type of Dev differs between unix platforms
		Inode:   uint64(st.Ino), //nolint:unconvert // the type of Ino differs between unix platforms
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}
