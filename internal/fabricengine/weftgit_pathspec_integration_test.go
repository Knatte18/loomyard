//go:build integration

// weftgit_pathspec_integration_test.go — integration coverage for
// weftPathspecFilter, the pre-stage filter CommitWeft runs immediately
// before f.Weft.StageAndCommit: one test per predicate clause, against real
// git, plus (added by card 14) the batch's own regression assertion that the
// widened default pathspec and this filter belong together. Package
// fabricengine (internal), reusing index_integration_test.go's
// newPlainWarpRepo/newFabric fixture helpers and syncweft_integration_test.go's
// writeWeftConfigContent, since both files share this package.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// lsFilesWeft returns `git ls-files`'s raw output for weftPath — the
// currently-tracked (index) path set, read fresh after whatever the test
// just did.
func lsFilesWeft(t *testing.T, weftPath string) string {
	t.Helper()

	cmd := exec.Command("git", "ls-files")
	cmd.Dir = weftPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v", weftPath, err)
	}
	return string(out)
}

// diffCachedQuietWeft reports whether `git diff --cached --quiet` exits 0 in
// weftPath — true means nothing at all is staged.
func diffCachedQuietWeft(t *testing.T, weftPath string) bool {
	t.Helper()

	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = weftPath
	err := cmd.Run()
	if err == nil {
		return true
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false
	}
	t.Fatalf("git diff --cached --quiet in %s: %v", weftPath, err)
	return false
}

// mustWriteFileWeft writes content to path, creating parent directories as
// needed, failing the test on any error.
func mustWriteFileWeft(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// TestCommitWeft_UntrackedNewFileCountsAsMatch covers the "untracked must
// count" predicate clause: a pathspec's first entry ("doesnotexist") matches
// nothing at all and must be silently dropped rather than failing the whole
// `git add`, while the second entry ("newmodule") names a brand-new,
// never-staged directory — untracked in the worktree, absent from the index
// — that must still count as a match and get committed. This is the exact
// shape a first-ever "_pattern/PATTERN.md" commit needs: a
// tracked-only-in-the-index predicate would filter it out and drop the very
// first PATTERN commit.
func TestCommitWeft_UntrackedNewFileCountsAsMatch(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "newmodule", "newfile.txt"), "brand new, never staged")

	sha, committed, err := f.CommitWeft([]string{"doesnotexist", "newmodule"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("CommitWeft() committed = false; want true")
	}
	if sha == "" {
		t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := lsFilesWeft(t, weftFixture.WeftPath)
	if !strings.Contains(tracked, "newmodule/newfile.txt") {
		t.Errorf("git ls-files = %q; want it to track newmodule/newfile.txt", tracked)
	}
}

// TestCommitWeft_IndexOnlyDeletionCountsAsMatch covers the "index-only must
// count" predicate clause: internal/initengine/undo.go's `lyx init --undo`
// commits a "_lyx" path that os.RemoveAll has just deleted from the
// worktree, surviving only in the index at that point. A
// worktree-existence-only predicate would silently break that deletion
// commit — this test seeds a tracked file, deletes it from disk only (never
// staged), then asserts CommitWeft still commits the deletion.
func TestCommitWeft_IndexOnlyDeletionCountsAsMatch(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	trackedPath := filepath.Join(weftFixture.WeftPath, "_lyx", "trackedfile.txt")
	mustWriteFileWeft(t, trackedPath, "tracked content")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "add", "_lyx/trackedfile.txt")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "seed tracked file")

	// Delete from disk only — the file survives in the index until something
	// stages the deletion. This is the exact state undo.go's os.RemoveAll
	// leaves the weft worktree in immediately before its own CommitWeft call.
	if err := os.Remove(trackedPath); err != nil {
		t.Fatalf("os.Remove(%q): %v", trackedPath, err)
	}

	sha, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("CommitWeft() committed = false; want true (the index-only deletion should count as a match)")
	}
	if sha == "" {
		t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := lsFilesWeft(t, weftFixture.WeftPath)
	if strings.Contains(tracked, "_lyx/trackedfile.txt") {
		t.Errorf("git ls-files = %q; want _lyx/trackedfile.txt no longer tracked after the deletion commit", tracked)
	}
}

