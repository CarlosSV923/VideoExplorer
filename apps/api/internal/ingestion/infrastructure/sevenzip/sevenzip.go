// Package sevenzip implements the Extractor port with the official 7-Zip
// command-line build (7zz, RNF-03a), run as a child process: the Go process
// never touches the archive's bytes, so memory stays flat for any archive size
// (RNF-01).
package sevenzip

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

// Binary is the 7-Zip executable (installed by scripts/install-7zip.sh).
const Binary = "7zz"

// noPassword is passed when the user gave none: a non-empty dummy makes 7zz
// fail fast on encrypted archives instead of waiting for a prompt.
const noPassword = "-videoexplorer-no-password-"

// attributeWarning is the warning of an extraction whose attributes 7zz could
// not set. It is logged, not shown: the data was verified (RF-05).
const attributeWarning = "attributes of the archive were not applied (the dataset does not allow it); the data was verified"

// Extractor implements application.Extractor.
type Extractor struct {
	bin string
}

var _ application.Extractor = (*Extractor)(nil)

// New builds an extractor using bin (empty means "7zz" from PATH).
func New(bin string) *Extractor {
	if bin == "" {
		bin = Binary
	}
	return &Extractor{bin: bin}
}

// Available reports whether the 7-Zip binary can be found.
func (e *Extractor) Available() bool {
	_, err := exec.LookPath(e.bin)
	return err == nil
}

func passwordArg(password string) string {
	if password == "" {
		return "-p" + noPassword
	}
	return "-p" + password
}

// List implements application.Extractor.
func (e *Extractor) List(ctx context.Context, archive, password string) (application.Listing, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, e.bin, "l", "-slt", "-ba", "-sccUTF-8", passwordArg(password), "--", archive) //nolint:gosec // fixed binary, archive path built by the app
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return application.Listing{}, ctx.Err()
		}
		return application.Listing{}, classify(err, stderr.String(), password)
	}
	return parseListing(stdout.String()), nil
}

// parseListing reads the `7zz l -slt` output: one block of key = value lines per entry.
func parseListing(out string) application.Listing {
	var l application.Listing
	var cur *application.ArchiveEntry
	flush := func() {
		if cur != nil && cur.Path != "" {
			l.Entries = append(l.Entries, *cur)
		}
		cur = nil
	}
	for line := range strings.SplitSeq(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimRight(line, "\r"), " = ")
		if !ok {
			if strings.TrimSpace(line) == "" {
				flush()
			}
			continue
		}
		if key == "Path" {
			flush()
			cur = &application.ArchiveEntry{Path: value}
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "Size":
			cur.Size, _ = strconv.ParseInt(value, 10, 64)
		case "CRC":
			cur.CRC = strings.ToUpper(value)
		case "Folder":
			cur.IsDir = value == "+"
		case "Attributes":
			cur.IsDir = cur.IsDir || strings.HasPrefix(value, "D")
			cur.Symlink = isSymlinkMode(value)
		case "Encrypted":
			cur.Encrypted = value == "+"
		}
	}
	flush()
	return l
}

// isSymlinkMode reports whether an Attributes value carries a Unix mode whose
// type is a link (`lrwxrwxrwx`), which 7zz prints for symbolic links.
func isSymlinkMode(attributes string) bool {
	for _, field := range strings.Fields(attributes) {
		if len(field) == 10 && field[0] == 'l' {
			return true
		}
	}
	return false
}

var percent = regexp.MustCompile(`(\d{1,3})%`)

