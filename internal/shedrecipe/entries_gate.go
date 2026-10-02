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
// it is must-pass and may hold the run, its "attempts" is the reject cap told to the gate, and its "pass_on_cap" tells the gate to let the rewrite after the cap's reject through rather than halt the run;
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

	if name == parentReviewGateName {
		closure, final, err := resolveParentReviewClosure(entry, attempts, passOnCap, env)
		if err != nil {
			return shuttleengine.GateEntry{}, err
		}
		return shuttleengine.GateEntry{Name: name, Gate: closure, Final: final, Attempts: attempts, MayHold: true}, nil
	}

	closure, err := resolvePlainGateClosure(entry, index, name, env)
	if err != nil {
		return shuttleengine.GateEntry{}, err
	}
	return shuttleengine.GateEntry{Name: name, Gate: closure, Attempts: attempts, PassOnCap: passOnCap}, nil
}

// parentReviewGateName is the one gate name whose entry may hold the run and whose cap its own store counts.
const parentReviewGateName = "parent-review"

// resolveParentReviewClosure checks the Env fields the "parent-review" gate needs and returns parentreview.NewGate's pair,
// with attempts told to the gate as its reject cap and passOnCap as whether the rewrite after the cap's reject passes rather than halting the run.
//
// The cap and the entry's own Attempts are the same value, which is what keeps the engine's in-memory failure count behind the store's rejected-round count.
// The only non-terminal failure the closure returns consumes one distinct rejected round, so the in-memory count is at most Cap-1 when the cap's reject arrives;
// that reject then fails terminal, or with passOnCap is the Cap-th failure, re-prompting once more before the next arrival passes, so the engine's own budget never ends the run.
// A later non-terminal failure added to the closure breaks this.
// The entry itself is never the engine's PassOnCap, since the gate's own store, not the engine's in-memory count, decides the let-through.
func resolveParentReviewClosure(entry string, attempts int, passOnCap bool, env Env) (shuttleengine.Gate, shuttleengine.Gate, error) {
	pr := env.ParentReview
	pr.Cap = attempts
	pr.PassAtCap = passOnCap
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
