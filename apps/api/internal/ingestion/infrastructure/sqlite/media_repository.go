package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite/sqlcgen"
)

// MediaProbeRepository implements application.ProbeStore: one probe of a video per file,
// kept in media_probes (RF-24).
type MediaProbeRepository struct {
	q *sqlcgen.Queries
}

var _ application.ProbeStore = (*MediaProbeRepository)(nil)

// NewMediaProbeRepository builds the repository over an open database.
func NewMediaProbeRepository(db *sql.DB) *MediaProbeRepository {
	return &MediaProbeRepository{q: sqlcgen.New(db)}
}

// Find implements application.ProbeStore. A stored probe that no longer reads back is an
// error: the caller probes the file again.
func (r *MediaProbeRepository) Find(ctx context.Context, device, inode uint64) (domain.FileIdentity, domain.MediaInfo, bool, error) {
	row, err := r.q.GetMediaProbe(ctx, sqlcgen.GetMediaProbeParams{Device: toSQL(device), Inode: toSQL(inode)})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FileIdentity{}, domain.MediaInfo{}, false, nil
	}
	if err != nil {
		return domain.FileIdentity{}, domain.MediaInfo{}, false, fmt.Errorf("ingestion: get media probe: %w", err)
	}
	var doc mediaDoc
	if err := json.Unmarshal([]byte(row.Info), &doc); err != nil {
		return domain.FileIdentity{}, domain.MediaInfo{}, false, fmt.Errorf("ingestion: read media probe: %w", err)
	}
	stored := domain.FileIdentity{Device: device, Inode: inode, Size: row.Size, ModTime: time.Unix(0, row.MtimeNs)}
	return stored, doc.toDomain(), true, nil
}

// Store implements application.ProbeStore.
func (r *MediaProbeRepository) Store(ctx context.Context, id domain.FileIdentity, info domain.MediaInfo) error {
	raw, err := json.Marshal(fromDomain(info))
	if err != nil {
		return fmt.Errorf("ingestion: encode media probe: %w", err)
	}
	if err := r.q.UpsertMediaProbe(ctx, sqlcgen.UpsertMediaProbeParams{
		Device:  toSQL(id.Device),
		Inode:   toSQL(id.Inode),
		Size:    id.Size,
		MtimeNs: id.ModTime.UnixNano(),
		Info:    string(raw),
	}); err != nil {
		return fmt.Errorf("ingestion: store media probe: %w", err)
	}
	return nil
}

// toSQL stores a device or inode number as SQLite's signed integer. The bits are kept, so
// toSQL and the read of the row give back the same number.
func toSQL(n uint64) int64 {
	return int64(n) //nolint:gosec // a bit-for-bit mapping: device and inode numbers are stored as their bits
}

// mediaDoc is the JSON form of a domain.MediaInfo in media_probes.info. The domain types
// carry no tags: the storage format is kept here.
type mediaDoc struct {
	Container string        `json:"container"`
	Duration  int64         `json:"durationNs"`
	Size      int64         `json:"size"`
	Video     videoDoc      `json:"video"`
	Audio     []audioDoc    `json:"audio"`
	Subtitles []subtitleDoc `json:"subtitles"`
}

type videoDoc struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	HDR    bool   `json:"hdr"`
}

type audioDoc struct {
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Default  bool   `json:"default"`
}

type subtitleDoc struct {
	Codec    string `json:"codec"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Forced   bool   `json:"forced"`
	SDH      bool   `json:"sdh"`
	Default  bool   `json:"default"`
}

func fromDomain(info domain.MediaInfo) mediaDoc {
	doc := mediaDoc{
		Container: info.Container,
		Duration:  int64(info.Duration),
		Size:      info.Size,
		Video:     videoDoc{Codec: info.Video.Codec, Width: info.Video.Width, Height: info.Video.Height, HDR: info.Video.HDR},
	}
	for _, a := range info.Audio {
		doc.Audio = append(doc.Audio, audioDoc{Codec: a.Codec, Channels: a.Channels, Language: a.Language, Title: a.Title, Default: a.Default})
	}
	for _, s := range info.Subtitles {
		doc.Subtitles = append(doc.Subtitles, subtitleDoc{
			Codec: s.Codec, Language: s.Language, Title: s.Title, Forced: s.Forced, SDH: s.SDH, Default: s.Default,
		})
	}
	return doc
}

func (d mediaDoc) toDomain() domain.MediaInfo {
	info := domain.MediaInfo{
		Container: d.Container,
		Duration:  time.Duration(d.Duration),
		Size:      d.Size,
		Video:     domain.VideoTrack{Codec: d.Video.Codec, Width: d.Video.Width, Height: d.Video.Height, HDR: d.Video.HDR},
	}
	for _, a := range d.Audio {
		info.Audio = append(info.Audio, domain.AudioTrack{Codec: a.Codec, Channels: a.Channels, Language: a.Language, Title: a.Title, Default: a.Default})
	}
	for _, s := range d.Subtitles {
		info.Subtitles = append(info.Subtitles, domain.SubtitleTrack{
			Codec: s.Codec, Language: s.Language, Title: s.Title, Forced: s.Forced, SDH: s.SDH, Default: s.Default,
		})
	}
	return info
}
