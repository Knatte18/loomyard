//go:build integration

// finalize_integration_test.go drives Finalize against a real two-worktree pair on both the task
// side and the parent side, built by the standard hub fixture (internal/hubforge), with only the
// model seam faked: fakeResolutionShuttle stands in for a real conflict-resolution session, writing
// real content to the conflicted file rather than asking a model. No test in this file contacts a
// real service or a real model.
//
// The scenario deliberately conflicts on the code side only -- the byte-identical conflict shape
// across both sides is already covered exhaustively by internal/fabricengine's own mergein_integration_test.go,
// and duplicating that matrix here would couple this file to another package's own check set (see
// this batch's own decision that the integration tier here stays small and purposeful). A second,
// non-conflicting file diverges on the records side instead, so "the records side is not a merge participant"
// (the records-local-only-files task's own Fabric.Merge/MergeIn change) has something concrete to
// assert: the task pair's records-only file must NOT reach the parent pair's records side.

package landingshed_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// finalizeConflictStencilFixture is a minimal, valid conflict stencil carrying exactly the two
// markers mergeresolve's own spec builder fills, mirroring mergeresolve_test.go's own fixture --
// this package's own conflict-resolution session never spawns a real model, so the stencil's prose
// content is irrelevant to this test, only its two placeholders.
const finalizeConflictStencilFixture = "# Conflict\n\n{{.parent_directive}}\n\nPaths:\n{{.conflicted_paths}}\n\nReport: {{.report_path}}\n"

// seedConflictStencil writes finalizeConflictStencilFixture at the layout mergeresolve's spec
// builder reads (stencilstore.Path's own baseDir/landing/<name>.md shape) and returns the baseDir a
// Deps.StencilsDir field should carry.
func seedConflictStencil(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	stencilkit.SeedInto(t, root)
	dir := filepath.Join(root, "landing")
	if err := os.WriteFile(filepath.Join(dir, "landing-template-conflict.md"), []byte(finalizeConflictStencilFixture), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict stencil): %v", err)
	}
	return root
}

// resolutionShuttle is the model seam this file fakes: on every Run call it overwrites each of
// paths (joined onto worktreeRoot) with resolved, marker-free content, standing in for a real
// conflict-resolution session's own file edits.
func resolutionShuttle(worktreeRoot, resolved string, paths ...string) *shedfake.MergeShuttle {
	return &shedfake.MergeShuttle{RunFn: func(shuttleengine.Spec) (shuttleengine.Result, error) {
		for _, p := range paths {
			if err := os.WriteFile(filepath.Join(worktreeRoot, p), []byte(resolved), 0o644); err != nil {
				return shuttleengine.Result{}, err
			}
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
	}}
}

// openFabricAtLanding opens a *fabricengine.Fabric on the code worktree at path, via
// lyxcwd.ResolveWorktree + fabricengine.Open -- the only production constructor either producer's
// pair-opener closures may legally wrap.
func openFabricAtLanding(t *testing.T, path string) *fabricengine.Fabric {
	t.Helper()
	l, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", path, err)
	}
	f, err := fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open(%s): %v", path, err)
	}
	return f
}

// configNoticeSeams wires Deps.ConfigChanges to fabricengine.ReadConfigChanges on the real pair at taskCode, against branch parent, and returns it with a Deps.Notify that appends every notice to notices.
func configNoticeSeams(t *testing.T, taskBranch, taskCode string, notices *[]string) (func() (fabricengine.ConfigChanges, error), func(string) error) {
	t.Helper()
	l, err := lyxcwd.ResolveWorktree(taskCode)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", taskCode, err)
	}
	changes := func() (fabricengine.ConfigChanges, error) {
		return fabricengine.ReadConfigChanges(l, taskBranch, "parent", []string{configengine.ConfigFileRel("loom")})
	}
	notify := func(line string) error {
		*notices = append(*notices, line)
		return nil
	}
	return changes, notify
}

