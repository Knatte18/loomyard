//go:build integration

// weftgit_pathspec_integration_test.go — integration coverage for
// weftPathspecFilter, the pre-stage filter CommitWeft runs immediately
// before f.weft.StageAndCommit: one test per predicate clause, against real
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

	"github.com/Knatte18/loomyard/internal/gitkit"
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

// TestCommitWeft_UntrackedNewFileCountsAsMatch covers the "untracked must count" predicate clause:
// a pathspec's first entry ("doesnotexist") matches nothing at all and must be silently dropped
// rather than failing the whole `git add`, while the second entry ("newmodule") names a brand-new,
// never-staged directory — untracked in the worktree, absent from the index — that must still count
// as a match and get committed.
// This is the exact shape a first-ever new-directory commit needs: a tracked-only-in-the-index
// predicate would filter it out and drop the very first commit under a brand-new optional junction.
func TestCommitWeft_UntrackedNewFileCountsAsMatch(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "newmodule", "newfile.txt"), "brand new, never staged")

	sha, committed, err := f.commitWeft([]string{"doesnotexist", "newmodule"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}
	if sha == "" {
		t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := lsFilesWeft(t, weftFixture.WeftPath)
	if !strings.Contains(tracked, "newmodule/newfile.txt") {
		t.Errorf("git ls-files = %q; want it to track newmodule/newfile.txt", tracked)
	}
}

// TestCommitWeft_IndexOnlyDeletionCountsAsMatch covers the "index-only must count" predicate
// clause: fabricengine.Unwire's (unwire.go) `_lyx` clear-and-commit step commits a "_lyx" path that
// os.RemoveAll has just deleted from the worktree, surviving only in the index at that point.
// A worktree-existence-only predicate would silently break that deletion commit — this test seeds a
// tracked file, deletes it from disk only (never staged), then asserts CommitWeft still commits the
// deletion.
func TestCommitWeft_IndexOnlyDeletionCountsAsMatch(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	trackedPath := filepath.Join(weftFixture.WeftPath, "_lyx", "trackedfile.txt")
	mustWriteFileWeft(t, trackedPath, "tracked content")
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "add", "_lyx/trackedfile.txt")
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "seed tracked file")

	// Delete from disk only — the file survives in the index until something
	// stages the deletion. This is the exact state undo.go's os.RemoveAll
	// leaves the weft worktree in immediately before its own CommitWeft call.
	if err := os.Remove(trackedPath); err != nil {
		t.Fatalf("os.Remove(%q): %v", trackedPath, err)
	}

	sha, committed, err := f.commitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true (the index-only deletion should count as a match)")
	}
	if sha == "" {
		t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := lsFilesWeft(t, weftFixture.WeftPath)
	if strings.Contains(tracked, "_lyx/trackedfile.txt") {
		t.Errorf("git ls-files = %q; want _lyx/trackedfile.txt no longer tracked after the deletion commit", tracked)
	}
}

// TestCommitWeft_ExcludeMagicPassesThroughUntouched is the mandatory guard case: a pathspec
// carrying a ":(exclude)" entry must pass it through untouched (never evaluated as a plain path to
// match) while a genuine positive entry alongside it still commits — and the excluded artifact must
// stay unstaged.
// Without this test, a filter that behaves correctly on plain paths could still silently re-stage
// machine-local artifacts by mis-evaluating exclusion magic as an ordinary non-matching entry.
func TestCommitWeft_ExcludeMagicPassesThroughUntouched(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "durable.txt"), "durable state")
	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "run.lock"), "machine-local lock")

	sha, committed, err := f.commitWeft([]string{"_lyx", ":(exclude)_lyx/*.lock"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}
	if sha == "" {
		t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := lsFilesWeft(t, weftFixture.WeftPath)
	if !strings.Contains(tracked, "_lyx/durable.txt") {
		t.Errorf("git ls-files = %q; want it to track _lyx/durable.txt", tracked)
	}
	if strings.Contains(tracked, "_lyx/run.lock") {
		t.Errorf("git ls-files = %q; want _lyx/run.lock excluded, never tracked", tracked)
	}
}

// TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing covers the early-return case: when
// the only non-magic entry in the pathspec matches nothing at all, CommitWeft must return ("",
// false, nil) WITHOUT ever calling StageAndCommit — leaving a genuinely dirty tracked file
// (modified, unstaged) and an untracked lock file completely untouched.
// Asserting the pre-existing HEAD SHA is unchanged and nothing is staged is what actually catches
// the regression this guards: handing git a pathspec of only ":(exclude)" magic with no positive
// entry is read as "everything except those," which would otherwise stage the entire weft worktree
// (including the dirty config.yaml) rather than nothing.
func TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	preSHA := currentSHA(t, weftFixture.WeftPath)

	// A genuinely dirty tracked file and an untracked lock file: if the
	// filter mishandled the all-negative pathspec by staging everything,
	// both would show up staged below.
	writeWeftConfigContent(t, weftFixture.WeftPath, "dirtied but must stay unstaged")
	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "run.lock"), "machine-local lock")

	sha, committed, err := f.commitWeft([]string{"doesnotexist", ":(exclude)_lyx/*.lock"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("commitWeft() committed = true; want false (the only positive entry matches nothing)")
	}
	if sha != "" {
		t.Errorf("commitWeft() sha = %q; want empty", sha)
	}

	if !diffCachedQuietWeft(t, weftFixture.WeftPath) {
		t.Errorf("git diff --cached --quiet reports staged changes; want nothing staged at all")
	}
	postSHA := currentSHA(t, weftFixture.WeftPath)
	if postSHA != preSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (no commit should have been made)", preSHA, postSHA)
	}
}

