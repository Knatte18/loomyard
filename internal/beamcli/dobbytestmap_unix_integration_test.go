//go:build !windows

// dobbytestmap_unix_integration_test.go pins startTestMap over a shell script that stands in for the lym binary.
// The test binary cannot stand in: tmuxkit.Main refuses a binary started with a positional first argument.

package beamcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// standInGateScript is the stand-in for the lym binary.
// The test runs it as `sh gate testmap --max N`, so $0 is `gate` and the arguments after it are the verb's.
// It records its command line and working directory, waits for the release file, and then records that it finished.
// A child that was ended early never writes the finish record.
const standInGateScript = `printf '%s\n%s\n' "$0 $*" "$(pwd -P)" > ran.tmp && mv ran.tmp ran.txt
i=0
while [ ! -e release ] && [ "$i" -lt 600 ]; do sleep 0.05; i=$((i+1)); done
: > done.tmp && mv done.tmp done.txt
`

// standInPollTick and standInDeadline bound how long a test waits for the stand-in's record files.
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
		name string
		// unwritableRegistry puts the run-proc directory below a regular file, so registering the child fails.
		unwritableRegistry bool
	}{
		{name: "the child runs gate testmap in the anchor and is registered as the test-map refresh"},
		{name: "a registration that fails leaves the child running and returns no error", unwritableRegistry: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			runProcDir := filepath.Join(t.TempDir(), "runproc")
			if tt.unwritableRegistry {
				blocker := filepath.Join(t.TempDir(), "blocker")
				if err := os.WriteFile(blocker, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				runProcDir = filepath.Join(blocker, "runproc")
			}
			if err := os.WriteFile(filepath.Join(anchor, "gate"), []byte(standInGateScript), 0o644); err != nil {
				t.Fatal(err)
			}
			// The release runs before the anchor's removal, so the stand-in is not left waiting.
			t.Cleanup(func() { _ = os.WriteFile(filepath.Join(anchor, "release"), nil, 0o644) })
			lookup := func() (string, error) { return "/bin/sh", nil }

			if err := startTestMap(lookup, anchor, runProcDir, 3); err != nil {
				t.Fatalf("startTestMap() = %v; want nil", err)
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
			if want := "gate testmap --max 3\n" + wantDir + "\n"; string(ran) != want {
				t.Errorf("the child recorded %q; want its command line and working directory %q", ran, want)
			}
			if tt.unwritableRegistry {
				if records, err := runproc.List(runProcDir, runproc.HubOwner); err == nil && len(records) != 0 {
					t.Fatalf("List() = %+v; want no record, since the registration failed", records)
				}
			} else {
				records, err := runproc.List(runProcDir, runproc.HubOwner)
				if err != nil {
					t.Fatalf("List() = _, %v; want nil", err)
				}
				if len(records) != 1 || records[0].Role != runproc.RoleTestMap {
					t.Fatalf("List() = %+v; want one record with role %q", records, runproc.RoleTestMap)
				}
				if want := []string{"/bin/sh", "gate", "testmap", "--max", "3"}; !slices.Equal(records[0].Argv, want) {
					t.Errorf("registered argv = %q; want %q", strings.Join(records[0].Argv, " "), strings.Join(want, " "))
				}
			}

			// A child that is still running after the start returned finishes once released.
			if err := os.WriteFile(filepath.Join(anchor, "release"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			awaitFile(t, filepath.Join(anchor, "done.txt"))
		})
	}
}
