// describe_test.go tests the describe sequence over fake deps and a fake GitHub.
// It covers the beams described at Beam-Await in any state, the draft, and the update of a recorded pull request.
// It also covers each refusal with its way forward and a description the recorder refuses.

package beamcli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/spf13/cobra"
)

// TestDescribeCmd_DraftFlagReachesTheDraftSeam asserts the cobra verb binds --draft.
// It also asserts the verb passes the flag, with the slug, to describe.
func TestDescribeCmd_DraftFlagReachesTheDraftSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		args         []string
		wantDrafted  bool
		wantRecorded bool
	}{
		{name: "WithDraftFlag", args: []string{"b", "--draft"}, wantDrafted: true},
		{name: "WithoutDraftFlag", args: []string{"b"}, wantRecorded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			w.ready["b"] = false
			cmd := describeCmdOver(func(cmd *cobra.Command, slug string, draft bool) (*map[string]any, error) {
				result, err := describe(cmd.Context(), w.deps(), slug, draft)
				if err != nil {
					return nil, err
				}
				return &result, nil
			})

			var out bytes.Buffer
			exit := clihelp.Execute(cmd, &out, tt.args)

			if exit != 0 {
				t.Fatalf("describe %v exit = %d, output %q; want 0", tt.args, exit, out.String())
			}
			if drafted := slices.Equal(w.drafted, []string{"b"}); drafted != tt.wantDrafted {
				t.Errorf("describe %v drafted = %v (%v); want %v", tt.args, drafted, w.drafted, tt.wantDrafted)
			}
			if recorded := slices.Equal(w.recorded, []string{"b"}); recorded != tt.wantRecorded {
				t.Errorf("describe %v recorded = %v (%v); want %v", tt.args, recorded, w.recorded, tt.wantRecorded)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const descriptionPath = "/hub/b/_lym/landing/summary.md"
	tests := []struct {
		name  string
		slug  string
		draft bool
		setup func(w *landWorld)
		// wantErr is empty for a described beam, else every substring the refusal names.
		wantErr []string
		// wantDrafted and wantRecorded report whether the DraftDescription and RecordDescription seams ran.
		wantDrafted  bool
		wantRecorded bool
		// wantEdits are the pull request Edit calls the verb makes.
		wantEdits []string
		// wantWayForward is a substring the answer's way forward names.
		wantWayForward string
	}{
		{name: "BeamAtBeamAwaitRunningRecorded", slug: "b", wantRecorded: true, wantWayForward: "lym beam pr open b"},
		{
			name: "BeamAtBeamAwaitFailedRecorded",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateFailed, CurrentProducer: pawlengine.NameBeamAwait}
			},
			wantRecorded:   true,
			wantWayForward: "lym beam pr open b",
		},
		{
			name:           "DraftIsWrittenAndNothingRecorded",
			slug:           "b",
			draft:          true,
			wantDrafted:    true,
			wantWayForward: "lym beam describe b",
		},
		{
			name: "RecordedPullRequestIsUpdatedWithTheText",
			slug: "b",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen}})
			},
			wantRecorded:   true,
			wantEdits:      []string{"5|Ship the beam|Groups the work."},
			wantWayForward: "lym beam ready b",
		},
		{
			name: "FailedPullRequestUpdateRefusedSayingTheDescriptionIsRecorded",
			slug: "b",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodEdit, prkit.Answer{Err: errors.New("GitHub is down")})
			},
			wantErr:      []string{"is recorded, but its pull request is not updated", "GitHub is down", "lym beam pr update b"},
			wantRecorded: true,
			wantEdits:    []string{"5|Ship the beam|Groups the work."},
		},
		{
			name: "UnreadableLandingRecordAfterRecordingNamesTheUpdate",
			slug: "b",
			setup: func(w *landWorld) {
				w.landingErr = errors.New("the record is corrupt")
			},
			wantErr:      []string{"is recorded, but the landing record is unreadable", "the record is corrupt", "lym beam pr update b"},
			wantRecorded: true,
		},
		{name: "EntryThatIsNoBeamRefused", slug: "ghost", wantErr: []string{"lym beam describe:", "not a beam", "lym board list"}},
		{
			name: "BeamWithNoRunRefusedNamingCreate",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.children, "b")
			},
			wantErr: []string{"no pawl run", "lym beam create b"},
		},
		{
			name: "RunAtFinalizeRefusedNamingStatus",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
			},
			wantErr: []string{"Finalize", "lym batten status b"},
		},
		{
			name: "RunAtBeamDownRefusedNamingStatus",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamDown}
			},
			wantErr: []string{pawlengine.NameBeamDown, "lym batten status b"},
		},
		{
			name:  "StandingReadyRecordRefusesTheDraftToo",
			slug:  "b",
			draft: true,
			setup: func(w *landWorld) {
				w.ready["b"] = true
			},
			wantErr: []string{"lym beam ready", "final", "lym batten status b"},
		},
		{
			name: "FailedReadyReadRefusedNamingDescribe",
			slug: "b",
			setup: func(w *landWorld) {
				w.readyErr = errors.New("the record is corrupt")
			},
			wantErr: []string{"the record is corrupt", "lym beam describe b"},
		},
		{
			name: "RecorderFindingKeepsItsWayForward",
			slug: "b",
			setup: func(w *landWorld) {
				w.recordErr = errors.New("landingshed: record the beam description: validate the description: empty-body: body is empty; way forward: fix the description at /hub/b/_lym/landing/summary.md, then run \"lym beam describe b\" again")
			},
			wantErr:      []string{"lym beam describe: landingshed:", "body is empty", "fix the description at"},
			wantRecorded: true,
		},
		{
			name:  "DraftFailureWithoutAWayForwardGetsOne",
			slug:  "b",
			draft: true,
			setup: func(w *landWorld) {
				w.draftErr = errors.New("the disk is full")
			},
			wantErr:     []string{"draft the description of b", "the disk is full", "way forward:", "lym beam describe b"},
			wantDrafted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			w.ready["b"] = false
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := describe(context.Background(), w.deps(), tt.slug, tt.draft)

			if drafted := len(w.drafted) > 0; drafted != tt.wantDrafted {
				t.Errorf("describe(%q, draft %v) ran the DraftDescription seam = %v; want %v", tt.slug, tt.draft, drafted, tt.wantDrafted)
			}
			if recorded := len(w.recorded) > 0; recorded != tt.wantRecorded {
				t.Errorf("describe(%q, draft %v) ran the RecordDescription seam = %v; want %v", tt.slug, tt.draft, recorded, tt.wantRecorded)
			}
			if edits := w.api.Calls(prkit.MethodEdit); !slices.Equal(edits, tt.wantEdits) {
				t.Errorf("describe(%q) Edit calls = %v; want %v", tt.slug, edits, tt.wantEdits)
			}
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("describe(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("describe(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("describe(%q) = %v; want the description recorded", tt.slug, err)
			}
			wayForward, _ := got["way_forward"].(string)
			if got["description"] != descriptionPath || !strings.Contains(wayForward, tt.wantWayForward) {
				t.Errorf("describe(%q) = %v; want the description's path and a way forward naming %q", tt.slug, got, tt.wantWayForward)
			}
		})
	}
}
