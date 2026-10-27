//go:build tier1

// anchor_integration_test.go drives `lym fabric anchor` through RunCLIIn over hubforge hubs:
// the print and the move of a pair, a child forked before its parent's move, and the integration of a prime's move on a second clone.
// A hub's prime warp tracks sub/ and is pushed before any second clone is made, so sub is a directory git tracks in every checkout.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// newAnchorCLIHub builds a "." hub whose prime warp tracks sub/, with the prime's warp and records branches pushed to the hub's remotes.
func newAnchorCLIHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	// The verb points the process-wide trace sink into the hub it moves, so the sink is reset before the hub's directories are removed:
	// a parallel test's record would otherwise recreate a logs directory inside them while they go.
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	gitkit.CommitFile(t, h.PrimeWorktree(), "sub/.keep", "", "track sub")
	if _, err := fabricengine.CoalescePushBothAt(h.PrimeWorktree(), h.PrimeRecords(), fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("push the prime: %v", err)
	}
	return h
}

// requireAnchorOK runs the verb from cwd, requires exit 0 and returns the decoded envelope.
func requireAnchorOK(t *testing.T, cwd string, args ...string) envelope.Envelope {
	t.Helper()

	code, output := runFabric(t, cwd, append([]string{"anchor"}, args...)...)
	if code != 0 {
		t.Fatalf("RunCLI(anchor %s) = %d; want 0\noutput: %s", strings.Join(args, " "), code, output)
	}
	return envelope.RequireOK(t, output)
}

