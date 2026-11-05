// raddlecontext.go builds the board context every raddle check reads: the run's Sketch, the board's entries, the raddle store's slugs, the board's type labels and the run profiles.

package loomcli

import (
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// raddleBoardContext builds the raddleparser.BoardContext of the raddle run at location.
// The Sketch is the run's own entry, whose slug is the worktree's, and its module labels are its labels that are not type labels.
// Entries are every entry of the hub's board, Store the slugs the raddle store holds, TypeLabels the board config's type labels and Profiles the profile names that hold a block for each recipe a part may name.
// filing is told to the context unchanged.
// A board with no entry for the run is an error naming the way forward.
func raddleBoardContext(location *lymcwd.Location, filing bool) (raddleparser.BoardContext, error) {
	boardDir := fabricengine.BoardDir(location.HubPath)
	tasks, err := boardengine.ReadTasks(boardDir)
	if err != nil {
		return raddleparser.BoardContext{}, fmt.Errorf("read the board: %w", err)
	}
	store, err := boardengine.ReadRaddleStore(boardDir)
	if err != nil {
		return raddleparser.BoardContext{}, err
	}
	dirs := fabricengine.ConfigDirs(location)
	boardConfig, err := boardengine.LoadConfig(dirs, "board")
	if err != nil {
		return raddleparser.BoardContext{}, err
	}
	profiles, err := runprofile.Load(dirs)
	if err != nil {
		return raddleparser.BoardContext{}, err
	}

	vocabulary := boardConfig.Vocabulary()
	sketch := seedSlug(location.WorktreeName)
	board := raddleparser.BoardContext{
		Sketch:     sketch,
		Entry:      sketch,
		Store:      store.Slugs(),
		TypeLabels: vocabulary.Types,
		Profiles:   map[string][]string{},
		Filing:     filing,
	}
	found := false
	for _, task := range tasks {
		if task.Slug == sketch {
			found = true
			for _, label := range task.Labels {
				if !vocabulary.IsType(label) {
					board.ModuleLabels = append(board.ModuleLabels, label)
				}
			}
		}
		board.Entries = append(board.Entries, raddleparser.BoardEntry{
			Slug:    task.Slug,
			Kind:    task.Kind,
			Labels:  task.Labels,
			Open:    !boardengine.IsDoneStatus(task.Status),
			Revised: revisedStamp(task.Revised),
		})
	}
	if !found {
		return raddleparser.BoardContext{}, fmt.Errorf("the board holds no entry for the run's Sketch %q; way forward: restore the entry with \"lym board upsert\", then run the check again", sketch)
	}

	for _, recipe := range []string{raddleparser.RecipeTapestry, raddleparser.RecipeDarn} {
		for _, name := range profiles.Names() {
			if _, holds := profiles.ByName[name][recipe]; holds {
				board.Profiles[recipe] = append(board.Profiles[recipe], name)
			}
		}
		slices.Sort(board.Profiles[recipe])
	}
	return board, nil
}

// revisedStamp renders a board entry's revised time as RFC 3339, empty for an entry that records none.
func revisedStamp(revised time.Time) string {
	if revised.IsZero() {
		return ""
	}
	return revised.UTC().Format(time.RFC3339)
}

// raddleCheckFor assembles the loomshed.RaddleCheck of the raddle run at location: the proposal and the Sketch's decision record under its worktree, and the board context built on demand.
func raddleCheckFor(location *lymcwd.Location) loomshed.RaddleCheck {
	return loomshed.RaddleCheck{
		ProposalPath: loomengine.RaddleProposal(location),
		RecordPath:   loomengine.DiscussionDecisionRecord(location),
		Board: func(filing bool) (raddleparser.BoardContext, error) {
			return raddleBoardContext(location, filing)
		},
	}
}
