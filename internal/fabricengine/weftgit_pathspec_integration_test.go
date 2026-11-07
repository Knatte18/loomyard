// weftgit_pathspec_integration_test.go holds integration coverage for weftPathspecFilter.
// That filter is the pre-stage step CommitWeft runs immediately before f.records.StageAndCommit.
// Each predicate clause has a test against real git.
// A regression test pins that the widened default pathspec and this filter belong together.
// The package is fabricengine_test.
// It reuses export_test.go's NewPlainWarpRepoForTest, NewFabricForTest and WriteWeftConfigContentForTest fixture shims.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// diffCachedQuietWeft reports whether nothing at all is staged in weftPath: `git diff --cached --quiet` exits 0.
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

// mustWriteFileWeft writes content to path, creating parent directories as needed, and fails the test on any error.
func mustWriteFileWeft(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// TestCommitWeft_UntrackedNewFileCountsAsMatch covers the "untracked must count" predicate clause.
// The first entry ("doesnotexist") matches nothing and must be dropped rather than failing the whole `git add`.
// The second entry ("newmodule") names a new directory, untracked in the worktree and absent from the index.
// It must still count as a match and get committed.
// A first-ever commit under a brand-new optional junction has this shape.
// A predicate that matched only tracked paths would drop that commit.
//
//testtiming:keep an untracked new directory counting as a pathspec match while a nonexistent first entry is dropped; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeft_UntrackedNewFileCountsAsMatch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	mustWriteFileWeft(t, filepath.Join(weftFixture.PrimeRecords(), "newmodule", "newfile.txt"), "brand new, never staged")

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"doesnotexist", "newmodule"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}
	if sha == "" {
		t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := strings.Join(gitkit.LsFiles(t, weftFixture.PrimeRecords()), "\n")
	if !strings.Contains(tracked, "newmodule/newfile.txt") {
		t.Errorf("git ls-files = %q; want it to track newmodule/newfile.txt", tracked)
	}
}

// TestCommitWeft_IndexOnlyDeletionCountsAsMatch covers the "index-only must count" predicate clause.
// fabricengine.Unwire (unwire.go) clears `_lym` and commits that deletion.
// os.RemoveAll has already deleted the path from the worktree, so it survives only in the index.
// A predicate that checked only worktree existence would silently break that deletion commit.
// This test seeds a tracked file and deletes it from disk without staging the deletion.
// It then asserts that CommitWeft still commits the deletion.
//
//testtiming:keep an index-only deletion counting as a pathspec match so Unwire's deletion commit lands; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeft_IndexOnlyDeletionCountsAsMatch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	trackedPath := filepath.Join(weftFixture.PrimeRecords(), "_lym", "trackedfile.txt")
	mustWriteFileWeft(t, trackedPath, "tracked content")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "add", "_lym/trackedfile.txt")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "commit", "-q", "-m", "seed tracked file")

	// Delete from disk only; the file stays in the index until something stages the deletion.
	// undo.go's os.RemoveAll leaves the weft worktree in this state before its own CommitWeft call.
	if err := os.Remove(trackedPath); err != nil {
		t.Fatalf("os.Remove(%q): %v", trackedPath, err)
	}

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true (the index-only deletion should count as a match)")
	}
	if sha == "" {
		t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := strings.Join(gitkit.LsFiles(t, weftFixture.PrimeRecords()), "\n")
	if strings.Contains(tracked, "_lym/trackedfile.txt") {
		t.Errorf("git ls-files = %q; want _lym/trackedfile.txt no longer tracked after the deletion commit", tracked)
	}
}

