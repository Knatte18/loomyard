// sketch.go — the Sketch: the built-in sketch type label, the one write that makes an entry a Sketch at the end of its Discussion-only run, and the migration of a task an older binary left discussed.

package boardengine

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/shedrun"
)

// SketchType is the built-in type label of a Sketch: a note whose Discussion was approved and whose decision record is stored, which never runs and waits for the triage that breaks it into tasks or a project.
// Every board's vocabulary carries it, whatever board.yaml lists, since lym itself writes it.
const SketchType = "sketch"

// sketchTypeDescription is the description of the built-in sketch type label when board.yaml does not list it.
const sketchTypeDescription = "a note whose Discussion was approved and whose decision record is stored, waiting for triage into tasks or a project"

// LegacyStatusDiscussed is the status an older binary left on a task whose Discussion-only run finished.
// A task still carrying it is no run's to start, so a start guard refuses it like a draft.
const LegacyStatusDiscussed = "discussed"

// ErrSketchRefused matches, through errors.Is, a MakeSketch the board refused as an invalid write, such as while another entry depends on the entry.
// Any other MakeSketch error is a failure of the write itself, after which the entry may or may not be a Sketch.
var ErrSketchRefused = errors.New("board refuses the entry as a Sketch")

// withSketchType returns types led by the built-in sketch type label, or types unchanged when board.yaml already lists it.
func withSketchType(types []Label) []Label {
	if slices.ContainsFunc(types, func(l Label) bool { return l.Name == SketchType }) {
		return types
	}
	return append([]Label{{Name: SketchType, Description: sketchTypeDescription}}, types...)
}

// IsSketch reports whether an entry of kind carrying labels is a Sketch: a note carrying the sketch type label.
func IsSketch(kind string, labels []string) bool {
	return kind == KindNote && slices.Contains(labels, SketchType)
}

// sketchOf returns t made a Sketch: a note whose labels are the sketch type label followed by t's labels that vocab does not report as type labels.
// It carries no status and none of the fields a run, a landing or the dependency graph reads: depends_on, isolated, recipe, profile, run_settings, pull_request and notify_on_land.
// Every other field, the discussed flag included, stays.
func sketchOf(t Task, vocab Vocabulary) Task {
	labels := []string{SketchType}
	for _, label := range t.Labels {
		if !vocab.IsType(label) {
			labels = append(labels, label)
		}
	}
	t.Kind = KindNote
	t.Labels = labels
	t.Status = nil
	t.DependsOn = []string{}
	t.Isolated = false
	t.Recipe = ""
	t.Profile = ""
	t.RunSettings = nil
	t.PullRequest = ""
	t.NotifyOnLand = false
	return t
}

// endsAsSketch reports whether the net change from before to now is a task becoming a Sketch with no status and no other scope change, the change a Discussion-only run's end makes to the entry its run holds.
// now must equal sketchOf(before, vocab) in every field scopeChanged compares, so a change to the title, body or any other scope field alongside it is no such change.
func endsAsSketch(before, now Task, vocab Vocabulary, removedDone map[string]bool) bool {
	return before.Kind == KindTask &&
		IsSketch(now.Kind, now.Labels) &&
		now.Status == nil &&
		!scopeChanged(sketchOf(before, vocab), now, removedDone)
}

// MakeSketch makes the entry slug a Sketch under the write lock, and writes nothing when the entry is absent, done or already a Sketch.
// It ends a Discussion-only run: the entry leaves its run status in the same write, which the run lock lets through, so it never reads as a released task.
// Like any write that makes an entry a note, it refuses while another entry depends on the entry, naming those entries; that refusal matches ErrSketchRefused.
func (b *Board) MakeSketch(slug string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		i, found := store.index(slug)
		if !found || isDone(store.tasks[i]) || IsSketch(store.tasks[i].Kind, store.tasks[i].Labels) {
			return noWrite{}, nil
		}
		incoming := sketchOf(store.tasks[i], b.vocab)
		if err := store.validateWrite(store.tasks, incoming); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSketchRefused, err)
		}
		store.tasks[i] = incoming
		return nil, nil
	}, nil)
	return err
}

// checkDiscussedPromotion refuses a write that makes a task of a discussed note whose decision record is stored, unless the task carries recipe raddle, the discussed flag and the sketch label.
// The rule keys on the flag and the stored record, never on the label, so a note a note write stripped of its sketch label is judged too.
// loaded is the entries before the write, under any rename the write made, and after the entries the write leaves; recordStored reports whether an entry's record file exists.
func checkDiscussedPromotion(loaded, after []Task, recordStored func(slug string) bool) error {
	loadedBySlug := make(map[string]Task, len(loaded))
	for _, task := range loaded {
		loadedBySlug[task.Slug] = task
	}
	for _, task := range after {
		before, existed := loadedBySlug[task.Slug]
		if task.Kind != KindTask || !existed || before.Kind != KindNote || !before.Discussed || !recordStored(task.Slug) {
			continue
		}
		if task.Recipe == shedrun.RecipeRaddle && task.Discussed && slices.Contains(task.Labels, SketchType) {
			continue
		}
		return fmt.Errorf("entry %q is a discussed note with a stored decision record, which becomes tasks only through a raddle run; way forward: `lym board promote %s --recipe raddle`", task.Slug, task.Slug)
	}
	return nil
}

// migrateDiscussedStatus makes a Sketch of every open task of the store carrying LegacyStatusDiscussed, in place.
// A task the write would refuse as a Sketch, such as one another entry depends on, keeps the status as a hand-set word.
func migrateDiscussedStatus(store *Store) {
	for i, task := range store.tasks {
		if task.Kind != KindTask || isDone(task) || task.Status == nil || *task.Status != LegacyStatusDiscussed {
			continue
		}
		sketch := sketchOf(task, *store.vocab)
		if store.validateWrite(store.tasks, sketch) == nil {
			store.tasks[i] = sketch
		}
	}
}
