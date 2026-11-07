// describe_test.go tests the describe sequence over fake deps.
// It covers the beams described at Beam-Await in any state.
// It also covers each refusal with its way forward and a description the writer refuses.

package beamcli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDescribe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		slug  string
		setup func(w *landWorld)
		// wantErr is empty for a described beam, else every substring the refusal names.
		wantErr []string
		// wantDescribed reports whether the Describe seam ran.
		wantDescribed bool
	}{
		{name: "BeamAtBeamAwaitRunningDescribed", slug: "b", wantDescribed: true},
		{
			name: "BeamAtBeamAwaitFailedDescribed",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateFailed, CurrentProducer: pawlengine.NameBeamAwait}
			},
			wantDescribed: true,
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
			name: "StandingReadyRecordRefusedNamingStatus",
			slug: "b",
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
			name: "ValidationFindingReturnedNamingBoardUpsert",
			slug: "b",
			setup: func(w *landWorld) {
				w.describeErr = errors.New("landingshed: write the beam description: validate the description: the brief is empty; way forward: fix the title or brief with \"lym board upsert\", then run \"lym beam describe b\" again")
			},
			wantErr:       []string{"the brief is empty", "lym board upsert"},
			wantDescribed: true,
		},
		{
			name: "WriterFailureWithoutAWayForwardGetsOne",
			slug: "b",
			setup: func(w *landWorld) {
				w.describeErr = errors.New("the commit failed")
			},
			wantErr:       []string{"the commit failed", "way forward:", "lym beam describe b"},
			wantDescribed: true,
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

			got, err := describe(w.deps(), tt.slug)

			if described := len(w.described) > 0; described != tt.wantDescribed {
				t.Errorf("describe(%q) ran the Describe seam = %v; want %v", tt.slug, described, tt.wantDescribed)
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
				t.Fatalf("describe(%q) = %v; want the description written", tt.slug, err)
			}
			if len(w.described) != 1 || w.described[0] != tt.slug {
				t.Errorf("describe(%q) called the Describe seam with %v; want exactly the slug once", tt.slug, w.described)
			}
			if got["description"] != "/hub/b/_lym/landing/summary.md" || !strings.Contains(got["way_forward"].(string), "lym beam pr open b") {
				t.Errorf("describe(%q) = %v; want the description's path and `lym beam pr open b`", tt.slug, got)
			}
		})
	}
}
