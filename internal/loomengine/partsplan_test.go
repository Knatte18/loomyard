// partsplan_test.go — untagged unit tests for PartsPlanTable.
// Pure Go over an in-memory Config, a written parts file, a temp-dir modelspec registry and a seeded stencils directory;
// every seat's prompt is composed through seatengine.Prompts, which starts nothing.

package loomengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// twoPartsFile is a parts file of two parts, alpha then beta.
const twoPartsFile = `# Parts: Demo

## Split note

The task splits along its two modules.

## Part: alpha

- Decisions: D1
- Takes in: the decision record
- Delivers: the alpha behaviour
- Exports:
  - ` + "`plan:internal/a#NewA`" + `
- Area:
  - ` + "`internal/a`" + `

## Part: beta

- Decisions: D2
- Takes in: alpha's export
- Delivers: the beta behaviour
- Depends on: alpha
- Area:
  - ` + "`internal/b`" + `
`

// partsPlanConfig returns a config whose plan and parts keys are set for PartsPlanTable.
func partsPlanConfig(planTimeoutMin int, chairTimeoutMin *int) Config {
	return Config{
		PlanTimeoutMin:       planTimeoutMin,
		PartWriter:           "sonnet[low]",
		PartsChair:           "opus[high]",
		PartsChairTimeoutMin: chairTimeoutMin,
	}
}