// Extract implements application.Extractor.
func (e *Extractor) Extract(ctx context.Context, archive, dest, password string, listing application.Listing,
	progress func(int),
) (string, error) {
	// A zip whose names use the Windows separator is normalized after 7zz (RF-10c). Its unsafe names are
	// refused here, before anything is written.
	zipped, err := isZip(archive)
	if err != nil {
		return "", fmt.Errorf("%w: read archive: %w", application.ErrCorrupt, err)
	}
	var moves []separatorMove
	if zipped {
		if moves, err = planSeparators(listing.Entries); err != nil {
			return "", err
		}
	}

	args := []string{"x", "-y", "-bsp1", "-bso0", "-bse2", "-sccUTF-8", "-o" + dest, passwordArg(password), "--", archive}
	cmd := exec.CommandContext(ctx, e.bin, args...) //nolint:gosec // fixed binary, paths built by the app
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("%w: start 7zz: %w", application.ErrCorrupt, err)
	}

	sc := bufio.NewScanner(stdout)
	sc.Split(splitProgress)
	for sc.Scan() {
		if m := percent.FindStringSubmatch(sc.Text()); m != nil && progress != nil {
			pct, _ := strconv.Atoi(m[1])
			progress(pct)
		}
	}

	warning := ""
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := stderr.String()
		if !onlyAttributeErrors(msg) {
			return "", classify(err, msg, password)
		}
		// TrueNAS SMB datasets (NFSv4 ACL, aclmode=restricted) reject chmod, so
		// 7zz cannot apply Unix modes stored in archives made on Linux. The
		// data is intact: verify proves it below (phase 0.5 finding).
		warning = attributeWarning
	}
	if err := normalizeSeparators(dest, moves); err != nil {
		return "", err
	}
	if err := verify(dest, listing); err != nil {
		return "", err
	}
	return warning, nil
}

// verify checks every extracted file against the archive index (RF-05).
func verify(dest string, l application.Listing) error {
	for _, entry := range l.Entries {
		if entry.IsDir {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(strings.ReplaceAll(entry.Path, `\`, "/")))
		info, err := os.Lstat(p)
		if err != nil {
			return fmt.Errorf("%w: %s missing after extraction", application.ErrCorrupt, entry.Path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is not a regular file", application.ErrUnsafeEntry, entry.Path)
		}
		if info.Size() != entry.Size {
			return fmt.Errorf("%w: %s is %d bytes, the archive says %d", application.ErrMismatch, entry.Path, info.Size(), entry.Size)
		}
		if entry.CRC == "" {
			continue
		}
		got, err := fileCRC(p)
		if err != nil {
			return err
		}
		if got != entry.CRC {
			return fmt.Errorf("%w: %s CRC %s, the archive says %s", application.ErrMismatch, entry.Path, got, entry.CRC)
		}
	}
	return nil
}

// fileCRC streams a file through CRC32 (hardware-accelerated: GB/s).
func fileCRC(p string) (string, error) {
	f, err := os.Open(p) //nolint:gosec // path from our own extraction folder
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08X", h.Sum32()), nil
}

// classify maps 7zz failures to application errors: a password problem, or
// an archive that 7-Zip could not read (RF-10c, archive_corrupt).
func classify(err error, stderr, password string) error {
	switch {
	case strings.Contains(stderr, "No space left on device"):
		// The disk filled up while 7zz wrote the files (RF-10c): no_space, not a damaged archive.
		return fmt.Errorf("%w: %s", application.ErrNoSpace, firstLine(stderr))
	case strings.Contains(stderr, "Wrong password"), strings.Contains(stderr, "Cannot open encrypted archive"):
		if password == "" {
			return application.ErrPasswordRequired
		}
		return application.ErrWrongPassword
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%w: 7zz exited with %d: %s", application.ErrCorrupt, exitErr.ExitCode(), firstLine(stderr))
	}
	return fmt.Errorf("%w: 7zz: %w: %s", application.ErrCorrupt, err, firstLine(stderr))
}

// onlyAttributeErrors reports whether 7zz failed only to set file attributes
// (the errors of a dataset that does not allow them), and nothing else.
func onlyAttributeErrors(stderr string) bool {
	found := false
	for line := range strings.SplitSeq(stderr, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "ERRORS:"), strings.HasPrefix(line, "Sub items Errors:"):
		case strings.Contains(line, "Cannot set file attribute"):
			found = true
		case strings.HasPrefix(line, "ERROR:") && strings.Contains(line, "attribute"):
			found = true
		default:
			if strings.HasPrefix(line, "ERROR") {
				return false
			}
		}
	}
	return found
}

// splitProgress splits 7zz progress output, which redraws using \b and \r.
func splitProgress(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' || b == '\b' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
