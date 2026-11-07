// raddlefile.go holds the board writes of a raddle run.
// It files a Sketch's approved entries with the raddle store, releases the filed parts, and ends the run's entry.

package boardengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// The staged files of a raddle store directory, beside its store.json.
const (
	raddleProposalFileName     = "proposal.yaml"
	raddleColdProposalFileName = "cold-proposal.yaml"
	raddleRecordFileName       = "decision-record.md"
)

// ErrRaddleRecheck is the error a raddle filing wraps when its re-check refuses the write.
// A caller matches it with errors.Is to tell that refusal from a failed write.
var ErrRaddleRecheck = errors.New("the board moved since the raddle proposal was checked")

// RaddleEntry is one entry a raddle run files: a task, or the project that groups the run's tasks.
type RaddleEntry struct {
	Slug  string
	Title string
	// Kind is KindTask or KindProject.
	Kind string
	// Labels holds the entry's type label and the Sketch's module labels; a filed entry never carries the sketch label.
	Labels []string
	// Recipe and Profile are the task's run recipe and run profile; both are empty for a project.
	Recipe  string
	Profile string
	// DependsOn lists the slugs the entry waits on; a slug that is no longer on the board when the entry is filed is dropped.
	DependsOn []string
	Body      string
	// Record is the entry's decision record, written as decision-<slug>.md in the write that files the entry.
	Record []byte
}

// RaddleFiling holds the board and store writes that file a Sketch whose raddle proposal was approved.
type RaddleFiling struct {
	// Sketch is the Sketch's slug, which names the store directory, and Created is the Sketch's created stamp.
	Sketch  string
	Created time.Time
	// Project is the Sketch's own project field, the level the entries are filed at; empty for the top level.
	Project string
	// Hash is the hash of the approved proposal.
	Hash string
	// RunEntry is the slug of the raddle run's own entry, which the filing leaves open and which no filed entry depends on.
	RunEntry string
	// ProjectEntry is the project the run files for the project and beam shapes, nil for the others; its parts are members of it.
	ProjectEntry *RaddleEntry
	// Parts are the filed tasks, in dependency order: a part depends only on entries on the board or filed before it.
	Parts []RaddleEntry
	// Release is whether the parts are released in the filing's own write; otherwise they are draft.
	Release bool
	// Proposal, ColdProposal and SketchRecord are the staged files.
	// They are stored as proposal.yaml, cold-proposal.yaml and decision-record.md.
	Proposal     []byte
	ColdProposal []byte
	SketchRecord []byte
	// Check is the re-check run inside the write over the entries and the store as they stand; a refusal stops the write.
	Check func(tasks []Task, store RaddleStore) error
}

// raddleFiled is the mutation result of a filing: the parts the write leaves released.
type raddleFiled struct {
	released []string
}

