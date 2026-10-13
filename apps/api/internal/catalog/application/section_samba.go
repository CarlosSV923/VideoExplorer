package application

import (
	"context"
	"errors"
	"path"
	"strconv"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// ErrNotSamba means a change that takes files for the ingestion was asked of an entry that Samba did not
// copy (RF-27a): the entries the ingestion takes are the copies of Samba only.
var ErrNotSamba = errors.New("the entry is not a copy of Samba")

// SambaFile is a file of an entry that Samba copied, as the section reads it now (RF-27a). The ingestion
// decides from it whether it is taken as an upload.
type SambaFile struct {
	// Entry is the id of the entry that holds the file.
	Entry int64
	// Rel is the file's place inside the entry, with "/" separators; "" for a loose entry's own file.
	Rel string
	// Name is the file's name, without folders.
	Name string
	Size int64
	// Source is the file's current path inside the library, built by the server.
	Source string
	// Copying says the file is not taken yet: it is still being copied (RF-27), or its entry is held by
	// an assignment (RF-26).
	Copying bool
}

// SambaMove is one file that TakeSamba moves out of its entry: its place inside the entry, and the
// staging path it goes to.
type SambaMove struct {
	Rel string
	Dst string
}

// SambaFiles lists the files of the entries that Samba copied, as the section reads them now (RF-27a).
// The reading is the one of the section, so the copying state is the same that the section shows.
func (s *Section) SambaFiles(ctx context.Context) ([]SambaFile, error) {
	var out []SambaFile
	err := s.d.Engine.Locked(func() error {
		entries, err := s.read(ctx)
		if err != nil {
			return err
		}
		now := s.now()
		for _, e := range entries {
			if e.Origin != domain.OriginSamba {
				continue
			}
			held := e.Lock.Held(now)
			for _, f := range e.Files {
				name := path.Base(f.Rel)
				if e.Loose {
					name = e.Name
				}
				out = append(out, SambaFile{
					Entry: e.ID, Rel: f.Rel, Name: name, Size: f.Size,
					Source: s.pathOf(e, f.Rel), Copying: held || e.copying[f.Rel],
				})
			}
		}
		return nil
	})
	return out, err
}

// TakeSamba moves files that Samba copied out of their entry to their staging paths (RF-27a). It is one
// change of the library (RNF-04): the moves, the records of the files that leave the entry, and the
// removal of the folders that it leaves empty, with their junk. It refuses an entry that an assignment
// holds (RF-26) or that a file of it is still being copied (RF-27), and a file that is not in the entry
// (ErrFileNotFound): the ingestion then takes the copies again at the next reading.
func (s *Section) TakeSamba(ctx context.Context, entry int64, moves []SambaMove) error {
	return s.d.Engine.Do(ctx, "section.take", "section:"+strconv.FormatInt(entry, 10), func(b *Builder) error {
		entries, err := s.read(ctx)
		if err != nil {
			return err
		}
		e := findEntry(entries, entry)
		if e == nil {
			return ErrEntryNotFound
		}
		if e.Origin != domain.OriginSamba {
			return ErrNotSamba
		}
		if e.Lock.Held(s.now()) {
			return ErrAssigning
		}
		if e.isCopying() {
			return ErrCopying
		}
		chosen := make([]domain.UnassignedFile, 0, len(moves))
		dst := make(map[string]string, len(moves))
		for _, m := range moves {
			f, ok := fileAt(e, m.Rel)
			if !ok {
				return ErrFileNotFound
			}
			chosen = append(chosen, f)
			dst[f.Rel] = m.Dst
		}
		return s.plan(ctx, b, e, chosen, leaveTake, dst)
	})
}

// fileAt returns the file of the entry at rel.
func fileAt(e *sectionEntry, rel string) (domain.UnassignedFile, bool) {
	for _, f := range e.Files {
		if f.Rel == rel {
			return f, true
		}
	}
	return domain.UnassignedFile{}, false
}
