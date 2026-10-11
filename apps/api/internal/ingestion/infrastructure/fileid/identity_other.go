//go:build !unix

package fileid

import (
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Identifier implements application.Identifier. Without device and inode numbers, a
// file has no identity here: its probes are read every time and not cached.
type Identifier struct{}

var _ application.Identifier = Identifier{}

// Identify implements application.Identifier: it always reports ErrNoFileIdentity.
func (Identifier) Identify(string) (domain.FileIdentity, error) {
	return domain.FileIdentity{}, application.ErrNoFileIdentity
}