// newFinalizeAt builds a Finalize that lands the task pair at taskCode into the parent pair at parentCode, over the fake conflict-resolution session shuttle.
// squash is the landing's squash setting;
// configChanges and notify fill the two config-notice seams and may be nil.
func newFinalizeAt(t *testing.T, taskBranch, taskCode, parentCode string, shuttle *shedfake.MergeShuttle, squash bool, configChanges func() (fabricengine.ConfigChanges, error), notify func(string) error) *landingshed.Finalize {
	t.Helper()
	deps := landingshed.NewTestDeps(t)
	deps.WorktreeRoot = taskCode
	deps.TaskBranch = taskBranch
	deps.ParentBranch = "parent"
	deps.StencilsDir = seedConflictStencil(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskCode), nil }
	deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, parentCode), nil }
	deps.Shuttle = shuttle
	deps.ConfigChanges = configChanges
	deps.Notify = notify
	deps.Config = landingshed.Config{
		Squash:             squash,
		Conflict:           "claude:test-model",
		ConflictTimeoutMin: 1,
		CoAuthoredBy:       "Test Author <test@example.com>",
	}
	fz, err := landingshed.NewFinalize(deps)
	if err != nil {
		t.Fatalf("NewFinalize() error = %v; want nil", err)
	}
	return fz
}

