// entries_parts.go implements partsWriteEntry and partsPlanWriteEntry, the Constructors for the "PartsWrite" and "PartsPlanWrite" registry rows.
// It also holds resolvePartGates, which turns the Parts-Plan-Write row's "part_gates" key into the per-part gates of its writer seats.
// Both entries run a told seat table on a MultiLLM producer behind loomshed.NewPlanWrite's commit decorator.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// PartSeats holds the per-part hooks of the Parts-Plan-Write table that only the entry can build, each keyed by a part's name.
// Env.PartsPlanTable receives them so the table builder wires one gate, one Accept and one findings path to each writer seat.
type PartSeats struct {
	// Gates returns the gate of the writer seat for the named part, or an error that fails the table.
	Gates func(part string) (shuttleengine.GateSpec, error)
	// Accept returns the Accept hook of the writer seat for the named part.
	Accept func(part string) func(shuttleengine.Result) error
	// FindingsPath returns the file the named part's exhausted gate keeps its findings in.
	FindingsPath func(part string) string
}

// partsWriteEntry is the Constructor for the "PartsWrite" registry row.
// It validates Env.PartsTable, Env.CommitParts, Env.Seats and Env.AnchorPath, and resolves the row's "gates" Config key through resolveGateSpec.
// It then builds a seat-table producer carrying loomshed.NewPartsDirRotator as its fresh-spawn preparation, behind loomshed.NewPlanWrite's post-Done commit decorator.
// The table arrives as a shedadapters.TableSource closure evaluated once per Call, and the resolved gate is stamped onto each table it returns.
// The row carries exactly one Config key, "gates".
func partsWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsWrite", "PartsTable", env.PartsTable); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsWrite", "CommitParts", env.CommitParts); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsWrite", "Seats", env.Seats); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PartsWrite", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	gate, err := resolveGateSpec("PartsWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	rotate := loomshed.NewPartsDirRotator(env.AnchorPath, env.Now)
	tables := func() (seatengine.Table, error) {
		table, err := env.PartsTable()
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, rotate).KeepGateFindings(env.gateFindingsPath(row.Name))
	return loomshed.NewPlanWrite(row.Name, inner, env.CommitParts), nil
}

// partsPlanWriteEntry is the Constructor for the "PartsPlanWrite" registry row.
// It validates Env.PartsPlanTable, Env.CommitPartsPlan, Env.Seats, Env.PlanIndex, Env.Rework.ArchiveWebster and the absolute Env.AnchorPath, Env.WorktreeRoot and Env.StencilsDir.
// It resolves the row's "gates" key through resolveGateSpec and its "part_gates" key through resolvePartGates.
// It builds a seat-table producer carrying loomshed.NewPartsPlanRotator as its fresh-spawn preparation, behind loomshed.NewPlanWrite's post-Done commit decorator.
// The table source hands Env.PartsPlanTable the per-part hooks and stamps the "gates" gate onto the chair of the table it returns.
// The row carries the Config keys "gates" and "part_gates".
func partsPlanWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, "gates", "part_gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsPlanWrite", "PartsPlanTable", env.PartsPlanTable); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsPlanWrite", "CommitPartsPlan", env.CommitPartsPlan); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsPlanWrite", "Seats", env.Seats); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsPlanWrite", "PlanIndex", env.PlanIndex); err != nil {
		return nil, err
	}
	if err := requireSeam("PartsPlanWrite", "Rework.ArchiveWebster", env.Rework.ArchiveWebster); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PartsPlanWrite", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PartsPlanWrite", "WorktreeRoot", env.WorktreeRoot); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PartsPlanWrite", "StencilsDir", env.StencilsDir); err != nil {
		return nil, err
	}
	gate, err := resolveGateSpec("PartsPlanWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	gates, err := resolvePartGates("PartsPlanWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	rotate := loomshed.NewPartsPlanRotator(env.AnchorPath, env.StencilsDir, env.Rework.ArchiveWebster, env.Now)
	planContext := planContextOf(env)
	seats := PartSeats{
		Gates: gates,
		Accept: func(part string) func(shuttleengine.Result) error {
			return loomshed.NewPartAccept(env.AnchorPath, env.WorktreeRoot, part, env.PlanIndex, planContext)
		},
		FindingsPath: func(part string) string { return env.gateFindingsPath(row.Name + "-" + part) },
	}
	tables := func() (seatengine.Table, error) {
		table, err := env.PartsPlanTable(seats)
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, rotate).KeepGateFindings(env.gateFindingsPath(row.Name))
	return loomshed.NewPlanWrite(row.Name, inner, env.CommitPartsPlan), nil
}

