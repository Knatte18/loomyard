// partsplan.go builds the seat table of the Parts-Plan-Write row, which writes the plan in parts.
// The table seats one chair that joins the parts into the plan, and one writer seat per part of the parts file.
// It is a pure builder: it reads the parts file at call time and the stencils it composes from,
// and it creates no part directories and spawns nothing.

package loomengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// PartsPlanRole is the agent-name role prefix of the Parts-Plan-Write row's seats.
const PartsPlanRole = "parts-plan"

// partWriterStencil is the stencil that opens every part writer seat.
const partWriterStencil = "loom-template-part-writer"

// partsChairStencil is the stencil that opens the Parts-Plan-Write chair.
const partsChairStencil = "loom-template-parts-chair"

// partsPlanLiveStrandsNote is the remedy both parts-file refusals add.
// The table fails before the resume probe can stop a live strand,
// so the remedy is to remove the task's `parts-plan` strands first.
const partsPlanLiveStrandsNote = "a resumed row whose chair or writers are still live keeps their strands, since the table fails before the probe can stop them, so first remove each `parts-plan` strand of the task with `lym reed remove --name <strand>`"

// PartsPlanTable builds the seatengine.Table of one Parts-Plan-Write run from the parts file under layout:
// a chair as the lead, then one writer seat per part in file order.
// The chair writes the join record `partsparser.JoinRecord`.
// It reads the decision record, the parts file and every part's overview.
// Each writer writes its part's overview and reads the decision record, the relevant list and the parts file.
// runTimeout is shuttle's run_timeout_min as a duration, told by the caller.
// The caller tells it so that a plan_timeout_min of 0 resolves to the effective plan timeout.
// It bounds every writer.
// The chair's seat timeout is cfg.PartsChairTimeout(runTimeout), and an error from it fails the table.
// gates, accept and findingsPath build each writer's gate, Accept and failed-gate findings path from its part's name.
// An error from gates fails the table.
// An absent parts file fails the table with the way forward of running the Parts-Write row again.
// A malformed one fails it with the way forward of `lym loom validate-parts`.
// specsDir and gateTiers are told by the caller and never derived here.
// The table carries no chair gate; the producer entry sets it.
// A PATTERN directive that cannot be rendered fails the table.
// A friction directive that cannot be rendered is left empty, and the failure is logged.
func PartsPlanTable(layout *lymcwd.Location, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, gateTiers string, runTimeout time.Duration, gates func(part string) (shuttleengine.GateSpec, error), accept func(part string) func(shuttleengine.Result) error, findingsPath func(part string) string) (seatengine.Table, error) {
	anchor := layout.AnchorPath()
	parts, err := readPartsFile(anchor)
	if err != nil {
		return seatengine.Table{}, err
	}

	chairChoice, err := resolveModelChoice(cfg.PartsChair, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsPlanTable: parts_chair role model-spec: %w", err)
	}
	writerChoice, err := resolveModelChoice(cfg.PartWriter, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsPlanTable: part_writer role model-spec: %w", err)
	}
	chairTimeout, err := cfg.PartsChairTimeout(runTimeout)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsPlanTable: %w", err)
	}

	declaredChanges, err := discussionparser.DeclaredPatternChanges(DiscussionDecisionRecord(layout))
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsPlanTable: %w re-step the row with %q", err, "lym loom start")
	}
	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer, declaredChanges)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PartsPlanTable: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	seatFriction := func(noteName string) map[string]string {
		directive, err := friction.Directive(friction.NotePath(frictionDir, noteName), stencilsDir, friction.RoleImplementer)
		if err != nil {
			// A friction directive is optional bookkeeping: a stencil read failure must never fail the Parts-Plan-Write spawn.
			logger.Warn("loom: friction directive failed, continuing without one", "role", "parts-plan-write", "note", noteName, "error", err)
			return nil
		}
		if directive == "" {
			return nil
		}
		return map[string]string{friction.MarkerName: directive}
	}

	partsFile := partsparser.File(anchor)
	overviews := make([]string, 0, len(parts.Parts))
	for _, part := range parts.Parts {
		overviews = append(overviews, partsparser.PartOverview(anchor, part.Name))
	}

	chairValues := map[string]string{
		"plan_dir":      planparser.PlanDir(anchor),
		"overview_path": planparser.PlanOverview(anchor),
		"join_path":     partsparser.JoinRecord(anchor),
	}
	for name, value := range seatFriction("Parts-Plan-Write") {
		chairValues[name] = value
	}
	chair := seatengine.Seat{
		Stencil:     partsChairStencil,
		ModelChoice: chairChoice,
		ModelKey:    shuttleengine.ModelKey{Module: tapestryModule, Key: "parts_chair"},
		Inputs:      append([]string{DiscussionDecisionRecord(layout), partsFile}, overviews...),
		Outputs:     []string{partsparser.JoinRecord(anchor)},
		ConfigRole:  agentconfig.RolePlan,
		Values:      chairValues,
		Timeout:     chairTimeout,
	}

	seats := []seatengine.Seat{chair}
	for _, part := range parts.Parts {
		gate, err := gates(part.Name)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: PartsPlanTable: gate of part %q: %w", part.Name, err)
		}
		writerValues := map[string]string{
			"part_name":     part.Name,
			"plan_dir":      partsparser.PartDir(anchor, part.Name),
			"overview_path": partsparser.PartOverview(anchor, part.Name),
		}
		for name, value := range seatFriction("Parts-Plan-Write-" + part.Name) {
			writerValues[name] = value
		}
		seats = append(seats, seatengine.Seat{
			Name:             part.Name,
			Stencil:          partWriterStencil,
			ModelChoice:      writerChoice,
			ModelKey:         shuttleengine.ModelKey{Module: tapestryModule, Key: "part_writer"},
			Inputs:           []string{DiscussionDecisionRecord(layout), DiscussionRelevant(layout), partsFile},
			Outputs:          []string{partsparser.PartOverview(anchor, part.Name)},
			ConfigRole:       agentconfig.RolePlan,
			Values:           writerValues,
			Gate:             gate,
			GateFindingsPath: findingsPath(part.Name),
			Accept:           accept(part.Name),
			Writer:           true,
		})
	}

	return seatengine.Table{
		RolePrefix: PartsPlanRole,
		Segment:    segmentcolor.Plan,
		Timeout:    cfg.PlanTimeout(runTimeout),
		Values: map[string]string{
			"decision_record_path": DiscussionDecisionRecord(layout),
			"relevant_path":        DiscussionRelevant(layout),
			"parts_path":           partsFile,
			"parts_dir":            partsparser.Dir(anchor),
			"specs_dir":            specsDir,
			"gate_tiers":           gateTiers,
			"pattern_directive":    patternDirective,
			friction.MarkerName:    "",
		},
		Optional: []string{"pattern_directive", friction.MarkerName},
		Seats:    seats,
	}, nil
}

// readPartsFile reads and parses the parts file of the worktree anchored at anchor.
// An absent file and a malformed one each return an error ending in a way forward.
func readPartsFile(anchor string) (*partsparser.Parts, error) {
	path := partsparser.File(anchor)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("loom: PartsPlanTable: the parts file %s is absent; way forward: run the Parts-Write row with `lym loom goto --to Parts-Write`; %s", path, partsPlanLiveStrandsNote)
		}
		return nil, fmt.Errorf("loom: PartsPlanTable: read the parts file %s: %w", path, err)
	}
	parts, err := partsparser.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("loom: PartsPlanTable: the parts file %s is malformed: %w; way forward: run `lym loom validate-parts`, fix what it reports in the parts file, and re-step the row; %s", path, err, partsPlanLiveStrandsNote)
	}
	return parts, nil
}
