// raddlecontext.go builds the board context that a raddle check reads.
// The context holds the run's Sketch, the board's entries, the raddle store's slugs and the board's type labels.
// It also holds the run profiles and the live PATTERN.md entries.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// raddleBoardContext builds the raddleparser.BoardContext of the raddle run at location.
// The Sketch is the run's own entry, whose slug is the worktree's.
// Its module labels are the entry's labels that are not type labels.
// Entries holds every entry of the hub's board and Store the slugs the raddle store holds.
// TypeLabels holds the board config's type labels.
// Profiles names the profiles that hold a block for each recipe a part may name.
// LivePattern answers from the worktree's PATTERN.md, and a worktree with none holds no live entry.
// The filing flag goes to the context unchanged.
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
	overview, err := os.ReadFile(pattern.File(location.WorktreePath()))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return raddleparser.BoardContext{}, fmt.Errorf("read PATTERN.md: %w", err)
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
		LivePattern: func(id string) bool {
			_, live := pattern.EntryBullet(overview, id)
			return live
		},
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

// raddleCheckFor assembles the loomshed.RaddleCheck of the raddle run at location.
// It names the proposal and the Sketch's decision record under the worktree,
// and the board context is built on demand.
func raddleCheckFor(location *lymcwd.Location) loomshed.RaddleCheck {
	return loomshed.RaddleCheck{
		ProposalPath: loomengine.RaddleProposal(location),
		RecordPath:   loomengine.DiscussionDecisionRecord(location),
		Board: func(filing bool) (raddleparser.BoardContext, error) {
			return raddleBoardContext(location, filing)
		},
	}
}
