//go:build integration

// remove_refusal_remedy_integration_test.go pins that a no-force Remove refusal — a dirty worktree or a failed status probe — leaves the pair and the weft origin untouched: no portal or launcher is torn down, no mutation is recorded, and no archive tag is pushed.
//
// Remove runs every refusal before its archive and the archive before every removal (remove.go's header states the order),
// so an operator told to commit or pass --force has lost nothing and the refusal needs no repair pointer.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestRemove_DirtyRefusalLeavesPairIntact builds a real pair, dirties its warp worktree, and asserts the no-force refusal tears nothing down, records nothing, and pushes no archive tag.
func TestRemove_DirtyRefusalLeavesPairIntact(t *testing.T) {
	t.Parallel()

	const slug = "remove-refusal-intact"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	topology := h.Topology

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	// An uncommitted TRACKED change is what makes the no-force gate refuse. It must be tracked:
	// Remove probes scopeAll, but a tracked change is the unambiguous case.
	warpPath := fabricengine.WorktreePath(l, slug)
	tracked := filepath.Join(warpPath, "tracked.md")
	gitkit.CommitFile(t, warpPath, "tracked.md", "committed\n", "seed tracked file")
	if err := os.WriteFile(tracked, []byte("committed\nuncommitted\n"), 0o644); err != nil {
		t.Fatalf("dirty %s: %v", tracked, err)
	}

	res, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove(force=false) on a dirty pair returned nil error; want a refusal")
	}

	msg := err.Error()
	if !strings.Contains(msg, "uncommitted changes") {
		t.Errorf("refusal must still say why it refused; got:\n%s", msg)
	}
	if strings.Contains(msg, "lyx fabric reconcile") {
		t.Errorf("refusal names the retired reconcile remedy; nothing was torn down:\n%s", msg)
	}
	if n := res.Mutated().Len(); n != 0 {
		t.Errorf("refusal recorded %d mutations; want 0", n)
	}
	assertPairIntact(t, l, slug)
	assertNoArchiveTag(t, h.WeftBare, weftRoot)
}

// TestRemove_WarpStatusProbeFailurePushesNoTag breaks the warp worktree's gitfile so the status probe fails, and asserts the refusal pushes no archive tag and tears nothing down.
func TestRemove_WarpStatusProbeFailurePushesNoTag(t *testing.T) {
	t.Parallel()

	const slug = "remove-probe-failure"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	topology := h.Topology

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	gitFile := filepath.Join(fabricengine.WorktreePath(l, slug), ".git")
	missing := filepath.Join(l.HubPath, "no-such-gitdir")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+filepath.ToSlash(missing)+"\n"), 0o644); err != nil {
		t.Fatalf("overwrite %s: %v", gitFile, err)
	}

	_, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove with a failing status probe returned nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), "check warp worktree status") {
		t.Errorf("error does not name the failed probe:\n%s", err.Error())
	}
	assertNoArchiveTag(t, h.WeftBare, weftRoot)
	for _, p := range []string{
		fabricengine.PortalLink(l, slug),
		fabricengine.LauncherDir(l, slug),
		fabricengine.WeftWorktreePath(l, slug),
	} {
		if _, statErr := os.Lstat(p); statErr != nil {
			t.Errorf("%s missing after a refused Remove: %v", p, statErr)
		}
	}
}

// assertPairIntact fails unless the pair's portal, launchers, both worktrees and both branches exist.
func assertPairIntact(t *testing.T, l *lyxcwd.Location, slug string) {
	t.Helper()

	for _, p := range []string{
		fabricengine.PortalLink(l, slug),
		fabricengine.LauncherDir(l, slug),
		fabricengine.WorktreePath(l, slug),
		fabricengine.WeftWorktreePath(l, slug),
	} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s missing after a refused Remove: %v", p, err)
		}
	}
	if !gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("warp branch gone after a refused Remove")
	}
	if !gitkit.BranchExists(t, mustWeftRepoRoot(t, l), fabricengine.WeftBranchName(slug)) {
		t.Errorf("weft branch gone after a refused Remove")
	}
}

// assertNoArchiveTag fails when any archive/ tag exists in either repo.
func assertNoArchiveTag(t *testing.T, repoRoots ...string) {
	t.Helper()

	for _, root := range repoRoots {
		if tags := archiveTagsAt(t, root); len(tags) != 0 {
			t.Errorf("archive tags in %s after a refused Remove = %v; want none", root, tags)
		}
	}
}

// TestRemove_StatusFailureNamesPathAndCommandOnce drives Remove against a hub-contained directory
// that is not a git checkout, and asserts the composed error names the probed path once and the git
// command once.
//
// Both wrappers in this chain — Remove's own "check warp worktree status" and worktreeDirty's
// "check for uncommitted changes in <dir>" — used to name the path, and, before the gitexec split,
// the inner one also named the git command that *gitexec.GitError now renders itself. The result put
// the same path twice and the same command twice ahead of git's own stderr, which is the only part
// of the message an operator can act on. Each layer now contributes exactly one new fact: what
// fabric was doing, where it probed, and what git said.
func TestRemove_StatusFailureNamesPathAndCommandOnce(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	topology := h.Topology

	// A plain directory inside the hub: it passes the slug and target-exists checks, then fails at
	// the dirtiness probe because it is not a git repository at all.
	const slug = "not-a-checkout"
	notACheckout := fabricengine.WorktreePath(l, slug)
	if err := os.MkdirAll(notACheckout, 0o755); err != nil {
		t.Fatalf("create %s: %v", notACheckout, err)
	}

	_, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove on a non-checkout directory returned nil error; want a failure from the dirtiness probe")
	}

	msg := err.Error()
	if occurrences := strings.Count(msg, notACheckout); occurrences != 1 {
		t.Errorf("error names the probed path %d time(s); want exactly 1 — only one layer should own the \"where\":\n%s", occurrences, msg)
	}
	if occurrences := strings.Count(msg, "git status --porcelain"); occurrences != 1 {
		t.Errorf("error names the git command %d time(s); want exactly 1 — *gitexec.GitError renders it, so no wrapper should:\n%s", occurrences, msg)
	}
	if !strings.Contains(msg, "not a git repository") {
		t.Errorf("error dropped git's own stderr, the only actionable part:\n%s", msg)
	}
}