func TestRunCLI_Anchor(t *testing.T) {
	t.Parallel()

	t.Run("The verb prints the anchor and moves a pair to sub", func(t *testing.T) {
		t.Parallel()
		h := newAnchorCLIHub(t)
		hubforge.AddPair(t, h, "mover")
		primeName := h.Location.WorktreeName

		printed := requireAnchorOK(t, h.PrimeWorktree())
		if printed.Raw["slug"] != primeName || printed.Raw["anchor"] != "." {
			t.Errorf("anchor printed = %v; want the acting prime's anchor .", printed.Raw)
		}
		if named := requireAnchorOK(t, h.PrimeWorktree(), "mover"); named.Raw["anchor"] != "." {
			t.Errorf("anchor mover printed = %v; want .", named.Raw)
		}

		moved := requireAnchorOK(t, h.PrimeWorktree(), "mover", "--set", "sub")

		if moved.Raw["changed"] != true || moved.Raw["anchor"] != "sub" || moved.Raw["from"] != "." {
			t.Errorf("anchor --set sub envelope = %v; want a change from . to sub", moved.Raw)
		}
		weft := h.PairRecordsSibling("mover")
		if marker := gitkit.Git(t, weft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
			t.Errorf("committed marker = %q; want sub", marker)
		}
		for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
			if isLink, err := fslink.IsLink(filepath.Join(h.PairCodeWorktree("mover"), "sub", name)); err != nil || !isLink {
				t.Errorf("%s at sub is no link (%v); want the pair wired there", name, err)
			}
			if _, err := os.Lstat(filepath.Join(h.PairCodeWorktree("mover"), name)); !os.IsNotExist(err) {
				t.Errorf("%s at the old anchor: %v; want it gone", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(h.Path, "_launchers", "sub", "mover")); err != nil {
			t.Errorf("launcher directory at sub: %v; want it", err)
		}
		if again := requireAnchorOK(t, h.PrimeWorktree(), "mover"); again.Raw["anchor"] != "sub" {
			t.Errorf("anchor mover printed after the move = %v; want sub", again.Raw)
		}
		if noop := requireAnchorOK(t, h.PrimeWorktree(), "mover", "--set", "sub"); noop.Raw["noop"] != true || noop.Raw["changed"] != false {
			t.Errorf("anchor --set sub again = %v; want a no-op", noop.Raw)
		}
	})

	t.Run("A child forked before its parent's move keeps its anchor, and merging it into the parent leaves the parent's", func(t *testing.T) {
		t.Parallel()
		h := newAnchorCLIHub(t)
		hubforge.AddPair(t, h, "parent")
		hubforge.AddPairWith(t, h, "child", fabricengine.AddOptions{Parent: "parent"})
		requireAnchorOK(t, h.PrimeWorktree(), "parent", "--set", "sub")

		child, err := lymcwd.NamedWorktree(h.Path, "child")
		if err != nil || child.AnchorRel != "." {
			t.Fatalf("anchor of the child = (%v, %v); want . kept", child, err)
		}
		if marker := gitkit.Git(t, h.PairRecordsSibling("child"), "show", "HEAD:"+lymcwd.AnchorFileName); marker != "." {
			t.Errorf("committed marker of the child = %q; want .", marker)
		}
		noteDir := filepath.Join(h.PairCodeWorktree("child"), lymdirs.LymDirName)
		if err := os.WriteFile(filepath.Join(noteDir, "child-note.txt"), []byte("note\n"), 0o644); err != nil {
			t.Fatalf("write the child's record: %v", err)
		}
		if code, output := runFabric(t, h.PairCodeWorktree("child"), "commit"); code != 0 {
			t.Fatalf("RunCLI(commit) in the child = %d; want 0\noutput: %s", code, output)
		}
		if got := gitkit.Git(t, h.PairRecordsSibling("child"), "show", "HEAD:"+lymdirs.LymDirName+"/child-note.txt"); got != "note" {
			t.Errorf("child-note.txt in the child's weft at its anchor = %q; want the record committed there", got)
		}

		gitkit.CommitFile(t, h.PairCodeWorktree("child"), "child-only.txt", "child\n", "child work")
		childBranch := gitkit.CurrentBranch(t, h.PairCodeWorktree("child"))
		parentAnchor := filepath.Join(h.PairCodeWorktree("parent"), "sub")
		if code, output := runFabric(t, parentAnchor, "merge", childBranch); code != 0 {
			t.Fatalf("RunCLI(merge %s) in the parent = %d; want 0\noutput: %s", childBranch, code, output)
		}

		if marker := gitkit.Git(t, h.PairRecordsSibling("parent"), "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
			t.Errorf("committed marker of the parent after the merge = %q; want sub, the weft being no merge participant", marker)
		}
		if parent, err := lymcwd.NamedWorktree(h.Path, "parent"); err != nil || parent.AnchorRel != "sub" {
			t.Errorf("anchor of the parent after the merge = (%v, %v); want sub", parent, err)
		}
		if _, err := os.Stat(filepath.Join(h.PairCodeWorktree("parent"), "child-only.txt")); err != nil {
			t.Errorf("the child's warp commit is not in the parent: %v", err)
		}
	})

	t.Run("A second clone stops at the prime's move and integrates it with its own records commits", func(t *testing.T) {
		t.Parallel()
		h := newAnchorCLIHub(t)
		second := hubforge.CloneSecond(t, h)
		secondRecords := second.PrimeRecords()
		gitkit.CommitFile(t, secondRecords, lymdirs.LymDirName+"/second.txt", "second\n", "records of the second machine")
		kept := filepath.Join(secondRecords, lymdirs.DotLymDirName, "kept.txt")
		if err := os.WriteFile(kept, []byte("kept\n"), 0o644); err != nil {
			t.Fatalf("write the untracked file in the second clone's .lym: %v", err)
		}

		requireAnchorOK(t, h.PrimeWorktree(), "--set", "sub")
		if _, err := fabricengine.CoalescePushBothAt(h.PrimeWorktree(), h.PrimeRecords(), fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("push the moved prime: %v", err)
		}

		recordsHead, codeHead := gitkit.RevParse(t, secondRecords, "HEAD"), gitkit.RevParse(t, second.PrimeWorktree(), "HEAD")
		for _, verb := range []string{"pull", "push"} {
			code, output := runFabric(t, second.PrimeWorktree(), verb)
			if code == 0 {
				t.Errorf("RunCLI(%s) in the second clone = 0; want the stop at the marker change\noutput: %s", verb, output)
			}
			if !strings.Contains(output, "lym fabric anchor") {
				t.Errorf("RunCLI(%s) output = %s; want the stop to name `lym fabric anchor`", verb, output)
			}
		}
		if got := gitkit.RevParse(t, secondRecords, "HEAD"); got != recordsHead {
			t.Errorf("second clone's records HEAD = %s; want %s unchanged", got, recordsHead)
		}
		if got := gitkit.RevParse(t, second.PrimeWorktree(), "HEAD"); got != codeHead {
			t.Errorf("second clone's code HEAD = %s; want %s unchanged", got, codeHead)
		}

		report, err := fabricengine.ReconcileOrigin(second.Location, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
		if err != nil {
			t.Fatalf("ReconcileOrigin() error = %v; want nil", err)
		}
		stopped := false
		for _, branch := range report.Branches {
			if branch.Prime && branch.Repo == "records" && branch.State == fabricengine.BranchAnchorStopped {
				stopped = true
			}
		}
		if !stopped {
			t.Errorf("ReconcileOrigin() branches = %+v; want the prime's records branch anchor_stopped", report.Branches)
		}
		if got, err := os.ReadFile(kept); err != nil || string(got) != "kept\n" {
			t.Errorf(".lym file after the reconcile = (%q, %v); want it unchanged", got, err)
		}

		integrated := requireAnchorOK(t, second.PrimeWorktree(), "--set", "sub")

		if integrated.Raw["changed"] != true || integrated.Raw["anchor"] != "sub" {
			t.Errorf("anchor --set sub envelope = %v; want the integration to have changed the anchor to sub", integrated.Raw)
		}
		if message, _ := integrated.Raw["dobby_start_error"].(string); message == "" || integrated.Raw["way_forward"] != "lym beam start" {
			t.Errorf("anchor --set sub envelope = %v; want the dobby start the test binary refuses reported with `lym beam start`", integrated.Raw)
		}
		if marker := gitkit.Git(t, secondRecords, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
			t.Errorf("committed marker after the integration = %q; want sub", marker)
		}
		if got := gitkit.Git(t, secondRecords, "show", "HEAD:sub/"+lymdirs.LymDirName+"/second.txt"); got != "second" {
			t.Errorf("sub/_lym/second.txt at HEAD = %q; want the second machine's commit carried to sub", got)
		}
		if upstream := gitkit.RevParse(t, secondRecords, "@{upstream}"); !gitkit.IsAncestor(t, secondRecords, upstream, "HEAD") {
			t.Errorf("upstream tip %s is no ancestor of HEAD; want the commits replayed on it", upstream)
		}
		if got, err := os.ReadFile(filepath.Join(secondRecords, "sub", lymdirs.DotLymDirName, "kept.txt")); err != nil || string(got) != "kept\n" {
			t.Errorf("sub/.lym/kept.txt = (%q, %v); want the file moved with .lym", got, err)
		}
		for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
			if isLink, err := fslink.IsLink(filepath.Join(second.PrimeWorktree(), "sub", name)); err != nil || !isLink {
				t.Errorf("%s at sub is no link (%v); want the prime wired there", name, err)
			}
		}
	})
}
