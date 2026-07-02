//go:build integration

// undo_test.go — tests for the lyx init --undo command.
//
// Tests cover the happy-path reversal, the two clean-no-op cases (never
// initialized, never weft-paired), idempotency of running --undo twice, the
// two hard-error junction-inconsistency guards (real directory, target
// mismatch) that must leave everything untouched, and partial-recovery from
// a prior interrupted --undo run. Tests seed fixtures via lyxtest, mirroring
// initcli_test.go's existing pattern.

package initcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/initcli"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// assertField asserts that result[key] is the string want.
func assertField(t *testing.T, result map[string]any, key, want string) {
	t.Helper()
	got, _ := result[key].(string)
	if got != want {
		t.Errorf("result[%q] = %q; want %q", key, got, want)
	}
}

// mustParseJSON unmarshals buf into a map, failing the test on error.
func mustParseJSON(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}
	return result
}

// mustReadFile reads path, failing the test on error.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// readExcludeContent resolves and reads the host worktree's .git/info/exclude
// file, mirroring the resolution logic in warpengine's seedGitExclude /
// unseedGitExclude so tests observe the same path the production code writes to.
func readExcludeContent(t *testing.T, l *hubgeometry.Layout, slug string) string {
	t.Helper()

	worktreePath := l.WorktreePath(slug)
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if err != nil || exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed: %v (exit %d)", err, exitCode)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}
	return string(content)
}

// excludeContainsLine reports whether content contains name as a line-exact match.
func excludeContainsLine(content, name string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// snapshotDir returns a deterministic, sorted newline-joined listing of every
// path under dir (relative to dir), for before/after equality checks on
// directory trees that must be left untouched by an aborted --undo.
func snapshotDir(t *testing.T, dir string) string {
	t.Helper()

	var entries []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}

// TestRunInit_Undo_HappyPath verifies that --undo fully reverses a prior init:
// the host junction, the weft-side content, the .gitignore block, and the
// .git/info/exclude entry are all gone, and the weft-side deletion was
// committed (push is exercised via WEFT_SKIP_PUSH, not asserted here — the
// real push path is covered separately by TestRunInit_Undo_PartialRecovery/b).
func TestRunInit_Undo_HappyPath(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)
	// CopyPairedLocal's weft-prime origin is left pointing at the shared
	// template bare (never rewritten); skip push so --undo cannot reach it.
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var buf bytes.Buffer
	if code := initcli.RunInit(&buf, []string{}); code != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
	}

	var buf2 bytes.Buffer
	code := initcli.RunInit(&buf2, []string{"--undo"})
	if code != 0 {
		t.Fatalf("RunInit(--undo) = %d; want 0, output: %s", code, buf2.String())
	}

	result := mustParseJSON(t, &buf2)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok flag is not true; output: %s", buf2.String())
	}
	assertField(t, result, "lyx_junction", "removed")
	assertField(t, result, "weft_content", "cleared")
	assertField(t, result, "git_exclude", "reverted")
	assertField(t, result, "gitignore", "reverted")

	// Host junction is gone.
	hostLink := f.Layout.HostLyxLinkHere()
	if _, statErr := os.Lstat(hostLink); !os.IsNotExist(statErr) {
		t.Errorf("host junction %s still exists after --undo", hostLink)
	}

	// Weft-side _lyx directory is gone.
	weftLyxDir := f.Layout.WeftLyxDir()
	if _, statErr := os.Stat(weftLyxDir); !os.IsNotExist(statErr) {
		t.Errorf("weft _lyx dir %s still exists after --undo", weftLyxDir)
	}

	// The deletion was committed: the _lyx pathspec is clean (scoped like
	// weftengine.Status's own dirty check, so the untracked .weft lock
	// directory Commit itself creates does not count as dirty).
	stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--", hubgeometry.LyxDirName}, f.Layout.WeftWorktree())
	if err != nil || exitCode != 0 {
		t.Fatalf("git status in weft worktree failed: %v (exit %d)", err, exitCode)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("weft worktree _lyx pathspec not clean after --undo: %q", stdout)
	}

	// The .gitignore managed block is fully removed, not just emptied.
	gitignorePath := filepath.Join(f.Layout.WorktreeRoot, ".gitignore")
	gitignoreContent := mustReadFile(t, gitignorePath)
	if strings.Contains(gitignoreContent, "lyx-managed") {
		t.Errorf(".gitignore still contains the managed-block marker: %q", gitignoreContent)
	}
	if strings.Contains(gitignoreContent, ".lyx/") {
		t.Errorf(".gitignore still contains the .lyx/ entry: %q", gitignoreContent)
	}

	// The .git/info/exclude line is gone.
	excludeContent := readExcludeContent(t, f.Layout, filepath.Base(f.Layout.WorktreeRoot))
	if excludeContainsLine(excludeContent, hubgeometry.LyxDirName) {
		t.Errorf(".git/info/exclude still contains %q line after --undo", hubgeometry.LyxDirName)
	}
}

