// entries_gate.go implements resolveGateSpec, the shared resolver every gated entry (DiscussionWrite,
// PlanWrite, BurlerRound) calls to turn its row's "gate"/"gate_attempts" Config keys into a
// shuttleengine.GateSpec.
//
// The selector is a declared string resolved against Env, exactly as bouncerEntry already resolves
// "commit_seam"/"approve_seam" against Env.CommitPlan/Env.CommitDiscussion/Env.ApprovePlan.
// Selecting the validator by switching on the row's own Name was rejected: that would make row
// names load-bearing in a second place beyond resume identity, where a renamed row would silently
// lose its gate rather than failing to build.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// resolveGateSpec is the single place the "gate" and "gate_attempts" Config keys are read and
// turned into a shuttleengine.GateSpec, shared by every gated entry.
//
// "gate" is an optional string read via configString, resolved against a closed two-value
// vocabulary: "discussion" requires env.DecisionRecordPath and env.SupportLogPath to pass
// requireAbsRoot and returns loomshed.NewDiscussionGate over them; "plan" requires
// env.AnchorPath and env.WorktreeRoot and returns loomshed.NewPlanGate over them; any other
// non-empty value is an error naming the key and both legal values. An absent "gate" returns the
// zero GateSpec, which is what every ungated row carries by saying nothing.
//
// "gate_attempts" is an optional int read via configInt. A "gate_attempts" present with no "gate"
// is an error naming both keys, never a silently-ignored key: it is unambiguously an author
// mistake, and this package's constructors already fail loud on malformed config rather than
// defaulting.
//
// Every error is qualified with entry so a recipe author with a typo is told which row spoke,
// matching the qualification resolveUnderRoot already applies.
//
// resolveGateSpec reads the "gate" and "gate_attempts" keys but never rejects an unknown key --
// each caller keeps owning its own configRejectUnknown call, which is where the Config Strictness
// Invariant's strictness lives.
func resolveGateSpec(entry string, cfg Config, env Env) (shuttleengine.GateSpec, error) {
	gate, err := configString(cfg, "gate", false)
	if err != nil {
		return shuttleengine.GateSpec{}, err
	}
	attempts, err := configInt(cfg, "gate_attempts", false)
	if err != nil {
		return shuttleengine.GateSpec{}, err
	}

	if gate == "" {
		if _, present := cfg["gate_attempts"]; present {
			return shuttleengine.GateSpec{}, fmt.Errorf("shedrecipe: %s: config key %q requires config key %q", entry, "gate_attempts", "gate")
		}
		return shuttleengine.GateSpec{}, nil
	}

	var closure shuttleengine.Gate
	switch gate {
	case "discussion":
		if err := requireAbsRoot(entry, "DecisionRecordPath", env.DecisionRecordPath); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		if err := requireAbsRoot(entry, "SupportLogPath", env.SupportLogPath); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		closure = loomshed.NewDiscussionGate(env.DecisionRecordPath, env.SupportLogPath)
	case "plan":
		if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		closure = loomshed.NewPlanGate(env.AnchorPath, env.WorktreeRoot)
	default:
		return shuttleengine.GateSpec{}, fmt.Errorf("shedrecipe: %s: config key %q must be %q or %q, got %q", entry, "gate", "discussion", "plan", gate)
	}

	return shuttleengine.GateSpec{Gate: closure, Attempts: attempts}, nil
}