// FileRaddle files the entries of f and its raddle store in one write under the board lock.
// It writes nothing when the store already holds a directory for the Sketch with the filing's created stamp and hash.
// That record is the done marker of the filing.
// When the board already holds every filed entry of the Sketch, it skips the check and the entry writes.
// That is the state a crash or a failed store write leaves after board.json was saved.
// The write then only writes the store.
// Otherwise it runs f.Check, wrapping a refusal in ErrRaddleRecheck.
// It then adds the project and the parts as discussed entries.
// Each entry's decision-<slug>.md is written before board.json is saved.
// After the save it writes the store directory, store.json last, so the write leaves its own done marker.
// The parts are released in the same write when f.Release is set and are draft otherwise; the run's entry stays open.
func (b *Board) FileRaddle(f RaddleFiling) error {
	var recordFiles []string
	var filed []string
	saved := false
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		held, err := ReadRaddleStore(b.boardPath)
		if err != nil {
			return nil, err
		}
		if entry, isHeld := held.Holds(f.Sketch); isHeld && entry.Sketch == f.Sketch && entry.Hash == f.Hash && entry.Created.Equal(entryTime(f.Created)) {
			return noWrite{}, nil
		}
		getTask := func(slug string) (Task, bool) { return store.GetTask(slug) }
		if slugs := raddleFilingSlugs(f); raddleFilingHeld(getTask, f.Sketch, slugs) {
			filed = slugs
			var released []string
			if f.Release {
				for _, part := range f.Parts {
					released = append(released, part.Slug)
				}
			}
			return raddleFiled{released: released}, nil
		}
		if f.Check != nil {
			if err := f.Check(store.Tasks(), held); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrRaddleRecheck, err)
			}
		}

		partProject := f.Project
		if f.ProjectEntry != nil {
			if err := b.fileRaddleEntry(store, f, *f.ProjectEntry, f.Project, &recordFiles); err != nil {
				return nil, err
			}
			filed = append(filed, f.ProjectEntry.Slug)
			partProject = f.ProjectEntry.Slug
		}
		var released []string
		for _, part := range f.Parts {
			if err := b.fileRaddleEntry(store, f, part, partProject, &recordFiles); err != nil {
				return nil, err
			}
			filed = append(filed, part.Slug)
			if f.Release {
				released = append(released, part.Slug)
			}
		}
		return raddleFiled{released: released}, nil
	}, func() error {
		saved = true
		return b.writeRaddleStore(f, filed)
	})
	if err != nil && !saved {
		for _, path := range recordFiles {
			_ = os.Remove(path)
		}
	}
	return err
}

// fileRaddleEntry adds one filed entry to store as a discussed entry, under project when project is not empty.
// It writes the entry's decision record file and appends the file's path to recordFiles.
// A dependency on the run's entry or on a slug the store does not hold is dropped.
func (b *Board) fileRaddleEntry(store *Store, f RaddleFiling, entry RaddleEntry, project string, recordFiles *[]string) error {
	fields := map[string]any{"slug": entry.Slug, "title": entry.Title, "kind": entry.Kind, "labels": entry.Labels, "body": entry.Body}
	if project != "" {
		fields["project"] = project
	}
	if entry.Recipe != "" {
		fields["recipe"] = entry.Recipe
	}
	if entry.Profile != "" {
		fields["profile"] = entry.Profile
	}
	var dependsOn []string
	for _, dep := range entry.DependsOn {
		if _, onBoard := store.index(dep); onBoard && dep != f.RunEntry {
			dependsOn = append(dependsOn, dep)
		}
	}
	if len(dependsOn) > 0 {
		fields["depends_on"] = dependsOn
	}
	if _, err := store.UpsertTask(fields); err != nil {
		return fmt.Errorf("file entry %q: %w", entry.Slug, err)
	}

	name := DecisionFileName(entry.Slug)
	if err := fsx.AtomicWrite(b.boardPath, name, string(entry.Record)); err != nil {
		return fmt.Errorf("write decision record %s: %w", name, err)
	}
	*recordFiles = append(*recordFiles, filepath.Join(b.boardPath, name))
	i, _ := store.index(entry.Slug)
	store.tasks[i].Discussed = true
	return nil
}

// writeRaddleStore writes the staged files of f's store directory, then its store.json.
// store.json records the filed slugs and is the done marker.
func (b *Board) writeRaddleStore(f RaddleFiling, filed []string) error {
	dir := RaddleStoreDir(b.boardPath, f.Sketch)
	staged := []struct {
		name    string
		content []byte
	}{
		{raddleProposalFileName, f.Proposal},
		{raddleColdProposalFileName, f.ColdProposal},
		{raddleRecordFileName, f.SketchRecord},
	}
	for _, file := range staged {
		if err := fsx.AtomicWrite(dir, file.name, string(file.content)); err != nil {
			return fmt.Errorf("write raddle store file %s: %w", file.name, err)
		}
	}
	record := RaddleStoreEntry{Created: entryTime(f.Created), Hash: f.Hash, Filed: filed}
	if f.ProjectEntry != nil {
		record.Project = f.ProjectEntry.Slug
	}
	path := filepath.Join(dir, raddleStoreFileName)
	lockPath := lock.TrackedLockPath(b.boardPath, "raddle-"+f.Sketch+"-"+raddleStoreFileName+swapLockSuffix)
	if err := state.WriteJSON(path, lockPath, record); err != nil {
		return fmt.Errorf("write raddle store file %s: %w", raddleStoreFileName, err)
	}
	return nil
}