// TestRunInit_Undo_NeverInitialized verifies that --undo is a clean no-op on
// a directory that was never lyx-initialized (host or weft side).
func TestRunInit_Undo_NeverInitialized(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)
	t.Setenv("WEFT_SKIP_PUSH", "1")

	// lyxtest.CopyPairedLocal's weft-prime template always pre-seeds
	// _lyx/config/placeholder purely as fixture scaffolding; production
	// warpengine spawn code never creates this file, so it does not reflect
	// a real never-initialized directory. Remove it (and the now-empty
	// _lyx/config and _lyx directories) so the fixture genuinely represents
	// "no weft-side content, no host init ever ran."
	weftLyxDir := filepath.Join(f.WeftPrime, hubgeometry.LyxDirName)
	if err := os.RemoveAll(weftLyxDir); err != nil {
		t.Fatalf("remove weft-prime placeholder _lyx: %v", err)
	}

	var buf bytes.Buffer
	code := initcli.RunInit(&buf, []string{"--undo"})
	if code != 0 {
		t.Fatalf("RunInit(--undo) = %d; want 0, output: %s", code, buf.String())
	}

	result := mustParseJSON(t, &buf)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok flag is not true; output: %s", buf.String())
	}
	assertField(t, result, "lyx_junction", "not_present")
	assertField(t, result, "weft_content", "not_present")
	assertField(t, result, "git_exclude", "unchanged")
	assertField(t, result, "gitignore", "unchanged")
	if _, hasError := result["error"]; hasError {
		t.Errorf("result has unexpected error field; output: %s", buf.String())
	}
}

// TestRunInit_Undo_NoWeftPairing covers the truly-unpaired host case (no weft
// sibling worktree at all — not merely "never init'd" but "never warp add'd
// either"). --undo must not create a stray weft sibling as a side effect.
func TestRunInit_Undo_NoWeftPairing(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a bare git repo (no weft sibling), mirroring
	// initcli_test.go's TestRunInit_NoPairing fixture.
	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	t.Chdir(tmpDir)
	t.Setenv("WEFT_SKIP_PUSH", "1")

	l, err := hubgeometry.Resolve(tmpDir)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve: %v", err)
	}

	var buf bytes.Buffer
	code := initcli.RunInit(&buf, []string{"--undo"})
	if code != 0 {
		t.Fatalf("RunInit(--undo) = %d; want 0, output: %s", code, buf.String())
	}

	result := mustParseJSON(t, &buf)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok flag is not true; output: %s", buf.String())
	}
	assertField(t, result, "weft_content", "not_present")
	if _, hasError := result["error"]; hasError {
		t.Errorf("result has unexpected error field; output: %s", buf.String())
	}

	// No <slug>-weft directory (nor a stray .weft lock dir under it) was
	// created as a side effect of the --undo call.
	if _, statErr := os.Stat(l.WeftWorktree()); !os.IsNotExist(statErr) {
		t.Errorf("--undo created a stray weft worktree at %s (stat err: %v)", l.WeftWorktree(), statErr)
	}
}

