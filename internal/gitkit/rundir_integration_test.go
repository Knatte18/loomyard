//go:build integration

// rundir_integration_test.go pins that a test binary running under tmuxkit.Main keeps all its temp files in a run directory of the lyx temp root and leaves nothing behind, after a passing and after a failing run.
// The test re-executes the gitkit test binary as its own helper.

package gitkit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtemp"
)

const (
	// helperModeEnv selects helper mode and carries the outcome the helper ends with, "pass" or "fail".
	helperModeEnv = "GITKIT_RUNDIR_HELPER"

	// helperRecordEnv names the file the helper writes its recorded paths to, one per line.
	helperRecordEnv = "GITKIT_RUNDIR_RECORD"
)

func TestRunDirectory_HoldsEveryTempFileAndIsGoneAfterTheRun(t *testing.T) {
	t.Parallel()
	if mode := os.Getenv(helperModeEnv); mode != "" {
		record := strings.Join([]string{os.Getenv("GIT_CONFIG_GLOBAL"), os.TempDir(), t.TempDir()}, "\n")
		if err := os.WriteFile(os.Getenv(helperRecordEnv), []byte(record), 0o600); err != nil {
			t.Fatal(err)
		}
		if mode == "fail" {
			t.Fatal("deliberate failure")
		}
		return
	}

	parentRun, err := lyxtemp.Dir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := lyxtemp.ProcessRoot()
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"pass", "fail"} {
		record := filepath.Join(t.TempDir(), "recorded-"+mode)
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
		cmd.Env = append(os.Environ(), helperModeEnv+"="+mode, helperRecordEnv+"="+record)
		err := cmd.Run()
		var exit *exec.ExitError
		switch {
		case mode == "pass" && err != nil:
			t.Fatalf("%s helper failed: %v", mode, err)
		case mode == "fail" && (!errors.As(err, &exit) || exit.ExitCode() == 0):
			t.Fatalf("%s helper error = %v, want a non-zero exit", mode, err)
		}

		recorded, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("%s helper recorded nothing: %v", mode, err)
		}
		paths := strings.Split(string(recorded), "\n")
		if len(paths) != 3 {
			t.Fatalf("%s helper recorded %q, want three paths", mode, recorded)
		}
		helperRun := runDirectoryOf(t, root.Path(), paths[0])
		if helperRun == parentRun {
			t.Fatalf("%s helper used the parent's run directory %s", mode, parentRun)
		}
		for _, path := range paths {
			if got := runDirectoryOf(t, root.Path(), path); got != helperRun {
				t.Errorf("%s helper path %s is in run directory %s, want %s", mode, path, got, helperRun)
			}
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s helper path %s survived the run: %v", mode, path, err)
			}
		}
		if _, err := os.Lstat(helperRun); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s helper run directory %s survived the run: %v", mode, helperRun, err)
		}
	}
}

// runDirectoryOf returns the run directory directly under root that holds path, failing the test when path lies elsewhere.
func runDirectoryOf(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		t.Fatalf("path %s is outside the lyx temp root %s", path, root)
	}
	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	return filepath.Join(root, first)
}
