// entries_gate.go implements resolveGateSpec, the shared resolver every gate-capable entry
// (DiscussionWrite, PlanWrite, Describe, BurlerRound, Webster, PRRework) calls to turn its row's
// "gate"/"gate_attempts" Config keys into a shuttleengine.GateSpec.
//
// The selector is a declared string resolved against Env, exactly as bouncerEntry already resolves
// "commit_seam"/"approve_seam" against Env.CommitPlan/Env.CommitDiscussion/Env.ApprovePlan.
// Selecting the validator by switching on the row's own Name was rejected: that would make row
// names load-bearing in a second place beyond resume identity, where a renamed row would silently
// lose its gate rather than failing to build.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// resolveGateSpec is the single place the "gate" and "gate_attempts" Config keys are read and
// turned into a shuttleengine.GateSpec, shared by every gated entry.
//
// "gate" is an optional string read via configString, resolved against a closed four-value
// vocabulary: "discussion" requires env.DecisionRecordPath and env.SupportLogPath to pass
// requireAbsRoot and returns loomshed.NewDiscussionGate over them; "plan" requires
// env.AnchorPath and env.WorktreeRoot and returns loomshed.NewPlanGate over them; "rework-plan"
// requires the same two roots plus a non-nil env.Rework.ReadCommitted and returns
// loomshed.NewReworkPlanGate over them; "description"
// requires env.DescriptionPath to pass requireAbsRoot and returns landingshed.NewDescriptionGate
// over it; any other non-empty value is an error naming the key and all four legal values. An
// absent "gate" returns the empty GateSpec, which is what every ungated row carries by saying
// nothing; a present one returns a one-entry list named after the "gate" value.
//
// "gate_attempts" is an optional int read via configInt, becoming that entry's Attempts, and
// defaultGateAttempts when absent, zero or negative, so no value turns the entry off.
// A "gate_attempts" present with no "gate"
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
	case "rework-plan":
		if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		if err := requireSeam(entry, "Rework.ReadCommitted", env.Rework.ReadCommitted); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		closure = loomshed.NewReworkPlanGate(env.AnchorPath, env.WorktreeRoot, env.Rework.ReadCommitted)
	case "description":
		if err := requireAbsRoot(entry, "DescriptionPath", env.DescriptionPath); err != nil {
			return shuttleengine.GateSpec{}, err
		}
		closure = landingshed.NewDescriptionGate(env.DescriptionPath)
	default:
		return shuttleengine.GateSpec{}, fmt.Errorf("shedrecipe: %s: config key %q must be %q, %q, %q or %q, got %q", entry, "gate", "discussion", "plan", "rework-plan", "description", gate)
	}

	if attempts <= 0 {
		attempts = defaultGateAttempts
	}
	return shuttleengine.GateSpec{{Name: gate, Gate: closure, Attempts: attempts}}, nil
}

// defaultGateAttempts is the re-prompt budget a row naming a "gate" without a positive
// "gate_attempts" gets, until the "gates" list replaces both keys.
const defaultGateAttempts = 3
