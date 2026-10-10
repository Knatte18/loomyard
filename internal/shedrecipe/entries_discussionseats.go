// entries_discussionseats.go implements discussionSeatsEntry, the Constructor for the "DiscussionSeats" registry row:
// it runs the told seat table of a chair and its advisors on a shedadapters.MultiLLMProducer behind loomshed.NewDiscussionWrite's commit decorator.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionSeatsEntry is the Constructor for the "DiscussionSeats" registry row: it validates Env.DiscussionTable, Env.CommitDiscussion and Env.Seats, resolves the row's "gates" Config key through resolveGateSpec, then returns loomshed.NewDiscussionWrite(name, shedadapters.NewMultiLLMProducerSourced(name, tables, env.Seats, env.Now, prepare), env.CommitDiscussion) -- a seat-table producer behind a commit decorator.
// Env.Shuttle is never read: the seats run through Env.Seats.
//
// The table arrives as a shedadapters.TableSource closure evaluated once per Call, so the chair's and advisors' stencils are read at call time.
// It is a closure rather than recipe Config because building the table needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing directly; internal/loomcli's wire() supplies it.
// The resolved gate is stamped onto each table the source returns, so the row's "gates" key guards the chair whatever table the source builds.
// A table with no advisors is the single-agent case: one chair writing the discussion alone.
//
// When the resolved gate list holds an enabled "parent-review" entry, the producer's fresh-spawn preparation calls Env.ParentReview.Store.PrepareRound, so a start opens the next round unless the latest one holds a reject or a superseding approve, which the gate must read, and a resume of a live chair continues the latest one.
// A disabled or absent entry leaves the preparation nil.
func discussionSeatsEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("DiscussionSeats", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionSeats", "DiscussionTable", env.DiscussionTable); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionSeats", "CommitDiscussion", env.CommitDiscussion); err != nil {
		return nil, err
	}
	if err := requireSeam("DiscussionSeats", "Seats", env.Seats); err != nil {
		return nil, err
	}
	var prepare func() error
	for _, ge := range gate {
		if ge.Name == "parent-review" && ge.Attempts > 0 {
			store := env.ParentReview.Store
			prepare = func() error {
				_, err := store.PrepareRound()
				return err
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
	inner := shedadapters.NewMultiLLMProducerSourced(name, tables, env.Seats, env.Now, prepare)
	return loomshed.NewDiscussionWrite(name, inner, env.CommitDiscussion), nil
}
