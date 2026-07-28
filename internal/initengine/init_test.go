//go:build integration

// init_test.go — tests for Init.
//
// Tests verify that Init activates junctions and reconciles config only when
// a weft pairing exists. Tests seeding a weft worktree fixture via lyxtest to
// provide the paired environment that Init requires.

package initengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestInit_FirstRun(t *testing.T) {
	// Use a paired fixture (host + weft) so Init has a weft pairing to activate.
	f := lyxtest.CopyPairedLocal(t)

	result, err := Init(f.Layout.WorktreeRoot)
	if err != nil {
		t.Fatalf("Init() = %v; want nil", err)
	}

	// This is the regression guard for the ordering fix (card 16's whole reason for
	// existing): both LyxDir and PatternDir must report "created" on a genuine first
	// run. Before the fix, WireJunctions' seeder had already materialised the
	// weft-side target by the time the host path was stated, so LyxDir would
	// silently (and incorrectly) report "exists" here.
	if result.LyxDir != "created" {
		t.Errorf("result.LyxDir = %q; want %q on first run", result.LyxDir, "created")
	}
	if result.PatternDir != "created" {
		t.Errorf("result.PatternDir = %q; want %q on first run", result.PatternDir, "created")
	}

	// Verify _lyx/config/ directories exist
	configDir := hubgeometry.ConfigDir(f.Layout.WorktreeRoot)
	if _, err := os.Stat(configDir); err != nil {
		t.Fatalf("_lyx/config not created: %v", err)
	}

	// Verify both junctions resolve and both weft directories exist.
	slug := filepath.Base(f.Layout.WorktreeRoot)
	if _, err := os.Stat(f.Layout.HostLyxLink(slug)); err != nil {
		t.Errorf("host _lyx junction does not resolve: %v", err)
	}
	if _, err := os.Stat(f.Layout.HostPatternLink(slug)); err != nil {
		t.Errorf("host _pattern junction does not resolve: %v", err)
	}
	if _, err := os.Stat(f.Layout.WeftLyxDirFor(slug)); err != nil {
		t.Errorf("weft _lyx directory does not exist: %v", err)
	}
	if _, err := os.Stat(f.Layout.WeftPatternDirFor(slug)); err != nil {
		t.Errorf("weft _pattern directory does not exist: %v", err)
	}

	// Verify both config files exist
	for _, module := range []string{"board", "fabric"} {
		cfgPath := hubgeometry.ConfigFile(f.Layout.WorktreeRoot, module)
		if _, err := os.Stat(cfgPath); err != nil {
			t.Errorf("%s.yaml not created: %v", module, err)
		}
	}

	// Verify .gitignore has the managed block
	gitignorePath := filepath.Join(f.Layout.WorktreeRoot, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
	contentStr := string(content)
	if !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Error(".gitignore missing start marker")
	}
	if !strings.Contains(contentStr, ".lyx/") {
		t.Error(".gitignore missing .lyx/ entry")
	}

	// Init -> configsync.ReconcileAll iterates configreg.Modules(), so the
	// registry is the single source of truth for the expected module count;
	// deriving it here means a newly registered module can never stale this
	// assertion.
	want := len(configreg.Modules())
	if len(result.Modules) != want {
		t.Errorf("len(result.Modules) = %d; want %d (from configreg.Modules())", len(result.Modules), want)
	}

	// Verify strict loads pass
	t.Run("StrictLoadsPass", func(t *testing.T) {
		_, err := boardengine.LoadConfig(f.Layout.WorktreeRoot, "board")
		if err != nil {
			t.Errorf("board.LoadConfig failed: %v", err)
		}

		_, err = fabricengine.LoadConfig(f.Layout.WorktreeRoot)
		if err != nil {
			t.Errorf("fabric.LoadConfig failed: %v", err)
		}
	})
}

func TestInit_Idempotent(t *testing.T) {
	// Use a paired fixture (host + weft) so Init has a weft pairing to activate.
	f := lyxtest.CopyPairedLocal(t)

	// First run
	if _, err := Init(f.Layout.WorktreeRoot); err != nil {
		t.Fatalf("first Init() = %v; want nil", err)
	}

	// Capture files and gitignore after first run
	boardPath := hubgeometry.ConfigFile(f.Layout.WorktreeRoot, "board")
	content1, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}

	gitignorePath := filepath.Join(f.Layout.WorktreeRoot, ".gitignore")
	gitignore1, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	// Second run
	result2, err := Init(f.Layout.WorktreeRoot)
	if err != nil {
		t.Fatalf("second Init() = %v; want nil", err)
	}

	// Verify files unchanged
	content2, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml after second run: %v", err)
	}
	if string(content1) != string(content2) {
		t.Error("board.yaml changed on second run (should be idempotent)")
	}

	// Verify gitignore unchanged
	gitignore2, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore after second run: %v", err)
	}
	if string(gitignore1) != string(gitignore2) {
		t.Error(".gitignore changed on second run (should be idempotent)")
	}

	// Verify result indicates no changes
	if result2.LyxDir != "exists" {
		t.Errorf("result2.LyxDir = %q; want %q", result2.LyxDir, "exists")
	}
	if result2.PatternDir != "exists" {
		t.Errorf("result2.PatternDir = %q; want %q", result2.PatternDir, "exists")
	}
	if result2.Gitignore != "unchanged" {
		t.Errorf("result2.Gitignore = %q; want %q", result2.Gitignore, "unchanged")
	}
	for _, m := range result2.Modules {
		if m.Applied {
			t.Errorf("module %s reports Applied=true on second run (should be idempotent)", m.Module)
		}
	}
}

// TestInit_NotAGitRepo verifies that Init run against a non-git temp directory
// surfaces hubgeometry's bare ErrNotAGitRepo sentinel with no
// "failed to resolve layout:" prefix and no raw "fatal:" git stderr.
func TestInit_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := Init(tmpDir)
	if err == nil {
		t.Fatal("Init() = nil; want error when not a git repository")
	}
	if err.Error() != "not a git repository" {
		t.Errorf("Init() error = %q; want exactly \"not a git repository\"", err.Error())
	}
}

// TestInit_NoPairing verifies that Init reports and returns early when
// no weft pairing exists (unpaired host from dormant Add).
//
// NOTE: This test intentionally does NOT use a paired fixture; it creates a bare
// git repo with no weft sibling to simulate the unpaired state.
func TestInit_NoPairing(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a bare git repo (no weft sibling).
	_, _, exitCode, initErr := gitexec.RunGit([]string{"init"}, tmpDir)
	if initErr != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", initErr, exitCode)
	}

	// Run Init; should report no pairing and return early.
	_, err := Init(tmpDir)
	if err == nil {
		t.Fatal("Init() = nil; want error when no pairing exists")
	}
	if !strings.Contains(err.Error(), "no weft pairing") {
		t.Errorf("Init() error missing 'no weft pairing'; got: %v", err)
	}

	// Verify .gitignore and config were NOT created (no reconciliation occurred).
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		t.Error(".gitignore should not exist when no pairing")
	}

	configDir := hubgeometry.ConfigDir(tmpDir)
	if _, err := os.Stat(configDir); err == nil {
		t.Error("_lyx/config should not exist when no pairing")
	}
}