// RaddlePartBodyPrefix returns the text a filed part's body begins with, which names the Sketch the part is filed from.
func RaddlePartBodyPrefix(sketch string) string {
	return fmt.Sprintf("Filed from the Sketch `%s`", sketch)
}

// RaddleProjectBodyPrefix returns the text a filed project's body begins with.
// The text names the Sketch whose entries the project groups.
func RaddleProjectBodyPrefix(sketch string) string {
	return fmt.Sprintf("Groups the entries filed from the Sketch `%s`", sketch)
}

// raddleFilingSlugs returns the slug of the filing's project, when it has one, followed by the slug of each part.
func raddleFilingSlugs(f RaddleFiling) []string {
	var slugs []string
	if f.ProjectEntry != nil {
		slugs = append(slugs, f.ProjectEntry.Slug)
	}
	for _, part := range f.Parts {
		slugs = append(slugs, part.Slug)
	}
	return slugs
}

// isRaddleFiledFrom reports whether task is a discussed entry whose body marks it as filed from the Sketch named sketch.
func isRaddleFiledFrom(task Task, sketch string) bool {
	return task.Discussed && (strings.HasPrefix(task.Body, RaddlePartBodyPrefix(sketch)) || strings.HasPrefix(task.Body, RaddleProjectBodyPrefix(sketch)))
}

// raddleFilingHeld reports whether get finds every slug of slugs as an entry filed from the Sketch named sketch.
// With no slugs it reports false.
func raddleFilingHeld(get func(slug string) (Task, bool), sketch string, slugs []string) bool {
	if len(slugs) == 0 {
		return false
	}
	for _, slug := range slugs {
		task, found := get(slug)
		if !found || !isRaddleFiledFrom(task, sketch) {
			return false
		}
	}
	return true
}

// HoldsRaddleFiling reports whether the board holds every slug of slugs as a discussed entry.
// Each such entry's body must mark it as filed from the Sketch named sketch.
// A retry uses it to tell a filing whose board write landed from one that did not.
// With no slugs it reports false, and it returns an error when a board read fails.
func (b *Board) HoldsRaddleFiling(sketch string, slugs []string) (bool, error) {
	if len(slugs) == 0 {
		return false, nil
	}
	for _, slug := range slugs {
		task, found, err := b.GetTask(slug)
		if err != nil {
			return false, err
		}
		if !found || !isRaddleFiledFrom(task, sketch) {
			return false, nil
		}
	}
	return true, nil
}

// ReleaseTasks clears the status of each task of slugs that is on the board, not done, and draft, in one write.
// Tasks that are absent, done or not draft are skipped.
// It writes nothing when no task is released.
func (b *Board) ReleaseTasks(slugs []string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		releasedAny := false
		for _, slug := range slugs {
			i, found := store.index(slug)
			if !found || isDone(store.tasks[i]) || !isDraft(store.tasks[i]) {
				continue
			}
			store.tasks[i].Status = nil
			releasedAny = true
		}
		if !releasedAny {
			return noWrite{}, nil
		}
		return nil, nil
	}, nil)
	return err
}

// EndRaddleEntry marks the entry slug done and clears its notify_on_land in one write.
// The run lock admits the write since neither field is scope.
// It writes nothing for an absent or done entry.
func (b *Board) EndRaddleEntry(slug string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		i, found := store.index(slug)
		if !found || isDone(store.tasks[i]) {
			return noWrite{}, nil
		}
		done := StatusDone
		store.tasks[i].Status = &done
		store.tasks[i].NotifyOnLand = false
		return nil, nil
	}, nil)
	return err
}
