//go:build integration

// weftgit_exclude_test.go proves fabric's own lock artifacts (.weft/ from
// Fabric.Commit's write lock, .gitrepo-push.lock from PushCoalesced) never
// surface as untracked dirt in the weft worktree: ensureWeftLockDir seeds
// them into the weft repo's info/exclude, so Remove's no-force dirty gate
// (a raw `git status --porcelain`, untracked included) cannot dead-end on
// artifacts a pathspec-scoped `fabric sync` can never clear.
//
// Package fabricengine_test to construct a real *fabricengine.Fabric against
// isolated warp/weft fixtures; shares the TestMain in testmain_test.go.
// newWarpFixture/newFabricPair/writeWeftConfig/gitStatusPorcelain below are
// this file's own fixture helpers, relocated here from
// weftgit_differential_test.go before its deletion (this file was already
// its only other consumer).

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newWarpFixture creates a minimal, isolated git repo at t.TempDir() on
// branch main with one commit — everything fabricengine.New/Fabric.Commit
// need from a warp repo, without any of fabric's own topology wiring.
func newWarpFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("warp"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// newFabricPair returns a *fabricengine.Fabric paired with a fresh
// newWarpFixture warp repo and a fresh lyxtest.CopyWeft weft fixture. The
// repo-wide fabric config is seeded at warpPath's parent directory (this
// fixture's stand-in Hub) so Fabric.Commit's RepoWiredNames read succeeds —
// Fabric.Commit classifies paths against that repo-wide config, unlike the
// pathspec-only CommitWeft this file's tests previously called.
func newFabricPair(t *testing.T) (*fabricengine.Fabric, lyxtest.WeftFixture) {
	t.Helper()

	warpPath := newWarpFixture(t)
	seedRepoWideFabricConfig(t, filepath.Dir(warpPath))
	weftFixture := lyxtest.CopyWeft(t)
	f, err := fabricengine.New(warpPath, weftFixture.WeftPath)
	if err != nil {
		t.Fatalf("fabricengine.New(%q, %q): %v", warpPath, weftFixture.WeftPath, err)
	}
	return f, weftFixture
}

// newFabricAtRelPath returns a *fabricengine.Fabric pairing a fresh
// newWarpFixture warp repo — recorded, via a written .fabric-anchor marker,
// as anchored at rel — with the given SHARED weftPath. This is what lets a
// single test exercise multiple RelPath depths against one weft checkout,
// mirroring the real "multiple hubs at different RelPath depths share one
// weft checkout" shape crossModuleMachineLocalExcludes is designed for:
// distinct warp repos (distinct Hubs, each recording its own anchor), all
// paired to the same weft worktree, rather than one Fabric handle whose
// RelPath is fixed at construction.
func newFabricAtRelPath(t *testing.T, weftPath, rel string) *fabricengine.Fabric {
	t.Helper()

	warpPath := newWarpFixture(t)
	hub := filepath.Dir(warpPath)
	seedRepoWideFabricConfig(t, hub)
	if err := os.WriteFile(filepath.Join(hubgeometry.BoardDir(hub), hubgeometry.FabricAnchorName), []byte(rel), 0o644); err != nil {
		t.Fatalf("write .fabric-anchor: %v", err)
	}

	f, err := fabricengine.New(warpPath, weftPath)
	if err != nil {
		t.Fatalf("fabricengine.New(%q, %q): %v", warpPath, weftPath, err)
	}
	return f
}

// writeWeftConfig overwrites the tracked _lyx/config.yaml file both
// CopyWeft fixtures ship with, the standard way this file dirties a weft
// worktree's pathspec-covered content.
func writeWeftConfig(t *testing.T, weftPath, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, "_lyx", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// gitStatusPorcelain returns `git status --porcelain`'s raw output for
// repoPath.
func gitStatusPorcelain(t *testing.T, repoPath string) string {
	t.Helper()

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain in %s: %v", repoPath, err)
	}
	return string(out)
}

// TestCommitWeft_LockArtifactsExcludedFromStatus commits scoped weft content
// (which creates the .weft lock dir) and drops a push lock file, then asserts
// neither artifact appears in `git status --porcelain` — the exact check
// Remove's no-force weft dirty gate runs.
func TestCommitWeft_LockArtifactsExcludedFromStatus(t *testing.T) {
	f, weftFixture := newFabricPair(t)
	writeWeftConfig(t, weftFixture.WeftPath, "modified for exclude test")

	if result, err := f.Commit([]string{"_lyx"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("Commit: %v", err)
	} else if !result.WeftCommitted {
		t.Fatal("Commit WeftCommitted = false; want true")
	}

	// Precondition: the commit really did materialize the lock dir — the
	// artifact this test claims is excluded must actually exist on disk.
	if _, err := os.Stat(filepath.Join(weftFixture.WeftPath, ".weft")); err != nil {
		t.Fatalf(".weft lock dir missing after Commit: %v", err)
	}

	// Materialize the push lock artifact the way an interrupted PushCoalesced
	// would leave it: a plain file at the worktree root.
	pushLock := filepath.Join(weftFixture.WeftPath, gitrepo.PushLockFileName)
	if err := os.WriteFile(pushLock, nil, 0o644); err != nil {
		t.Fatalf("write push lock artifact: %v", err)
	}

	status := gitStatusPorcelain(t, weftFixture.WeftPath)
	if strings.Contains(status, ".weft") {
		t.Errorf("git status --porcelain reports .weft as dirt: %q; want it git-excluded", status)
	}
	if strings.Contains(status, gitrepo.PushLockFileName) {
		t.Errorf("git status --porcelain reports %s as dirt: %q; want it git-excluded", gitrepo.PushLockFileName, status)
	}
}

// mustWriteFile writes content to path, creating parent directories as
// needed, failing the test on any error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// gitLsFiles returns `git ls-files`'s raw output for repoPath — the tracked
// (committed-or-staged) file set, as opposed to gitStatusPorcelain's
// untracked/dirty view.
func gitLsFiles(t *testing.T, repoPath string) string {
	t.Helper()

	cmd := exec.Command("git", "ls-files")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v", repoPath, err)
	}
	return string(out)
}

// TestCommitWeft_CrossModuleMachineLocalArtifactsExcludedAtAnyDepth proves
// F-B's fix (CONSTRAINTS.md's Weft Git Invariant, "Cross-module exclusions"):
// fabric's OWN sync pathspec — fabricengine.ScopedPathspec(relPath,
// []string{"_lyx"}), positive entries only, no exclusions, the exact shape
// internal/fabriccli/weft_verbs.go builds for `lyx fabric sync`/`lyx config
// <module> --set ...` — never stages another module's lock file, pause
// flag, or rendered-prompt directory. Before this fix, that exact pathspec
// permanently tracked all of them, because the caller had no exclusion
// lever at all.
//
// Exercised at the weft worktree root AND at two nested RelPath depths in
// the SAME weft checkout (multiple host hubs share one weft worktree at
// different RelPath offsets) — proving crossModuleMachineLocalExcludes'
// `**/` prefix actually reaches every depth, not just the root. A durable
// per-module state file is written and committed alongside the excluded
// artifacts at every depth, proving the exclusion is exact and does not
// over-match real state.
func TestCommitWeft_CrossModuleMachineLocalArtifactsExcludedAtAnyDepth(t *testing.T) {
	weftFixture := lyxtest.CopyWeft(t)

	relPaths := []string{".", "sub", "wts/some-task"}
	for _, rel := range relPaths {
		lyxDir := filepath.Join(weftFixture.WeftPath, filepath.FromSlash(rel), "_lyx")
		mustWriteFile(t, filepath.Join(lyxDir, "builder", "run.lock"), "lock")
		mustWriteFile(t, filepath.Join(lyxDir, "builder", "pause"), "")
		mustWriteFile(t, filepath.Join(lyxDir, "builder", "state.json"), "{}")
		mustWriteFile(t, filepath.Join(lyxDir, "webster", "pause"), "")
		mustWriteFile(t, filepath.Join(lyxDir, "webster", "prompts", "01.md"), "prompt")
	}

	for _, rel := range relPaths {
		// Each rel gets its own Fabric handle (its own warp repo/Hub, anchored
		// at rel) sharing this one weft worktree, since Fabric.Commit
		// classifies a call's files against its own Fabric's fixed RelPath.
		f := newFabricAtRelPath(t, weftFixture.WeftPath, rel)
		pathspec := fabricengine.ScopedPathspec(rel, []string{"_lyx"})
		if _, err := f.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Commit(rel=%q, pathspec=%v): %v", rel, pathspec, err)
		}
	}

	tracked := gitLsFiles(t, weftFixture.WeftPath)
	for _, rel := range relPaths {
		lyxRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(rel), "_lyx"))
		for _, excluded := range []string{
			lyxRel + "/builder/run.lock",
			lyxRel + "/builder/pause",
			lyxRel + "/webster/pause",
			lyxRel + "/webster/prompts/01.md",
		} {
			if strings.Contains(tracked, excluded) {
				t.Errorf("git ls-files at rel=%q tracks %q; want it excluded\nfull ls-files:\n%s", rel, excluded, tracked)
			}
		}

		durable := lyxRel + "/builder/state.json"
		if !strings.Contains(tracked, durable) {
			t.Errorf("git ls-files at rel=%q does not track durable %q; want it committed\nfull ls-files:\n%s", rel, durable, tracked)
		}
	}
}