// writePartsFile writes content as the parts file of layout.
func writePartsFile(t *testing.T, layout *lymcwd.Location, content string) {
	t.Helper()
	path := partsparser.File(layout.AnchorPath())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// partsPlanHooks are the told per-part closures of PartsPlanTable, recording the parts they were built for and the parts whose Accept ran.
type partsPlanHooks struct {
	gated, accepts, accepted []string
}

func (h *partsPlanHooks) gates(part string) (shuttleengine.GateSpec, error) {
	h.gated = append(h.gated, part)
	return shuttleengine.GateSpec{{Name: "gate-" + part}}, nil
}

func (h *partsPlanHooks) accept(part string) func(shuttleengine.Result) error {
	h.accepts = append(h.accepts, part)
	return func(shuttleengine.Result) error {
		h.accepted = append(h.accepted, part)
		return nil
	}
}

func (h *partsPlanHooks) findingsPath(part string) string {
	return "/findings/" + part + ".md"
}

// buildPartsPlanTable builds the table of layout over the hooks and the config.
func buildPartsPlanTable(t *testing.T, layout *lymcwd.Location, stencilsDir string, cfg Config, runTimeout time.Duration, hooks *partsPlanHooks) (seatengine.Table, error) {
	t.Helper()
	return PartsPlanTable(layout, stencilsDir, newTestSpecsDir(t), cfg, discussionTableRegistry(t), "none", runTimeout, hooks.gates, hooks.accept, hooks.findingsPath)
}

// TestPartsPlanTable verifies PartsPlanTable builds the chair and one writer seat per part of the parts file, their timeouts, models and composed prompts, and refuses an absent or malformed parts file with its way forward.
//
//testtiming:keep pins every seat field and the timeout resolution, which no producer-level test asserts
func TestPartsPlanTable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("one writer seat per part in file order, and the chair over every part overview", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "backend")
		writePartsFile(t, layout, twoPartsFile)
		stencilsDir := newTestStencilsDir(t)
		hooks := &partsPlanHooks{}

		table, err := buildPartsPlanTable(t, layout, stencilsDir, partsPlanConfig(120, nil), 0, hooks)
		if err != nil {
			t.Fatalf("PartsPlanTable(...) = _, %v; want nil error", err)
		}
		if err := table.Validate(stencilsDir, ""); err != nil {
			t.Fatalf("table.Validate() = %v; want nil", err)
		}
		anchor := layout.AnchorPath()
		if table.RolePrefix != PartsPlanRole || table.Segment != segmentcolor.Plan || table.Interactive {
			t.Errorf("table role/segment/interactive = %q/%q/%v; want %s/%q/false", table.RolePrefix, table.Segment, table.Interactive, PartsPlanRole, segmentcolor.Plan)
		}
		if len(table.Seats) != 3 {
			t.Fatalf("table has %d seats; want the chair and two writers", len(table.Seats))
		}

		chair := table.Chair()
		wantChairInputs := []string{DiscussionDecisionRecord(layout), partsparser.File(anchor), partsparser.PartOverview(anchor, "alpha"), partsparser.PartOverview(anchor, "beta")}
		if !slices.Equal(chair.Inputs, wantChairInputs) {
			t.Errorf("chair inputs = %v; want %v", chair.Inputs, wantChairInputs)
		}
		if want := []string{partsparser.JoinRecord(anchor)}; !slices.Equal(chair.Outputs, want) {
			t.Errorf("chair outputs = %v; want the join record %v", chair.Outputs, want)
		}
		if chair.Model != "opus" || chair.Effort != "high" || chair.ModelKey != (shuttleengine.ModelKey{Module: "tapestry", Key: "parts_chair"}) {
			t.Errorf("chair model/effort/key = %q/%q/%+v; want opus/high/tapestry parts_chair", chair.Model, chair.Effort, chair.ModelKey)
		}
		if chair.Writer || len(chair.Gate) != 0 {
			t.Errorf("chair writer/gate = %v/%v; want a plain lead with no gate", chair.Writer, chair.Gate)
		}

		for i, part := range []string{"alpha", "beta"} {
			seat := table.Seats[i+1]
			if seat.Name != part || !seat.Writer || seat.Stencil != "loom-template-part-writer" {
				t.Errorf("seat %d name/writer/stencil = %q/%v/%q; want %q/true/loom-template-part-writer", i+1, seat.Name, seat.Writer, seat.Stencil, part)
			}
			if want := []string{partsparser.PartOverview(anchor, part)}; !slices.Equal(seat.Outputs, want) {
				t.Errorf("seat %q outputs = %v; want %v", part, seat.Outputs, want)
			}
			if want := []string{DiscussionDecisionRecord(layout), DiscussionRelevant(layout), partsparser.File(anchor)}; !slices.Equal(seat.Inputs, want) {
				t.Errorf("seat %q inputs = %v; want %v", part, seat.Inputs, want)
			}
			if len(seat.Gate) != 1 || seat.Gate[0].Name != "gate-"+part {
				t.Errorf("seat %q gate = %v; want the entry gates(%q) built", part, seat.Gate, part)
			}
			if want := hooks.findingsPath(part); seat.GateFindingsPath != want {
				t.Errorf("seat %q GateFindingsPath = %q; want %q", part, seat.GateFindingsPath, want)
			}
			if seat.Model != "sonnet" || seat.Effort != "low" || seat.ModelKey != (shuttleengine.ModelKey{Module: "tapestry", Key: "part_writer"}) {
				t.Errorf("seat %q model/effort/key = %q/%q/%+v; want sonnet/low/tapestry part_writer", part, seat.Model, seat.Effort, seat.ModelKey)
			}
			if seat.ForkSubagents {
				t.Errorf("seat %q ForkSubagents = true; want false", part)
			}
			if err := seat.Accept(shuttleengine.Result{}); err != nil {
				t.Errorf("seat %q Accept() = %v; want the told accept's nil", part, err)
			}
		}
		if want := []string{"alpha", "beta"}; !slices.Equal(hooks.gated, want) || !slices.Equal(hooks.accepts, want) || !slices.Equal(hooks.accepted, want) {
			t.Errorf("gates/accept builds/Accept calls = %v/%v/%v; want %v for each, in file order", hooks.gated, hooks.accepts, hooks.accepted, want)
		}

		prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
		if err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
		}
		for _, part := range []string{"alpha", "beta"} {
			prompt := prompts[part]
			for _, want := range []string{"`" + part + "`", partsparser.PartDir(anchor, part), partsparser.PartOverview(anchor, part)} {
				if !strings.Contains(prompt, want) {
					t.Errorf("writer %q prompt lacks %q", part, want)
				}
			}
		}
		chairPrompt := prompts[""]
		for _, want := range []string{planparser.PlanDir(anchor), partsparser.JoinRecord(anchor), partsparser.Dir(anchor)} {
			if !strings.Contains(chairPrompt, want) {
				t.Errorf("chair prompt lacks %q", want)
			}
		}
		for name, prompt := range prompts {
			if strings.Contains(prompt, "{{") {
				t.Errorf("seat %q prompt carries an unfilled marker", name)
			}
		}
	})

	t.Run("the timeouts resolve against the effective plan timeout", func(t *testing.T) {
		t.Parallel()
		minutes := func(n int) *int { return &n }
		tests := []struct {
			name            string
			planTimeoutMin  int
			chairTimeoutMin *int
			runTimeout      time.Duration
			wantWriters     time.Duration
			wantChair       time.Duration
			wantErr         bool
		}{
			{"unset chair takes twice plan_timeout_min", 120, nil, time.Hour, 120 * time.Minute, 240 * time.Minute, false},
			{"unset chair with plan_timeout_min 0 takes twice the run timeout", 0, nil, 90 * time.Minute, 90 * time.Minute, 180 * time.Minute, false},
			{"a set chair timeout above the plan timeout is kept", 120, minutes(300), time.Hour, 120 * time.Minute, 300 * time.Minute, false},
			{"a set chair timeout not above the run timeout with plan_timeout_min 0 fails", 0, minutes(90), 90 * time.Minute, 0, 0, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, "")
				writePartsFile(t, layout, twoPartsFile)
				table, err := buildPartsPlanTable(t, layout, newTestStencilsDir(t), partsPlanConfig(tt.planTimeoutMin, tt.chairTimeoutMin), tt.runTimeout, &partsPlanHooks{})
				if tt.wantErr {
					if err == nil || !strings.Contains(err.Error(), "parts_chair_timeout_min") {
						t.Errorf("PartsPlanTable(...) = _, %v; want an error naming parts_chair_timeout_min", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("PartsPlanTable(...) = _, %v; want nil error", err)
				}
				if table.Timeout != tt.wantWriters {
					t.Errorf("table timeout = %s; want %s", table.Timeout, tt.wantWriters)
				}
				if got := table.Chair().Timeout; got != tt.wantChair {
					t.Errorf("chair timeout = %s; want %s", got, tt.wantChair)
				}
			})
		}
	})

	t.Run("an absent or malformed parts file is refused with its way forward", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name, content string
			write         bool
			wantForward   string
		}{
			{"absent", "", false, "lym loom goto --to Parts-Write"},
			{"malformed", "not a parts file\n", true, "lym loom validate-parts"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, "")
				if tt.write {
					writePartsFile(t, layout, tt.content)
				}
				_, err := buildPartsPlanTable(t, layout, newTestStencilsDir(t), partsPlanConfig(120, nil), 0, &partsPlanHooks{})
				if err == nil {
					t.Fatal("PartsPlanTable(...) = _, nil; want a refusal")
				}
				for _, want := range []string{"way forward:", tt.wantForward, "lym reed remove --name"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("PartsPlanTable(%s parts file) error = %q; want it to carry %q", tt.name, err, want)
					}
				}
			})
		}
	})
}