// TestCommitWeft_ExcludeMagicPassesThroughUntouched checks that a ":(exclude)" entry is not matched as a plain path.
// A positive entry alongside it still commits, and the excluded artifact stays unstaged.
// Without this check, a filter could mis-evaluate the magic and silently re-stage machine-local artifacts.
//
//testtiming:keep an :(exclude) pathspec entry passing through untouched while a positive entry still commits and the excluded artifact stays unstaged; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeft_ExcludeMagicPassesThroughUntouched(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	mustWriteFileWeft(t, filepath.Join(weftFixture.PrimeRecords(), "_lym", "durable.txt"), "durable state")
	mustWriteFileWeft(t, filepath.Join(weftFixture.PrimeRecords(), "_lym", "run.lock"), "machine-local lock")

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym", ":(exclude)_lym/*.lock"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}
	if sha == "" {
		t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	tracked := strings.Join(gitkit.LsFiles(t, weftFixture.PrimeRecords()), "\n")
	if !strings.Contains(tracked, "_lym/durable.txt") {
		t.Errorf("git ls-files = %q; want it to track _lym/durable.txt", tracked)
	}
	if strings.Contains(tracked, "_lym/run.lock") {
		t.Errorf("git ls-files = %q; want _lym/run.lock excluded, never tracked", tracked)
	}
}

// TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing covers the early return.
// When the only non-magic entry matches no file, CommitWeft returns ("", false, nil) without calling StageAndCommit.
// A modified, unstaged tracked file and an untracked lock file must stay untouched.
// Asserting that HEAD is unchanged and nothing is staged catches the regression this guards:
// git reads a pathspec of only exclusion magic as "everything except those".
// Such a pathspec stages the whole weft worktree.
func TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	preSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	// The dirty tracked file and the untracked lock file must not show up staged below.
	// A filter that stages everything for an all-exclusion pathspec would stage both.
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "dirtied but must stay unstaged")
	mustWriteFileWeft(t, filepath.Join(weftFixture.PrimeRecords(), "_lym", "run.lock"), "machine-local lock")

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"doesnotexist", ":(exclude)_lym/*.lock"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("commitWeft() committed = true; want false (the only positive entry matches nothing)")
	}
	if sha != "" {
		t.Errorf("commitWeft() sha = %q; want empty", sha)
	}

	if !diffCachedQuietWeft(t, weftFixture.PrimeRecords()) {
		t.Errorf("git diff --cached --quiet reports staged changes; want nothing staged at all")
	}
	postSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())
	if postSHA != preSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (no commit should have been made)", preSHA, postSHA)
	}
}

// resolvedDefaultRoutingNames returns the routing names that a fabric commit uses by default.
// It resolves the fabric config template's real default pathspec and runs it through pathspecNames.
// It uses neither a literal nor the raw config directories, so a template or structural change reaches the result.
func resolvedDefaultRoutingNames(t *testing.T) []string {
	t.Helper()

	resolved, err := yamlengine.Resolve([]byte(fabricengine.ConfigTemplate()), nil)
	if err != nil {
		t.Fatalf("yamlengine.Resolve(fabricengine.ConfigTemplate()): %v", err)
	}
	var cfg fabricengine.Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal resolved config template: %v", err)
	}
	return fabricengine.PathspecNamesForTest(cfg)
}

// TestResolvedDefaultRoutingNames_IsLymAlone asserts that the resolved default routing set is "_lym" alone.
// It guards against a future template change silently widening the default.
//
//testtiming:keep the resolved default routing set being _lym alone so a template change cannot silently widen it; coverage of its blocks by other tests does not show an assertion of this
func TestResolvedDefaultRoutingNames_IsLymAlone(t *testing.T) {
	tiergate.Require(t, 1)
	dirs := resolvedDefaultRoutingNames(t)
	if len(dirs) != 1 || dirs[0] != "_lym" {
		t.Fatalf("resolvedDefaultRoutingNames() = %v; want [_lym]", dirs)
	}
}

// TestCommitWeft_WidenedPathspecTolerance_LymChangeStillCommitsWithEmptyOptionalDir covers a widened pathspec.
// The routing set is hand-supplied as "_lym" and "_extra", not taken from the template default.
// The pathspec filter stays live for any repo whose fabric.yaml pathspec widens beyond "_lym".
// With no files under "_extra", a "_lym" change must still commit.
// Without the filter, `git add -- _lym _extra` fails whole once "_extra" matches nothing.
// CommitWeft's "did not match any files" tolerance then turns that into ("", false, nil) with no error,
// so the test asserts that the commit happened rather than that no error was returned.
// The test covers both an absent "_extra" and a present but empty one.
// Git tracks files, not directories, so an empty "_extra/" is a normal state for an optional entry.
func TestCommitWeft_WidenedPathspecTolerance_LymChangeStillCommitsWithEmptyOptionalDir(t *testing.T) {
	tiergate.Require(t, 1)
	dirs := []string{"_lym", "_extra"}

	t.Run("OptionalDirWhollyAbsent", func(t *testing.T) {
		t.Parallel()

		warpPath := fabricengine.NewPlainWarpRepoForTest(t)
		weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

		if _, err := os.Stat(filepath.Join(weftFixture.PrimeRecords(), "_extra")); !os.IsNotExist(err) {
			t.Fatalf("precondition: _extra must not exist in this fixture; Stat err = %v", err)
		}

		fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "lym change, _extra wholly absent")

		sha, committed, err := fabricengine.CommitWeftForTest(f, dirs, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("commitWeft() error = %v; want nil", err)
		}
		if !committed {
			t.Fatalf("commitWeft() committed = false; want true (the _lym change must still commit)")
		}
		if sha == "" {
			t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
		}
	})

	t.Run("OptionalDirExistsButEmpty", func(t *testing.T) {
		t.Parallel()

		warpPath := fabricengine.NewPlainWarpRepoForTest(t)
		weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

		// git tracks files, not directories: a materialised-but-empty "_extra/" still has nothing for a pathspec to match.
		if err := os.MkdirAll(filepath.Join(weftFixture.PrimeRecords(), "_extra"), 0o755); err != nil {
			t.Fatalf("MkdirAll _extra: %v", err)
		}

		fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "lym change, _extra present but empty")

		sha, committed, err := fabricengine.CommitWeftForTest(f, dirs, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("commitWeft() error = %v; want nil", err)
		}
		if !committed {
			t.Fatalf("commitWeft() committed = false; want true (the _lym change must still commit)")
		}
		if sha == "" {
			t.Errorf("commitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
		}
	})
}
