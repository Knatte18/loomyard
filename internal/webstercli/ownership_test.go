package webstercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// TestRefuseOrphanedMaster_RefusesAMasterWhoseRunEnded pins the bracket-verb guard.
// A call from a Master strand while run.lock is free refuses with `run_ended` and its way forward.
// A Master strand whose lock probe fails refuses with the way forward to re-run and no `run_ended`.
// The verb never runs.
// A Master strand under a held run.lock, another strand and an unnamed shell reach the verb.
// The verb tree built by addVerbs wraps each bracket verb and no other verb.
// It is not parallel: it sets the strand name variable with t.Setenv.
func TestRefuseOrphanedMaster_RefusesAMasterWhoseRunEnded(t *testing.T) {
	tiergate.Require(t, 0)

	const masterCaller = "hub:task-a:" + websterengine.MerriamStrandRole
	tests := []struct {
		name    string
		caller  string
		runLive bool
		// probeFails makes run.lock a directory, so the lock probe errors.
		probeFails bool
		wantRan    bool
		// wantRefusal is the text the refusal carries, and wantWayForward its way forward.
		wantRefusal    string
		wantWayForward string
		wantRunEnded   bool
	}{
		{
			name: "a Master strand whose run ended", caller: masterCaller,
			wantRefusal: "record-batch refuses: no live `lym webster run` owns this state", wantWayForward: "way forward: end your turn with no further tool call", wantRunEnded: true,
		},
		{
			name: "a Master strand whose lock probe fails", caller: masterCaller, probeFails: true,
			wantRefusal: "record-batch could not tell whether a live `lym webster run` owns this state", wantWayForward: "way forward: re-run the verb",
		},
		{name: "a Master strand under a live run", caller: masterCaller, runLive: true, wantRan: true},
		{name: "another strand", caller: "hub:task-a:darn", wantRan: true},
		{name: "an unnamed shell", wantRan: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scratchDir := t.TempDir()
			if tt.runLive {
				held, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, "run.lock"))
				if err != nil || !locked {
					t.Fatalf("hold run.lock = locked %v, err %v; want it held", locked, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}
			if tt.probeFails {
				if err := os.Mkdir(filepath.Join(scratchDir, "run.lock"), 0o755); err != nil {
					t.Fatalf("make run.lock a directory: %v", err)
				}
			}
			t.Setenv(agentname.StrandNameEnv, tt.caller)
			ran := false
			c := &websterCLI{geom: websterengine.Geometry{ScratchDir: scratchDir}}
			cmd := c.refuseOrphanedMaster(&cobra.Command{
				Use: "record-batch",
				RunE: func(*cobra.Command, []string) error {
					ran = true
					return nil
				},
			})

			var out bytes.Buffer
			code := clihelp.Execute(cmd, &out, nil)

			if ran != tt.wantRan {
				t.Fatalf("verb ran = %v; want %v (output %q)", ran, tt.wantRan, out.String())
			}
			if tt.wantRan {
				return
			}
			if code == 0 {
				t.Errorf("exit = 0; want non-zero for a refusal")
			}
			env := envelope.RequireErr(t, out.String(), tt.wantRefusal)
			if runEnded := env.Raw["run_ended"] == true; runEnded != tt.wantRunEnded {
				t.Errorf("envelope = %v; want run_ended %v", env.Raw, tt.wantRunEnded)
			}
			envelope.RequireErr(t, out.String(), tt.wantWayForward)
		})
	}

	t.Run("addVerbs wraps each bracket verb and no other", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, masterCaller)
		c := &websterCLI{geom: websterengine.Geometry{ScratchDir: t.TempDir(), WebsterDir: t.TempDir()}}
		verbs := []struct {
			name        string
			args        []string
			wantRefused bool
		}{
			{"begin-batch", []string{"1"}, true},
			{"await-batch", []string{"1"}, true},
			{"record-batch", []string{"1"}, true},
			{"recover-batch", []string{"1"}, true},
			{"fix-verify", nil, true},
			{"fix-plan", nil, true},
			{"status", nil, false},
		}
		for _, verb := range verbs {
			parent := &cobra.Command{Use: "webster", RunE: clihelp.GroupRunE}
			c.addVerbs(parent)

			var out bytes.Buffer
			clihelp.Execute(parent, &out, append([]string{verb.name}, verb.args...))

			if refused := strings.Contains(out.String(), `"run_ended":true`); refused != verb.wantRefused {
				t.Errorf("%s with a free run.lock refused for an ended run = %v; want %v (output %q)", verb.name, refused, verb.wantRefused, out.String())
			}
		}
	})
}
