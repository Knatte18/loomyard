// trace_test.go covers Trace over fakes of its three quarry seams, with a fixed plan and Webster state, and NewTracer's missing-plan and missing-state errors over temp-dir fixtures.

package failuretrace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/Knatte18/quarry/quarry"
)

const traceHead = "headsha"

// tracePlan is a plan of cards that each target one kind of ref.
func tracePlan() *planparser.Plan {
	return &planparser.Plan{Language: "go", Cards: []planparser.Card{
		{Number: 3, Slug: "alpha", Targets: []string{"internal/a#Run"}},
		{Number: 4, Slug: "beta", Targets: []string{"internal/b/b.go#"}},
		{Number: 5, Slug: "gamma", Targets: []string{"internal/c#Helper"}},
		{Number: 6, Slug: "delta", Targets: []string{"internal/d#Gone"}},
		{Number: 7, Slug: "eps", Targets: []string{"internal/e#Moved"}},
		{Number: 12, Slug: "tests", Targets: []string{"plan:internal/t#TestNew"}},
		{Number: 13, Slug: "nobatch", Targets: []string{"internal/n#Thing"}},
	}}
}

// traceState records cards 3 and 4 in one batch, 5 and 6 in another, card 7 in a batch with no card commit, and card 12 in a batch record that predates the Cards field.
func traceState() *websterengine.State {
	return &websterengine.State{Batches: map[int]*websterengine.BatchState{
		1:  {Slug: "alpha", Cards: []string{"03-alpha", "04-beta"}, StartSHA: "s1", CardSHAs: []string{"c3", "c4"}},
		2:  {Slug: "gamma", Cards: []string{"05-gamma", "06-delta"}, StartSHA: "s2", CardSHAs: []string{"c5", "c6"}},
		7:  {Slug: "eps", Cards: []string{"07-eps"}, StartSHA: "s7"},
		12: {Slug: "tests", StartSHA: "s12", CardSHAs: []string{"c12"}},
	}}
}

// buildFailure is the go test output of a package that fails to build with one error at location.
func buildFailure(pkg, location string) string {
	return "# " + testModule + "/" + pkg + "\n" + location + ": boom\nFAIL\t" + testModule + "/" + pkg + " [build failed]\n"
}

// found is an enclose answer holding one member in file.
func found(file, member string) quarry.EncloseResult {
	return quarry.EncloseResult{Status: quarry.StatusFound, File: file, Symbols: []quarry.Symbol{{ID: member, File: file}}}
}

// outsideMembers is an enclose answer for a location inside no member of file.
func outsideMembers(file string) quarry.EncloseResult {
	return quarry.EncloseResult{Status: quarry.StatusNotFound, File: file, Unit: file + "#"}
}