// resolvePartGates reads the row's "part_gates" Config key as resolveGateSpec reads "gates", with the same element keys, and returns the closure that builds a part's gate spec.
// The one legal gate name is "plan-part", which requires the absolute env.AnchorPath and env.WorktreeRoot and a non-nil env.PlanIndex; any other name is refused naming it.
// The list is validated once, here; an absent key returns a closure that gates no part, and a present empty list is an error.
// Each element yields, for a part, one shuttleengine.GateEntry whose gate is loomshed.NewPartPlanGate over that part.
func resolvePartGates(entry string, cfg Config, env Env) (func(part string) (shuttleengine.GateSpec, error), error) {
	elems, present, err := configMapList(cfg, "part_gates")
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: %s: %w", entry, err)
	}
	if !present {
		return func(string) (shuttleengine.GateSpec, error) { return nil, nil }, nil
	}
	if len(elems) == 0 {
		return nil, fmt.Errorf("shedrecipe: %s: config key %q must not be an empty list; way forward: list a %q gate or remove the key", entry, "part_gates", "plan-part")
	}
	type element struct {
		name      string
		attempts  int
		passOnCap bool
	}
	parsed := make([]element, 0, len(elems))
	seen := make(map[string]bool, len(elems))
	for i, elem := range elems {
		wrap := func(err error) error {
			return fmt.Errorf("shedrecipe: %s: config key %q element %d: %w", entry, "part_gates", i, err)
		}
		if err := configRejectUnknown(elem, gateEntryKeys...); err != nil {
			return nil, wrap(err)
		}
		name, err := configString(elem, "name", true)
		if err != nil {
			return nil, wrap(err)
		}
		attempts, err := configInt(elem, "attempts", true)
		if err != nil {
			return nil, wrap(err)
		}
		if attempts < 0 {
			return nil, wrap(fmt.Errorf("config key %q must not be negative, got %d", "attempts", attempts))
		}
		passOnCap, err := configBool(elem, "pass_on_cap", false)
		if err != nil {
			return nil, wrap(err)
		}
		if name != "plan-part" {
			return nil, wrap(fmt.Errorf("config key %q must be %q, got %q; way forward: name the gate %q", "name", "plan-part", name, "plan-part"))
		}
		if seen[name] {
			return nil, wrap(fmt.Errorf("repeats gate name %q", name))
		}
		seen[name] = true
		parsed = append(parsed, element{name: name, attempts: attempts, passOnCap: passOnCap})
	}
	if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
		return nil, err
	}
	if err := requireSeam(entry, "PlanIndex", env.PlanIndex); err != nil {
		return nil, err
	}
	planContext := planContextOf(env)
	return func(part string) (shuttleengine.GateSpec, error) {
		spec := make(shuttleengine.GateSpec, 0, len(parsed))
		for _, p := range parsed {
			gate := loomshed.NewPartPlanGate(env.AnchorPath, env.WorktreeRoot, env.RunScratchDir, part, env.PlanIndex, planContext)
			spec = append(spec, shuttleengine.GateEntry{Name: p.name, Gate: gate, Attempts: p.attempts, PassOnCap: p.passOnCap})
		}
		return spec, nil
	}, nil
}
