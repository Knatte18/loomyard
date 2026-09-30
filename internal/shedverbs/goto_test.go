// goto_test.go covers the generic goto body: a blocked run moves onto the named row paused,
// and each refusal names its way forward, listing the valid producers where the target is at fault.

package shedverbs

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

func gotoTexts() VerbTexts {
	return VerbTexts{Goto: VerbText{Use: "goto", Short: "move the fake shed onto a row"}}
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
	seedBlocked(t, paths, "A")

	env, code := execEnvelope(t, gotoCmd(gotoTexts(), gotoSpec(paths)), []string{"--to", "B"})
	if code != 0 {
		t.Fatalf("exit code = %d; want 0 (env %v)", code, env)
	}
	if env["current_producer"] != "B" || env["state"] != "paused" {
		t.Errorf("envelope current_producer/state = %v/%v; want B/paused", env["current_producer"], env["state"])
	}
	if env["run_id"] != "run-1" || env["status_file"] != paths.StatusPath {
		t.Errorf("envelope run_id/status_file = %v/%v", env["run_id"], env["status_file"])
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("re-read status: found=%v err=%v", found, err)
	}
	if st.CurrentProducer != "B" || st.State != shedengine.StatePaused {
		t.Errorf("status = %s/%s; want B/paused", st.CurrentProducer, st.State)
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
			seedBlocked(t, paths, "A")

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
	seedBlocked(t, paths, "A")
	held, locked, err := lock.TryAcquireWriteLock(paths.LockPath)
	if err != nil || !locked {
		t.Fatalf("acquire run lock: locked=%v err=%v", locked, err)
	}
	defer held.Release()

	env, code := execEnvelope(t, gotoCmd(gotoTexts(), gotoSpec(paths)), []string{"--to", "B"})
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if msg, _ := env["error"].(string); !strings.Contains(msg, "lyx shed pause") {
		t.Errorf("error %q does not name lyx shed pause", msg)
	}
}
