// slugrule.go — the write rule that applies the pair slug rule, through a seam, to the task and project slugs a write makes.

package boardengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// checkNewSlugs refuses a write that makes a task or project slug the validate seam refuses or the raddle store holds.
// A write makes a slug when it adds an entry of kind task or project, makes a note a task or a project, or renames an entry to renamedTo.
// loaded is the entries before the write, under any rename the write made, and after the entries the write leaves.
// A write that leaves an entry's slug and kind as they were is never judged, so a slug the rule now refuses keeps taking writes,
// and an entry that becomes a project from a task keeps its slug.
// A nil validate checks nothing of the seam; the store is judged either way.
// store is the raddle store as it stood before the write.
// A made slug it holds, as a Sketch slug or as a filed slug, is refused.
// The one exception is the made entry whose slug is a store entry's Sketch slug and whose created stamp is that entry's recorded one, a live raddle run's own Sketch.
// storeErr is the store's read error, which refuses a write that makes a slug and no other.
func checkNewSlugs(loaded, after []Task, renamedTo string, validate func(slug string) error, store RaddleStore, storeErr error) error {
	loadedBySlug := make(map[string]Task, len(loaded))
	for _, task := range loaded {
		loadedBySlug[task.Slug] = task
	}
	for _, task := range after {
		if task.Kind != KindTask && task.Kind != KindProject {
			continue
		}
		before, existed := loadedBySlug[task.Slug]
		made := !existed || before.Kind == KindNote || task.Slug == renamedTo
		if !made {
			continue
		}
		promotion := existed && before.Kind == KindNote
		if validate != nil {
			if err := validate(task.Slug); err != nil {
				return newSlugError(task.Slug, err, after, validate, store, promotion)
			}
		}
		if storeErr != nil {
			return fmt.Errorf("entry %q cannot be made while the raddle store is unreadable: %w", task.Slug, storeErr)
		}
		held, isHeld := store.Holds(task.Slug)
		if !isHeld || (held.Sketch == task.Slug && task.Created.Equal(held.Created)) {
			continue
		}
		reason := fmt.Errorf("the slug is filed by the raddle run of Sketch %q", held.Sketch)
		if held.Sketch == task.Slug {
			reason = fmt.Errorf("the slug is the Sketch of a raddle run the raddle store holds")
		}
		return newSlugError(task.Slug, reason, after, validate, store, promotion)
	}
	return nil
}

// newSlugError is the refusal of the slug the rule or the raddle store refuses, naming the entry and the reason.
// The way forward is a slug both admit: the slug without its records suffix, else the slug with -task appended.
// It offers none when that suggestion fails the rule, is held by the store or names an existing entry, since an upsert under an existing slug edits that entry instead of creating one.
// promotion is true for a note made a task or project, which keeps its slug: the way forward is then `lym board rename` of the note, a note's rename not being judged.
func newSlugError(slug string, reason error, after []Task, validate func(slug string) error, store RaddleStore, promotion bool) error {
	suggestion := slug + "-task"
	if warp, paired := fabricengine.WeftWarpSlug(slug); paired {
		suggestion = warp
	}
	_, storeHolds := store.Holds(suggestion)
	offerable := !storeHolds && (validate == nil || validate(suggestion) == nil)
	for _, task := range after {
		offerable = offerable && task.Slug != suggestion
	}
	switch {
	case promotion && offerable:
		return fmt.Errorf("entry %q cannot take this slug: %w: rename the note with `lym board rename` to a slug that is admitted, such as %q, then promote it", slug, reason, suggestion)
	case promotion:
		return fmt.Errorf("entry %q cannot take this slug: %w: rename the note with `lym board rename` to a slug that is admitted, then promote it", slug, reason)
	case offerable:
		return fmt.Errorf("entry %q cannot take this slug: %w: use the slug %q instead", slug, reason, suggestion)
	}
	return fmt.Errorf("entry %q cannot take this slug: %w: choose another slug the rule admits", slug, reason)
}
