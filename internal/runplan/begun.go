// begun.go reads which cards of a plan a run's batch records hold as begun or done.
// Loom's plan gate and renumber verbs read them through the active implementer's state.

package runplan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// DoneCards returns the cards of plan, in plan order, that belong to a batch st records terminal with status done.
// It reads the batch records alone: a record's own card ids name its cards,
// and a record written before those ids existed names the single card NN-<Slug>.
// A recorded id the plan lacks is skipped, so a card renamed since its batch ran counts as not done.
// A nil state returns nil.
//
// The loom plan gate uses it to scope its tree-dependent checks:
// a done card's work is already in the tree, so it is history, not a target.
func DoneCards(plan *planparser.Plan, st *State) []planparser.Card {
	if st == nil {
		return nil
	}

	done := make(map[string]bool)
	for number, bs := range st.Batches {
		if bs == nil || !bs.Terminal || bs.Status != DigestStatusDone {
			continue
		}
		ids := bs.Cards
		if len(ids) == 0 {
			ids = []string{fmt.Sprintf("%02d-%s", number, bs.Slug)}
		}
		for _, id := range ids {
			done[id] = true
		}
	}

	var cards []planparser.Card
	for _, c := range plan.Cards {
		if done[CardID(c)] {
			cards = append(cards, c)
		}
	}
	return cards
}

// LastBegunCard returns the highest card number among the cards of every batch record st holds, terminal or not.
// It returns 0 when there is none, including for a nil state.
// A record's cards are its own card ids.
// A record written before those ids existed names the single card numbered by its key.
// It bounds the begun set, whose cards are frozen, so it reads the records alone.
func LastBegunCard(st *State) int {
	if st == nil {
		return 0
	}

	last := 0
	for number, bs := range st.Batches {
		if bs == nil {
			continue
		}
		if len(bs.Cards) == 0 {
			last = max(last, number)
			continue
		}
		for _, id := range bs.Cards {
			last = max(last, cardIDNumber(id))
		}
	}
	return last
}

// EditedDoneCards returns, sorted, the NN-slug ids of edited cards whose batch is terminal and done.
// A card is edited when its file no longer hashes to the CardHashes entry recorded at begin.
// The plan's own source reads each card file, so an overlaid plan is judged by its proposed bytes.
// It hashes the files the way the recorded CardHashes were computed,
// and it reads only the record and those bytes, never parsing the plan.
// A nil state contributes nothing.
// So does a record without CardHashes, a batch not terminal done, or a recorded id the plan lacks.
// An unreadable card file is returned as an error.
//
// The plan gate uses it to refuse an edit to a card whose work has landed.
func EditedDoneCards(plan *planparser.Plan, st *State) ([]string, error) {
	if st == nil {
		return nil, nil
	}

	cardsByID := make(map[string]planparser.Card, len(plan.Cards))
	for _, c := range plan.Cards {
		cardsByID[CardID(c)] = c
	}

	var edited []string
	for _, bs := range st.Batches {
		if bs == nil || !bs.Terminal || bs.Status != DigestStatusDone {
			continue
		}
		var recorded []planparser.Card
		for id := range bs.CardHashes {
			if c, ok := cardsByID[id]; ok {
				recorded = append(recorded, c)
			}
		}
		now, err := CardHashesFrom(batcher.Batch{Cards: recorded}, plan.ReadFile)
		if err != nil {
			return nil, err
		}
		for id, hash := range now {
			if hash != bs.CardHashes[id] {
				edited = append(edited, id)
			}
		}
	}
	sort.Strings(edited)
	return edited, nil
}

// cardIDNumber returns the number of a NN-<slug> card id, or 0 when the id has none.
func cardIDNumber(id string) int {
	num, _, _ := strings.Cut(id, "-")
	n, _ := strconv.Atoi(num)
	return n
}
