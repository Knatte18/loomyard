// goto_test.go covers the generic goto body: a blocked run moves onto the named row paused,
// and each refusal names its way forward, listing the valid producers where the target is at fault.

package shedverbs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

func gotoTexts() VerbTexts {
	return VerbTexts{Goto: VerbText{Use: "goto", Short: "move the fake shed onto a row", Audience: clihelp.AudienceOperator}}
}

func gotoSpec(paths testPaths) *Spec {
	return &Spec{
		StatusPath:     paths.StatusPath,
		LockPath:       paths.LockPath,
		StatusLockPath: paths.StatusLockPath,
		RunID:          "run-1",
		Routing: shedengine.Routing{
			Entry:     "A",
			Producers: []shedengine.ProducerDef{stubRow("A"), stubRow("B")},
		},
	}
}

func seedBlocked(t *testing.T, paths testPaths, current string) {
	t.Helper()
	if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, shedengine.Status{
		CurrentProducer: current,
		State:           shedengine.StateBlocked,
		Error:           "stuck",
		History:         []shedengine.HistoryEntry{},
	}); err != nil {
		t.Fatalf("seed blocked status: %v", err)
	}
}

// TestGotoCmd_MovesBlockedRunOntoRow asserts a blocked run lands paused on the target and the file is updated.
func TestGotoCmd_MovesBlockedRunOntoRow(t *testing.T) {
	paths := newTestPaths(t)
	seedBlocked(t, paths, "B")

	cmd := gotoCmd(gotoTexts(), gotoSpec(paths))
	if got := cmd.Annotations[clihelp.AudienceAnnotation]; got != clihelp.AudienceOperator {
		t.Errorf("audience annotation = %q; want the text's %q", got, clihelp.AudienceOperator)
	}

	env, code := execEnvelope(t, cmd, []string{"--to", "A"})
	if code != 0 {
		t.Fatalf("exit code = %d; want 0 (env %v)", code, env)
	}
	if env["current_producer"] != "A" || env["state"] != "paused" {
		t.Errorf("envelope current_producer/state = %v/%v; want A/paused", env["current_producer"], env["state"])
	}
	if env["run_id"] != "run-1" || env["status_file"] != paths.StatusPath {
		t.Errorf("envelope run_id/status_file = %v/%v", env["run_id"], env["status_file"])
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("re-read status: found=%v err=%v", found, err)
	}
	if st.CurrentProducer != "A" || st.State != shedengine.StatePaused {
		t.Errorf("status = %s/%s; want A/paused", st.CurrentProducer, st.State)
	}
}

// TestGotoCmd_RefusesMissingOrUnknownTarget asserts both refusals end in a way forward listing the valid producer names.
func TestGotoCmd_RefusesMissingOrUnknownTarget(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "NoTo", args: nil},
		{name: "UnknownRow", args: []string{"--to", "Nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			seedBlocked(t, paths, "B")

			env, code := execEnvelope(t, gotoCmd(gotoTexts(), gotoSpec(paths)), tt.args)
			if code != 1 {
				t.Fatalf("exit code = %d; want 1", code)
			}
			msg, _ := env["error"].(string)
			if !strings.Contains(msg, "way forward: re-run goto with --to naming one of: A, B") {
				t.Errorf("error %q does not end in a way forward listing the valid names %q", msg, "A, B")
			}
		})
	}
}

// TestGotoCmd_RefusesHeldRunLock asserts a held run lock names lyx shed pause.
func TestGotoCmd_RefusesHeldRunLock(t *testing.T) {
	paths := newTestPaths(t)
	seedBlocked(t, paths, "B")
	held, locked, err := lock.TryAcquireWriteLock(paths.LockPath)
	if err != nil || !locked {
		t.Fatalf("acquire run lock: locked=%v err=%v", locked, err)
	}
	defer held.Release()

	env, code := execEnvelope(t, gotoCmd(gotoTexts(), gotoSpec(paths)), []string{"--to", "A"})
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if msg, _ := env["error"].(string); !strings.Contains(msg, "lyx shed pause") {
		t.Errorf("error %q does not name lyx shed pause", msg)
	}
}

// TestGotoCmd_PassesToldWayForwardTexts asserts the verb hands the spec's run-id and missing-status clause to the engine, seen in a refusal.
func TestGotoCmd_PassesToldWayForwardTexts(t *testing.T) {
	paths := newTestPaths(t)
	spec := gotoSpec(paths)
	spec.MissingStatusWayForward = "way forward: told clause"

	env, code := execEnvelope(t, gotoCmd(gotoTexts(), spec), []string{"--to", "A"})
	if code == 0 {
		t.Fatalf("exit code = 0 over a missing status file; want a refusal (env %v)", env)
	}
	if msg, _ := env["error"].(string); !strings.Contains(msg, "way forward: told clause") {
		t.Errorf("message = %q; want the spec's missing-status clause", msg)
	}

	seedRunning(t, paths)
	env, code = execEnvelope(t, gotoCmd(gotoTexts(), spec), []string{"--to", "A"})
	if code == 0 {
		t.Fatalf("exit code = 0 over a running run; want a refusal (env %v)", env)
	}
	if msg, _ := env["error"].(string); !strings.Contains(msg, "lyx shed pause run-1") {
		t.Errorf("message = %q; want the running refusal to name the spec's run-id", msg)
	}
}

func seedRunning(t *testing.T, paths testPaths) {
	t.Helper()
	if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, shedengine.Status{
		CurrentProducer: "B",
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	}); err != nil {
		t.Fatalf("seed running status: %v", err)
	}
}

// TestGotoCmd_PreGotoRefusalLeavesStatusUntouched asserts a refusing PreGoto hook puts its message on the envelope and leaves the status file byte-identical.
func TestGotoCmd_PreGotoRefusalLeavesStatusUntouched(t *testing.T) {
	paths := newTestPaths(t)
	seedBlocked(t, paths, "B")
	before, err := os.ReadFile(paths.StatusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}

	spec := gotoSpec(paths)
	spec.Hooks.PreGoto = func(_ context.Context, target string) error {
		return errors.New("hook refused " + target)
	}
	env, code := execEnvelope(t, gotoCmd(gotoTexts(), spec), []string{"--to", "A"})
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if msg, _ := env["error"].(string); msg != "hook refused A" {
		t.Errorf("error = %q; want the hook's message verbatim", msg)
	}
	after, err := os.ReadFile(paths.StatusPath)
	if err != nil {
		t.Fatalf("re-read status: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("status file changed under a refusing hook")
	}
}

// TestGotoCmd_PassingPreGotoMoves asserts a passing hook still lets the move through;
// the nil-hook case is TestGotoCmd_MovesBlockedRunOntoRow.
func TestGotoCmd_PassingPreGotoMoves(t *testing.T) {
	paths := newTestPaths(t)
	seedBlocked(t, paths, "B")
	spec := gotoSpec(paths)
	spec.Hooks.PreGoto = func(context.Context, string) error { return nil }
	env, code := execEnvelope(t, gotoCmd(gotoTexts(), spec), []string{"--to", "A"})
	if code != 0 || env["current_producer"] != "A" {
		t.Errorf("code=%d env=%v; want a move onto A", code, env)
	}
}
