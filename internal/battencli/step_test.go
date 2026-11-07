// step_test.go tests battenPreStep and the step verb end to end.
// It covers seeding an absent status before shed.Step reads it and each of the three refusal-kind mappings.
// It also checks that BuildShed is non-nil for verb == "step".
// Every fixture is newFakeReceiver (run_test.go).
// So no real I/O goes beyond t.TempDir(), no git spawn, and no process spawn.

package battencli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// battenPreStep seeds a fresh status file, with producer Worktree-Create and state running, when none is persisted yet.
// It runs before shed.Step reads the status.
// Without the seed, stepLocked's read gate hits an absent status and hard-errors.
// shedverbs/step.go reports that as kind: "producer".
// A driver would then repair round after round, so an unseeded first step must not surface as a producer failure.
//
//testtiming:keep pins the seeded status's current producer and state, which the end-to-end step test never reads
func TestBattenPreStep_SeedsAbsentStatus(t *testing.T) {
	tiergate.Require(t, 0)
	c := newFakeReceiver(t, nil)

	if _, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath); err != nil || found {
		t.Fatalf("precondition: status read = (found=%v, err=%v); want (false, nil)", found, err)
	}

	rowNoticePath := filepath.Join(c.env.ScratchDir, battenrecipe.NameWorktreeTeardown+"-row-notice")
	if err := os.WriteFile(rowNoticePath, []byte("failure 3\n"), 0o644); err != nil {
		t.Fatalf("plant row-notice marker: %v", err)
	}

	kind, err := c.battenPreStep(context.Background())
	if err != nil {
		t.Fatalf("battenPreStep() = (%q, %v); want (\"\", nil)", kind, err)
	}
	if kind != "" {
		t.Errorf("battenPreStep() kind = %q; want empty on success", kind)
	}
	if _, err := os.Stat(rowNoticePath); !os.IsNotExist(err) {
		t.Errorf("row-notice marker stat error = %v; want it cleared by the fresh seed", err)
	}

	st, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("status after battenPreStep = (found=%v, err=%v); want (true, nil)", found, err)
	}
	if st.CurrentProducer != battenrecipe.NameWorktreeCreate {
		t.Errorf("st.CurrentProducer = %q; want %q", st.CurrentProducer, battenrecipe.NameWorktreeCreate)
	}
	if st.State != shedengine.StateRunning {
		t.Errorf("st.State = %q; want %q", st.State, shedengine.StateRunning)
	}
}

// battenPreStep maps each pre-producer failure to its own refusal kind, with a non-nil error that is never transient.
// A held run lock is shedverbs.KindBusy.
// A status file that exists but fails to decode is shedverbs.KindUnseeded.
// Any other pre-producer failure, such as a status already in StateDone, is shedverbs.KindBootstrap.
// Its remedy names the whole abandon path.
func TestBattenPreStep_RefusalKinds(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		arrange    func(t *testing.T, c *battenCLI)
		wantKind   string
		wantSubstr []string
	}{
		{
			name: "RunLockHeldIsBusy",
			arrange: func(t *testing.T, c *battenCLI) {
				held, err := lock.AcquireWriteLock(c.shedPaths.LockPath)
				if err != nil {
					t.Fatalf("acquire run lock in test: %v", err)
				}
				t.Cleanup(func() { _ = held.Release() })
			},
			wantKind: shedverbs.KindBusy,
		},
		{
			name: "DecodeFailureIsUnseeded",
			arrange: func(t *testing.T, c *battenCLI) {
				if err := os.WriteFile(c.shedPaths.StatusPath, []byte("not valid json{{{"), 0o644); err != nil {
					t.Fatalf("seed corrupt status file: %v", err)
				}
			},
			wantKind: shedverbs.KindUnseeded,
		},
		{
			// A torn-down pair keeps its task branch locally and on the remote.
			// Deleting the run directory alone therefore leads straight into the create row's leftover-branch refusal.
			name: "DoneSlugIsBootstrap",
			arrange: func(t *testing.T, c *battenCLI) {
				writeStatus(t, c, shedengine.Status{
					CurrentProducer: battenrecipe.NameWorktreeTeardown,
					State:           shedengine.StateDone,
				})
			},
			wantKind:   shedverbs.KindBootstrap,
			wantSubstr: []string{"lym batten cleanup --apply", "task branch", "locally and on the remote", "to run it again", "different slug"},
		},
		{
			// A step drives no run another machine owns,
			// so the reconcile's owned_elsewhere verdict refuses it as a bootstrap failure.
			name: "RunOwnedElsewhereIsBootstrap",
			arrange: func(t *testing.T, c *battenCLI) {
				writeStatus(t, c, shedengine.Status{
					CurrentProducer: battenrecipe.NameLand,
					State:           shedengine.StateRunning,
				})
				c.reconcile = func(string) (fabricengine.OriginReport, error) {
					return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: c.slug, State: fabricengine.EntryOwnedElsewhere, Owner: "there", AdoptSubject: "some-beam"}}}, nil
				}
			},
			wantKind:   shedverbs.KindBootstrap,
			wantSubstr: []string{`"there"`, "`lym adopt some-beam`"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			tt.arrange(t, c)

			kind, err := c.battenPreStep(context.Background())
			if err == nil {
				t.Fatal("battenPreStep() = nil error; want a refusal")
			}
			if kind != tt.wantKind {
				t.Errorf("battenPreStep() kind = %q; want %q", kind, tt.wantKind)
			}
			if got := shedengine.TransientOf(err); got != "" {
				t.Errorf("TransientOf(battenPreStep error) = %q; want empty", got)
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("battenPreStep() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestStepCmd_FreshSlugNeverSurfacesKindProducer drives the step verb on a fresh slug with no persisted status.
// It runs the verb end to end and asserts it never surfaces kind: "producer".
// Conflating the unseeded and producer cases would make both read as "nothing here yet".
// battenPreStep's seed lets shed.Step's Worktree-Create row run to completion against newFakeReceiver's fakes.
func TestStepCmd_FreshSlugNeverSurfacesKindProducer(t *testing.T) {
	tiergate.Require(t, 0)
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(battenVerbCommand(c, "step"), &out, []string{c.slug})

	if strings.Contains(out.String(), `"kind":"producer"`) {
		t.Errorf("step() output = %q; must never surface kind: \"producer\" for a fresh slug", out.String())
	}
	if exitCode != 0 {
		t.Errorf("step() exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf("step() output missing ok:true envelope; got: %q", out.String())
	}
}
