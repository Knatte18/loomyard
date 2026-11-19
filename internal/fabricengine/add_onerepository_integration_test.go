// add_onerepository_integration_test.go covers Add, health, reconcile and unwire in a one-repository hub.
// The hub is built by hubforge.NewOneRepoHub, so each pair is one worktree on one branch with real records directories.
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// requireRealDirectory fails the test unless path is a directory and no link.
func requireRealDirectory(t *testing.T, path string) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Errorf("Lstat(%s): %v; want a real directory", path, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("%s has mode %v; want a real directory", path, info.Mode())
	}
}

// requireNoRecordsSibling fails the test when the hub holds a records sibling or a second branch for slug.
func requireNoRecordsSibling(t *testing.T, h *hubforge.Hub, slug, branch string) {
	t.Helper()

	if _, err := os.Stat(weftname.SiblingPath(h.Path, slug)); !os.IsNotExist(err) {
		t.Errorf("a records sibling of %s exists (stat err %v); want none", slug, err)
	}
	if gitkit.BranchExists(t, h.PrimeWorktree(), fabricengine.RecordsBranchName(branch)) {
		t.Errorf("branch %s exists; want no second branch", fabricengine.RecordsBranchName(branch))
	}
}

func TestAdd_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.NewOneRepoHub(t, "backend")
	opts := fabricengine.AddOptions{SkipPush: true}
	// The rows share one hub and run in order, since the last one reconciles the hub.
	var prefix string

	t.Run("LeavesOneWorktreeWithRealRecordsDirectories", func(t *testing.T) {
		const slug = "one"
		res := hubforge.AddPairWith(t, h, slug, opts)
		prefix = strings.TrimSuffix(res.Branch, slug)

		anchored := filepath.Join(res.Path, "backend")
		requireRealDirectory(t, filepath.Join(anchored, "_lym"))
		requireRealDirectory(t, filepath.Join(anchored, ".lym"))
		if marker := gitkit.Git(t, res.Path, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "backend" {
			t.Errorf("committed marker = %q; want backend", marker)
		}
		excludeLines := gitkit.ExcludeLines(t, res.Path)
		if !strings.Contains(strings.Join(excludeLines, "\n"), "/backend/.lym") {
			t.Errorf("exclude lines = %q; want the anchored .lym line", excludeLines)
		}
		for _, line := range excludeLines {
			if strings.Contains(line, "_lym") || strings.Contains(line, ".weft") {
				t.Errorf("exclude line %q hides records or a weft artifact; want none", line)
			}
		}
		requireNoRecordsSibling(t, h, slug, res.Branch)
	})

	t.Run("ScratchLinksToTheWorktreesOwnDotLym", func(t *testing.T) {
		pair := fabricengine.WorktreePath(h.Location, "one")
		link := filepath.Join(pair, "backend", ".scratch")
		got, err := filepath.EvalSymlinks(link)
		if err != nil {
			t.Fatalf("EvalSymlinks(%s): %v", link, err)
		}
		want, err := filepath.EvalSymlinks(filepath.Join(pair, "backend", ".lym", "scratch"))
		if err != nil {
			t.Fatalf("EvalSymlinks of the worktree's own .lym/scratch: %v", err)
		}
		if got != want {
			t.Errorf(".scratch resolves to %s; want %s", got, want)
		}
	})

	t.Run("ATaskForkedFromAParentCarryingRunRecordsStartsWithNone", func(t *testing.T) {
		commitRunRecords(t, h.Location, h.PrimeWorktree())
		if tracked := trackedUnderRoot(t, h.Location, h.PrimeWorktree(), "HEAD"); tracked == "" {
			t.Fatalf("the parent tracks no run records; want the fixture to commit some")
		}

		res := hubforge.AddPairWith(t, h, "norecords", opts)

		if _, err := os.Stat(filepath.Join(res.Path, "backend", shedrun.RunsRootRel())); !os.IsNotExist(err) {
			t.Errorf("run-records root on the new worktree's disk (stat err %v); want none", err)
		}
		if tracked := trackedUnderRoot(t, h.Location, res.Path, "HEAD"); tracked != "" {
			t.Errorf("new branch tracks run records:\n%s", tracked)
		}
	})

	t.Run("AddedAtAnotherAnchorCarriesTheTrailer", func(t *testing.T) {
		gitkit.CommitFile(t, h.PrimeWorktree(), "y/.keep", "", "track y")
		gitkit.CommitFile(t, h.PrimeWorktree(), "backend/_lym/config/carried.yaml", "key: value\n", "track a records file")

		res := hubforge.AddPairWith(t, h, "moved", fabricengine.AddOptions{SkipPush: true, Anchor: "y"})

		message := gitkit.Git(t, res.Path, "log", "-1", "--format=%B")
		if want := fabricengine.AnchorChangeTrailerKey + ": y"; !strings.Contains(message, want) {
			t.Errorf("first records commit message = %q; want the trailer %q", message, want)
		}
		if marker := gitkit.Git(t, res.Path, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "y" {
			t.Errorf("committed marker = %q; want y", marker)
		}
		requireRealDirectory(t, filepath.Join(res.Path, "y", "_lym"))
		if got := gitkit.Git(t, res.Path, "show", "HEAD:y/_lym/config/carried.yaml"); got != "key: value" {
			t.Errorf("y/_lym/config/carried.yaml at HEAD = %q; want the fork's records file moved under the new anchor", got)
		}
		if tracked := gitkit.Git(t, res.Path, "ls-files", "--", "backend/_lym"); tracked != "" {
			t.Errorf("tracked paths under the old anchor's records = %q; want none", tracked)
		}
		if _, err := os.Stat(filepath.Join(res.Path, "backend", "_lym")); !os.IsNotExist(err) {
			t.Errorf("old anchor's records directory on disk (stat err %v); want it moved away", err)
		}
		requireNoRecordsSibling(t, h, "moved", res.Branch)
	})

	t.Run("AFailedWiringRollsBackTheOneWorktreeAndItsBranch", func(t *testing.T) {
		const slug = "failed"
		// A dangling link where `.lym` goes makes the directory creation of the wiring fail.
		fabricengine.SetBeforeWireForTest(h.Topology, func() {
			link := filepath.Join(fabricengine.WorktreePath(h.Location, slug), "backend", ".lym")
			if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), link); err != nil {
				t.Errorf("plant the dangling link: %v", err)
			}
		})
		defer fabricengine.SetBeforeWireForTest(h.Topology, nil)

		if _, err := h.Topology.Add(h.Location, slug, opts); err == nil {
			t.Fatalf("Add succeeded; want the wiring to fail")
		}

		if _, err := os.Stat(fabricengine.WorktreePath(h.Location, slug)); !os.IsNotExist(err) {
			t.Errorf("task worktree remains (stat err %v); want it rolled back", err)
		}
		if gitkit.BranchExists(t, h.PrimeWorktree(), prefix+slug) {
			t.Errorf("task branch %s remains; want it deleted", prefix+slug)
		}
	})

	t.Run("AdoptingALocalBranchTakesTheSameRoute", func(t *testing.T) {
		const slug = "adopted"
		gitkit.Git(t, h.PrimeWorktree(), "branch", prefix+slug)

		res := hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true, AdoptLocal: true})

		if got := gitkit.CurrentBranch(t, res.Path); got != prefix+slug {
			t.Errorf("adopted worktree is on %s; want %s", got, prefix+slug)
		}
		requireRealDirectory(t, filepath.Join(res.Path, "backend", "_lym"))
		requireRealDirectory(t, filepath.Join(res.Path, "backend", ".lym"))
		requireNoRecordsSibling(t, h, slug, res.Branch)
	})

	pairAt := func(t *testing.T, slug string) *lymcwd.Location {
		t.Helper()
		l, err := lymcwd.NamedWorktree(h.Path, slug)
		if err != nil {
			t.Fatalf("NamedWorktree(%s): %v", slug, err)
		}
		return l
	}

	t.Run("StatusAndConfigChangesReadTheOneBranch", func(t *testing.T) {
		const configRel = "_lym/config/probe.yaml"
		pair := fabricengine.WorktreePath(h.Location, "one")
		gitkit.CommitFile(t, pair, filepath.ToSlash(filepath.Join("backend", configRel)), "key: value\n", "change a config file")

		status, err := h.Topology.Status(h.Location)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		found := false
		for _, pairStatus := range status.Pairs {
			if filepath.Clean(pairStatus.CodeWorktree) != filepath.Clean(pair) {
				continue
			}
			found = true
			if !pairStatus.InSync || pairStatus.WarpBranch != pairStatus.WeftBranch || pairStatus.DriftReason != "" {
				t.Errorf("Status of the pair = %+v; want in sync on one branch", pairStatus)
			}
		}
		if !found {
			t.Errorf("Status lists no pair at %s", pair)
		}

		live, err := h.Topology.RecordsBranchLive(h.Location, "one", opts)
		if err != nil || !live {
			t.Errorf("RecordsBranchLive = %v, %v; want true, nil", live, err)
		}

		changes, err := fabricengine.ReadConfigChanges(h.Location, prefix+"one", gitkit.CurrentBranch(t, h.PrimeWorktree()), []string{configRel})
		if err != nil {
			t.Fatalf("ReadConfigChanges: %v", err)
		}
		if want := []string{"backend/" + configRel}; strings.Join(changes.Files, ",") != strings.Join(want, ",") {
			t.Errorf("ReadConfigChanges files = %v; want %v", changes.Files, want)
		}
	})

	t.Run("ResolveReadsTheAnchorFromTheMarker", func(t *testing.T) {
		l, err := lymcwd.Resolve(filepath.Join(fabricengine.WorktreePath(h.Location, "one"), "backend"))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if l.AnchorRel != "backend" {
			t.Errorf("AnchorRel = %q; want backend", l.AnchorRel)
		}
	})

	t.Run("UnwireChangesNothing", func(t *testing.T) {
		res, err := fabricengine.Unwire(filepath.Join(fabricengine.WorktreePath(h.Location, "one"), "backend"), true)
		if err != nil {
			t.Fatalf("Unwire: %v", err)
		}
		if len(res.JunctionsRemoved) != 0 || res.GitExclude != "unchanged" {
			t.Errorf("Unwire = %+v; want no junction removed and the exclude unchanged", res)
		}
		requireRealDirectory(t, filepath.Join(fabricengine.WorktreePath(h.Location, "one"), "backend", "_lym"))
	})

	t.Run("ReconcileReportsNoJunctionCause", func(t *testing.T) {
		ok, reason, err := fabricengine.Healthy(pairAt(t, "one"))
		if err != nil || !ok {
			t.Errorf("Healthy = %v, %+v, %v; want ok", ok, reason, err)
		}

		res, err := h.Topology.Reconcile(h.Location)
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		for _, pair := range res.Pairs {
			if pair.Action == fabricengine.ReconcileActionJunctionRepointed || strings.Contains(pair.Detail, "junction") {
				t.Errorf("Reconcile of %s = %+v; want no junction repair", pair.CodeWorktree, pair)
			}
		}
	})
}