func TestTrace(t *testing.T) {
	t.Parallel()

	allFiles := []string{"internal/a/a.go", "internal/b/b.go", "internal/c/c.go", "internal/d/d.go", "internal/e/e.go", "internal/n/n.go", "internal/z/z.go", "internal/r/r.go"}
	tests := []struct {
		name     string
		log      string
		enclose  map[string]quarry.EncloseResult
		enclErr  error
		resolved map[string]string
		deleted  map[string]quarry.Symbol
		noPlan   bool
		noState  bool
		want     verifytree.FailureTrace
		wantErr  bool
	}{
		{
			name:    "a build error inside a member a card targets names that card, its batch and its range",
			log:     buildFailure("internal/a", "internal/a/a.go:3:5"),
			enclose: map[string]quarry.EncloseResult{"internal/a/a.go:3": found("internal/a/a.go", "internal/a#Run")},
			want: verifytree.FailureTrace{
				Cards:      []verifytree.TracedCard{{Number: 3, Glyphs: []string{"internal/a#Run"}, Locations: []string{"internal/a/a.go:3"}, Batch: 1, Range: verifytree.CommitRange{From: "s1", To: "c4"}}},
				LaterRange: verifytree.CommitRange{From: "c4", To: traceHead},
			},
		},
		{
			name:    "an unused import in a file a card targets names that card",
			log:     buildFailure("internal/b", "internal/b/b.go:2:8"),
			enclose: map[string]quarry.EncloseResult{"internal/b/b.go:2": outsideMembers("internal/b/b.go")},
			want: verifytree.FailureTrace{
				Cards:      []verifytree.TracedCard{{Number: 4, Glyphs: []string{"internal/b/b.go#"}, Locations: []string{"internal/b/b.go:2"}, Batch: 1, Range: verifytree.CommitRange{From: "s1", To: "c4"}}},
				LaterRange: verifytree.CommitRange{From: "c4", To: traceHead},
			},
		},
		{
			name:     "an unused import in a file where a card targets only a member that resolves there names that card",
			log:      buildFailure("internal/c", "internal/c/c.go:1:8"),
			enclose:  map[string]quarry.EncloseResult{"internal/c/c.go:1": outsideMembers("internal/c/c.go")},
			resolved: map[string]string{"internal/c#Helper": "internal/c/c.go"},
			want: verifytree.FailureTrace{
				Cards:      []verifytree.TracedCard{{Number: 5, Glyphs: []string{"internal/c#Helper"}, Locations: []string{"internal/c/c.go:1"}, Batch: 2, Range: verifytree.CommitRange{From: "s2", To: "c6"}}},
				LaterRange: verifytree.CommitRange{From: "c6", To: traceHead},
			},
		},
		{
			name:    "a member target that resolves only at its batch start through the delta matches",
			log:     buildFailure("internal/d", "internal/d/d.go:5:2"),
			enclose: map[string]quarry.EncloseResult{"internal/d/d.go:5": outsideMembers("internal/d/d.go")},
			deleted: map[string]quarry.Symbol{"s2": {ID: "internal/d#Gone", File: "internal/d/d.go"}},
			want: verifytree.FailureTrace{
				Cards:      []verifytree.TracedCard{{Number: 6, Glyphs: []string{"internal/d#Gone"}, Locations: []string{"internal/d/d.go:5"}, Batch: 2, Range: verifytree.CommitRange{From: "s2", To: "c6"}}},
				LaterRange: verifytree.CommitRange{From: "c6", To: traceHead},
			},
		},
		{
			name:    "a member target that resolves neither at head nor in the delta leaves its location untraced",
			log:     buildFailure("internal/e", "internal/e/e.go:4:1"),
			enclose: map[string]quarry.EncloseResult{"internal/e/e.go:4": outsideMembers("internal/e/e.go")},
			want:    verifytree.FailureTrace{Untraced: []string{"internal/e/e.go:4"}},
		},
		{
			name:    "a failing test a card creates under a plan handle names that card, found in a batch record without Cards",
			log:     "--- FAIL: TestNew (0.00s)\n    t_test.go:9: boom\nFAIL\nFAIL\t" + testModule + "/internal/t\t0.1s\n",
			enclose: map[string]quarry.EncloseResult{"internal/t/t_test.go:9": found("internal/t/t_test.go", "internal/t#TestNew")},
			want: verifytree.FailureTrace{
				Cards:      []verifytree.TracedCard{{Number: 12, Glyphs: []string{"internal/t#TestNew"}, Locations: []string{"internal/t/t_test.go:9"}, Batch: 12, Range: verifytree.CommitRange{From: "s12", To: "c12"}}},
				LaterRange: verifytree.CommitRange{From: "c12", To: traceHead},
			},
		},
		{
			name:    "a batch with no card commit gives an empty range and, as the only matched batch, an empty later range",
			log:     buildFailure("internal/e", "internal/e/e.go:4:1"),
			enclose: map[string]quarry.EncloseResult{"internal/e/e.go:4": found("internal/e/e.go", "internal/e#Moved")},
			want: verifytree.FailureTrace{
				Cards: []verifytree.TracedCard{{Number: 7, Glyphs: []string{"internal/e#Moved"}, Locations: []string{"internal/e/e.go:4"}, Batch: 7}},
			},
		},
		{
			name:    "a card Webster holds no record of gets batch 0 and an empty range",
			log:     buildFailure("internal/n", "internal/n/n.go:6:1"),
			enclose: map[string]quarry.EncloseResult{"internal/n/n.go:6": found("internal/n/n.go", "internal/n#Thing")},
			want: verifytree.FailureTrace{
				Cards: []verifytree.TracedCard{{Number: 13, Glyphs: []string{"internal/n#Thing"}, Locations: []string{"internal/n/n.go:6"}}},
			},
		},
		{
			name: "two matched batches give cards in number order and the later range from the higher batch",
			log:  buildFailure("internal/c", "internal/c/c.go:1:1") + buildFailure("internal/a", "internal/a/a.go:3:5"),
			enclose: map[string]quarry.EncloseResult{
				"internal/c/c.go:1": found("internal/c/c.go", "internal/c#Helper"),
				"internal/a/a.go:3": found("internal/a/a.go", "internal/a#Run"),
			},
			want: verifytree.FailureTrace{
				Cards: []verifytree.TracedCard{
					{Number: 3, Glyphs: []string{"internal/a#Run"}, Locations: []string{"internal/a/a.go:3"}, Batch: 1, Range: verifytree.CommitRange{From: "s1", To: "c4"}},
					{Number: 5, Glyphs: []string{"internal/c#Helper"}, Locations: []string{"internal/c/c.go:1"}, Batch: 2, Range: verifytree.CommitRange{From: "s2", To: "c6"}},
				},
				LaterRange: verifytree.CommitRange{From: "c6", To: traceHead},
			},
		},
		{
			name: "a rejected location and a location no card's targets hold are untraced as path and line",
			log:  buildFailure("internal/z", "internal/z/z.go:1:1") + buildFailure("internal/r", "internal/r/r.go:2:1"),
			enclose: map[string]quarry.EncloseResult{
				"internal/z/z.go:1": found("internal/z/z.go", "internal/z#Q"),
				"internal/r/r.go:2": {Target: "internal/r/r.go:2", Reason: "unreadable", Error: "cannot read"},
			},
			want: verifytree.FailureTrace{Untraced: []string{"internal/z/z.go:1", "internal/r/r.go:2"}},
		},
		{
			name: "an opaque identity's bare name is untraced as written and asks quarry nothing",
			log:  "./y.go:4:2: syntax error\n",
			want: verifytree.FailureTrace{Untraced: []string{"./y.go:4"}},
		},
		{
			name:    "an enclose error is returned",
			log:     buildFailure("internal/a", "internal/a/a.go:3:5"),
			enclErr: errors.New("quarry down"),
			wantErr: true,
		},
		{name: "a nil plan is an error", log: "x", noPlan: true, wantErr: true},
		{name: "a nil state is an error", log: "x", noState: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := Input{
				Log:       tt.log,
				Head:      traceHead,
				Placement: testPlacement(allFiles...),
				Plan:      tracePlan(),
				State:     traceState(),
				Enclose: func(rev string, targets []string) ([]quarry.EncloseResult, error) {
					if tt.enclErr != nil {
						return nil, tt.enclErr
					}
					if rev != traceHead {
						t.Errorf("Enclose rev = %q; want %q", rev, traceHead)
					}
					results := make([]quarry.EncloseResult, len(targets))
					for i, target := range targets {
						results[i] = quarry.EncloseResult{Target: target}
						if answer, ok := tt.enclose[target]; ok {
							results[i] = answer
						}
					}
					return results, nil
				},
				Resolve: func(targets []string) ([]quarry.ResolveResult, error) {
					results := make([]quarry.ResolveResult, len(targets))
					for i, target := range targets {
						results[i] = quarry.ResolveResult{Target: target, Status: quarry.StatusNotFound}
						if file, ok := tt.resolved[target]; ok {
							results[i] = quarry.ResolveResult{Target: target, Status: quarry.StatusFound, Symbols: []quarry.Symbol{{ID: target, File: file}}}
						}
					}
					return results, nil
				},
				Delta: func(from, to string) (quarry.GitDeltaAnswer, error) {
					if to != traceHead {
						t.Errorf("Delta to = %q; want %q", to, traceHead)
					}
					answer := quarry.GitDeltaAnswer{From: from}
					if symbol, ok := tt.deleted[from]; ok {
						answer.Deleted = []quarry.Symbol{symbol}
					}
					return answer, nil
				},
			}
			if tt.noPlan {
				in.Plan = nil
			}
			if tt.noState {
				in.State = nil
			}

			got, err := Trace(in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Trace() error = %v; want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Trace() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestNewTracer(t *testing.T) {
	t.Parallel()

	const overview = "---\nformat: 5\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — only — a card\n"
	const card = "# Card 1 — only\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n"
	tests := []struct {
		name      string
		withPlan  bool
		wantError string
	}{
		{name: "a missing plan directory is an error", withPlan: false, wantError: "failuretrace: read the plan:"},
		{name: "a plan with no webster state is an error", withPlan: true, wantError: "failuretrace: no webster state to trace against"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			if tt.withPlan {
				planDir := planparser.PlanDir(anchor)
				if err := os.MkdirAll(planDir, 0o755); err != nil {
					t.Fatalf("create plan dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
					t.Fatalf("write overview fixture: %v", err)
				}
				if err := os.WriteFile(filepath.Join(planDir, "01-only.md"), []byte(card), 0o644); err != nil {
					t.Fatalf("write card fixture: %v", err)
				}
			}

			_, err := NewTracer(anchor, anchor)("x", traceHead)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("NewTracer()(log, head) error = %v; want one containing %q", err, tt.wantError)
			}
		})
	}
}
