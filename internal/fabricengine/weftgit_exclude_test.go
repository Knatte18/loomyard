//go:build integration

// weftgit_exclude_test.go proves fabric's own lock artifacts (.weft/ from
// Fabric.Commit's write lock, .gitrepo-push.lock from PushCoalesced) never
// surface as untracked dirt in the weft worktree: ensureWeftLockDir seeds
// them into the weft repo's info/exclude, so Remove's no-force dirty gate
// (a raw `git status --porcelain`, untracked included) never dead-ends on
// them. It also proves every module's machine-local artifact — locks, pause
// flags, rendered prompts — is kept out of weft history structurally, by
// living under .lyx and so never falling inside a weft-commit pathspec,
// rather than by an exclude-layer pattern.
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
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
	f, err := fabricengine.NewPairedForTest(warpPath, weftFixture.WeftPath)
	if err != nil {
		t.Fatalf("fabricengine.NewPairedForTest(%q, %q): %v", warpPath, weftFixture.WeftPath, err)
	}
	return f, weftFixture
}

// newFabricAtRelPath returns a *fabricengine.Fabric pairing a fresh
// newWarpFixture warp repo — recorded, via a written .lyx-anchor marker,
// as anchored at rel — with the given SHARED weftPath. This is what lets a
// single test exercise multiple RelPath depths against one weft checkout,
// mirroring the real "multiple hubs at different RelPath depths share one
// weft checkout" shape: distinct warp repos (distinct Hubs, each recording
// its own anchor), all paired to the same weft worktree, rather than one
// Fabric handle whose RelPath is fixed at construction.
func newFabricAtRelPath(t *testing.T, weftPath, rel string) *fabricengine.Fabric {
	t.Helper()

	warpPath := newWarpFixture(t)
	hub := filepath.Dir(warpPath)
	seedRepoWideFabricConfig(t, hub)
	if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(hub), lyxcwd.AnchorFileName), []byte(rel), 0o644); err != nil {
		t.Fatalf("write .lyx-anchor: %v", err)
	}

	f, err := fabricengine.NewPairedForTest(warpPath, weftPath)
	if err != nil {
		t.Fatalf("fabricengine.NewPairedForTest(%q, %q): %v", warpPath, weftPath, err)
	}
	return f
}

