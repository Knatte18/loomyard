// ownercheck_test.go pins the owner refusal of `lyx loom start` and `lyx loom resume` over a fake owner verdict.

package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestStartAndResume_RefuseARunOwnedElsewhere drives both verbs over a receiver with a fake ownerOf:
// an owner elsewhere refuses with the kind owned_elsewhere, the owner and `lyx adopt`, before the bootstrap lock is taken;
// a verdict error refuses naming its cause and the re-run; a run owned here goes on; and an absent status file never calls the seam.
func TestStartAndResume_RefuseARunOwnedElsewhere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusExists bool
		verdict      ownerVerdict
		verdictErr   error
		wantCalls    int
		// wantKind is the refusal's kind, empty when the verb must go on past the owner check.
		wantKind string
		// wantMessage lists substrings of the refusal's error.
		wantMessage []string
	}{
		{
			name:         "OwnedElsewhereRefusesBeforeTheBootstrapLock",
			statusExists: true,
			verdict:      ownerVerdict{Owner: "there", Elsewhere: true},
			wantCalls:    1,
			wantKind:     shedrun.StartOwnedElsewhereKind,
			wantMessage:  []string{`"there"`, "lyx adopt pair"},
		},
		{
			name:         "VerdictErrorRefusesNamingTheRerun",
			statusExists: true,
			verdictErr:   errors.New("origin reconcile exploded"),
			wantCalls:    1,
			wantMessage:  []string{"origin reconcile exploded", `run "lyx loom %s" again`},
		},
		{
			name:         "OwnedHereGoesOn",
			statusExists: true,
			wantCalls:    1,
		},
		{
			name: "AbsentStatusFileNeverCallsTheSeam",
		},
	}
	verbs := map[string]func(c *loomCLI) *cobra.Command{
		"start":  (*loomCLI).startCmd,
		"resume": (*loomCLI).resumeCmd,
	}
	for verb, build := range verbs {
		for _, tt := range tests {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				c, _ := newTestSpawnAndWaitReceiver(t, &fakeDriverStarter{}, &fakeDriverPaneProbeFull{strandsFn: noStrands})
				calls := 0
				c.ownerOf = func(*lyxcwd.Location, string) (ownerVerdict, error) {
					calls++
					return tt.verdict, tt.verdictErr
				}
				if tt.statusExists {
					if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{State: shedengine.StateDone}); err != nil {
						t.Fatalf("write status: %v", err)
					}
				}

				ctx, _ := clihelp.NewExitContext(context.Background())
				var out bytes.Buffer
				cmd := build(c)
				cmd.SetContext(ctx)
				cmd.SetOut(&out)
				_ = cmd.RunE(cmd, nil)

				if calls != tt.wantCalls {
					t.Errorf("%s called ownerOf %d times; want %d", verb, calls, tt.wantCalls)
				}
				var envelope struct {
					Error string `json:"error"`
					Kind  string `json:"kind"`
					Owner string `json:"owner"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatalf("decode envelope %q: %v", out.String(), err)
				}
				if tt.wantKind != "" && envelope.Kind != tt.wantKind {
					t.Errorf("%s refusal kind = %q; want %q; envelope %s", verb, envelope.Kind, tt.wantKind, out.String())
				}
				if tt.wantKind == "" && envelope.Kind == shedrun.StartOwnedElsewhereKind {
					t.Errorf("%s refused as owned elsewhere; want it to go past the owner check; envelope %s", verb, out.String())
				}
				if tt.wantKind == shedrun.StartOwnedElsewhereKind && envelope.Owner != tt.verdict.Owner {
					t.Errorf("%s refusal owner = %q; want %q", verb, envelope.Owner, tt.verdict.Owner)
				}
				for _, want := range tt.wantMessage {
					want = strings.ReplaceAll(want, "%s", verb)
					if !strings.Contains(envelope.Error, want) {
						t.Errorf("%s refusal error = %q; want it to carry %q", verb, envelope.Error, want)
					}
				}
				refused := len(tt.wantMessage) > 0
				if _, err := os.Stat(loomengine.LoomBootstrapLock(c.location)); refused && err == nil {
					t.Errorf("%s took the bootstrap lock before refusing", verb)
				}
			})
		}
	}
}
