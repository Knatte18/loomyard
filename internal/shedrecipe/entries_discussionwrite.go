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

// discussionWriteEntry is the Constructor for the "DiscussionWrite" registry row: it validates
// Env.DiscussionSpec, Env.CommitDiscussion, and Env.Shuttle, then returns
// loomshed.NewDiscussionWrite(name, shedadapters.NewSingleLLMProducer(name, env.DiscussionSpec,
// env.Shuttle, env.Now), env.CommitDiscussion) -- a SingleLLMProducer behind a commit decorator.
//
// The Spec arrives as an injected shedadapters.SpecSource closure rather than as recipe Config
// because building it needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this
// package from importing directly; internal/loomcli's wire() is what supplies the closure.
//
// The generic "SingleLLM" entry is not reused here for two reasons: {{.slug}} and {{.mode_rules}}
// are per-run values a static Config.tokens map cannot carry, and a generic row's own model/effort
// Config keys would bypass the "discussion" role's model-spec resolution and its timeout entirely.
func discussionWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "DiscussionSpec", env.DiscussionSpec); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "CommitDiscussion", env.CommitDiscussion); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	inner := shedadapters.NewSingleLLMProducer(name, env.DiscussionSpec, env.Shuttle, env.Now, nil)
	return loomshed.NewDiscussionWrite(name, inner, env.CommitDiscussion), nil
}
