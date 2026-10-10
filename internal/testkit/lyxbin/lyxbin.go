// Package lyxbin builds the `lyx` binary from `./cmd/lyx` for `integration`, `tmux` and `llm` tests.
//
// It is the one kit exempt from the Testkit Invariant's spawn-import rule,
// and it runs nothing but that `go build`.
// A test binary builds `lyx` at most once, or not at all when `gateslot.PrebuiltLyxEnv` names a binary a gate run already built from the same tree.
// Only tagged test files may call it; Test Tier Purity bans the `lyxbin.` token elsewhere.
package lyxbin

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

// ErrStalePrebuilt is returned, wrapped with the path, when `gateslot.PrebuiltLyxEnv` names a file that is missing or not executable.
var ErrStalePrebuilt = errors.New("lyxbin: the prebuilt lyx binary is missing or not executable")

// buildResult is one cached build, filled by the first caller of its key.
type buildResult struct {
	once sync.Once
	path string
	err  error
}

var (
	// buildsMu guards builds.
	buildsMu sync.Mutex
	// builds holds one result per prebuilt-variable value and ldflags pair, for the life of the test binary.
	builds = map[string]*buildResult{}
)

// prebuilt reads `gateslot.PrebuiltLyxEnv`.
// Unset answers ("", false, nil), an existing executable file answers (path, true, nil), and anything else answers ErrStalePrebuilt wrapped with the path.
func prebuilt() (string, bool, error) {
	path := os.Getenv(gateslot.PrebuiltLyxEnv)
	if path == "" {
		return "", false, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return "", false, fmt.Errorf("%w: %s", ErrStalePrebuilt, path)
	}
	return path, true, nil
}

// compile builds `./cmd/lyx` from the module root into dir and returns the binary path.
// An empty ldflags omits the `-ldflags` argument.
func compile(dir, ldflags string) (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("lyxbin: could not determine kit source location")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))

	bin := filepath.Join(dir, "lyx")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/lyx")

	cmd := exec.Command("go", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build ./cmd/lyx: %w\n%s", err, out)
	}
	return bin, nil
}

// BuildInto returns the binary `gateslot.PrebuiltLyxEnv` names when ldflags is empty and the variable is set.
// Otherwise it builds `./cmd/lyx` from the module root into dir, with ldflags as `-ldflags` when non-empty, and returns the binary path.
// The error carries the build's combined output, or wraps ErrStalePrebuilt when the variable names no executable file.
func BuildInto(dir, ldflags string) (string, error) {
	if ldflags == "" {
		path, ok, err := prebuilt()
		if err != nil || ok {
			return path, err
		}
	}
	return compile(dir, ldflags)
}

// buildOnce returns the binary for ldflags, building it at most once per test binary and per pair of prebuilt-variable value and ldflags.
// An empty ldflags returns the prebuilt binary when one is set; every other build goes into a fresh `os.MkdirTemp("", "lyxbin-")`, which `tmuxkit.Main` has pointed at the package's private directory.
// A non-empty ldflags always builds, since a stamped build can never be the prebuilt one.
func buildOnce(ldflags string) (string, error) {
	key := os.Getenv(gateslot.PrebuiltLyxEnv) + "\x00" + ldflags
	buildsMu.Lock()
	result, ok := builds[key]
	if !ok {
		result = &buildResult{}
		builds[key] = result
	}
	buildsMu.Unlock()

	result.once.Do(func() {
		if ldflags == "" {
			if path, ok, err := prebuilt(); err != nil || ok {
				result.path, result.err = path, err
				return
			}
		}
		dir, err := os.MkdirTemp("", "lyxbin-")
		if err != nil {
			result.err = err
			return
		}
		result.path, result.err = compile(dir, ldflags)
	})
	return result.path, result.err
}

// Build returns the `lyx` binary, built at most once per test binary.
// The binary outlives the calling test.
func Build(tb testing.TB) string {
	tb.Helper()
	return BuildWithLDFlags(tb, "")
}

// BuildWithLDFlags is Build with the given linker flags.
func BuildWithLDFlags(tb testing.TB, ldflags string) string {
	tb.Helper()
	bin, err := buildOnce(ldflags)
	if err != nil {
		tb.Fatalf("%v", err)
	}
	return bin
}
