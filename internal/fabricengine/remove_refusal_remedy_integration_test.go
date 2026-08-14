//go:build integration

// remove_refusal_remedy_integration_test.go pins the remedy pointer Remove's no-force refusals carry
// when their own portal and launcher teardown has already run.
//
// Remove tears the portal and launchers down before the no-force dirtiness gates, deliberately, so
// the teardown still runs when the worktree directory is already gone (remove.go's header states the
// rule). The cost is that a REFUSED remove — the operator was told to commit or pass --force — has
// nonetheless already destroyed that pair's portal junction and launcher scripts. That loss is
// self-healing via `lyx fabric reconcile`, and the mutation record already reports it with
// partial=true, but an operator reading only the refusal has no reason to suspect it happened.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestRemove_RefusalNamesStrandedPortalTeardown builds a real pair, dirties its warp worktree, and
// asserts the no-force refusal both reports the teardown it already performed and names the
// reconcile remedy — and that reconcile genuinely restores what was torn down.
func TestRemove_RefusalNamesStrandedPortalTeardown(t *testing.T) {
	t.Parallel()

	const slug = "remove-refusal-remedy"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	portalPath := filepath.Join(l.HubPath, "_portals", slug)
	launcherDir := filepath.Join(l.HubPath, "_launchers", slug)
	for _, path := range []string{portalPath, launcherDir} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("precondition: Add must have created %s; Lstat err = %v", path, err)
		}
	}

	// An uncommitted TRACKED change is what makes the no-force gate refuse. It must be tracked:
	// Remove probes scopeAll, but a tracked change is the unambiguous case.
	warpPath := fabricengine.WorktreePath(l, slug)
	tracked := filepath.Join(warpPath, "tracked.md")
	if err := os.WriteFile(tracked, []byte("committed\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", tracked, err)
	}
	gitkit.MustRun(t, warpPath, "git", "add", "tracked.md")
	gitkit.MustRun(t, warpPath, "git", "commit", "-m", "seed tracked file")
	if err := os.WriteFile(tracked, []byte("committed\nuncommitted\n"), 0o644); err != nil {
		t.Fatalf("dirty %s: %v", tracked, err)
	}

	res, err := topology.Remove(l, slug, false)
	if err == nil {
		t.Fatalf("Remove(force=false) on a dirty pair returned nil error; want a refusal")
	}

	msg := err.Error()
	if !strings.Contains(msg, "uncommitted changes") {
		t.Errorf("refusal must still say why it refused; got:\n%s", msg)
	}
	if !strings.Contains(msg, "lyx fabric reconcile") {
		t.Errorf("refusal does not name the reconcile remedy for the portal/launcher teardown it already performed; an operator has no reason to suspect the loss:\n%s", msg)
	}

	// The record must agree with the message: this refusal really did strand a teardown.
	if res.Mutated().Len() == 0 {
		t.Fatalf("refusal recorded no mutations, so the remedy pointer would be claiming a loss that did not happen")
	}

	// The stranding the message describes is real, not hypothetical.
	for _, path := range []string{portalPath, launcherDir} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Errorf("expected %s to have been torn down before the refusal; Lstat err = %v", path, statErr)
		}
	}

	// And the remedy the message names actually works — otherwise the pointer is worse than silence.
	if _, reconcileErr := topology.Reconcile(l); reconcileErr != nil {
		t.Fatalf("Reconcile (the remedy the refusal names) failed: %v", reconcileErr)
	}
	for _, path := range []string{portalPath, launcherDir} {
		if _, statErr := os.Lstat(path); statErr != nil {
			t.Errorf("Reconcile did not restore %s; the remedy named in the refusal must be sufficient: %v", path, statErr)
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

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	// A plain directory inside the hub: it passes the slug and target-exists checks, then fails at
	// the dirtiness probe because it is not a git repository at all.
	const slug = "not-a-checkout"
	notACheckout := fabricengine.WorktreePath(l, slug)
	if err := os.MkdirAll(notACheckout, 0o755); err != nil {
		t.Fatalf("create %s: %v", notACheckout, err)
	}

	_, err := topology.Remove(l, slug, false)
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

// TestRemove_RefusalWithNothingStrandedOmitsRemedy pins the other direction: a refusal that tore
// nothing down must not tell the operator to repair a hub that is intact. A second refused attempt
// is the ordinary way to reach this state — the first attempt already removed the portal and
// launchers, so the second records nothing.
func TestRemove_RefusalWithNothingStrandedOmitsRemedy(t *testing.T) {
	t.Parallel()

	const slug = "remove-refusal-no-remedy"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	warpPath := fabricengine.WorktreePath(l, slug)
	tracked := filepath.Join(warpPath, "tracked.md")
	if err := os.WriteFile(tracked, []byte("committed\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", tracked, err)
	}
	gitkit.MustRun(t, warpPath, "git", "add", "tracked.md")
	gitkit.MustRun(t, warpPath, "git", "commit", "-m", "seed tracked file")
	if err := os.WriteFile(tracked, []byte("committed\nuncommitted\n"), 0o644); err != nil {
		t.Fatalf("dirty %s: %v", tracked, err)
	}

	if _, err := topology.Remove(l, slug, false); err == nil {
		t.Fatalf("first Remove(force=false) returned nil error; want a refusal")
	}

	res, err := topology.Remove(l, slug, false)
	if err == nil {
		t.Fatalf("second Remove(force=false) returned nil error; want a refusal")
	}
	if res.Mutated().Len() != 0 {
		t.Fatalf("precondition: the second refusal must record nothing (the first already tore the portal down); got %d entries", res.Mutated().Len())
	}
	if strings.Contains(err.Error(), "lyx fabric reconcile") {
		t.Errorf("a refusal that stranded nothing must not point at a repair; got:\n%s", err.Error())
	}
}
