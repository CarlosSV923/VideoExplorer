package zipstream_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/zipstream"
)

func textEntry(name, content string, mod time.Time) zipstream.Entry {
	return zipstream.Entry{Name: name, Size: int64(len(content)), Modified: mod, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(content)), nil
	}}
}

func zeros(name string, n int64) zipstream.Entry {
	return zipstream.Entry{Name: name, Size: n, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(io.LimitReader(zeroReader{}, n)), nil
	}}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type counter struct{ n int64 }

func (c *counter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func TestSizeMatchesTheWriterAndTheArchiveIsValid(t *testing.T) {
	t.Parallel()
	mod := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	tests := map[string][]zipstream.Entry{
		"empty":           nil,
		"one file":        {textEntry("Inside/Inside.mkv", "video", mod)},
		"unicode names":   {textEntry("Película/字幕.srt", "x", mod), textEntry("日本/episodio.mkv", "yy", mod)},
		"without mtime":   {textEntry("a/b.bin", "abc", time.Time{})},
		"folder title":    {textEntry("Lizzie Borden/Season 01/episode.mkv", "video", mod)},
		"many small ones": manyEntries(300),
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := zipstream.Write(&buf, entries); err != nil {
				t.Fatal(err)
			}
			if got, want := zipstream.Size(entries), int64(buf.Len()); got != want {
				t.Fatalf("Size = %d, written = %d", got, want)
			}
			r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatal(err)
			}
			if len(r.File) != len(entries) {
				t.Fatalf("archive has %d files, want %d", len(r.File), len(entries))
			}
			for i, f := range r.File {
				if f.Name != entries[i].Name || f.Method != zip.Store || int64(f.UncompressedSize64) != entries[i].Size {
					t.Fatalf("file %d = %s method %d size %d", i, f.Name, f.Method, f.UncompressedSize64)
				}
				rc, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.Copy(io.Discard, rc); err != nil {
					t.Fatalf("%s: %v", f.Name, err)
				}
				_ = rc.Close()
			}
		})
	}
}

func manyEntries(n int) []zipstream.Entry {
	out := make([]zipstream.Entry, n)
	for i := range out {
		out[i] = textEntry("title/file"+strconv.Itoa(i), strconv.Itoa(i), time.Time{})
	}
	return out
}

func TestSizeWithZip64(t *testing.T) {
	t.Parallel()
	entries := []zipstream.Entry{
		zeros("Movie/Movie.mkv", 1<<32+5),
		textEntry("Movie/Movie.srt", "subtitle after the 4 GB mark", time.Time{}),
	}
	exactly := []zipstream.Entry{zeros("Movie/edge.bin", 1<<32-1)}
	var c counter
	if err := zipstream.Write(&c, entries); err != nil {
		t.Fatal(err)
	}
	if got := zipstream.Size(entries); got != c.n {
		t.Fatalf("Size = %d, written = %d", got, c.n)
	}
	c = counter{}
	if err := zipstream.Write(&c, exactly); err != nil {
		t.Fatal(err)
	}
	if got := zipstream.Size(exactly); got != c.n {
		t.Fatalf("edge Size = %d, written = %d", got, c.n)
	}
}

func TestManyEntriesUseZip64EndRecords(t *testing.T) {
	t.Parallel()
	entries := manyEntries(70000)
	var c counter
	if err := zipstream.Write(&c, entries); err != nil {
		t.Fatal(err)
	}
	if got := zipstream.Size(entries); got != c.n {
		t.Fatalf("Size = %d, written = %d", got, c.n)
	}
}

func TestEntriesThatChangedSizeFail(t *testing.T) {
	t.Parallel()
	e := textEntry("a", "abc", time.Time{})
	e.Size = 10
	if err := zipstream.Write(io.Discard, []zipstream.Entry{e}); err == nil {
		t.Fatal("a short file must fail the archive")
	}
}