// TestRunInit_Undo_Idempotent verifies that running --undo a second time in a
// row (after a prior init and a first successful --undo) is a clean no-op,
// matching TestRunInit_Undo_NeverInitialized's expected output shape.
func TestRunInit_Undo_Idempotent(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var buf bytes.Buffer
	if code := initcli.RunInit(&buf, []string{}); code != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
	}

	var buf1 bytes.Buffer
	if code := initcli.RunInit(&buf1, []string{"--undo"}); code != 0 {
		t.Fatalf("first RunInit(--undo) = %d; want 0, output: %s", code, buf1.String())
	}

	var buf2 bytes.Buffer
	code := initcli.RunInit(&buf2, []string{"--undo"})
	if code != 0 {
		t.Fatalf("second RunInit(--undo) = %d; want 0, output: %s", code, buf2.String())
	}

	result := mustParseJSON(t, &buf2)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok flag is not true; output: %s", buf2.String())
	}
	assertField(t, result, "lyx_junction", "not_present")
	assertField(t, result, "weft_content", "not_present")
	assertField(t, result, "git_exclude", "unchanged")
	assertField(t, result, "gitignore", "unchanged")
}

// TestRunInit_Undo_RealDirectoryGuard verifies that --undo hard-errors and
// leaves everything untouched when the host _lyx path has been externally
// corrupted into a real directory after a prior init.
func TestRunInit_Undo_RealDirectoryGuard(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var buf bytes.Buffer
	if code := initcli.RunInit(&buf, []string{}); code != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
	}

	// Simulate external corruption: replace the junction with a real
	// directory containing a file.
	hostLink := f.Layout.HostLyxLinkHere()
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("remove junction to replace it: %v", err)
	}
	if err := os.MkdirAll(hostLink, 0o755); err != nil {
		t.Fatalf("mkdir real directory at %s: %v", hostLink, err)
	}
	marker := filepath.Join(hostLink, "marker.txt")
	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	weftLyxDir := f.Layout.WeftLyxDir()
	weftContentBefore := snapshotDir(t, weftLyxDir)
	gitignorePath := filepath.Join(f.Layout.WorktreeRoot, ".gitignore")
	gitignoreBefore := mustReadFile(t, gitignorePath)
	excludeBefore := readExcludeContent(t, f.Layout, filepath.Base(f.Layout.WorktreeRoot))

	var buf2 bytes.Buffer
	code := initcli.RunInit(&buf2, []string{"--undo"})
	if code == 0 {
		t.Fatalf("RunInit(--undo) = 0; want non-zero (error) on real-directory guard, output: %s", buf2.String())
	}

	result := mustParseJSON(t, &buf2)
	if _, hasError := result["error"]; !hasError {
		t.Errorf("result missing error field; output: %s", buf2.String())
	}

	// The real directory and its content must be untouched.
	content := mustReadFile(t, marker)
	if content != "real content" {
		t.Errorf("marker file content changed: %q", content)
	}

	// The weft-side content must be untouched.
	if got := snapshotDir(t, weftLyxDir); got != weftContentBefore {
		t.Errorf("weft _lyx content changed; want untouched on abort\nbefore: %s\nafter:  %s", weftContentBefore, got)
	}

	// The .gitignore managed block must be untouched.
	if got := mustReadFile(t, gitignorePath); got != gitignoreBefore {
		t.Error(".gitignore changed; want untouched on abort")
	}

	// The .git/info/exclude entry must be untouched.
	if got := readExcludeContent(t, f.Layout, filepath.Base(f.Layout.WorktreeRoot)); got != excludeBefore {
		t.Error(".git/info/exclude changed; want untouched on abort")
	}
}

