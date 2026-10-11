package ffprobe

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// fixture is a real video of testdata/ with what ffprobe reads from it. The JSON of
// the same name is what ffprobe 8.1.3 printed for the file (the image's ffprobe).
type fixture struct {
	name string
	file string // testdata file name; its JSON is file without the extension, plus .json
	want domain.MediaInfo
}

var fixtures = []fixture{
	{
		name: "H.264 1080p with two audio tracks and two subtitle tracks",
		file: "h264_1080p_two_audio_subs.mkv",
		want: domain.MediaInfo{
			Container: "matroska,webm",
			Duration:  1023 * time.Millisecond,
			Size:      8147,
			Video:     domain.VideoTrack{Codec: "h264", Width: 1920, Height: 1080},
			Audio: []domain.AudioTrack{
				{Codec: "aac", Channels: 2, Language: "eng", Title: "English", Default: true},
				{Codec: "aac", Channels: 1, Language: "spa", Title: "Espanol"},
			},
			Subtitles: []domain.SubtitleTrack{
				{Codec: "subrip", Language: "spa", Title: "Forzados", Forced: true},
				{Codec: "subrip", Language: "eng", Title: "SDH", SDH: true},
			},
		},
	},
	{
		name: "HEVC 2160p 10-bit with PQ transfer (HDR10)",
		file: "hevc_2160p_hdr10.mkv",
		want: domain.MediaInfo{
			Container: "matroska,webm",
			Duration:  1023 * time.Millisecond,
			Size:      9372,
			Video:     domain.VideoTrack{Codec: "hevc", Width: 3840, Height: 2160, HDR: true},
			Audio: []domain.AudioTrack{
				{Codec: "aac", Channels: 2, Language: "eng"},
			},
		},
	},
	{
		name: "H.264 480p without audio",
		file: "h264_480p_no_audio.mp4",
		want: domain.MediaInfo{
			Container: "mov,mp4,m4a,3gp,3g2,mj2",
			Duration:  time.Second,
			Size:      3726,
			Video:     domain.VideoTrack{Codec: "h264", Width: 854, Height: 480},
		},
	},
}

func TestParseOutputReadsTheFixtureJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range fixtures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile("testdata/" + jsonName(tt.file))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseOutput(data)
			if err != nil {
				t.Fatalf("ParseOutput: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseOutput = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseOutputOfDocumentsThatAreNotVideosOrAreUnreadable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document string
		wantErr  error // nil: any error that is not ErrNotVideo
	}{
		{
			name:     "a file with only an audio track is not a video",
			document: `{"streams":[{"codec_type":"audio","codec_name":"aac"}],"format":{"format_name":"mp3"}}`,
			wantErr:  application.ErrNotVideo,
		},
		{
			name:     "a file with only cover art is not a video",
			document: `{"streams":[{"codec_type":"video","codec_name":"mjpeg","disposition":{"attached_pic":1}}],"format":{}}`,
			wantErr:  application.ErrNotVideo,
		},
		{
			name:     "a file with no streams is not a video",
			document: `{"streams":[],"format":{}}`,
			wantErr:  application.ErrNotVideo,
		},
		{
			name:     "output that is not JSON is an error of the probe, not a verdict on the file",
			document: `{"streams":`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseOutput([]byte(tt.document))
			if err == nil {
				t.Fatal("ParseOutput succeeded, want an error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && errors.Is(err, application.ErrNotVideo) {
				t.Fatalf("err = %v, want something other than ErrNotVideo", err)
			}
		})
	}
}

func TestParseOutputChoosesTheMainVideoAndReadsItsFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document string
		want     domain.MediaInfo
	}{
		{
			name: "cover art before the video is skipped",
			document: `{"streams":[
				{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1,"default":1}},
				{"index":1,"codec_type":"video","codec_name":"h264","width":1280,"height":720,"disposition":{"default":1}}
			],"format":{"format_name":"matroska,webm"}}`,
			want: domain.MediaInfo{Container: "matroska,webm", Video: domain.VideoTrack{Codec: "h264", Width: 1280, Height: 720}},
		},
		{
			name: "the default video track wins over an earlier one",
			document: `{"streams":[
				{"index":0,"codec_type":"video","codec_name":"h264","width":640,"height":360,"disposition":{"default":0}},
				{"index":1,"codec_type":"video","codec_name":"hevc","width":1920,"height":1080,"disposition":{"default":1}}
			],"format":{}}`,
			want: domain.MediaInfo{Video: domain.VideoTrack{Codec: "hevc", Width: 1920, Height: 1080}},
		},
		{
			name: "without a default, the first video track",
			document: `{"streams":[
				{"index":0,"codec_type":"video","codec_name":"h264","width":640,"height":360},
				{"index":1,"codec_type":"video","codec_name":"hevc","width":1920,"height":1080}
			],"format":{}}`,
			want: domain.MediaInfo{Video: domain.VideoTrack{Codec: "h264", Width: 640, Height: 360}},
		},
		{
			name: "HLG transfer is HDR",
			document: `{"streams":[{"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,
				"color_transfer":"arib-std-b67"}],"format":{}}`,
			want: domain.MediaInfo{Video: domain.VideoTrack{Codec: "hevc", Width: 3840, Height: 2160, HDR: true}},
		},
		{
			name: "a Dolby Vision configuration record is HDR",
			document: `{"streams":[{"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,
				"side_data_list":[{"side_data_type":"DOVI configuration record"}]}],"format":{}}`,
			want: domain.MediaInfo{Video: domain.VideoTrack{Codec: "hevc", Width: 3840, Height: 2160, HDR: true}},
		},
		{
			name: "BT.709 transfer is not HDR",
			document: `{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,
				"color_transfer":"bt709"}],"format":{}}`,
			want: domain.MediaInfo{Video: domain.VideoTrack{Codec: "h264", Width: 1920, Height: 1080}},
		},
		{
			name: "subtitle flags: forced, hearing impaired and default",
			document: `{"streams":[
				{"codec_type":"video","codec_name":"h264","width":1920,"height":1080},
				{"codec_type":"subtitle","codec_name":"ass","tags":{"LANGUAGE":"lat","TITLE":"Latino"},
				 "disposition":{"forced":1,"hearing_impaired":1,"default":1}}
			],"format":{}}`,
			want: domain.MediaInfo{
				Video: domain.VideoTrack{Codec: "h264", Width: 1920, Height: 1080},
				Subtitles: []domain.SubtitleTrack{
					{Codec: "ass", Language: "lat", Title: "Latino", Forced: true, SDH: true, Default: true},
				},
			},
		},
		{
			name: "missing duration and size stay at zero",
			document: `{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080}],
				"format":{"format_name":"matroska,webm","duration":"N/A","size":"unknown"}}`,
			want: domain.MediaInfo{Container: "matroska,webm", Video: domain.VideoTrack{Codec: "h264", Width: 1920, Height: 1080}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseOutput([]byte(tt.document))
			if err != nil {
				t.Fatalf("ParseOutput: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseOutput = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// jsonName is the name of the JSON that holds what ffprobe printed for a fixture file.
func jsonName(file string) string {
	return strings.TrimSuffix(file, filepath.Ext(file)) + ".json"
}