// TestFinalize_OverRealHub lands tasks into a parent pair of one hub, a step at a time.
//
// The first step builds a task pair and a parent pair off the hub, diverges both on conflict.txt (a genuine code-side conflict) and diverges the task pair alone on a second, clean records-side file, runs Finalize with a fake session that writes a real resolution to the conflicted file, and asserts that the parent pair's code side afterward carries the task's resolved content and the squash setting took effect there, that the parent pair's records side is left byte-identical -- the records side is not a merge participant, per this task's own no-caller-facing-signature-change-in-fabric-shaped change to Fabric.Merge -- and that no merge record is left behind on either pair.
//
// The second step lands a second task into the same parent once, then calls Finalize again on the same pair, and asserts the second call is Done with no second landing commit.
// A fresh Finalize over a parent that already carries the task's squashed diff behaves the same way.
// Both Finalize calls of that step report the task's committed loom config change through the notice seam,
// and the parent's own copy of the file is left byte-identical.
//
// The third step lands a fresh task with squash off and asserts the same notice and the same untouched parent config.
//
// The steps share one hub and one parent pair,
// and the later ones rely on the first having landed into that parent,
// so no step runs in parallel;
// the top-level test calls t.Parallel because the hub is its own.
func TestFinalize_OverRealHub(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "task")
	hubforge.AddPair(t, h, "parent")
	parentCode, parentRecords := h.PairCodeWorktree("parent"), h.PairRecordsSibling("parent")

	if !t.Run("resolves a conflict and squash merges into the parent", func(t *testing.T) {
		taskCode, taskRecords := h.PairCodeWorktree("task"), h.PairRecordsSibling("task")

		// The genuine code-side conflict: both branches add conflict.txt independently, off a common
		// ancestor where it does not exist.
		gitkit.CommitFile(t, taskCode, "conflict.txt", "task content\n", "task: add conflict.txt")
		gitkit.CommitFile(t, parentCode, "conflict.txt", "parent content\n", "parent: add conflict.txt")

		// A clean, non-conflicting records-side divergence on the task pair alone, so "the parent pair
		// carries the task's content on both sides" has a concrete records-side fact to assert.
		gitkit.CommitFile(t, taskRecords, "task-note.txt", "task records note\n", "task: add task-note.txt")

		shuttle := resolutionShuttle(taskCode, "resolved content\n", "conflict.txt")
		fz := newFinalizeAt(t, "task", taskCode, parentCode, shuttle, true, nil, nil)

		// Captured before Call so the records-side-not-a-merge-participant assertion below has a concrete
		// before/after pair to compare, mirroring internal/fabricengine's own before/HEAD
		// convention for its records side (see e.g. its merge-local integration test).
		parentRecordsBefore := gitkit.RevParse(t, parentRecords, "HEAD")

		shedfake.RequireOutcome(t, fz, shedengine.Done)
		if len(shuttle.Specs) != 1 {
			t.Errorf("fake shuttle Run() called %d time(s); want exactly 1 (one conflict-resolution session)", len(shuttle.Specs))
		}

		// The parent pair's code side carries the task's resolved content.
		codeContent, err := os.ReadFile(filepath.Join(parentCode, "conflict.txt"))
		if err != nil {
			t.Fatalf("read parent code conflict.txt: %v", err)
		}
		if string(codeContent) != "resolved content\n" {
			t.Errorf("parent code conflict.txt = %q; want the resolved content %q", codeContent, "resolved content\n")
		}

		// The parent pair's records side is left byte-identical: it is not a merge participant, so the
		// task pair's records-only task-note.txt never reaches it, on either the file-tree or the commit
		// graph -- mirroring internal/fabricengine's own "records side is not a merge participant" assertion
		// shape (see e.g. its merge-local integration test's byte-identical check).
		if _, err := os.Stat(filepath.Join(parentRecords, "task-note.txt")); !os.IsNotExist(err) {
			t.Errorf("os.Stat(parent records task-note.txt) error = %v; want a not-exist error -- the records side is not a merge participant", err)
		}
		if got := gitkit.RevParse(t, parentRecords, "HEAD"); got != parentRecordsBefore {
			t.Errorf("parent records HEAD = %q; want byte-identical %q -- the records side is not a merge participant", got, parentRecordsBefore)
		}

		// The squash setting took effect on the parent pair's code side: a single-parent commit.
		codeHEAD := gitkit.RevParse(t, parentCode, "HEAD")
		if got := len(strings.Fields(gitkit.Git(t, parentCode, "log", "-1", "--format=%P", codeHEAD))); got != 1 {
			t.Errorf("parent code HEAD %s has %d parents; want exactly 1 (a squash commit)", codeHEAD, got)
		}

		// The landing commit's subject is the description title, its body is the description body, and
		// exactly one Co-Authored-By trailer carries the configured value.
		msgCmd := exec.Command("git", "log", "-1", "--format=%B")
		msgCmd.Dir = parentCode
		msgOut, err := msgCmd.Output()
		if err != nil {
			t.Fatalf("git log -1 --format=%%B in %s: %v", parentCode, err)
		}
		wantMsg := "A landing title\n\nA landing body.\n\nCo-Authored-By: Test Author <test@example.com>"
		if got := strings.TrimSpace(string(msgOut)); got != wantMsg {
			t.Errorf("landing commit message = %q; want %q", got, wantMsg)
		}
		if n := strings.Count(string(msgOut), "Co-Authored-By:"); n != 1 {
			t.Errorf("landing commit carries %d Co-Authored-By lines; want exactly 1", n)
		}

		// No merge record is left behind on either pair. fabricengine's own MergeRecordExistsForTest is
		// a test-only export reachable only from fabricengine's own test binary, so this package checks
		// the same fact through the public MergeInProgress accessor instead.
		taskHandle := openFabricAtLanding(t, taskCode)
		if inProgress, err := taskHandle.MergeInProgress(); err != nil || inProgress {
			t.Errorf("task pair MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
		}
		parentHandle := openFabricAtLanding(t, parentCode)
		if inProgress, err := parentHandle.MergeInProgress(); err != nil || inProgress {
			t.Errorf("parent pair MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
		}
	}) {
		return
	}

	// Relies on the first step having landed into the parent pair, so this task's merge-in carries that landing in cleanly.
	if !t.Run("an already landed parent is idempotent", func(t *testing.T) {
		hubforge.AddPair(t, h, "second-task")
		secondCode := h.PairCodeWorktree("second-task")
		gitkit.CommitFile(t, secondCode, "feature.txt", "task feature\n", "task: add feature.txt")
		commitTaskLoomConfig(t, h, "second-task")
		parentConfigBefore := readParentLoomConfig(t, h)

		var notices []string
		changes, notify := configNoticeSeams(t, "second-task", secondCode, &notices)
		shedfake.RequireOutcome(t, newFinalizeAt(t, "second-task", secondCode, parentCode, resolutionShuttle(secondCode, ""), true, changes, notify), shedengine.Done)
		requireOneConfigNotice(t, notices, "second-task")
		headAfterFirst := gitkit.RevParse(t, parentCode, "HEAD")

		// A second Finalize over the now already-landed parent queues the notice again.
		shedfake.RequireOutcome(t, newFinalizeAt(t, "second-task", secondCode, parentCode, resolutionShuttle(secondCode, ""), true, changes, notify), shedengine.Done)
		if got := gitkit.RevParse(t, parentCode, "HEAD"); got != headAfterFirst {
			t.Errorf("parent code HEAD = %q after second Finalize; want unchanged %q (no second landing commit)", got, headAfterFirst)
		}
		if len(notices) != 2 {
			t.Errorf("notices = %q after the second Finalize; want one per Finalize", notices)
		}
		requireParentLoomConfigUnchanged(t, h, parentConfigBefore)
	}) {
		return
	}

	// Lands a fresh task with squash off, after the idempotent step so the parent already carries the earlier landing.
	t.Run("a non-squash landing reports the config change", func(t *testing.T) {
		hubforge.AddPair(t, h, "third-task")
		thirdCode := h.PairCodeWorktree("third-task")
		gitkit.CommitFile(t, thirdCode, "third.txt", "third feature\n", "task: add third.txt")
		commitTaskLoomConfig(t, h, "third-task")
		parentConfigBefore := readParentLoomConfig(t, h)

		var notices []string
		changes, notify := configNoticeSeams(t, "third-task", thirdCode, &notices)
		shedfake.RequireOutcome(t, newFinalizeAt(t, "third-task", thirdCode, parentCode, resolutionShuttle(thirdCode, ""), false, changes, notify), shedengine.Done)
		requireOneConfigNotice(t, notices, "third-task")
		requireParentLoomConfigUnchanged(t, h, parentConfigBefore)
	})
}

// commitTaskLoomConfig commits a loom config change on the task pair's tracked side, which is the change Finalize reports and never carries.
func commitTaskLoomConfig(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), configengine.ConfigFileRel("loom"), "task_setting: "+slug+"\n", slug+": change loom config")
}

// readParentLoomConfig returns the parent pair's loom config file bytes, or nil when the parent has none.
func readParentLoomConfig(t *testing.T, h *hubforge.Hub) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.PairRecordsSibling("parent"), configengine.ConfigFileRel("loom")))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read parent loom config: %v", err)
	}
	return b
}

// requireParentLoomConfigUnchanged fails unless the parent pair's loom config file is byte-identical to before.
func requireParentLoomConfigUnchanged(t *testing.T, h *hubforge.Hub, before []byte) {
	t.Helper()
	if got := readParentLoomConfig(t, h); string(got) != string(before) {
		t.Errorf("parent loom config = %q after landing; want byte-identical %q -- landing never carries a config change", got, before)
	}
}

// requireOneConfigNotice fails unless notices holds a notice naming taskBranch and the loom config file.
func requireOneConfigNotice(t *testing.T, notices []string, taskBranch string) {
	t.Helper()
	if len(notices) == 0 {
		t.Fatal("no notice queued; want one naming the changed config file")
	}
	last := notices[len(notices)-1]
	for _, want := range []string{taskBranch, "loom.yaml"} {
		if !strings.Contains(last, want) {
			t.Errorf("notice %q does not carry %q", last, want)
		}
	}
}
