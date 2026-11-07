// entries_discussionwrite.go implements discussionWriteEntry, the Constructor for the "DiscussionWrite" registry row.
// The row runs the told seat table through a shedadapters.MultiLLMProducer.
// The producer sits behind loomshed.NewDiscussionWrite's commit decorator.
// The seat table is the Discussion-Write lead, with its advisors when the table has any.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionWriteEntry is the Constructor for the "DiscussionWrite" registry row.
// It validates Env.DiscussionTable, Env.CommitDiscussion and Env.Seats, and resolves the row's "gates" Config key.
// It returns a seat-table producer under the commit decorator.
// It keeps gate findings at the row's gate findings path.
// Env.Shuttle is never read: the seats run through Env.Seats.
//
// The table arrives as a shedadapters.TableSource closure evaluated once per Call, so the seats' stencils are read at call time.
// It is a closure rather than recipe Config;
// building the table needs a *lymcwd.Location.
// The Shed Recipe Registry Invariant bars this package from importing lymcwd directly.
// The loomcli wiring supplies it.
// The resolved gate is stamped onto each table the source returns,
// so the row's "gates" key guards the lead whatever table the source builds.
// A table with no advisors is the single-agent case: one lead writing the discussion alone.
//
// The row's "gates" key names the validator explicitly, so one key means one thing at every gated site,
// and a reader of the recipe sees which validator guards each row without opening Go.
// This entry therefore hard-codes no validator choice of its own.
//
// A "fixer" key on a gate element is read through splitGateFixer, and the resolved spec goes through applyGateFixer.
func discussionWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	cfg, fixer, err := splitGateFixer("DiscussionWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	gate, err := resolveGateSpec("DiscussionWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	gate, err = applyGateFixer(row.Name, gate, fixer, env)
	if err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "DiscussionTable", env.DiscussionTable); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "CommitDiscussion", env.CommitDiscussion); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionWrite", "Seats", env.Seats); err != nil {
		return nil, err
	}
	tables := func() (seatengine.Table, error) {
		table, err := env.DiscussionTable()
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil).KeepGateFindings(env.gateFindingsPath(row.Name))
	return loomshed.NewDiscussionWrite(row.Name, inner, env.CommitDiscussion), nil
}
