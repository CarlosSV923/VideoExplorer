package sevenzip_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sevenzip"
)

// require7zz skips without 7-Zip, except where REQUIRE_7ZZ is set (the dev
// image and CI), so these tests can never silently stop running there.
func require7zz(t *testing.T) *sevenzip.Extractor {
	t.Helper()
	e := sevenzip.New("")
	if !e.Available() {
		if os.Getenv("REQUIRE_7ZZ") != "" {
			t.Fatal("7zz not found but REQUIRE_7ZZ is set")
		}
		t.Skip("7zz not installed")
	}
	return e
}

// makeArchive writes files into a temp dir and packs them with 7zz.
func makeArchive(t *testing.T, name string, files map[string]string, args ...string) string {
	t.Helper()
	src := t.TempDir()
	for p, content := range files {
		full := filepath.Join(src, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(t.TempDir(), name)
	cmd := exec.CommandContext(t.Context(), "7zz", append(append([]string{"a", "-bso0", "-bsp0"}, args...), archive, ".")...)
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7zz a: %v\n%s", err, out)
	}
	return archive
}

// makeZip writes a zip with the given entries (name, content, mode) in Go, so
// that the entries can be anything 7zz would refuse to create: a path that
// leaves the folder, a link.
type entry struct {
	name    string
	content string
	mode    fs.FileMode
}

func makeZip(t *testing.T, entries ...entry) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Store}
		hdr.SetMode(e.mode)
		f, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "entries.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// makeRAR4 writes a RAR 4 archive with one stored (uncompressed) file, by hand:
// 7-Zip can read RAR but cannot create it, and the format is small enough to
// write here. 7zz reads the result in the tests.
func makeRAR4(t *testing.T, name, content string) string {
	t.Helper()
	block := func(typ byte, flags uint16, body []byte) []byte {
		size := uint16(7 + len(body))
		hdr := make([]byte, 7)
		hdr[2] = typ
		binary.LittleEndian.PutUint16(hdr[3:], flags)
		binary.LittleEndian.PutUint16(hdr[5:], size)
		crc := crc32.ChecksumIEEE(append(append([]byte{}, hdr[2:]...), body...))
		binary.LittleEndian.PutUint16(hdr[0:], uint16(crc))
		return append(hdr, body...)
	}
	var out []byte
	out = append(out, 'R', 'a', 'r', '!', 0x1A, 0x07, 0x00)
	out = append(out, block(0x73, 0, make([]byte, 6))...) // archive header: reserved fields
	fileBody := make([]byte, 25+len(name))
	binary.LittleEndian.PutUint32(fileBody[0:], uint32(len(content)))                // packed size
	binary.LittleEndian.PutUint32(fileBody[4:], uint32(len(content)))                // unpacked size
	fileBody[8] = 0                                                                  // host OS: MS-DOS
	binary.LittleEndian.PutUint32(fileBody[9:], crc32.ChecksumIEEE([]byte(content))) // file CRC
	binary.LittleEndian.PutUint32(fileBody[13:], 0)                                  // modification time
	fileBody[17] = 29                                                                // unpack version 2.9
	fileBody[18] = 0x30                                                              // method: store
	binary.LittleEndian.PutUint16(fileBody[19:], uint16(len(name)))                  // name size
	binary.LittleEndian.PutUint32(fileBody[21:], 0x20)                               // attributes: archive
	copy(fileBody[25:], name)
	out = append(out, block(0x74, 0x8000, fileBody)...)
	out = append(out, []byte(content)...)
	out = append(out, block(0x7B, 0x4000, nil)...)
	path := filepath.Join(t.TempDir(), "game.rar")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

var gameFiles = map[string]string{
	"Wrapper/INSIDE [0100D2D009028000][v0].nsp":             "PFS0 base game bytes",
	"Wrapper/INSIDE [0100D2D009028800][v196608][1.0.3].nsp": "PFS0 update bytes",
	"Wrapper/leeme.txt": "hola",
}

func TestListAndExtractVerifies(t *testing.T) {
	t.Parallel()
	e := require7zz(t)

	for _, format := range []string{"zip", "7z"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			archive := makeArchive(t, "game."+format, gameFiles, "-t"+format)

			listing, err := e.List(t.Context(), archive, "")
			if err != nil {
				t.Fatal(err)
			}
			if listing.Encrypted() || listing.TotalSize() != int64(len("PFS0 base game bytes")+len("PFS0 update bytes")+4) {
				t.Fatalf("listing = %+v", listing)
			}
			files := 0
			for _, entry := range listing.Entries {
				if !entry.IsDir {
					files++
					if entry.CRC == "" {
						t.Errorf("%s has no CRC", entry.Path)
					}
				}
			}
			if files != 3 {
				t.Fatalf("files in listing = %d", files)
			}

			dest := t.TempDir()
			warning, err := e.Extract(t.Context(), archive, dest, "", listing, nil)
			if err != nil || warning != "" {
				t.Fatalf("Extract: %v (warning %q)", err, warning)
			}
			got, err := os.ReadFile(filepath.Join(dest, "Wrapper", "INSIDE [0100D2D009028000][v0].nsp"))
			if err != nil || string(got) != "PFS0 base game bytes" {
				t.Fatalf("extracted = %q, %v", got, err)
			}
		})
	}
}

