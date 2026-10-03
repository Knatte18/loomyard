// Package lyxbin builds the `lyx` binary from `./cmd/lyx` for `integration` and `smoke` tests.
//
// It is the one kit exempt from the Testkit Invariant's spawn-import rule,
// and it runs nothing but that `go build`.
// Only tagged test files may call it; Test Tier Purity bans the `lyxbin.` token elsewhere.
package lyxbin

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// BuildInto builds `./cmd/lyx` from the module root into dir and returns the binary path.
// An empty ldflags omits the `-ldflags` argument.
// The error carries the build's combined output.
func BuildInto(dir, ldflags string) (string, error) {
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

// Build builds the binary into tb's temp dir, which lives only as long as the calling test.
// A caller that builds once per test binary calls BuildInto on a longer-lived directory instead.
func Build(tb testing.TB) string {
	tb.Helper()
	return BuildWithLDFlags(tb, "")
}

// BuildWithLDFlags is Build with the given linker flags.
func BuildWithLDFlags(tb testing.TB, ldflags string) string {
	tb.Helper()
	bin, err := BuildInto(tb.TempDir(), ldflags)
	if err != nil {
		tb.Fatalf("%v", err)
	}
	return bin
}
