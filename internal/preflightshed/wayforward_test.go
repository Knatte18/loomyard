// wayforward_test.go covers wayForward's per-check mapping over in-memory Reports, and the reconcile Stuck reasons no real hub reaches cheaply.
// The rows reached from a real hub (a dirty task worktree, a `_lyx` checkout off its paired branch, an unparseable config, a held hub lock) belong to the integration suite;
// this file stays offline.

package preflightshed

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/preflight"
)

func TestReconcileRefusal_NamesTheCauseAndLoomResume(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{
			"CommitFailureNamesNoFile",
			&hubreconcile.WorktreeError{Worktree: "/hub/pair-a", Err: errors.New("commit refused")},
			`hub config reconcile failed in /hub/pair-a: commit refused; way forward: fix the cause named above, then run "lyx loom resume" in the task worktree`,
		},
		{
			"OtherError",
			errors.New("hubreconcile: list code worktrees: boom"),
			`hub config reconcile failed: hubreconcile: list code worktrees: boom; way forward: fix the cause named above, then run "lyx loom resume" in the task worktree`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := reconcileRefusal(tt.err); got != tt.want {
				t.Errorf("reconcileRefusal() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestWayForward(t *testing.T) {
	for _, tt := range []struct {
		name    string
		failure preflight.Failure
		want    string
	}{
		{"WorktreeClean", preflight.Failure{Check: preflight.CheckWorktreeClean, Reason: "dirty"}, "lyx fabric commit"},
		{"FabricSync", preflight.Failure{Check: preflight.CheckFabricSync, Reason: "off branch"}, "lyx fabric checkout"},
		{"FabricReady", preflight.Failure{Check: preflight.CheckFabricReady, Reason: "no _lyx worktree"}, "lyx fabric reconcile"},
		{"Junction", preflight.Failure{Check: preflight.CheckJunction, Reason: "broken link"}, "lyx fabric reconcile"},
		{"JunctionConfigLoad", preflight.Failure{Check: preflight.CheckJunction, Reason: "junction check unavailable: cannot load fabric.yaml: bad"}, ""},
		{"Geometry", preflight.Failure{Check: preflight.CheckGeometry, Reason: "no repo"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var r preflight.Report
			r.AddFailure(tt.failure.Check, tt.failure.Reason)
			got := wayForward(r)
			if tt.want == "" {
				if got != "" {
					t.Errorf("wayForward() = %q; want none", got)
				}
				return
			}
			if !strings.HasPrefix(got, "; way forward: ") || !strings.Contains(got, tt.want) || !strings.Contains(got, "re-step") || !strings.HasSuffix(got, "; a session lyx refuses the verb from reports status: FAILED and the orch runs it") {
				t.Errorf("wayForward() = %q; want a trailing way forward naming %q", got, tt.want)
			}
		})
	}

	var both preflight.Report
	both.AddFailure(preflight.CheckFabricReady, "a")
	both.AddFailure(preflight.CheckJunction, "b")
	if got := wayForward(both); strings.Count(got, "lyx fabric reconcile") != 1 {
		t.Errorf("wayForward(ready+junction) = %q; want reconcile named once", got)
	}
}