// TestRunInit_Undo_TargetMismatch verifies that --undo hard-errors and leaves
// everything untouched when the host junction has been externally
// re-pointed at an unrelated directory after a prior init.
func TestRunInit_Undo_TargetMismatch(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var buf bytes.Buffer
	if code := initcli.RunInit(&buf, []string{}); code != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
	}

	// Replace the valid junction with one pointing at an unrelated directory.
	hostLink := f.Layout.HostLyxLinkHere()
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("remove junction to replace it: %v", err)
	}
	wrongTarget := filepath.Join(f.Hub, "unrelated-target")
	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
		t.Fatalf("mkdir unrelated target: %v", err)
	}
	if err := fslink.CreateDirLink(hostLink, wrongTarget); err != nil {
		t.Fatalf("CreateDirLink(%s, %s): %v", hostLink, wrongTarget, err)
	}

	weftLyxDir := f.Layout.WeftLyxDir()
	weftContentBefore := snapshotDir(t, weftLyxDir)
	gitignorePath := filepath.Join(f.Layout.WorktreeRoot, ".gitignore")
	gitignoreBefore := mustReadFile(t, gitignorePath)
	excludeBefore := readExcludeContent(t, f.Layout, filepath.Base(f.Layout.WorktreeRoot))

	var buf2 bytes.Buffer
	code := initcli.RunInit(&buf2, []string{"--undo"})
	if code == 0 {
		t.Fatalf("RunInit(--undo) = 0; want non-zero (error) on target mismatch, output: %s", buf2.String())
	}

	result := mustParseJSON(t, &buf2)
	if _, hasError := result["error"]; !hasError {
		t.Errorf("result missing error field; output: %s", buf2.String())
	}

	// The mismatched junction must still exist and still point at the wrong target.
	isLink, err := fslink.IsLink(hostLink)
	if err != nil {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if !isLink {
		t.Errorf("junction %s no longer a link after aborted --undo", hostLink)
	}
	resolved, err := fslink.PointsTo(hostLink)
	if err != nil {
		t.Fatalf("fslink.PointsTo(%s): %v", hostLink, err)
	}
	wantResolved, err := filepath.EvalSymlinks(wrongTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", wrongTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf("junction target = %s; want still pointing at wrong target %s", resolved, wantResolved)
	}

	// The weft-side content, .gitignore, and .git/info/exclude must all be untouched.
	if got := snapshotDir(t, weftLyxDir); got != weftContentBefore {
		t.Errorf("weft _lyx content changed; want untouched on abort\nbefore: %s\nafter:  %s", weftContentBefore, got)
	}
	if got := mustReadFile(t, gitignorePath); got != gitignoreBefore {
		t.Error(".gitignore changed; want untouched on abort")
	}
	if got := readExcludeContent(t, f.Layout, filepath.Base(f.Layout.WorktreeRoot)); got != excludeBefore {
		t.Error(".git/info/exclude changed; want untouched on abort")
	}
}