// resolvedDefaultRoutingNames resolves the fabric config template's REAL default pathspec and runs
// it through pathspecNames -- not a hand-written literal, and not the raw, unfiltered Config.Dirs()
// -- so TestResolvedDefaultRoutingNames_IsLyxAlone below exercises whatever fabric's own commit
// routing actually builds today: structuralCommittedDirs ("_lyx") alone, the template's pathspec
// default now being empty, catching both a future template default change and a future structural-set
// change.
func resolvedDefaultRoutingNames(t *testing.T) []string {
	t.Helper()

	resolved, err := yamlengine.Resolve([]byte(ConfigTemplate()), nil)
	if err != nil {
		t.Fatalf("yamlengine.Resolve(ConfigTemplate()): %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal resolved config template: %v", err)
	}
	return pathspecNames(cfg)
}

// TestResolvedDefaultRoutingNames_IsLyxAlone asserts the real, resolved default routing set —
// structuralCommittedDirs ("_lyx") alone, now that template.yaml's pathspec default is empty — still
// guards against a future template change silently re-widening the default.
// This is split out from the multi-entry tolerance regression below because an empty default leaves
// resolvedDefaultRoutingNames with nothing to inject an "_extra" name through: the two properties this
// file used to pin together (the real default's exact shape, and weftPathspecFilter's tolerance of a
// wider pathspec with an empty optional entry) no longer share one subject.
func TestResolvedDefaultRoutingNames_IsLyxAlone(t *testing.T) {
	dirs := resolvedDefaultRoutingNames(t)
	if len(dirs) != 1 || dirs[0] != "_lyx" {
		t.Fatalf("resolvedDefaultRoutingNames() = %v; want [_lyx]", dirs)
	}
}

// TestCommitWeft_WidenedPathspecTolerance_LyxChangeStillCommitsWithEmptyOptionalDir proves card 13's
// pathspec-tolerance filter still holds for a genuinely widened, multi-entry pathspec, even though the
// real template default no longer exercises that shape on its own (see
// TestResolvedDefaultRoutingNames_IsLyxAlone above).
// It deliberately does NOT ride the real template default: it hand-supplies a two-name routing set
// ("_lyx", "_extra") instead, because weftPathspecFilter remains live production behaviour for any
// repo whose fabric.yaml pathspec actually does widen beyond "_lyx" — and an empty default is exactly
// what would let a regression in that filter go unnoticed if this coverage were dropped along with the
// old widened-by-default shape.
// With NO files under "_extra" at all, a genuine "_lyx" change still commits.
// Without weftPathspecFilter, this is exactly the silent regression the batch scope describes: `git
// add -- _lyx _extra` fails in its entirety the moment `_extra` matches nothing, and CommitWeft's own
// pre-existing "did not match any files" tolerance swallows that into ("", false, nil) with no error —
// so the only way to catch it is asserting the commit actually happened, not checking for an error.
// Covers both shapes an empty "_extra" can take — wholly absent and present-but-empty — since git
// tracks files, not directories, and a materialised-but-empty "_extra/" is a normal, expected state
// for any optional pathspec entry a repo has not yet populated.
func TestCommitWeft_WidenedPathspecTolerance_LyxChangeStillCommitsWithEmptyOptionalDir(t *testing.T) {
	dirs := []string{"_lyx", "_extra"}

	t.Run("OptionalDirWhollyAbsent", func(t *testing.T) {
		t.Parallel()

		warpPath := newPlainWarpRepo(t)
		weftFixture := gitkit.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		if _, err := os.Stat(filepath.Join(weftFixture.WeftPath, "_extra")); !os.IsNotExist(err) {
			t.Fatalf("precondition: _extra must not exist in this fixture; Stat err = %v", err)
		}

		writeWeftConfigContent(t, weftFixture.WeftPath, "lyx change, _extra wholly absent")

		sha, committed, err := f.commitWeft(dirs, DefaultCommitMessage, SyncOptions{})
		if err != nil {
			t.Fatalf("commitWeft() error = %v; want nil", err)
		}
		if !committed {
			t.Fatalf("commitWeft() committed = false; want true (the _lyx change must still commit)")
		}
		if sha == "" {
			t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
		}
	})

	t.Run("OptionalDirExistsButEmpty", func(t *testing.T) {
		t.Parallel()

		warpPath := newPlainWarpRepo(t)
		weftFixture := gitkit.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		// git tracks files, not directories: a materialised-but-empty
		// "_extra/" still has nothing for a pathspec to match.
		if err := os.MkdirAll(filepath.Join(weftFixture.WeftPath, "_extra"), 0o755); err != nil {
			t.Fatalf("MkdirAll _extra: %v", err)
		}

		writeWeftConfigContent(t, weftFixture.WeftPath, "lyx change, _extra present but empty")

		sha, committed, err := f.commitWeft(dirs, DefaultCommitMessage, SyncOptions{})
		if err != nil {
			t.Fatalf("commitWeft() error = %v; want nil", err)
		}
		if !committed {
			t.Fatalf("commitWeft() committed = false; want true (the _lyx change must still commit)")
		}
		if sha == "" {
			t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
		}
	})
}