// writeWeftConfig overwrites the tracked _lyx/config.yaml file both
// CopyWeft fixtures ship with, the standard way this file dirties a weft
// worktree's pathspec-covered content.
func writeWeftConfig(t *testing.T, weftPath, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, lyxdirs.LyxDirName, "config.yaml"), []byte(content), 0o644); err != nil {
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

// TestCommitWeft_LockArtifactsExcludedFromStatus commits scoped weft content (which creates the
// .weft lock dir) and drops a push lock file, then asserts neither artifact appears in `git status
// --porcelain` — the exact check Remove's no-force weft dirty gate runs.
func TestCommitWeft_LockArtifactsExcludedFromStatus(t *testing.T) {
	f, weftFixture := newFabricPair(t)
	writeWeftConfig(t, weftFixture.WeftPath, "modified for exclude test")

	if result, err := f.Commit([]string{lyxdirs.LyxDirName}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
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

// nonEmptyExcludeLines splits raw info/exclude content into its non-empty,
// non-comment lines, in file order — the shape seedWeftArtifactExcludes'
// entries take, with git's own boilerplate `#`-comment lines filtered out.
func nonEmptyExcludeLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

// TestCommitWeft_SeedsFabricArtifactsOnlyAndIsIdempotent proves
// seedWeftArtifactExcludes' post-retirement shape: it seeds exactly fabric's
// own .weft/ lock directory and gitrepo's push-lock filename into the weft
// repo's info/exclude, none of the retired cross-module machine-local
// patterns, and re-seeding via a second commit leaves the file
// byte-identical.
func TestCommitWeft_SeedsFabricArtifactsOnlyAndIsIdempotent(t *testing.T) {
	f, weftFixture := newFabricPair(t)
	writeWeftConfig(t, weftFixture.WeftPath, "modified for exclude test")

	if _, err := f.Commit([]string{lyxdirs.LyxDirName}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	excludePath := filepath.Join(weftFixture.WeftPath, ".git", "info", "exclude")
	first, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read info/exclude: %v", err)
	}

	wantLines := []string{".weft/", gitrepo.PushLockFileName}
	gotLines := nonEmptyExcludeLines(string(first))
	if len(gotLines) != len(wantLines) {
		t.Fatalf("info/exclude entries = %v; want exactly %v", gotLines, wantLines)
	}
	for i, want := range wantLines {
		if gotLines[i] != want {
			t.Errorf("info/exclude entry %d = %q; want %q (full: %v)", i, gotLines[i], want, gotLines)
		}
	}

	retiredPatterns := []string{
		"**/" + lyxdirs.LyxDirName + "/*/**/*.lock",
		"**/" + lyxdirs.LyxDirName + "/*/pause",
		"**/" + lyxdirs.LyxDirName + "/*/prompts/",
	}
	for _, retired := range retiredPatterns {
		if strings.Contains(string(first), retired) {
			t.Errorf("info/exclude seeds retired cross-module pattern %q; want it gone", retired)
		}
	}

	// A second Commit re-runs ensureWeftLockDir/seedWeftArtifactExcludes;
	// the exclude file must come out byte-identical, not merely
	// equivalent, proving the re-seed is a true no-op.
	writeWeftConfig(t, weftFixture.WeftPath, "modified again for exclude test")
	if _, err := f.Commit([]string{lyxdirs.LyxDirName}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("second Commit: %v", err)
	}
	second, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read info/exclude after second commit: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("info/exclude changed on re-seed:\nfirst:\n%s\nsecond:\n%s", first, second)
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

// TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth proves the structural
// replacement for F-B's fix (formerly CONSTRAINTS.md's Weft Git Invariant, "Cross-module
// exclusions"): fabric's OWN sync pathspec — fabricengine.ScopedPathspec(relPath,
// []string{lyxdirs.LyxDirName}), positive entries only, no exclusions, the exact shape
// internal/fabriccli/weft_verbs.go builds for `lyx fabric sync`/`lyx config <module> --set ...` —
// never stages another module's lock file, pause flag, or rendered-prompt directory, because those
// now live under .lyx and so fall outside the _lyx-scoped pathspec entirely rather than being
// caught by a git-exclude pattern.
//
// Exercised at the weft worktree root AND at two nested RelPath depths in the SAME weft checkout
// (multiple host hubs share one weft worktree at different RelPath offsets) — proving the
// exclusion holds at every depth, not just the root. A durable per-module state file is written
// under _lyx and committed alongside the .lyx artifacts at every depth, proving the property is
// exact and does not over-match real state.
func TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth(t *testing.T) {
	weftFixture := lyxtest.CopyWeft(t)

	relPaths := []string{".", "sub", "wts/some-task"}
	for _, rel := range relPaths {
		dotLyxDir := filepath.Join(weftFixture.WeftPath, filepath.FromSlash(rel), lyxdirs.DotLyxDirName)
		mustWriteFile(t, filepath.Join(dotLyxDir, "builder", "run.lock"), "lock")
		mustWriteFile(t, filepath.Join(dotLyxDir, "builder", "pause"), "")
		mustWriteFile(t, filepath.Join(dotLyxDir, "webster", "pause"), "")
		mustWriteFile(t, filepath.Join(dotLyxDir, "webster", "prompts", "01.md"), "prompt")

		lyxDir := filepath.Join(weftFixture.WeftPath, filepath.FromSlash(rel), lyxdirs.LyxDirName)
		mustWriteFile(t, filepath.Join(lyxDir, "builder", "state.json"), "{}")
	}

	for _, rel := range relPaths {
		// Each rel gets its own Fabric handle (its own warp repo/Hub, anchored
		// at rel) sharing this one weft worktree, since Fabric.Commit
		// classifies a call's files against its own Fabric's fixed RelPath.
		f := newFabricAtRelPath(t, weftFixture.WeftPath, rel)
		pathspec := fabricengine.ScopedPathspec(rel, []string{lyxdirs.LyxDirName})
		if _, err := f.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Commit(rel=%q, pathspec=%v): %v", rel, pathspec, err)
		}
	}

	tracked := gitLsFiles(t, weftFixture.WeftPath)
	for _, rel := range relPaths {
		lyxRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(rel), lyxdirs.LyxDirName))
		durable := lyxRel + "/builder/state.json"
		if !strings.Contains(tracked, durable) {
			t.Errorf("git ls-files at rel=%q does not track durable %q; want it committed\nfull ls-files:\n%s", rel, durable, tracked)
		}
	}

	for _, forbidden := range []string{".lock", "pause", "prompts"} {
		if strings.Contains(tracked, forbidden) {
			t.Errorf("git ls-files tracks a %q-named entry; want none\nfull ls-files:\n%s", forbidden, tracked)
		}
	}
}