// TestRunInit_Undo_PartialRecovery covers recovery from a --undo run that was
// interrupted partway through: part (a) simulates a crash right after the
// junction was removed but before weft content was cleared; part (b)
// simulates a crash right after the weft-side deletion was committed but
// before it was pushed (the "Push runs unconditionally" Shared Decision).
func TestRunInit_Undo_PartialRecovery(t *testing.T) {
	t.Run("a", func(t *testing.T) {
		f := lyxtest.CopyPairedLocal(t)
		t.Chdir(f.Layout.WorktreeRoot)
		t.Setenv("WEFT_SKIP_PUSH", "1")

		var buf bytes.Buffer
		if code := initcli.RunInit(&buf, []string{}); code != 0 {
			t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
		}

		// Simulate a crash between removing the junction and clearing weft
		// content: remove only the host junction, leaving weft content in place.
		hostLink := f.Layout.HostLyxLinkHere()
		if err := fslink.Remove(hostLink); err != nil {
			t.Fatalf("remove host junction: %v", err)
		}

		var buf2 bytes.Buffer
		code := initcli.RunInit(&buf2, []string{"--undo"})
		if code != 0 {
			t.Fatalf("recovery RunInit(--undo) = %d; want 0, output: %s", code, buf2.String())
		}

		result := mustParseJSON(t, &buf2)
		if ok, _ := result["ok"].(bool); !ok {
			t.Errorf("ok flag is not true; output: %s", buf2.String())
		}
		assertField(t, result, "lyx_junction", "not_present")
		assertField(t, result, "weft_content", "cleared")

		weftLyxDir := f.Layout.WeftLyxDir()
		if _, statErr := os.Stat(weftLyxDir); !os.IsNotExist(statErr) {
			t.Errorf("weft _lyx dir %s still exists after recovery --undo", weftLyxDir)
		}
	})

	t.Run("b", func(t *testing.T) {
		// CopyPairedLocal's weft-prime origin points at an unrewritten shared
		// template bare, unsupported as a real push target; use CopyPaired,
		// whose bares are rewritten per-test, so the real push below is safe.
		f := lyxtest.CopyPaired(t)
		t.Chdir(f.Layout.WorktreeRoot)

		// lyxtest.CopyPaired's weft-prime template (unlike CopyWeft's) has no
		// upstream tracking established; a real weft worktree gets that from
		// warpengine.Add's own push -u during spawn (weftwiring.go), so
		// establish the same baseline here before simulating the partial run.
		lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "push", "-u", "origin", "main")

		var buf bytes.Buffer
		if code := initcli.RunInit(&buf, []string{}); code != 0 {
			t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
		}

		// Manually perform the weft-side deletion and commit (mirroring what
		// step 4 of runUndo would do) but do not push, simulating a prior
		// --undo run that committed locally but failed to push. runUndo's
		// step 3 (junction removal) always runs before step 4, so a run that
		// reached step 4 necessarily already removed the host junction too;
		// mirror that here so the full --undo call below sees an
		// already-clean junction step (no-op) rather than a corrupted one
		// (unseedLyxJunction validates the weft-side target still exists
		// before touching the link, which the deletion below removes).
		hostLink := f.Layout.HostLyxLinkHere()
		if err := fslink.Remove(hostLink); err != nil {
			t.Fatalf("remove host junction: %v", err)
		}
		weftLyxDir := f.Layout.WeftLyxDir()
		if err := os.RemoveAll(weftLyxDir); err != nil {
			t.Fatalf("remove weft _lyx dir: %v", err)
		}
		lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "add", "--", hubgeometry.LyxDirName)
		lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "commit", "-m", "lyx init --undo: clear _lyx")

		// Run --undo without WEFT_SKIP_PUSH set, so the real push path
		// executes and recovers the stranded local commit.
		var buf2 bytes.Buffer
		code := initcli.RunInit(&buf2, []string{"--undo"})
		if code != 0 {
			t.Fatalf("recovery RunInit(--undo) = %d; want 0, output: %s", code, buf2.String())
		}

		result := mustParseJSON(t, &buf2)
		if ok, _ := result["ok"].(bool); !ok {
			t.Errorf("ok flag is not true; output: %s", buf2.String())
		}

		// Resolve the explicit "main" branch ref rather than the bare repo's
		// own HEAD symref: `git init --bare` sets HEAD to the configured
		// default branch name at init time, which the first push does not
		// retarget, so it may not point at the "main" branch every fixture
		// in this repo actually uses.
		localHead, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "refs/heads/main"}, f.Layout.WeftWorktree())
		if err != nil || exitCode != 0 {
			t.Fatalf("git rev-parse refs/heads/main in weft worktree failed: %v (exit %d)", err, exitCode)
		}
		bareHead, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "refs/heads/main"}, f.WeftBare)
		if err != nil || exitCode != 0 {
			t.Fatalf("git rev-parse refs/heads/main in weft bare failed: %v (exit %d)", err, exitCode)
		}
		if strings.TrimSpace(localHead) != strings.TrimSpace(bareHead) {
			t.Errorf("weft local HEAD %s != weft bare HEAD %s after --undo push", strings.TrimSpace(localHead), strings.TrimSpace(bareHead))
		}
	})
}
