package ffprobe

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// output is the part of the `ffprobe -print_format json -show_format -show_streams`
// document that the probe reads.
type output struct {
	Streams []stream `json:"streams"`
	Format  format   `json:"format"`
}

type format struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	Size       string `json:"size"`
}

type stream struct {
	CodecType     string            `json:"codec_type"`
	CodecName     string            `json:"codec_name"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	Channels      int               `json:"channels"`
	ColorTransfer string            `json:"color_transfer"`
	SideData      []sideData        `json:"side_data_list"`
	Disposition   map[string]int    `json:"disposition"`
	Tags          map[string]string `json:"tags"`
}

type sideData struct {
	Type string `json:"side_data_type"`
}

// Transfer characteristics that mark a high dynamic range signal: PQ (HDR10, HDR10+)
// and HLG. Primaries BT.2020 alone do not, since SDR content can use them.
var hdrTransfers = []string{"smpte2084", "arib-std-b67"}

// ParseOutput maps the JSON that ffprobe prints for a file into the domain (RF-24, RF-44).
// A file without a video track is not a video: it fails with application.ErrNotVideo.
// Cover art is not a video track; when there are several tracks, the default one wins,
// and otherwise the first.
func ParseOutput(data []byte) (domain.MediaInfo, error) {
	var out output
	if err := json.Unmarshal(data, &out); err != nil {
		return domain.MediaInfo{}, fmt.Errorf("ffprobe: unreadable output: %w", err)
	}

	info := domain.MediaInfo{
		Container: out.Format.FormatName,
		Duration:  parseSeconds(out.Format.Duration),
		Size:      parseInt(out.Format.Size),
	}
	video, ok := mainVideo(out.Streams)
	if !ok {
		return domain.MediaInfo{}, fmt.Errorf("%w: no video track", application.ErrNotVideo)
	}
	info.Video = domain.VideoTrack{
		Codec:  video.CodecName,
		Width:  video.Width,
		Height: video.Height,
		HDR:    isHDR(video),
	}
	for _, s := range out.Streams {
		switch s.CodecType {
		case "audio":
			info.Audio = append(info.Audio, domain.AudioTrack{
				Codec:    s.CodecName,
				Channels: s.Channels,
				Language: tag(s.Tags, "language"),
				Title:    tag(s.Tags, "title"),
				Default:  flag(s.Disposition, "default"),
			})
		case "subtitle":
			info.Subtitles = append(info.Subtitles, domain.SubtitleTrack{
				Codec:    s.CodecName,
				Language: tag(s.Tags, "language"),
				Title:    tag(s.Tags, "title"),
				Forced:   flag(s.Disposition, "forced"),
				SDH:      flag(s.Disposition, "hearing_impaired"),
				Default:  flag(s.Disposition, "default"),
			})
		}
	}
	return info, nil
}

// mainVideo picks the main video track: the default one that is not cover art, or the
// first one that is not cover art.
func mainVideo(streams []stream) (stream, bool) {
	var first stream
	found := false
	for _, s := range streams {
		if s.CodecType != "video" || flag(s.Disposition, "attached_pic") {
			continue
		}
		if flag(s.Disposition, "default") {
			return s, true
		}
		if !found {
			first, found = s, true
		}
	}
	return first, found
}

// isHDR reports whether the video track signals HDR: a PQ or HLG transfer, or a Dolby
// Vision configuration record.
func isHDR(s stream) bool {
	if slices.Contains(hdrTransfers, s.ColorTransfer) {
		return true
	}
	for _, sd := range s.SideData {
		if strings.HasPrefix(sd.Type, "DOVI") {
			return true
		}
	}
	return false
}

// flag reads one disposition flag (ffprobe prints 0 or 1).
func flag(disposition map[string]int, name string) bool {
	return disposition[name] == 1
}

// tag reads a metadata tag. Matroska and MP4 write the keys in lower case, but
// ffprobe prints them as the file wrote them, so the lookup ignores case.
func tag(tags map[string]string, name string) string {
	if v, ok := tags[name]; ok {
		return v
	}
	for k, v := range tags {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// parseSeconds reads ffprobe's duration, in seconds with decimals. A value it cannot
// read is zero (not reported).
func parseSeconds(s string) time.Duration {
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil || sec < 0 {
		return 0
	}
	// Rounded to the nearest nanosecond: 1.023 seconds is not exact in binary floating point.
	return time.Duration(math.Round(sec * float64(time.Second)))
}

// parseInt reads a number that ffprobe prints as a string. A value it cannot read is zero.
func parseInt(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