func TestRARArchiveIsListedAndExtracted(t *testing.T) {
	t.Parallel()
	e := require7zz(t)
	archive := makeRAR4(t, "Movie.mkv", "video bytes")

	listing, err := e.List(t.Context(), archive, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Path != "Movie.mkv" || listing.Entries[0].CRC == "" {
		t.Fatalf("listing = %+v", listing.Entries)
	}
	dest := t.TempDir()
	if _, err := e.Extract(t.Context(), archive, dest, "", listing, nil); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "Movie.mkv"))
	if err != nil || string(got) != "video bytes" {
		t.Fatalf("extracted = %q, %v", got, err)
	}
}

func TestHeaderEncryptedArchive(t *testing.T) {
	t.Parallel()
	e := require7zz(t)
	archive := makeArchive(t, "secret.7z", gameFiles, "-t7z", "-pS3cret", "-mhe=on")

	if _, err := e.List(t.Context(), archive, ""); !errors.Is(err, application.ErrPasswordRequired) {
		t.Fatalf("no password: %v", err)
	}
	if _, err := e.List(t.Context(), archive, "nope"); !errors.Is(err, application.ErrWrongPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	listing, err := e.List(t.Context(), archive, "S3cret")
	if err != nil || !listing.Encrypted() {
		t.Fatalf("right password: %+v %v", listing, err)
	}
	if _, err := e.Extract(t.Context(), archive, t.TempDir(), "S3cret", listing, nil); err != nil {
		t.Fatalf("extract: %v", err)
	}
}

func TestFileEncryptedZip(t *testing.T) {
	t.Parallel()
	e := require7zz(t)
	archive := makeArchive(t, "secret.zip", gameFiles, "-tzip", "-pS3cret")

	listing, err := e.List(t.Context(), archive, "")
	if err != nil || !listing.Encrypted() {
		t.Fatalf("names are visible, contents encrypted: %+v %v", listing, err)
	}
	if _, err := e.Extract(t.Context(), archive, t.TempDir(), "nope", listing, nil); !errors.Is(err, application.ErrWrongPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := e.Extract(t.Context(), archive, t.TempDir(), "S3cret", listing, nil); err != nil {
		t.Fatalf("right password: %v", err)
	}
}

func TestCorruptArchiveIsArchiveCorrupt(t *testing.T) {
	t.Parallel()
	e := require7zz(t)
	archive := makeArchive(t, "game.7z", map[string]string{"a.iso": string(make([]byte, 4096))}, "-t7z", "-mx0")
	listing, err := e.List(t.Context(), archive, "")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(archive)
	data[100] ^= 0xFF // flip a byte inside the stored data
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Extract(t.Context(), archive, t.TempDir(), "", listing, nil); !errors.Is(err, application.ErrCorrupt) {
		t.Fatalf("corrupt archive err = %v", err)
	}
}

func TestIndexThatDisagreesWithTheFilesIsAMismatch(t *testing.T) {
	t.Parallel()
	e := require7zz(t)

	tests := []struct {
		name   string
		change func(*application.Listing)
	}{
		{name: "CRC32", change: func(l *application.Listing) {
			for i := range l.Entries {
				l.Entries[i].CRC = "DEADBEEF"
			}
		}},
		{name: "size", change: func(l *application.Listing) {
			for i := range l.Entries {
				l.Entries[i].Size++
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive := makeArchive(t, "game.zip", map[string]string{"a.iso": "real data"}, "-tzip")
			listing, err := e.List(t.Context(), archive, "")
			if err != nil {
				t.Fatal(err)
			}
			tt.change(&listing)
			if _, err := e.Extract(t.Context(), archive, t.TempDir(), "", listing, nil); !errors.Is(err, application.ErrMismatch) {
				t.Fatalf("err = %v, want ErrMismatch", err)
			}
		})
	}
}

func TestListShowsLinksAndPathsThatLeaveTheFolder(t *testing.T) {
	t.Parallel()
	e := require7zz(t)

	tests := []struct {
		name        string
		entries     []entry
		wantPath    string
		wantSymlink bool
	}{
		{
			name:        "symbolic link",
			entries:     []entry{{name: "Movie/link.mkv", content: "/etc/passwd", mode: fs.ModeSymlink | 0o777}},
			wantPath:    "Movie/link.mkv",
			wantSymlink: true,
		},
		{
			name:     "zip-slip path",
			entries:  []entry{{name: "../../evil.mkv", content: "x", mode: 0o644}},
			wantPath: "../../evil.mkv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			listing, err := e.List(t.Context(), makeZip(t, tt.entries...), "")
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			var found *application.ArchiveEntry
			for i := range listing.Entries {
				if listing.Entries[i].Path == tt.wantPath {
					found = &listing.Entries[i]
				}
			}
			if found == nil {
				t.Fatalf("entry %q not listed: %+v", tt.wantPath, listing.Entries)
			}
			if found.Symlink != tt.wantSymlink {
				t.Errorf("Symlink = %v, want %v", found.Symlink, tt.wantSymlink)
			}
		})
	}
}

func TestNotAnArchive(t *testing.T) {
	t.Parallel()
	e := require7zz(t)
	p := filepath.Join(t.TempDir(), "game.nsp")
	_ = os.WriteFile(p, []byte("PFS0 not an archive"), 0o600)
	if _, err := e.List(t.Context(), p, ""); !errors.Is(err, application.ErrCorrupt) {
		t.Fatalf("err = %v", err)
	}
}
