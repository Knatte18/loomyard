//go:build tier1

// fixplan_test.go covers the `fix-plan` verb's envelopes and state persistence over the verbs fixture: the no-record refusal, a running snapshot, a done verdict and a dead fixer, each driven through clihelp.Execute.

package webstercli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestFixPlanCmd drives the verb's first call, which spawns the plan fixer and returns a running snapshot, then the second call that attaches to it and reaches a terminal result.
func TestFixPlanCmd(t *testing.T) {
	// spawnWait makes the spawn call return its running snapshot at once.
	const spawnWait = "1ns"

	tests := []struct {
		name string
		// arrange acts on the fixture after the spawn call and before the attaching call.
		arrange     func(t *testing.T, fx *verbsFixture)
		wantExit    int
		wantStatus  string
		wantVerdict string
		wantReason  string
		wantRecord  string
	}{
		{
			name: "a fixed reply is done and saves the fixer's verdict",
			arrange: func(t *testing.T, fx *verbsFixture) {
				if err := os.WriteFile(websterengine.PlanFixReplyPath(fx.CLI.geom.ScratchDir), []byte("verdict: fixed\n- 02: Old -> New\n"), 0o644); err != nil {
					t.Fatalf("write reply: %v", err)
				}
			},
			wantExit:    0,
			wantStatus:  websterengine.DigestStatusDone,
			wantVerdict: websterengine.PlanFixVerdictFixed,
			wantRecord:  websterengine.DigestStatusDone,
		},
		{
			name:       "a gone strand is dead, exits non-zero naming the stuck way forward",
			arrange:    func(_ *testing.T, fx *verbsFixture) { fx.Reed.Strands = nil },
			wantExit:   1,
			wantStatus: websterengine.DigestStatusDead,
			wantReason: websterengine.DeadReasonDied,
			wantRecord: websterengine.DigestStatusDead,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			fx.initState(t)
			if err := os.MkdirAll(fx.CLI.geom.ScratchDir, 0o755); err != nil {
				t.Fatalf("mkdir scratch dir: %v", err)
			}
			conflict := &websterengine.PlanConflictError{Batch: 2, Cause: websterengine.ConflictCauseTreeMoved, Findings: []string{"glyph-not-found/02: base.go#Old"}}
			if err := websterengine.WriteConflictRecord(fx.CLI.geom.ScratchDir, conflict); err != nil {
				t.Fatalf("WriteConflictRecord() = %v", err)
			}

			var spawnOut strings.Builder
			if exitCode := clihelp.Execute(fx.CLI.fixPlanCmd(), &spawnOut, []string{"--wait", spawnWait}); exitCode != 0 {
				t.Fatalf("fix-plan (spawn) = %d; want 0, output: %s", exitCode, spawnOut.String())
			}
			if got := decodeEnvelope(t, spawnOut.String())["status"]; got != websterengine.DigestStatusRunning {
				t.Fatalf("fix-plan (spawn) status = %v; want %q, output: %s", got, websterengine.DigestStatusRunning, spawnOut.String())
			}
			tt.arrange(t, fx)

			var out strings.Builder
			exitCode := clihelp.Execute(fx.CLI.fixPlanCmd(), &out, nil)
			if exitCode != tt.wantExit {
				t.Fatalf("fix-plan (attach) = %d; want %d, output: %s", exitCode, tt.wantExit, out.String())
			}
			envelope := decodeEnvelope(t, out.String())
			if envelope["status"] != tt.wantStatus {
				t.Errorf("status = %v; want %q, output: %s", envelope["status"], tt.wantStatus, out.String())
			}
			if got, _ := envelope["verdict"].(string); got != tt.wantVerdict {
				t.Errorf("verdict = %q; want %q, output: %s", got, tt.wantVerdict, out.String())
			}
			if got, _ := envelope["dead_reason"].(string); got != tt.wantReason {
				t.Errorf("dead_reason = %q; want %q", got, tt.wantReason)
			}
			if tt.wantExit != 0 && !strings.Contains(out.String(), "way forward: write outcome: stuck") {
				t.Errorf("output = %s; want the stuck way forward", out.String())
			}

			loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
			if err != nil || loaded == nil || len(loaded.PlanFixers) != 1 {
				t.Fatalf("LoadState() = %+v, %v; want a state with the one plan fixer record", loaded, err)
			}
			if rec := loaded.PlanFixers[0]; rec.Status != tt.wantRecord || rec.DeadReason != tt.wantReason || rec.Verdict != tt.wantVerdict {
				t.Errorf("saved plan fixer record status %q, dead reason %q, verdict %q; want %q, %q, %q", rec.Status, rec.DeadReason, rec.Verdict, tt.wantRecord, tt.wantReason, tt.wantVerdict)
			}
		})
	}

	t.Run("no plan-conflict record refuses with its way forward and spawns nothing", func(t *testing.T) {
		t.Setenv("FABRIC_SKIP_GIT", "1")
		fx := newVerbsFixture(t)
		fx.initState(t)
		var out strings.Builder
		exitCode := clihelp.Execute(fx.CLI.fixPlanCmd(), &out, nil)
		if exitCode != 1 || !strings.Contains(out.String(), "only right after begin-batch refuses with plan_conflict") {
			t.Errorf("fix-plan = %d, output %s; want exit 1 naming the way forward", exitCode, out.String())
		}
		if len(fx.Reed.AddedSpecs) != 0 {
			t.Errorf("AddStrand calls = %d; want no spawn", len(fx.Reed.AddedSpecs))
		}
	})
}
