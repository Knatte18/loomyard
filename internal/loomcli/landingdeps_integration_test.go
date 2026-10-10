//go:build integration

// landingdeps_integration_test.go drives landingDeps' config-change seams over a real hub: ConfigChanges reads a pair's committed config files through real branches,
// and Notify queues into the prime's orch notice directory.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestLandingDeps_ConfigNoticeSeamsOverRealHub asserts ConfigChanges reports a per-worktree module's changed config file and never a hub-wide module's, that Notify queues one notice in the prime's notice directory only once an orch strand is recorded, and that CommitParentRecords commits the parent's run records alone, and nothing while it holds none.
// The steps share one hub and one prime orch state,
// and the queueing step relies on the no-strand step having queued nothing,
// so none runs in parallel.
func TestLandingDeps_ConfigNoticeSeamsOverRealHub(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task")
	taskRecords := h.PairRecordsSibling("task")
	gitkit.CommitFile(t, taskRecords, configengine.ConfigFileRel("loom"), "task_setting: changed\n", "task: change loom config")
	gitkit.CommitFile(t, taskRecords, configengine.ConfigFileRel("board"), "task_setting: changed\n", "task: change board config")

	taskLocation, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("task"))
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(task) error = %v; want nil", err)
	}
	deps := landingDeps(taskLocation, websterengine.Geometry{}, "task", "https://example.com/o.git", "main",
		true, func() error { return nil }, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, planVerifySource(taskLocation))

	t.Run("ConfigChanges reports the per-worktree file and not the hub-wide one", func(t *testing.T) {
		changes, err := deps.ConfigChanges()
		if err != nil {
			t.Fatalf("ConfigChanges() error = %v; want nil", err)
		}
		want := filepath.ToSlash(configengine.ConfigFileRel("loom"))
		if len(changes.Files) != 1 || changes.Files[0] != want {
			t.Errorf("ConfigChanges().Files = %q; want exactly [%q]", changes.Files, want)
		}
	})

	paths := orchcli.PrimePaths(h.Location)
	noticeCount := func() int {
		entries, err := os.ReadDir(paths.NoticesDir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("ReadDir(%q) error = %v; want nil", paths.NoticesDir, err)
		}
		return len(entries)
	}

	if !t.Run("Notify with no orch strand recorded returns nil and queues nothing", func(t *testing.T) {
		if err := deps.Notify("loom: a notice"); err != nil {
			t.Fatalf("Notify() error = %v; want nil", err)
		}
		if got := noticeCount(); got != 0 {
			t.Errorf("notice files = %d; want 0 without an orch strand", got)
		}
	}) {
		return
	}

	t.Run("Notify with an orch strand recorded queues one notice in the prime's directory", func(t *testing.T) {
		if err := orchengine.SaveState(paths, orchengine.State{Strand: "orch-strand-guid"}); err != nil {
			t.Fatalf("SaveState() error = %v; want nil", err)
		}
		if err := deps.Notify("loom: a notice"); err != nil {
			t.Fatalf("Notify() error = %v; want nil", err)
		}
		if got := noticeCount(); got != 1 {
			t.Errorf("notice files = %d; want 1", got)
		}
	})

	primeRecords := h.PrimeRecords()
	headBefore := gitkit.Git(t, primeRecords, "rev-parse", "HEAD")
	if !t.Run("CommitParentRecords commits nothing while the parent holds no run record", func(t *testing.T) {
		if err := deps.CommitParentRecords(); err != nil {
			t.Fatalf("CommitParentRecords() error = %v; want nil", err)
		}
		if got := gitkit.Git(t, primeRecords, "rev-parse", "HEAD"); got != headBefore {
			t.Errorf("parent HEAD = %s, was %s; want no commit", got, headBefore)
		}
	}) {
		return
	}

	t.Run("CommitParentRecords commits the parent's run records and leaves a dirty path elsewhere alone", func(t *testing.T) {
		record := filepath.Join(primeRecords, shedrun.RunsRootRel(), "sibling", "status.json")
		if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		elsewhere := filepath.Join(primeRecords, "elsewhere.txt")
		if err := os.WriteFile(elsewhere, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := deps.CommitParentRecords(); err != nil {
			t.Fatalf("CommitParentRecords() error = %v; want nil", err)
		}
		if got := gitkit.Git(t, primeRecords, "status", "--porcelain"); got != "?? elsewhere.txt" {
			t.Errorf("parent status = %q; want only the path outside the run records left dirty", got)
		}
		want := "loom: run records checkpoint for landing " + taskLocation.WorktreeName
		if got := gitkit.Git(t, primeRecords, "log", "-1", "--format=%s"); got != want {
			t.Errorf("parent commit subject = %q; want %q, naming the landing slug", got, want)
		}
	})
}
