// entries_gate.go implements resolveGateSpec, the shared resolver every gate-capable entry (DiscussionWrite, PlanWrite, Describe, BurlerRound, Webster, PRRework) calls to turn its row's "gates" Config key into a shuttleengine.GateSpec.
//
// Each element selects a validator by a declared name resolved against Env, exactly as bouncerEntry already resolves "commit_seam"/"approve_seam" against Env.CommitPlan/Env.CommitDiscussion/Env.ApprovePlan.
// Selecting the validator by switching on the row's own Name was rejected: that would make row
// names load-bearing in a second place beyond resume identity, where a renamed row would silently
// lose its gate rather than failing to build.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentreview"
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
// Each element carries a required "name", resolved against a closed five-value vocabulary:
// "discussion" requires env.DecisionRecordPath and env.SupportLogPath to pass requireAbsRoot and returns loomshed.NewDiscussionGate over them;
// "plan" requires env.AnchorPath and env.WorktreeRoot and returns loomshed.NewPlanGate over them;
// "rework-plan" requires the same two roots plus a non-nil env.Rework.ReadCommitted and returns loomshed.NewReworkPlanGate over them;
// "description" requires env.DescriptionPath to pass requireAbsRoot and returns landingshed.NewDescriptionGate over it;
// "parent-review" requires the absolute Env.ParentReview.Store.Root, Store.LockDir, DecisionRecord and SupportLog, a non-empty Slug and both render seams, and returns parentreview.NewGate's closure pair, the second being the entry's Final closure;
// it is must-pass and may hold the run, its "attempts" is the reject cap told to the gate, and an element setting "pass_on_cap: true" is refused;
// any other value is an error naming the key and all five legal values.
// A name may appear once per list.
//
// "attempts" is required, a non-negative integer read via configInt so 0 is a present value:
// 0 turns the entry off, yet the entry still resolves its closure and its Env requirements, so a typo in an off gate fails loud.
// "pass_on_cap" is an optional bool, false when absent.
//
// Every error is qualified with entry so a recipe author with a typo is told which row spoke, matching the qualification resolveUnderRoot already applies.
//
// resolveGateSpec reads only the "gates" key and rejects unknown keys only inside each element -- each caller keeps owning its own configRejectUnknown call for the row's own keys, which is where the Config Strictness Invariant's strictness lives.
func resolveGateSpec(entry string, cfg Config, env Env) (shuttleengine.GateSpec, error) {
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

	spec := make(shuttleengine.GateSpec, 0, len(elems))
	seen := make(map[string]bool, len(elems))
	for i, elem := range elems {
		ge, err := resolveGateEntry(entry, i, elem, env)
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
func resolveGateEntry(entry string, index int, elem Config, env Env) (shuttleengine.GateEntry, error) {
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

	if name == parentReviewGateName && passOnCap {
		return shuttleengine.GateEntry{}, wrap(fmt.Errorf("gate %q refuses %q: true, because it escalates at its cap rather than letting the run through", name, "pass_on_cap"))
	}

	closure, final, err := resolveGateClosure(entry, index, name, attempts, env)
	if err != nil {
		return shuttleengine.GateEntry{}, err
	}
	return shuttleengine.GateEntry{Name: name, Gate: closure, Final: final, Attempts: attempts, PassOnCap: passOnCap, MayHold: name == parentReviewGateName}, nil
}

// parentReviewGateName is the one gate name whose entry may hold the run and escalates at its cap.
const parentReviewGateName = "parent-review"

// resolveGateClosure maps the name of the row's "gates" element at index onto its validator closure, checking the Env fields that validator needs.
// attempts is the element's "attempts" value, which "parent-review" tells its gate as the reject cap.
// The second return is the entry's optional Final closure, non-nil for "parent-review" only.
func resolveGateClosure(entry string, index int, name string, attempts int, env Env) (shuttleengine.Gate, shuttleengine.Gate, error) {
	if name == parentReviewGateName {
		return resolveParentReviewClosure(entry, attempts, env)
	}
	closure, err := resolvePlainGateClosure(entry, index, name, env)
	return closure, nil, err
}

// resolveParentReviewClosure checks the Env fields the "parent-review" gate needs and returns parentreview.NewGate's pair, with attempts told to the gate as its reject cap.
//
// The cap and the entry's own Attempts are the same value, which is what keeps the engine's in-memory failure count behind the store's rejected-round count.
// The only non-terminal failure the closure returns consumes one distinct rejected round, so the in-memory count is at most Cap-1 when the cap's reject arrives, and that reject fails terminal before the engine's own budget is spent.
// A later non-terminal failure added to the closure breaks this.
func resolveParentReviewClosure(entry string, attempts int, env Env) (shuttleengine.Gate, shuttleengine.Gate, error) {
	pr := env.ParentReview
	pr.Cap = attempts
	if err := requireAbsRoot(entry, "ParentReview.Store.Root", pr.Store.Root); err != nil {
		return nil, nil, err
	}
	if err := requireAbsRoot(entry, "ParentReview.Store.LockDir", pr.Store.LockDir); err != nil {
		return nil, nil, err
	}
	if err := requireNonEmpty(entry, "ParentReview.Slug", pr.Slug); err != nil {
		return nil, nil, err
	}
	if err := requireSeam(entry, "ParentReview.RenderDelivery", pr.RenderDelivery); err != nil {
		return nil, nil, err
	}
	if err := requireSeam(entry, "ParentReview.RenderBrief", pr.RenderBrief); err != nil {
		return nil, nil, err
	}
	if err := requireAbsRoot(entry, "ParentReview.DecisionRecord", pr.DecisionRecord); err != nil {
		return nil, nil, err
	}
	if err := requireAbsRoot(entry, "ParentReview.SupportLog", pr.SupportLog); err != nil {
		return nil, nil, err
	}
	gate, final := parentreview.NewGate(pr)
	return gate, final, nil
}

// resolvePlainGateClosure resolves the four validator names that carry no Final closure.
func resolvePlainGateClosure(entry string, index int, name string, env Env) (shuttleengine.Gate, error) {
	switch name {
	case "discussion":
		if err := requireAbsRoot(entry, "DecisionRecordPath", env.DecisionRecordPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "SupportLogPath", env.SupportLogPath); err != nil {
			return nil, err
		}
		return loomshed.NewDiscussionGate(env.DecisionRecordPath, env.SupportLogPath), nil
	case "plan":
		if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return nil, err
		}
		return loomshed.NewPlanGate(env.AnchorPath, env.WorktreeRoot), nil
	case "rework-plan":
		if err := requireAbsRoot(entry, "AnchorPath", env.AnchorPath); err != nil {
			return nil, err
		}
		if err := requireAbsRoot(entry, "WorktreeRoot", env.WorktreeRoot); err != nil {
			return nil, err
		}
		if err := requireSeam(entry, "Rework.ReadCommitted", env.Rework.ReadCommitted); err != nil {
			return nil, err
		}
		return loomshed.NewReworkPlanGate(env.AnchorPath, env.WorktreeRoot, env.Rework.ReadCommitted), nil
	case "description":
		if err := requireAbsRoot(entry, "DescriptionPath", env.DescriptionPath); err != nil {
			return nil, err
		}
		return landingshed.NewDescriptionGate(env.DescriptionPath), nil
	default:
		return nil, fmt.Errorf("shedrecipe: %s: config key %q element %d: config key %q must be %q, %q, %q, %q or %q, got %q", entry, "gates", index, "name", "discussion", "plan", "rework-plan", "description", "parent-review", name)
	}
}
