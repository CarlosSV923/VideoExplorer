package application

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// PartialHasher computes the partial hash of a file under the library (RF-24). The composition
// root wires the ingestion's PartialHashOf, which refuses symbolic links (RNF-02).
type PartialHasher interface {
	PartialHash(ctx context.Context, path string) (string, error)
}

// seasonFolder matches the folder of a season, as spec §5 names it (Season 01, Season 00).
var seasonFolder = regexp.MustCompile(`(?i)^Season \d+$`)

// DuplicateFinder finds what already holds the place a new video is going to take (RF-24): the
// videos the registry keeps for the title, and the videos the disk holds in its folder, even
// those the registry does not know. A video that is the same content as the new one (same size
// and partial hash) is an exact copy. Nothing is changed here.
type DuplicateFinder struct {
	repo     LibraryRepository
	files    Files
	hasher   PartialHasher
	videoExt map[string]bool
}

// NewDuplicateFinder builds the finder. videoExts are the extensions of the videos, with their
// dot (VIDEO_EXTENSIONS); a file with another extension is never a duplicate.
func NewDuplicateFinder(repo LibraryRepository, files Files, hasher PartialHasher, videoExts []string) *DuplicateFinder {
	ext := make(map[string]bool, len(videoExts))
	for _, e := range videoExts {
		ext[strings.ToLower(e)] = true
	}
	return &DuplicateFinder{repo: repo, files: files, hasher: hasher, videoExt: ext}
}

// Find returns the duplicates of target in the title whose folder is folder (a path inside the
// library, with "/" separators). incoming is the new video's path inside the library, where it
// waits to be assigned (in _unassigned/). Only the videos of the same size as the new one are read
// for their partial hash.
func (d *DuplicateFinder) Find(ctx context.Context, folder string, target domain.Target, incoming string) (domain.Duplicates, error) {
	existing, err := d.registered(ctx, folder)
	if err != nil {
		return domain.Duplicates{}, err
	}
	onDisk, err := d.onDisk(folder, target.Kind)
	if err != nil {
		return domain.Duplicates{}, err
	}
	existing = append(existing, onDisk...)

	size, err := d.files.Size(d.files.LibraryPath(incoming))
	if err != nil {
		return domain.Duplicates{}, fmt.Errorf("catalog: size of the new video: %w", err)
	}
	found, err := domain.FindDuplicates(target, existing, domain.Fingerprint{Size: size})
	if err != nil {
		return domain.Duplicates{}, err
	}
	// Only the duplicates of the same size can be the same content: their partial hash is read,
	// and the new video's once, when there is one.
	var incomingHash string
	for i := range found.Files {
		f := &found.Files[i]
		if f.Size != size {
			continue
		}
		if f.PartialHash == "" {
			hash, err := d.hasher.PartialHash(ctx, d.files.LibraryPath(f.Rel))
			if err != nil {
				return domain.Duplicates{}, fmt.Errorf("catalog: partial hash of %s: %w", f.Rel, err)
			}
			f.PartialHash = hash
		}
		if incomingHash == "" {
			hash, err := d.hasher.PartialHash(ctx, d.files.LibraryPath(incoming))
			if err != nil {
				return domain.Duplicates{}, fmt.Errorf("catalog: partial hash of the new video: %w", err)
			}
			incomingHash = hash
		}
		if domain.SameContent(domain.Fingerprint{Size: size, PartialHash: incomingHash}, domain.Fingerprint{Size: f.Size, PartialHash: f.PartialHash}) {
			found.ExactCopy = true
		}
	}
	return found, nil
}

// registered returns the videos the registry keeps in the title's folder, trashed ones included.
func (d *DuplicateFinder) registered(ctx context.Context, folder string) ([]domain.Existing, error) {
	title, found, err := d.repo.TitleByFolder(ctx, folder)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	files, err := d.repo.Files(ctx, title.ID)
	if err != nil {
		return nil, err
	}
	var out []domain.Existing
	for _, f := range files {
		if f.Kind != domain.FileVideo {
			continue
		}
		out = append(out, domain.Existing{
			Rel: f.Rel, ID: f.ID, Place: f.Place, Size: f.Size, PartialHash: f.PartialHash,
			Version: f.Version, VersionKnown: true, Trashed: !f.Live(),
		})
	}
	return out, nil
}

// onDisk returns the videos the disk holds for the title: the files of its folder for a movie,
// and those of its season folders for a series. A video whose name does not tell its place is
// left out: it cannot be told apart from the other chapters.
func (d *DuplicateFinder) onDisk(folder string, kind domain.TitleKind) ([]domain.Existing, error) {
	dirs := []string{folder}
	if kind == domain.KindSeries {
		names, err := d.files.Names(d.files.LibraryPath(folder))
		if err != nil {
			return nil, fmt.Errorf("catalog: list %s: %w", folder, err)
		}
		dirs = dirs[:0]
		for _, n := range names {
			if seasonFolder.MatchString(n) {
				dirs = append(dirs, folder+"/"+n)
			}
		}
	}
	var out []domain.Existing
	for _, dir := range dirs {
		names, err := d.files.Names(d.files.LibraryPath(dir))
		if err != nil {
			return nil, fmt.Errorf("catalog: list %s: %w", dir, err)
		}
		for _, n := range names {
			if !d.videoExt[strings.ToLower(path.Ext(n))] {
				continue
			}
			rel := dir + "/" + n
			place, ok := domain.PlaceOfName(kind, rel)
			if !ok {
				continue
			}
			size, err := d.files.Size(d.files.LibraryPath(rel))
			if err != nil {
				return nil, fmt.Errorf("catalog: size of %s: %w", rel, err)
			}
			version, known := versionOnDisk(kind, folder, rel)
			out = append(out, domain.Existing{Rel: rel, Place: place, Size: size, Version: version, VersionKnown: known})
		}
	}
	return out, nil
}
