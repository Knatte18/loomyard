// entries_discussionwrite.go implements discussionWriteEntry, the Constructor for the
// "DiscussionWrite" registry row: it wraps a shedadapters.SingleLLMProducer in
// loomshed.NewDiscussionWrite's commit decorator, so it lives in its own file rather than in
// entries_simple.go, whose header comment describes only the plain single-constructor shape.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionWriteEntry is the Constructor for the "DiscussionWrite" registry row: it validates Env.DiscussionSpec, Env.PrepareDiscussionSpawn, Env.CommitDiscussion, and Env.Shuttle, resolves the row's "gates" Config key through resolveGateSpec, then returns loomshed.NewDiscussionWrite(name, shedadapters.NewSingleLLMProducerGated(name, env.DiscussionSpec, env.Shuttle, env.Now, prepare, gate), env.CommitDiscussion) -- a gated SingleLLMProducer behind a commit decorator.
//
// The producer's fresh-spawn preparation runs Env.PrepareDiscussionSpawn first and then, when the resolved gate list holds an enabled "parent-review" entry, calls Env.ParentReview.Store.PrepareRound; it returns an empty amendment and the first error.
// A start therefore opens the next round unless the latest one holds a reject or a superseding approve, which the gate must read, and an attach runs neither step and continues the latest round.
// A disabled or absent parent-review entry leaves only the first step.
//
// The Spec arrives as an injected shedadapters.SpecSource closure rather than as recipe Config
// because building it needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this
// package from importing directly; internal/loomcli's wire() is what supplies the closure.
//
// The generic "SingleLLM" entry is not reused here for two reasons: {{.slug}} and {{.mode_rules}}
// are per-run values a static Config.tokens map cannot carry, and a generic row's own model/effort
// Config keys would bypass the "discussion" role's model-spec resolution and its timeout entirely.
//
// The row carries a "gates" key even though this entry's own dedicated constructor could imply the validator, so that one key means one thing at every gated site and a reader of the recipe can see which validator guards each row without opening Go;
// this entry therefore never hard-codes a validator choice of its own.
func discussionWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("DiscussionWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "DiscussionSpec", env.DiscussionSpec); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "PrepareDiscussionSpawn", env.PrepareDiscussionSpawn); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "CommitDiscussion", env.CommitDiscussion); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	var prepareRound func() error
	for _, ge := range gate {
		if ge.Name == "parent-review" && ge.Attempts > 0 {
			store := env.ParentReview.Store
			prepareRound = func() error {
				_, err := store.PrepareRound()
				return err
			}
			break
		}
	}
	prepareSpawn := env.PrepareDiscussionSpawn
	prepare := func() (string, error) {
		if err := prepareSpawn(); err != nil {
			return "", err
		}
		if prepareRound == nil {
			return "", nil
		}
		return "", prepareRound()
	}
	inner := shedadapters.NewSingleLLMProducerGated(name, env.DiscussionSpec, env.Shuttle, env.Now, prepare, gate)
	return loomshed.NewDiscussionWrite(name, inner, env.CommitDiscussion), nil
}
