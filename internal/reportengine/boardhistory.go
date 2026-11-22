// boardhistory.go reads the board branch's history for the landing joins:
// per slug, the board commits at which its entry reads done and the labels it carries.

package reportengine

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// olderBoardPage is the number of board commits older than the window that are listed at a time while looking for labels.
const olderBoardPage = 64

// commitSpan is a run of consecutive board commits, by index into boardHistory.times, first and last included.
type commitSpan struct {
	first, last int
}

// labelSpan is a run of consecutive board commits at which an entry carries one set of labels.
type labelSpan struct {
	commitSpan
	labels []string
}

// boardHistory is what the landing joins read from the board's history:
// the committer time of each board commit read, oldest first.
// Per slug it holds the spans of those commits at which its entry reads done,
// and the spans at which it carries one set of labels.
// It holds no parsed board, so it grows with the entries' changes and not with the board's size.
//
// Commits older than the oldest one read stay reachable through before.
// It is the first parent of that commit, empty when it has none.
// readOlderLabels reads them, and olderLabels holds the labels it found per slug.
type boardHistory struct {
	times  []time.Time
	done   map[string][]commitSpan
	labels map[string][]labelSpan

	board       Reader
	before      string
	olderLabels map[string][]string
}

// readBoardHistory reads the board once at each commit of its first-parent line committed at or after floor.
// The commits come oldest first.
// It also reads the board in effect at floor, the first commit of that line older than it.
// A zero floor reads the whole history.
// A commit holding no board, or one that does not parse, holds no entry.
func readBoardHistory(board Reader, tip string, floor time.Time) (*boardHistory, error) {
	newestFirst, err := board.Log(tip, gitrepo.LogQuery{FirstParent: true, Since: floor})
	if err != nil {
		return nil, fmt.Errorf("reportengine: walk the board branch: %w", err)
	}
	if !floor.IsZero() {
		before := tip
		if n := len(newestFirst); n > 0 {
			before = ""
			if parents := newestFirst[n-1].Parents; len(parents) > 0 {
				before = parents[0]
			}
		}
		if before != "" {
			prior, err := board.Log(before, gitrepo.LogQuery{Limit: 1})
			if err != nil {
				return nil, fmt.Errorf("reportengine: read the board commit before %s: %w", floor.Format(time.RFC3339), err)
			}
			newestFirst = append(newestFirst, prior...)
		}
	}

	history := &boardHistory{board: board, done: map[string][]commitSpan{}, labels: map[string][]labelSpan{}, olderLabels: map[string][]string{}}
	for k := len(newestFirst) - 1; k >= 0; k-- {
		tasks, err := boardTasksAt(board, newestFirst[k].SHA)
		if err != nil {
			return nil, err
		}
		history.add(newestFirst[k].Committed, tasks)
	}
	if n := len(newestFirst); n > 0 && len(newestFirst[n-1].Parents) > 0 {
		history.before = newestFirst[n-1].Parents[0]
	}
	return history, nil
}

// readOlderLabels finds the labels of each of slugs in the newest older board commit that holds it.
// Older means older than every commit read.
// It reads those commits newest first, once each, and stops when every slug has its labels or the history ends.
// A slug whose entry no older commit holds gets no labels.
// Only the labels are kept.
func (h *boardHistory) readOlderLabels(slugs []string) error {
	pending := map[string]bool{}
	for _, slug := range slugs {
		pending[slug] = true
	}
	for h.before != "" && len(pending) > 0 {
		page, err := h.board.Log(h.before, gitrepo.LogQuery{FirstParent: true, Limit: olderBoardPage})
		if err != nil {
			return fmt.Errorf("reportengine: walk the board branch before its window: %w", err)
		}
		for _, commit := range page {
			tasks, err := boardTasksAt(h.board, commit.SHA)
			if err != nil {
				return err
			}
			for _, task := range tasks {
				if pending[task.Slug] {
					h.olderLabels[task.Slug] = slices.Clone(task.Labels)
					delete(pending, task.Slug)
				}
			}
			if len(pending) == 0 {
				return nil
			}
		}
		h.before = ""
		if parents := page[len(page)-1].Parents; len(parents) > 0 {
			h.before = parents[0]
		}
	}
	return nil
}

// boardTasksAt returns the board entries at a board commit.
// A commit holding no board, or one that does not parse, holds none.
func boardTasksAt(board Reader, sha string) ([]boardengine.Task, error) {
	data, err := board.FileAtRevision(sha, boardengine.BoardFileRel())
	switch {
	case errors.Is(err, gitrepo.ErrPathNotAtRevision):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reportengine: read the board at %s: %w", sha, err)
	}
	tasks, err := boardengine.ParseBoardTasks(data)
	if err != nil {
		return nil, nil
	}
	return tasks, nil
}

// add appends the board commit committed at committed, holding tasks, as the newest commit read.
// A slug's first entry on the board gives its labels, and any of its entries reading done marks it done.
func (h *boardHistory) add(committed time.Time, tasks []boardengine.Task) {
	index := len(h.times)
	h.times = append(h.times, committed)
	labelled := map[string]bool{}
	done := map[string]bool{}
	for _, task := range tasks {
		if task.Status != nil && *task.Status == boardStatusDone && !done[task.Slug] {
			done[task.Slug] = true
			h.done[task.Slug] = extendSpans(h.done[task.Slug], index)
		}
		if labelled[task.Slug] {
			continue
		}
		labelled[task.Slug] = true
		spans := h.labels[task.Slug]
		if n := len(spans); n > 0 && spans[n-1].last == index-1 && slices.Equal(spans[n-1].labels, task.Labels) {
			spans[n-1].last = index
			continue
		}
		h.labels[task.Slug] = append(spans, labelSpan{commitSpan: commitSpan{first: index, last: index}, labels: slices.Clone(task.Labels)})
	}
}

// extendSpans adds the commit index to spans, extending the newest span when index follows it.
func extendSpans(spans []commitSpan, index int) []commitSpan {
	if n := len(spans); n > 0 && spans[n-1].last == index-1 {
		spans[n-1].last = index
		return spans
	}
	return append(spans, commitSpan{first: index, last: index})
}

// doneAfter returns the time of the first board commit after open at which slug's entry reads done.
func (h *boardHistory) doneAfter(slug string, open time.Time) (time.Time, bool) {
	for _, span := range h.done[slug] {
		for k := span.first; k <= span.last; k++ {
			if h.times[k].After(open) {
				return h.times[k], true
			}
		}
	}
	return time.Time{}, false
}

// labelsAt returns the labels of slug's entry in the newest board commit at or before at that holds it.
// A slug no commit read holds at that time takes the labels readOlderLabels found, when it found any.
func (h *boardHistory) labelsAt(slug string, at time.Time) ([]string, bool) {
	spans := h.labels[slug]
	for s := len(spans) - 1; s >= 0; s-- {
		for k := spans[s].last; k >= spans[s].first; k-- {
			if !h.times[k].After(at) {
				return spans[s].labels, true
			}
		}
	}
	labels, found := h.olderLabels[slug]
	return labels, found
}
