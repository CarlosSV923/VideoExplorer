package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// toolTimeout bounds one run of an external program (`7zz i`, `ffprobe -version`).
const toolTimeout = 3 * time.Second

// maxExcerpt is how much of a program's output a failure message quotes.
const maxExcerpt = 200

// WritableDir checks that this process can create and remove a hidden file in
// dir (RF-63, RNF-03). On TrueNAS the usual mistake is a dataset without the
// Modify ACL for the container user, so the failure names that user and asks
// for the permission.
func WritableDir(name string, code httpapi.CheckCode, dir string) Check {
	clean := filepath.Clean(dir)
	return Check{Name: name, Code: code, Run: func(context.Context) *Failure {
		fail := func(err error) *Failure {
			uid, gid := os.Getuid(), os.Getgid()
			return &Failure{
				Path: clean,
				UID:  &uid,
				GID:  &gid,
				Message: fmt.Sprintf("cannot create and remove a file in %s as uid=%d gid=%d: %v. "+
					"Give that user the Modify permission (ACL, inherited) on the dataset",
					clean, uid, gid, err),
			}
		}
		f, err := os.CreateTemp(clean, ".videoexplorer-write-check-*")
		if err != nil {
			return fail(err)
		}
		path := f.Name()
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return fail(err)
		}
		if err := os.Remove(path); err != nil {
			return fail(err)
		}
		return nil
	}}
}

// Runner runs the external programs of the Tool checks. The composition root
// uses ExecRunner; tests inject a fake so they need no binaries.
type Runner interface {
	// LookPath finds the program in the PATH, as exec.LookPath does.
	LookPath(file string) (string, error)
	// Output runs the program at path and returns its combined output.
	Output(ctx context.Context, path string, args ...string) ([]byte, error)
}

// ExecRunner runs programs with os/exec.
type ExecRunner struct{}

// LookPath implements Runner.
func (ExecRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Output implements Runner. The program and its arguments are fixed by the
// composition root, never by a request.
func (ExecRunner) Output(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput() //nolint:gosec // fixed program and arguments
}

// Tool checks that an external program is in the PATH and starts: it runs
// args (for example "-version") and must exit 0 within toolTimeout (RF-63,
// RNF-03a). A missing or broken program fails with code, so the UI can name it.
func Tool(name string, code httpapi.CheckCode, runner Runner, program string, args ...string) Check {
	return Check{Name: name, Code: code, Run: func(ctx context.Context) *Failure {
		path, err := runner.LookPath(program)
		if err != nil {
			return &Failure{Message: fmt.Sprintf("%s is not in PATH: %v", program, err)}
		}
		ctx, cancel := context.WithTimeout(ctx, toolTimeout)
		defer cancel()
		out, err := runner.Output(ctx, path, args...)
		if err != nil {
			return &Failure{Message: fmt.Sprintf("%s %s did not start: %v. Output: %s",
				program, strings.Join(args, " "), err, excerpt(out))}
		}
		return nil
	}}
}

// excerpt returns the first line of a program's output, shortened for a message.
func excerpt(out []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if line == "" {
		return "no output"
	}
	if len(line) > maxExcerpt {
		line = strings.ToValidUTF8(line[:maxExcerpt], "") + "..."
	}
	return line
}
