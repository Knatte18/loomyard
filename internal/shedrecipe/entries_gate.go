// entries_gate.go implements resolveGateSpec, the shared resolver every gate-capable entry calls to turn its row's "gates" Config key into a shuttleengine.GateSpec.
//
// Each element selects a validator by a declared name resolved against Env, exactly as bouncerEntry already resolves "commit_seam"/"approve_seam" against Env.CommitPlan/Env.CommitDiscussion/Env.ApprovePlan.
// Selecting the validator by switching on the row's own Name was rejected: that would make row
// names load-bearing in a second place beyond resume identity, where a renamed row would silently
// lose its gate rather than failing to build.

package shedrecipe

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateEntryKeys are the keys one element of a row's "gates" list may carry.
var gateEntryKeys = []string{"name", "attempts", "pass_on_cap"}

// resolveGateSpec is the single place the "gates" Config key is read and turned into a shuttleengine.GateSpec, shared by every gated entry.
//
// "gates" is an optional list of maps, read via configMapList.
// An absent key returns the empty GateSpec, which is what every ungated row carries by saying nothing;
// a present empty list is an error, since it is an author mistake rather than an ungated row.
//
// Each element carries a required "name", resolved against a closed four-value vocabulary:
// "discussion" requires env.DecisionRecordPath and env.SupportLogPath to pass requireAbsRoot and returns loomshed.NewDiscussionGate over them, or, when env.DiscussionHistory is set, loomshed.NewDarnDiscussionGate over them, the history seam and a non-nil env.CommitDiscussion;
// on a loom run, where env.DiscussionHistory is unset, "discussion" also requires env.RelevantPath to pass requireAbsRoot and passes it to the gate;
// "plan" requires env.AnchorPath, env.WorktreeRoot and a non-nil env.PlanIndex and returns loomshed.NewPlanGate over them;
// "description" requires env.DescriptionPath to pass requireAbsRoot and returns landingshed.NewDescriptionGate over it;
// "verify" requires the absolute env.WorktreeRoot and env.VerifyDir and a non-nil env.VerifyCommand, takes the optional env.VerifyMergeBase and env.VerifyTreeScope, and returns loomshed.NewVerifyGate over them, with the site label "<row name> gate";
// the gate's told command is env.VerifyCommand read for the row's gate key, and its tree scope is env.VerifyTreeScope read for that key, so the row runs its own key of the hub's gate.yaml;
// env.VerifyTreeScope alone selects the darn form, which requires env.VerifyMergeBase, the base of both lints in that form, and otherwise env.VerifyMergeBase is the base of the headroom lint;
// any other value is an error naming the key and all four legal values.
// A name may appear once per list.
//
// "attempts" is required, a non-negative integer read via configInt so 0 is a present value:
// 0 turns the entry off, yet the entry still resolves its closure and its Env requirements, so a typo in an off gate fails loud.
// "pass_on_cap" is an optional bool on every element, false when absent.
//
// Every error is qualified with entry so a recipe author with a typo is told which row spoke, matching the qualification resolveUnderRoot already applies.
//
// resolveGateSpec reads only the "gates" key and rejects unknown keys only inside each element -- each caller keeps owning its own configRejectUnknown call for the row's own keys, which is where the Config Strictness Invariant's strictness lives.
func resolveGateSpec(entry string, cfg Config, env Env) (shuttleengine.GateSpec, error) {
	return resolveRowGateSpec(entry, entry, cfg, env)
}

// resolveRowGateSpec is resolveGateSpec for a caller that also knows its row's name, which the "verify" gate renders into its site label as "<row> gate".
// The "verify" gate runs the gate key the row's optional "gate_key" string names, else the row name in lower case;
// "gate_key" selects only which key of the hub's gate.yaml the row runs, and the key's tiers and scope stay hub-file only.
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

// resolveGateEntry resolves one element of a row's "gates" list, wrapping every config-accessor error with the entry and the element's index because those accessors name neither.
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

// planContextOf builds the plan gates' PlanContext from env: the hub's rename tools and refused raw commands, and the decision record on a loom run.
// A darn run has no PatternRecordPath, so its plan is not compared with a record.
// The PATTERN.md base is read at the merge base the verify gate's headroom lint is told.
func planContextOf(env Env) loomshed.PlanContext {
	return loomshed.PlanContext{
		RenameTools:        env.RenameTools,
		RefuseRaw:          env.RefuseRaw,
		DecisionRecordPath: env.PatternRecordPath,
		PatternBase:        loomshed.PatternBaseReader(env.WorktreeRoot, env.VerifyMergeBase),
	}
}

// resolvePlainGateClosure resolves the four validator names of the gate vocabulary.
// The "verify" gate is told env.VerifyHold, which holds its verify under a pause.
func resolvePlainGateClosure(entry, row, gateKey string, index int, name string, env Env) (shuttleengine.Gate, error) {
	switch name {
	case "discussion":
		if err := requireAbsRoot(entry, "DecisionRecordPath", env.DecisionRecordPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "SupportLogPath", env.SupportLogPath); err != nil {
			return nil, err
		}
		if env.DiscussionHistory != nil {
			if err := requireSeam(entry, "CommitDiscussion", env.CommitDiscussion); err != nil {
				return nil, err
			}
			return loomshed.NewDarnDiscussionGate(env.DecisionRecordPath, env.SupportLogPath, env.DiscussionHistory, env.CommitDiscussion), nil
		}
		if err := requireAbsRoot(entry, "RelevantPath", env.RelevantPath); err != nil {
			return nil, err
		}
		return loomshed.NewDiscussionGate(env.DecisionRecordPath, env.SupportLogPath, env.RelevantPath), nil
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
	default:
		return nil, fmt.Errorf("shedrecipe: %s: config key %q element %d: config key %q must be %q, %q, %q or %q, got %q", entry, "gates", index, "name", "discussion", "plan", "description", "verify", name)
	}
}
