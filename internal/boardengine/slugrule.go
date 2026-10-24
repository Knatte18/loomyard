// slugrule.go — the write rule that applies the pair slug rule, through a seam, to the task and project slugs a write makes.

package boardengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// checkNewSlugs refuses a write that makes a task or project slug the validate seam refuses.
// A write makes a slug when it adds an entry of kind task or project, makes a note a task or a project, or renames an entry to renamedTo.
// loaded is the entries before the write, under any rename the write made, and after the entries the write leaves.
// A write that leaves an entry's slug and kind as they were is never judged, so a slug the rule now refuses keeps taking writes,
// and an entry that becomes a project from a task keeps its slug.
// A nil validate checks nothing.
func checkNewSlugs(loaded, after []Task, renamedTo string, validate func(slug string) error) error {
	if validate == nil {
		return nil
	}
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
		if err := validate(task.Slug); err != nil {
			return newSlugError(task.Slug, err, after, validate)
		}
	}
	return nil
}

// newSlugError is the refusal of the slug the rule refuses, naming the entry and the rule's own reason.
// The way forward is a slug the rule admits: the slug without its records suffix, else the slug with -task appended.
// It offers none when that suggestion fails the rule or names an existing entry, since an upsert under an existing slug edits that entry instead of creating one.
func newSlugError(slug string, reason error, after []Task, validate func(slug string) error) error {
	suggestion := slug + "-task"
	if warp, paired := fabricengine.WeftWarpSlug(slug); paired {
		suggestion = warp
	}
	offerable := validate(suggestion) == nil
	for _, task := range after {
		offerable = offerable && task.Slug != suggestion
	}
	if offerable {
		return fmt.Errorf("entry %q cannot take this slug: %w: use the slug %q instead", slug, reason, suggestion)
	}
	return fmt.Errorf("entry %q cannot take this slug: %w: choose another slug the rule admits", slug, reason)
}
