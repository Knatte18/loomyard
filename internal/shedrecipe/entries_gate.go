// entries_gate.go implements resolveGateSpec, which turns a row's "gates" Config key into a shuttleengine.GateSpec.
//
// Each element selects its validator by a declared name resolved against Env, not by the row's Name.
// Keying the validator on the row's Name would make a rename silently drop its gate instead of failing to build.

package shedrecipe

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateEntryKeys are the keys one element of a row's "gates" list may carry.
var gateEntryKeys = []string{"name", "attempts", "pass_on_cap"}

// resolveGateSpec is the single place the "gates" Config key is read and turned into a shuttleengine.GateSpec.
// Every gated entry resolves its gates here.
//
// "gates" is an optional list of maps.
// An absent key returns a nil GateSpec, which is what every ungated row carries by saying nothing.
// A present empty list is an error, since it is an author mistake rather than an ungated row.
//
// Each element carries a required "name", resolved against a closed vocabulary:
// "clean" requires env.WorktreeRoot to pass requireAbsRoot and returns loomshed.NewCleanGate over it;
// "discussion" requires env.DecisionRecordPath and env.SupportLogPath to pass requireAbsRoot;
// it also requires a non-nil env.DiscussionHistory, and returns loomshed.NewDiscussionGate over them;
// when env.RecordEngine is DiscussionWrite, "discussion" also requires env.RelevantPath to pass requireAbsRoot;
// it also requires env.WorktreeRoot to pass requireAbsRoot and a non-nil env.PlanIndex;
// it then passes env.RelevantPath and env.WorktreeRoot to the gate, with the citation checks on;
// the citation checks read the worktree and the index;
// "plan" requires env.AnchorPath, env.WorktreeRoot and a non-nil env.PlanIndex;
// it returns loomshed.NewPlanGate over them;
// "parts" requires env.AnchorPath, env.WorktreeRoot, env.DecisionRecordPath and a non-nil env.Setting;
// it returns loomshed.NewPartsGate over them, capped by the tapestry.parts_max setting;
// "raddle" requires the absolute env.RaddleWrite.Check.ProposalPath and env.RaddleWrite.Check.RecordPath;
// it also requires the non-nil env.RaddleWrite.Check.Board and env.RaddleWrite.Deps.StoreCold;
// it returns loomshed.NewRaddleGate over them;
// "description" requires env.DescriptionPath to pass requireAbsRoot;
// it returns landingshed.NewDescriptionGate over it;
// "verify" requires the absolute env.WorktreeRoot and env.VerifyDir and a non-nil env.VerifyCommand;
// it takes the optional env.VerifyMergeBase and env.VerifyTreeScope, and returns loomshed.NewVerifyGate over them;
// the site label is "<row name> gate";
// the gate's told command is env.VerifyCommand read for the row's gate key;
// its tree scope is env.VerifyTreeScope read for that key, so the row runs its own key of the hub's gate.yaml;
// env.VerifyTreeScope alone selects the darn form;
// it requires env.VerifyMergeBase, the base of both lints in that form;
// otherwise env.VerifyMergeBase is the base of the headroom lint;
// "drift" requires the env.Drift seams Config, Status, Head, CaptureCurrent, TrailerCommit, FileAt and Compare;
// it returns driftengine.NewDriftGate over them;
// "expectation" requires the env.Drift seams Config, TrailerCommit and FileAt;
// it returns driftengine.NewExpectationGate over them;
// "judgement" returns driftengine.NewJudgementGate over env.Drift, which reads only its Paths and requires no seam;
// any other value is an error naming the key and every legal value.
// A name may appear once per list.
//
// "attempts" is a required non-negative integer, and 0 is a present value.
// 0 turns the entry off, yet the entry still resolves its closure and its Env requirements,
// so a typo in an off gate fails loud.
// "pass_on_cap" is an optional bool on every element, false when absent.
//
// Every error names entry, so a recipe author with a typo is told which row spoke.
//
// resolveGateSpec reads only the "gates" key and rejects unknown keys only inside each element.
// Each caller rejects the unknown keys of its own row with its own check.
func resolveGateSpec(entry string, cfg Config, env Env) (shuttleengine.GateSpec, error) {
	return resolveRowGateSpec(entry, entry, cfg, env)
}

