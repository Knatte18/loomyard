//go:build !windows

// dobbytestmap_unix_integration_test.go pins startTestMap over a shell script that stands in for the lym binary.
// The test binary cannot stand in: tmuxkit.Main refuses a binary started with a positional first argument.

package beamcli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// standInGateScript is the stand-in for the lym binary.
// The test runs it as `sh gate testmap --upkeep`, so $0 is `gate` and the arguments after it are the verb's.
// It records whether its standard output is a pipe, its command line and its working directory.
// It then waits for the release file.
// It then exits with the code the code file holds, printing a failure line for a code other than 0.
const standInGateScript = `if [ -p /dev/stdout ]; then echo pipe > stdout-kind; else echo not-pipe > stdout-kind; fi
printf '%s\n%s\n' "$0 $*" "$(pwd -P)" > ran.tmp && mv ran.tmp ran.txt
i=0
while [ ! -e release ] && [ "$i" -lt 600 ]; do sleep 0.05; i=$((i+1)); done
code=$(cat code)
[ "$code" = 0 ] || echo "gate testmap: cannot register the upkeep pass"
exit "$code"
`

// standInPollTick and standInDeadline bound how long a test waits for the stand-in's record files and exit.
const (
	standInPollTick = 10 * time.Millisecond
	standInDeadline = 10 * time.Second
)

// awaitFile returns once a file exists at path, and fails the test when none does within standInDeadline.
func awaitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(standInDeadline)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not appear within %s", path, standInDeadline)
		}
		time.Sleep(standInPollTick)
	}
}

func TestStartTestMap(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name     string
		code     int
		wantExit dobbyengine.PassExit
	}{
		{name: "a clean exit is delivered with code 0 and no error text", code: 0, wantExit: dobbyengine.PassExit{}},
		{
			name:     "a failed exit is delivered with its code and the error text its output carries",
			code:     1,
			wantExit: dobbyengine.PassExit{Code: 1, Error: "gate testmap: cannot register the upkeep pass"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			if err := os.WriteFile(filepath.Join(anchor, "gate"), []byte(standInGateScript), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(anchor, "code"), []byte(strconv.Itoa(tt.code)), 0o644); err != nil {
				t.Fatal(err)
			}
			// The release runs before the anchor's removal, so the stand-in is not left waiting.
			t.Cleanup(func() { _ = os.WriteFile(filepath.Join(anchor, "release"), nil, 0o644) })
			lookup := func() (string, error) { return "/bin/sh", nil }

			exit, err := startTestMap(lookup, anchor)
			if err != nil {
				t.Fatalf("startTestMap() = _, %v; want nil", err)
			}

			awaitFile(t, filepath.Join(anchor, "ran.txt"))
			ran, err := os.ReadFile(filepath.Join(anchor, "ran.txt"))
			if err != nil {
				t.Fatal(err)
			}
			wantDir, err := filepath.EvalSymlinks(anchor)
			if err != nil {
				t.Fatal(err)
			}
			if want := "gate testmap --upkeep\n" + wantDir + "\n"; string(ran) != want {
				t.Errorf("the child recorded %q; want its command line and working directory %q", ran, want)
			}
			kind, err := os.ReadFile(filepath.Join(anchor, "stdout-kind"))
			if err != nil {
				t.Fatal(err)
			}
			if string(kind) != "not-pipe\n" {
				t.Errorf("the child's standard output is %q; want a file, so a dead dobby cannot break the pass's writes", kind)
			}
			select {
			case got := <-exit:
				t.Fatalf("exit delivered %+v while the child still waits; want nothing yet", got)
			default:
			}

			if err := os.WriteFile(filepath.Join(anchor, "release"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-exit:
				if got != tt.wantExit {
					t.Errorf("exit = %+v; want %+v", got, tt.wantExit)
				}
			case <-time.After(standInDeadline):
				t.Fatalf("no exit delivered within %s of the release", standInDeadline)
			}
		})
	}
}
