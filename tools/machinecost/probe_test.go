// probe_test.go runs whole probes with counts of one against temp folders.
// It pins the report's measurement names, the skipped parts,
// and that every folder a probe made is gone, after a failed run too.

package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
)

func TestRunProbe_MeasuresEveryPartAndLeavesNoFolders(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	dirs := []NamedDir{{Name: "a", Path: t.TempDir()}, {Name: "b", Path: t.TempDir()}}
	config := ProbeConfig{Dirs: dirs, Starts: 1, FileOps: 1, Binaries: 1, GitRounds: 1}
	startedAt := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	report, err := runProbe(t.Context(), config, startedAt, io.Discard)
	if err != nil {
		t.Fatalf("runProbe() error: %v", err)
	}

	want := []string{
		"start/true/median_us", "start/true/children_user_us", "start/true/children_sys_us",
		"start/git_version/median_us", "start/git_version/children_user_us", "start/git_version/children_sys_us",
		"newbinary/a/first_run_ms", "newbinary/a/second_run_ms",
		"fileops/a/create_write_close_delete_us", "fileops/a/stat_us", "fileops/a/mkdir_rmdir_us",
		"fileops/a/write_4k_us", "fileops/a/write_4k_fsync_us",
		"newbinary/b/first_run_ms", "newbinary/b/second_run_ms",
		"fileops/b/create_write_close_delete_us", "fileops/b/stat_us", "fileops/b/mkdir_rmdir_us",
		"fileops/b/write_4k_us", "fileops/b/write_4k_fsync_us",
		"git/init/wall_ms", "git/init/user_ms", "git/init/sys_ms", "git/init/processes",
		"git/clone/wall_ms", "git/clone/user_ms", "git/clone/sys_ms", "git/clone/processes",
		"git/add/wall_ms", "git/add/user_ms", "git/add/sys_ms", "git/add/processes",
		"git/commit/wall_ms", "git/commit/user_ms", "git/commit/sys_ms", "git/commit/processes",
		"git/push/wall_ms", "git/push/user_ms", "git/push/sys_ms", "git/push/processes",
	}
	got := make([]string, 0, len(report.Measurements))
	for _, m := range report.Measurements {
		got = append(got, m.Name)
		if m.Unit == unitCount && m.Value < 1 {
			t.Errorf("measurement %s = %v; want at least one process", m.Name, m.Value)
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("runProbe() measurement names mismatch (-want +got):\n%s", diff)
	}
	wantSkipped := []string{"start/lym_help: no lym on PATH and no -lym flag"}
	if diff := cmp.Diff(wantSkipped, report.Skipped); diff != "" {
		t.Errorf("runProbe() skipped mismatch (-want +got):\n%s", diff)
	}
	assertNoProbeFolders(t, dirs[0].Path, dirs[1].Path, filepath.Dir(report.TempDir))

	noDirs, err := runProbe(t.Context(), ProbeConfig{Starts: 1, FileOps: 1, Binaries: 1, GitRounds: 1}, startedAt, io.Discard)
	if err != nil {
		t.Fatalf("runProbe() without dirs error: %v", err)
	}
	wantSkipped = append(wantSkipped, "newbinary, fileops: no -dir folder given")
	if diff := cmp.Diff(wantSkipped, noDirs.Skipped); diff != "" {
		t.Errorf("runProbe() without dirs skipped mismatch (-want +got):\n%s", diff)
	}
}

// Not parallel: it looks for leftovers in the process's run folder, where a probe running beside it has its own.
func TestRunProbe_FailedRunLeavesNoFolders(t *testing.T) {
	tiergate.Require(t, 1)
	valid := t.TempDir()
	regularFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(regularFile, nil, 0o644); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	dirs := []NamedDir{{Name: "a", Path: valid}, {Name: "b", Path: regularFile}}
	config := ProbeConfig{Dirs: dirs, Starts: 1, FileOps: 1, Binaries: 1, GitRounds: 1}

	if _, err := runProbe(t.Context(), config, time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC), io.Discard); err == nil {
		t.Fatal("runProbe() error = nil; want the folder that is a file to fail it")
	}

	runDir, err := lymtemp.Dir()
	if err != nil {
		t.Fatalf("lymtemp.Dir() error: %v", err)
	}
	assertNoProbeFolders(t, valid, runDir)
}

// assertNoProbeFolders reports every folder of the probe's making still present in folders.
func assertNoProbeFolders(t *testing.T, folders ...string) {
	t.Helper()
	for _, folder := range folders {
		leftovers, err := filepath.Glob(filepath.Join(folder, probeFolderPrefix+"*"))
		if err != nil {
			t.Fatalf("glob %s: %v", folder, err)
		}
		if len(leftovers) != 0 {
			t.Errorf("runProbe() left %v behind", leftovers)
		}
	}
}
