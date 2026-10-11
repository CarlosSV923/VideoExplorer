// Package workspace implements the extraction's working area on the staging
// folder of the library (.videoexplorer/staging, RNF-04): an upload's file is
// the archive while it waits, and each job extracts into its own folder under
// extract/. The two are on the library's file system, so moving an archive to
// the trash is a rename.
package workspace

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/diskspace"
)

// extractDirName is the folder, inside staging, where jobs extract. Its name has
// no dot, and tus ids never have a folder: the upload listing skips folders.
const extractDirName = "extract"

// volumesDirName is the folder, inside staging, where the volumes of a claimed set gather (RF-03).
const volumesDirName = "volumes"

// Area implements application.Workspace.
type Area struct {
	root  string
	space func(path string) (uint64, error)
}

var _ application.Workspace = (*Area)(nil)

// New builds the area over the staging folder root. space may be nil: the free
// space of the file system is then measured with diskspace.Free.
func New(root string, space func(path string) (uint64, error)) *Area {
	if space == nil {
		space = diskspace.Free
	}
	return &Area{root: root, space: space}
}

// name makes a job id safe as one path component. Job ids are tus ids, which
// the server makes, so this only guards against a bug elsewhere.
func name(id domain.JobID) string {
	return filepath.Base(string(id))
}

// ArchivePath implements application.Workspace.
func (a *Area) ArchivePath(id domain.JobID) string {
	return filepath.Join(a.root, name(id))
}

// ArchiveExists implements application.Workspace.
func (a *Area) ArchiveExists(id domain.JobID) bool {
	info, err := os.Lstat(a.ArchivePath(id))
	return err == nil && info.Mode().IsRegular()
}

// VolumesDir implements application.Workspace: the folder of one claimed set.
func (a *Area) VolumesDir(group string) string {
	return filepath.Join(a.root, volumesDirName, name(domain.JobID(group)))
}

// VolumePath implements application.Workspace.
func (a *Area) VolumePath(group, file string) string {
	return filepath.Join(a.VolumesDir(group), filepath.Base(file))
}

// VolumeExists implements application.Workspace.
func (a *Area) VolumeExists(group, file string) bool {
	info, err := os.Lstat(a.VolumePath(group, file))
	return err == nil && info.Mode().IsRegular()
}

// ClaimVolume implements application.Workspace. The volume's staged file moves
// into its set's folder, under its own name, so that 7-Zip finds its sisters
// there. A volume already moved (after a restart) is left where it is.
func (a *Area) ClaimVolume(id domain.JobID, group, file string) error {
	dst := a.VolumePath(group, file)
	src := a.ArchivePath(id)
	if a.VolumeExists(group, file) && !a.ArchiveExists(id) {
		return nil
	}
	if err := os.MkdirAll(a.VolumesDir(group), 0o750); err != nil {
		return fmt.Errorf("workspace: create volumes folder: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("workspace: claim volume: %w", err)
	}
	return nil
}

// RemoveVolumes implements application.Workspace.
func (a *Area) RemoveVolumes(group string) error {
	if err := os.RemoveAll(a.VolumesDir(group)); err != nil {
		return fmt.Errorf("workspace: remove volumes: %w", err)
	}
	return nil
}

// ExtractionDir implements application.Workspace.
func (a *Area) ExtractionDir(id domain.JobID) string {
	return filepath.Join(a.root, extractDirName, name(id))
}

// ResetExtraction implements application.Workspace.
func (a *Area) ResetExtraction(id domain.JobID) error {
	if err := os.RemoveAll(a.ExtractionDir(id)); err != nil {
		return fmt.Errorf("workspace: clear extraction: %w", err)
	}
	if err := os.MkdirAll(a.ExtractionDir(id), 0o750); err != nil {
		return fmt.Errorf("workspace: create extraction folder: %w", err)
	}
	return nil
}

// RemoveExtraction implements application.Workspace.
func (a *Area) RemoveExtraction(id domain.JobID) error {
	if err := os.RemoveAll(a.ExtractionDir(id)); err != nil {
		return fmt.Errorf("workspace: remove extraction: %w", err)
	}
	return nil
}

// CheckExtraction implements application.Workspace: anything extracted that is
// neither a regular file nor a folder is refused (RF-04, RNF-02).
func (a *Area) CheckExtraction(id domain.JobID) error {
	root := a.ExtractionDir(id)
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		return fmt.Errorf("%w: %s (%s)", application.ErrUnsafeEntry, rel, d.Type())
	})
}

// FreeSpace implements application.Workspace.
func (a *Area) FreeSpace() (uint64, error) {
	return a.space(a.root)
}

// ReadHeader implements application.Workspace.
func (a *Area) ReadHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the staged file of an upload, built by the server
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:read], nil
}

// ExtractionExists implements application.Workspace.
func (a *Area) ExtractionExists(id domain.JobID) bool {
	info, err := os.Lstat(a.ExtractionDir(id))
	return err == nil && info.IsDir()
}

// ExtractedFiles implements application.Workspace: the regular files under the
// extraction folder, in the order the walk visits them.
func (a *Area) ExtractedFiles(id domain.JobID) ([]application.ExtractedFile, error) {
	root := a.ExtractionDir(id)
	var out []application.ExtractedFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, application.ExtractedFile{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("workspace: list extraction: %w", err)
	}
	return out, nil
}

// ExtractedPath implements application.Workspace. The relative path comes from the
// extraction itself, which refused any entry that leaves the folder (RNF-02).
func (a *Area) ExtractedPath(id domain.JobID, rel string) string {
	return filepath.Join(a.ExtractionDir(id), filepath.FromSlash(rel))
}

// FileExists implements application.Workspace.
func (a *Area) FileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
