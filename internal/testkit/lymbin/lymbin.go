// Package lymbin builds the `lym` binary from `./cmd/lym` for `integration`, `tmux` and `llm` tests.
//
// It is the one kit exempt from the Testkit Invariant's spawn-import rule,
// and it runs nothing but that `go build` of ./cmd/lym and the built binary under group kill.
// A test binary builds `lym` at most once, or not at all when `gateslot.PrebuiltLymEnv` names a binary a gate run already built from the same tree.
// Only tagged test files may call it; Test Tier Purity bans the `lymbin.` token elsewhere.
package lymbin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// ErrStalePrebuilt is returned, wrapped with the path, when `gateslot.PrebuiltLymEnv` names a file that is missing or not executable.
var ErrStalePrebuilt = errors.New("lymbin: the prebuilt lym binary is missing or not executable")

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

// prebuilt reads `gateslot.PrebuiltLymEnv`.
// Unset answers ("", false, nil), an existing executable file answers (path, true, nil), and anything else answers ErrStalePrebuilt wrapped with the path.
func prebuilt() (string, bool, error) {
	path := os.Getenv(gateslot.PrebuiltLymEnv)
	if path == "" {
		return "", false, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return "", false, fmt.Errorf("%w: %s", ErrStalePrebuilt, path)
	}
	return path, true, nil
}

// compile builds `./cmd/lym` from the module root, which scankit.ModuleRoot finds from the test binary's starting directory, into dir and returns the binary path.
// An empty ldflags omits the `-ldflags` argument.
func compile(dir, ldflags string) (string, error) {
	root, err := scankit.ModuleRoot()
	if err != nil {
		return "", fmt.Errorf("lymbin: locate the module root: %w", err)
	}

	bin := filepath.Join(dir, "lym")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/lym")

	cmd := exec.Command("go", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build ./cmd/lym: %w\n%s", err, out)
	}
	return bin, nil
}

// BuildInto returns the binary `gateslot.PrebuiltLymEnv` names when ldflags is empty and the variable is set.
// Otherwise it builds `./cmd/lym` from the module root into dir, with ldflags as `-ldflags` when non-empty, and returns the binary path.
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
// An empty ldflags returns the prebuilt binary when one is set; every other build goes into a fresh `lymtemp.MkdirTemp("lymbin-")` in the process run directory.
// A non-empty ldflags always builds, since a stamped build can never be the prebuilt one.
func buildOnce(ldflags string) (string, error) {
	key := os.Getenv(gateslot.PrebuiltLymEnv) + "\x00" + ldflags
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
		dir, err := lymtemp.MkdirTemp("lymbin-")
		if err != nil {
			result.err = err
			return
		}
		result.path, result.err = compile(dir, ldflags)
	})
	return result.path, result.err
}

// ErrTimeout is returned, wrapped with the arguments, when Run's command outlives its timeout.
var ErrTimeout = errors.New("lymbin: the command outlived its timeout")

// Run runs bin with args in dir, bounded by timeout, in its own process group, and returns its combined output and exit code.
// The process environment is inherited, so the hermetic git variables reach the child.
// A deadline kills the whole group, so no descendant outlives the command, and answers exit code -1 with ErrTimeout.
// Any exit of the command answers a nil error; the error is otherwise the start failure.
// It calls no testing.TB method, so a concurrent caller can use it.
func Run(bin, dir string, timeout time.Duration, args ...string) (output string, exitCode int, err error) {
	return RunWithEnv(bin, dir, nil, timeout, args...)
}

// RunWithEnv is Run with extraEnv appended to the inherited environment of the child alone,
// so a parallel test can set a variable for one invocation without t.Setenv.
func RunWithEnv(bin, dir string, extraEnv []string, timeout time.Duration, args ...string) (output string, exitCode int, err error) {
	ctx, cancel := context.WithTimeoutCause(context.Background(), timeout, fmt.Errorf("%w after %s", ErrTimeout, timeout))
	defer cancel()
	return runUntilDone(ctx, bin, dir, extraEnv, args...)
}

// runUntilDone runs bin with args in dir, in its own process group, until it exits or ctx ends, and returns its combined output and exit code.
// extraEnv, when non-nil, is appended to the inherited environment.
// A ctx whose cause wraps ErrTimeout kills the whole group and answers exit code -1 with that cause, so the caller decides when the timeout starts.
// Any exit of the command answers a nil error; the error is otherwise the start failure.
func runUntilDone(ctx context.Context, bin, dir string, extraEnv []string, args ...string) (output string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	proc.ConfigureGroupKill(cmd, func(int, error) {})
	out, runErr := cmd.CombinedOutput()
	if cause := context.Cause(ctx); errors.Is(cause, ErrTimeout) {
		return string(out), -1, fmt.Errorf("%w: %v in %s; output so far:\n%s", cause, args, dir, out)
	}
	if runErr == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), -1, fmt.Errorf("lymbin: start %s %v: %w", bin, args, runErr)
}

// Build returns the `lym` binary, built at most once per test binary.
// The binary outlives the calling test.
func Build(tb testing.TB) string {
	tb.Helper()
	return BuildWithLDFlags(tb, "")
}

// PutFirstOnPath copies the built `lym` binary into a temp directory under the name `lym` and puts that directory first on `PATH` for the test.
// The built binary may carry a content-derived name, which a shell looking up `lym` would not find.
// It sets process-global state through t.Setenv, so the calling test is not parallel.
func PutFirstOnPath(t *testing.T) {
	t.Helper()
	built := Build(t)
	data, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	name := "lym"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
