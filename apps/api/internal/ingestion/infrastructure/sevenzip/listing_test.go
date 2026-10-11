package sevenzip

import "testing"

func TestParseListingReadsTheIndexOfSlt(t *testing.T) {
	t.Parallel()

	out := "Path = Movie/Movie.mkv\n" +
		"Folder = -\n" +
		"Size = 13\n" +
		"Attributes =  -rw-r--r--\n" +
		"Encrypted = -\n" +
		"CRC = 829b0f52\n" +
		"\n" +
		"Path = Movie/link.mkv\n" +
		"Size = 13\n" +
		"Attributes =  lrwxrwxrwx\n" +
		"CRC = 829B0F52\n" +
		"\n" +
		"Path = Movie\n" +
		"Folder = +\n" +
		"Attributes = D drwxr-xr-x\n" +
		"\n" +
		"Path = Secret.mkv\n" +
		"Size = 4\n" +
		"Attributes = A_ -rw-r--r--\n" +
		"Encrypted = +\n"

	got := parseListing(out).Entries
	if len(got) != 4 {
		t.Fatalf("entries = %d, want 4: %+v", len(got), got)
	}
	tests := []struct {
		path      string
		size      int64
		crc       string
		isDir     bool
		symlink   bool
		encrypted bool
	}{
		{path: "Movie/Movie.mkv", size: 13, crc: "829B0F52"},
		{path: "Movie/link.mkv", size: 13, crc: "829B0F52", symlink: true},
		{path: "Movie", isDir: true},
		{path: "Secret.mkv", size: 4, encrypted: true},
	}
	for i, tt := range tests {
		e := got[i]
		if e.Path != tt.path || e.Size != tt.size || e.CRC != tt.crc || e.IsDir != tt.isDir || e.Symlink != tt.symlink || e.Encrypted != tt.encrypted {
			t.Errorf("entry %d = %+v, want %+v", i, e, tt)
		}
	}
}

func TestIsSymlinkModeReadsOnlyAUnixLinkType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attributes string
		want       bool
	}{
		{attributes: " lrwxrwxrwx", want: true},
		{attributes: "A_ lrwxrwxrwx", want: true},
		{attributes: " -rw-r--r--"},
		{attributes: "A_ -rw-r--r--"},
		{attributes: "D drwxr-xr-x"},
		{attributes: "A"},
		{attributes: ""},
		{attributes: "lrwx"}, // too short to be a mode
	}
	for _, tt := range tests {
		t.Run(tt.attributes, func(t *testing.T) {
			t.Parallel()

			if got := isSymlinkMode(tt.attributes); got != tt.want {
				t.Errorf("isSymlinkMode(%q) = %v, want %v", tt.attributes, got, tt.want)
			}
		})
	}
}