// resolveRowGateSpec turns a row's "gates" Config key into a shuttleengine.GateSpec.
// It takes the row's name as well as its entry.
// The site label of the "verify" gate is "<row> gate".
// The "verify" gate runs the gate key the row's optional "gate_key" string names, else the row name in lower case;
// "gate_key" selects only which key of the hub's gate.yaml the row runs,
// and the key's tiers and scope stay hub-file only.
// Errors stay qualified with entry, so a row rename never changes what an author is told.
func resolveRowGateSpec(entry, row string, cfg Config, env Env) (shuttleengine.GateSpec, error) {
	elems, present, err := configMapList(cfg, "gates")
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: %s: %w", entry, err)
	}
	if !present {
		return nil, nil
	}
	if len(elems) == 0 {
		return nil, fmt.Errorf("shedrecipe: %s: config key %q must not be an empty list", entry, "gates")
	}
	gateKey := strings.ToLower(row)
	if raw, ok := cfg["gate_key"]; ok {
		named, isString := raw.(string)
		if !isString {
			return nil, fmt.Errorf("shedrecipe: %s: row %q config key %q must be a string, got %T; way forward: set %q to the name of a gate.yaml key, a string, or remove it to run the key named for the row", entry, row, "gate_key", raw, "gate_key")
		}
		if named != "" {
			gateKey = named
		}
	}

	spec := make(shuttleengine.GateSpec, 0, len(elems))
	seen := make(map[string]bool, len(elems))
	for i, elem := range elems {
		ge, err := resolveGateEntry(entry, row, gateKey, i, elem, env)
		if err != nil {
			return nil, err
		}
		if seen[ge.Name] {
			return nil, fmt.Errorf("shedrecipe: %s: config key %q element %d repeats gate name %q", entry, "gates", i, ge.Name)
		}
		seen[ge.Name] = true
		spec = append(spec, ge)
	}
	return spec, nil
}

// resolveGateEntry resolves one element of a row's "gates" list.
// It wraps every config-accessor error with the entry and the element's index, since those accessors name neither.
func resolveGateEntry(entry, row, gateKey string, index int, elem Config, env Env) (shuttleengine.GateEntry, error) {
	wrap := func(err error) error {
		return fmt.Errorf("shedrecipe: %s: config key %q element %d: %w", entry, "gates", index, err)
	}

	if err := configRejectUnknown(elem, gateEntryKeys...); err != nil {
		return shuttleengine.GateEntry{}, wrap(err)
	}
	name, err := configString(elem, "name", true)
	if err != nil {
		return shuttleengine.GateEntry{}, wrap(err)
	}
	attempts, err := configInt(elem, "attempts", true)
	if err != nil {
		return shuttleengine.GateEntry{}, wrap(err)
	}
	if attempts < 0 {
		return shuttleengine.GateEntry{}, wrap(fmt.Errorf("config key %q must not be negative, got %d", "attempts", attempts))
	}
	passOnCap, err := configBool(elem, "pass_on_cap", false)
	if err != nil {
		return shuttleengine.GateEntry{}, wrap(err)
	}
	closure, err := resolvePlainGateClosure(entry, row, gateKey, index, name, env)
	if err != nil {
		return shuttleengine.GateEntry{}, err
	}
	return shuttleengine.GateEntry{Name: name, Gate: closure, Attempts: attempts, PassOnCap: passOnCap}, nil
}

// planContextOf builds the PlanContext that the plan gates check a plan against.
// It reads the hub's rename tools, refused raw commands and tier tags from env.
// It sets the seam-test reader over the worktree, and the decision record on a tapestry run.
// A darn run has no PatternRecordPath, so its plan is not compared with a record.
// The PATTERN.md base is read at the merge base the verify gate's headroom lint is told.
func planContextOf(env Env) loomshed.PlanContext {
	return loomshed.PlanContext{
		RenameTools:        env.RenameTools,
		RefuseRaw:          env.RefuseRaw,
		TierTags:           env.TierTags,
		SeamTests:          loomshed.SeamTestReader(env.WorktreeRoot),
		DecisionRecordPath: env.PatternRecordPath,
		PatternBase:        loomshed.PatternBaseReader(env.WorktreeRoot, env.VerifyMergeBase),
	}
}

