//go:build integration

// fixverify_test.go covers the `fix-verify` verb's envelopes and state persistence over the verbs fixture: the no-run refusal, a running snapshot, a done reply and a dead fixer, each driven through clihelp.Execute.

package webstercli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestFixVerifyCmd drives the verb's first call, which spawns the cold fixer and returns a running snapshot, then the second call that attaches to it and reaches a terminal result.
func TestFixVerifyCmd(t *testing.T) {
	// spawnWait makes the spawn call return its running snapshot at once.
	const spawnWait = "1ns"

	tests := []struct {
		name string
		// arrange acts on the fixture after the spawn call and before the attaching call.
		arrange     func(t *testing.T, fx *verbsFixture)
		wantExit    int
		wantStatus  string
		wantReason  string
		wantReply   string
		wantRecord  string
		wantRecDead string
	}{
		{
			name: "a reply file is done and saves the fixer's status",
			arrange: func(t *testing.T, fx *verbsFixture) {
				if err := os.WriteFile(websterengine.VerifyFixReplyPath(fx.CLI.geom.ScratchDir), []byte("fixed a"), 0o644); err != nil {
					t.Fatalf("write reply: %v", err)
				}
			},
			wantExit:   0,
			wantStatus: websterengine.DigestStatusDone,
			wantReply:  "fixed a",
			wantRecord: websterengine.DigestStatusDone,
		},
		{
			name:        "a gone strand is dead, exits non-zero and saves the fixer's status",
			arrange:     func(_ *testing.T, fx *verbsFixture) { fx.Reed.Strands = nil },
			wantExit:    1,
			wantStatus:  websterengine.DigestStatusDead,
			wantReason:  websterengine.DeadReasonDied,
			wantRecord:  websterengine.DigestStatusDead,
			wantRecDead: websterengine.DeadReasonDied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			fx.initState(t)
			report := websterengine.VerifyGateReport{Attempt: 2, Cap: 2, Fixer: websterengine.FixerCold}
			if err := os.MkdirAll(fx.CLI.geom.ReportsDir, 0o755); err != nil {
				t.Fatalf("mkdir reports dir: %v", err)
			}
			if err := websterengine.WriteVerifyGateReport(websterengine.VerifyGateReportPath(fx.CLI.geom.ReportsDir), report); err != nil {
				t.Fatalf("WriteVerifyGateReport() = %v", err)
			}

			var spawnOut strings.Builder
			if exitCode := clihelp.Execute(fx.CLI.fixVerifyCmd(), &spawnOut, []string{"--wait", spawnWait}); exitCode != 0 {
				t.Fatalf("fix-verify (spawn) = %d; want 0, output: %s", exitCode, spawnOut.String())
			}
			if got := decodeEnvelope(t, spawnOut.String())["status"]; got != websterengine.DigestStatusRunning {
				t.Fatalf("fix-verify (spawn) status = %v; want %q, output: %s", got, websterengine.DigestStatusRunning, spawnOut.String())
			}
			tt.arrange(t, fx)

			var out strings.Builder
			exitCode := clihelp.Execute(fx.CLI.fixVerifyCmd(), &out, nil)
			if exitCode != tt.wantExit {
				t.Fatalf("fix-verify (attach) = %d; want %d, output: %s", exitCode, tt.wantExit, out.String())
			}
			envelope := decodeEnvelope(t, out.String())
			if envelope["status"] != tt.wantStatus {
				t.Errorf("status = %v; want %q, output: %s", envelope["status"], tt.wantStatus, out.String())
			}
			if got, _ := envelope["reply"].(string); got != tt.wantReply {
				t.Errorf("reply = %q; want %q", got, tt.wantReply)
			}
			if got, _ := envelope["dead_reason"].(string); got != tt.wantReason {
				t.Errorf("dead_reason = %q; want %q", got, tt.wantReason)
			}

			loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
			if err != nil || loaded == nil || len(loaded.VerifyFixers) != 1 {
				t.Fatalf("LoadState() = %+v, %v; want a state with the one fixer record", loaded, err)
			}
			if rec := loaded.VerifyFixers[0]; rec.Status != tt.wantRecord || rec.DeadReason != tt.wantRecDead {
				t.Errorf("saved fixer record status %q, dead reason %q; want %q, %q", rec.Status, rec.DeadReason, tt.wantRecord, tt.wantRecDead)
			}
		})
	}

	t.Run("no state.json refuses with the run-first way forward", func(t *testing.T) {
		t.Setenv("FABRIC_SKIP_GIT", "1")
		fx := newVerbsFixture(t)
		var out strings.Builder
		exitCode := clihelp.Execute(fx.CLI.fixVerifyCmd(), &out, nil)
		if exitCode != 1 || !strings.Contains(out.String(), `run \"lyx webster run\" first`) {
			t.Errorf("fix-verify = %d, output %s; want exit 1 naming the run-first way forward", exitCode, out.String())
		}
		if len(fx.Reed.AddedSpecs) != 0 {
			t.Errorf("AddStrand calls = %d; want no spawn", len(fx.Reed.AddedSpecs))
		}
	})
}

// decodeEnvelope parses one verb envelope into its fields.
func decodeEnvelope(t *testing.T, body string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", body, err)
	}
	return envelope
}