// TestCommitWeft_ExcludeMagicPassesThroughUntouched is the mandatory guard
// case: a pathspec carrying a ":(exclude)" entry must pass it through
// untouched (never evaluated as a plain path to match) while a genuine
// positive entry alongside it still commits — and the excluded artifact must
// stay unstaged. Without this test, a filter that behaves correctly on plain
// paths could still silently re-stage machine-local artifacts by
// mis-evaluating exclusion magic as an ordinary non-matching entry.
func TestCommitWeft_ExcludeMagicPassesThroughUntouched(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "durable.txt"), "durable state")
	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "run.lock"), "machine-local lock")

	sha, committed, err := f.CommitWeft([]string{"_lyx", ":(exclude)_lyx/*.lock"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("CommitWeft() committed = false; want true")
	}
	if sha == "" {
		t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := lsFilesWeft(t, weftFixture.WeftPath)
	if !strings.Contains(tracked, "_lyx/durable.txt") {
		t.Errorf("git ls-files = %q; want it to track _lyx/durable.txt", tracked)
	}
	if strings.Contains(tracked, "_lyx/run.lock") {
		t.Errorf("git ls-files = %q; want _lyx/run.lock excluded, never tracked", tracked)
	}
}

// TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing covers the
// early-return case: when the only non-magic entry in the pathspec matches
// nothing at all, CommitWeft must return ("", false, nil) WITHOUT ever
// calling StageAndCommit — leaving a genuinely dirty tracked file
// (modified, unstaged) and an untracked lock file completely untouched.
// Asserting the pre-existing HEAD SHA is unchanged and nothing is staged is
// what actually catches the regression this guards: handing git a pathspec
// of only ":(exclude)" magic with no positive entry is read as "everything
// except those," which would otherwise stage the entire weft worktree
// (including the dirty config.yaml) rather than nothing.
func TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	preSHA := currentSHA(t, weftFixture.WeftPath)

	// A genuinely dirty tracked file and an untracked lock file: if the
	// filter mishandled the all-negative pathspec by staging everything,
	// both would show up staged below.
	writeWeftConfigContent(t, weftFixture.WeftPath, "dirtied but must stay unstaged")
	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "run.lock"), "machine-local lock")

	sha, committed, err := f.CommitWeft([]string{"doesnotexist", ":(exclude)_lyx/*.lock"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("CommitWeft() committed = true; want false (the only positive entry matches nothing)")
	}
	if sha != "" {
		t.Errorf("CommitWeft() sha = %q; want empty", sha)
	}

	if !diffCachedQuietWeft(t, weftFixture.WeftPath) {
		t.Errorf("git diff --cached --quiet reports staged changes; want nothing staged at all")
	}
	postSHA := currentSHA(t, weftFixture.WeftPath)
	if postSHA != preSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (no commit should have been made)", preSHA, postSHA)
	}
}

// resolvedDefaultPathspecDirs resolves the fabric config template's REAL
// default pathspec and splits it via Config.Dirs -- not a hand-written
// literal — so the regression test below exercises whatever the template
// actually declares today, catching a future default change too.
func resolvedDefaultPathspecDirs(t *testing.T) []string {
	t.Helper()

	resolved, err := yamlengine.Resolve([]byte(ConfigTemplate()), nil)
	if err != nil {
		t.Fatalf("yamlengine.Resolve(ConfigTemplate()): %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal resolved config template: %v", err)
	}
	return cfg.Dirs()
}

// TestCommitWeft_WidenedDefaultPathspec_LyxChangeStillCommitsWithNoPattern is
// this batch's single most important regression assertion, proving card 13
// (the pathspec-tolerance filter) and card 14 (the widened default
// pathspec) belong together in one batch: with the real, resolved default
// pathspec — "_lyx _pattern" — and NO files under "_pattern" at all, a
// genuine "_lyx" change still commits. Without weftPathspecFilter, this is
// exactly the silent regression the batch scope describes: `git add --
// _lyx _pattern` fails in its entirety the moment `_pattern` matches
// nothing, and CommitWeft's own pre-existing "did not match any files"
// tolerance swallows that into ("", false, nil) with no error — so the
// only way to catch it is asserting the commit actually happened, not
// checking for an error. Covers both shapes an empty "_pattern" can take —
// wholly absent and present-but-empty — since git tracks files, not
// directories, and a materialised-but-empty "_pattern/" is the normal,
// expected state for this whole task while content migration stays out of
// scope.
func TestCommitWeft_WidenedDefaultPathspec_LyxChangeStillCommitsWithNoPattern(t *testing.T) {
	dirs := resolvedDefaultPathspecDirs(t)
	if len(dirs) != 2 || dirs[0] != "_lyx" || dirs[1] != "_pattern" {
		t.Fatalf("resolved default pathspec dirs = %v; want [_lyx _pattern]", dirs)
	}

	t.Run("PatternDirWhollyAbsent", func(t *testing.T) {
		t.Parallel()

		warpPath := newPlainWarpRepo(t)
		weftFixture := lyxtest.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		if _, err := os.Stat(filepath.Join(weftFixture.WeftPath, "_pattern")); !os.IsNotExist(err) {
			t.Fatalf("precondition: _pattern must not exist in this fixture; Stat err = %v", err)
		}

		writeWeftConfigContent(t, weftFixture.WeftPath, "lyx change, _pattern wholly absent")

		sha, committed, err := f.CommitWeft(dirs, DefaultCommitMessage, SyncOptions{})
		if err != nil {
			t.Fatalf("CommitWeft() error = %v; want nil", err)
		}
		if !committed {
			t.Fatalf("CommitWeft() committed = false; want true (the _lyx change must still commit)")
		}
		if sha == "" {
			t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
		}
	})

	t.Run("PatternDirExistsButEmpty", func(t *testing.T) {
		t.Parallel()

		warpPath := newPlainWarpRepo(t)
		weftFixture := lyxtest.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		// git tracks files, not directories: a materialised-but-empty
		// "_pattern/" still has nothing for a pathspec to match.
		if err := os.MkdirAll(filepath.Join(weftFixture.WeftPath, "_pattern"), 0o755); err != nil {
			t.Fatalf("MkdirAll _pattern: %v", err)
		}

		writeWeftConfigContent(t, weftFixture.WeftPath, "lyx change, _pattern present but empty")

		sha, committed, err := f.CommitWeft(dirs, DefaultCommitMessage, SyncOptions{})
		if err != nil {
			t.Fatalf("CommitWeft() error = %v; want nil", err)
		}
		if !committed {
			t.Fatalf("CommitWeft() committed = false; want true (the _lyx change must still commit)")
		}
		if sha == "" {
			t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
		}
	})
}