// resolvePlainGateClosure resolves the validator names of the gate vocabulary.
// The "parts" gate requires the absolute env.AnchorPath and env.DecisionRecordPath and a non-nil env.Setting.
// It reads its cap from the tapestry.parts_max setting.
// The "verify" gate is told env.VerifyHold, which holds its verify under a pause.
func resolvePlainGateClosure(entry, row, gateKey string, index int, name string, env Env) (shuttleengine.Gate, error) {
	switch name {
	case "clean":
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return nil, err
		}
		return loomshed.NewCleanGate(env.WorktreeRoot), nil
	case "discussion":
		if err := requireAbsRoot(entry, "DecisionRecordPath", env.DecisionRecordPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "SupportLogPath", env.SupportLogPath); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "DiscussionHistory", env.DiscussionHistory); err != nil {
			return nil, err
		}
		check := loomshed.DiscussionCheck{
			RecordPath:     env.DecisionRecordPath,
			SupportLogPath: env.SupportLogPath,
			History:        env.DiscussionHistory,
		}
		if env.RecordEngine == "DiscussionWrite" {
			if err := requireAbsRoot(entry, "RelevantPath", env.RelevantPath); err != nil {
				return nil, err
			}
			if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
				return nil, err
			}
			if err := requireSeam(entry, "PlanIndex", env.PlanIndex); err != nil {
				return nil, err
			}
			check.RelevantPath = env.RelevantPath
			check.WorktreeRoot = env.WorktreeRoot
			check.Index = env.PlanIndex
			check.Citations = true
		}
		return loomshed.NewDiscussionGate(check), nil
	case "plan":
		if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "PlanIndex", env.PlanIndex); err != nil {
			return nil, err
		}
		return loomshed.NewPlanGate(env.AnchorPath, env.WorktreeRoot, env.RunScratchDir, env.PlanIndex, planContextOf(env)), nil
	case "parts":
		if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "DecisionRecordPath", env.DecisionRecordPath); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "Setting", env.Setting); err != nil {
			return nil, err
		}
		value, err := env.Setting("tapestry", "parts_max")
		if err != nil {
			return nil, fmt.Errorf("shedrecipe: %s: config key %q element %d: setting tapestry.parts_max: %w", entry, "gates", index, err)
		}
		max, ok := value.(int)
		if !ok || max < 1 {
			return nil, fmt.Errorf("shedrecipe: %s: config key %q element %d: setting tapestry.parts_max holds %v, want an integer of 1 or more; way forward: set tapestry.parts_max in tapestry.yaml to the most parts a parts file may hold", entry, "gates", index, value)
		}
		return loomshed.NewPartsGate(env.AnchorPath, env.WorktreeRoot, env.DecisionRecordPath, max), nil
	case "raddle":
		if err := requireAbsRoot(entry, "RaddleWrite.Check.ProposalPath", env.RaddleWrite.Check.ProposalPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "RaddleWrite.Check.RecordPath", env.RaddleWrite.Check.RecordPath); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "RaddleWrite.Check.Board", env.RaddleWrite.Check.Board); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "RaddleWrite.Deps.StoreCold", env.RaddleWrite.Deps.StoreCold); err != nil {
			return nil, err
		}
		return loomshed.NewRaddleGate(env.RaddleWrite.Check, env.RaddleWrite.Deps.StoreCold), nil
	case "description":
		if err := requireAbsRoot(entry, "DescriptionPath", env.DescriptionPath); err != nil {
			return nil, err
		}
		return landingshed.NewDescriptionGate(env.DescriptionPath), nil
	case "verify":
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "VerifyDir", env.VerifyDir); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "VerifyCommand", env.VerifyCommand); err != nil {
			return nil, err
		}
		command := func() (string, error) { return env.VerifyCommand(gateKey) }
		var treeScope func() (string, bool, error)
		if env.VerifyTreeScope != nil {
			treeScope = func() (string, bool, error) { return env.VerifyTreeScope(gateKey) }
		}
		return loomshed.NewVerifyGate(env.WorktreeRoot, env.VerifyDir, row+" gate", command, env.VerifyMergeBase, treeScope, env.VerifyHold), nil
	case "drift":
		d := env.Drift
		err := requireDriftSeams(entry, driftSeams{
			{"Drift.Config", d.Config},
			{"Drift.Status", d.Status},
			{"Drift.Head", d.Head},
			{"Drift.CaptureCurrent", d.CaptureCurrent},
			{"Drift.TrailerCommit", d.TrailerCommit},
			{"Drift.FileAt", d.FileAt},
			{"Drift.Compare", d.Compare},
		})
		if err != nil {
			return nil, err
		}
		return driftengine.NewDriftGate(driftDepsFrom(env)), nil
	case "expectation":
		d := env.Drift
		err := requireDriftSeams(entry, driftSeams{
			{"Drift.Config", d.Config},
			{"Drift.TrailerCommit", d.TrailerCommit},
			{"Drift.FileAt", d.FileAt},
		})
		if err != nil {
			return nil, err
		}
		return driftengine.NewExpectationGate(driftDepsFrom(env)), nil
	case "judgement":
		return driftengine.NewJudgementGate(driftDepsFrom(env)), nil
	default:
		return nil, fmt.Errorf("shedrecipe: %s: config key %q element %d: config key %q must be %q, %q, %q, %q, %q, %q, %q, %q, %q or %q, got %q", entry, "gates", index, "name", "clean", "discussion", "plan", "parts", "raddle", "description", "verify", "drift", "expectation", "judgement", name)
	}
}
