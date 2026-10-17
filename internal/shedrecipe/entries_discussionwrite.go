// entries_discussionwrite.go implements discussionWriteEntry, the Constructor for the "DiscussionWrite" registry row:
// it runs the told seat table of the Discussion-Write lead (and its advisors, when the table has any) on a shedadapters.MultiLLMProducer behind loomshed.NewDiscussionWrite's commit decorator,
// so it lives in its own file rather than in entries_simple.go, whose header comment describes only the plain single-constructor shape.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionWriteEntry is the Constructor for the "DiscussionWrite" registry row: it validates Env.DiscussionTable, Env.CommitDiscussion and Env.Seats, resolves the row's "gates" Config key through resolveGateSpec, then returns loomshed.NewDiscussionWrite(name, shedadapters.NewMultiLLMProducerSourced(name, tables, env.Seats, env.Now, prepare), env.CommitDiscussion) -- a seat-table producer behind a commit decorator.
// Env.Shuttle is never read: the seats run through Env.Seats.
//
// The table arrives as a shedadapters.TableSource closure evaluated once per Call, so the seats' stencils are read at call time.
// It is a closure rather than recipe Config because building the table needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing directly; internal/loomcli's wire() supplies it.
// The resolved gate is stamped onto each table the source returns, so the row's "gates" key guards the lead whatever table the source builds.
// A table with no advisors is the single-agent case: one lead writing the discussion alone.
//
// When the resolved gate list holds an enabled "parent-review" entry, the producer's fresh-spawn preparation calls Env.ParentReview.Store.PrepareRound and returns an empty amendment, so a start opens the next round unless the latest one holds no request, a reject or a superseding approve, which the gate must read, and a resume of a live lead continues the latest one.
// A disabled or absent entry leaves the preparation nil.
//
// The row carries a "gates" key even though this entry's own dedicated constructor could imply the validator, so that one key means one thing at every gated site and a reader of the recipe can see which validator guards each row without opening Go;
// this entry therefore never hard-codes a validator choice of its own.
func discussionWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("DiscussionWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
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
	var prepare func() (string, error)
	for _, ge := range gate {
		if ge.Name == "parent-review" && ge.Attempts > 0 {
			store := env.ParentReview.Store
			prepare = func() (string, error) {
				_, err := store.PrepareRound()
				return "", err
			}
			break
		}
	}
	tables := func() (seatengine.Table, error) {
		table, err := env.DiscussionTable()
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, prepare)
	return loomshed.NewDiscussionWrite(row.Name, inner, env.CommitDiscussion), nil
}
