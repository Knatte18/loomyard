// raddlefile.go holds the board writes of a raddle run.
// It files a Sketch's approved entries, its tasks as drafts, with the raddle store, and can end the run's entry.
// A family filing turns the run's entry into the family's project instead of ending it,
// and every filing moves the dependencies on the run's entry onto the filed parts.

package boardengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// RaddleFileProducer is the producer half of the run status the Raddle-File row writes on its run's entry.
// The run lock admits a family filing's change of the entry into a project only while the entry's status names it.
// The name is duplicated here, since this package imports no recipe package; loomshed's test pins it to its row.
const RaddleFileProducer = "Raddle-File"

// ErrRaddleRecheck is the error a raddle filing wraps when its re-check refuses the write.
// A caller matches it with errors.Is to tell that refusal from a failed write.
var ErrRaddleRecheck = errors.New("the board moved since the raddle proposal was checked")

// ErrRaddleRepointRefused is wrapped by a raddle filing that the board refused.
// The refused part is moving an entry's dependency off the run's entry; the wrapped refusal names the entry.
// A caller matches it with errors.Is to tell that refusal, which no new split settles, from a failed write.
var ErrRaddleRepointRefused = errors.New("the board refuses to move an entry's dependency off the raddle run's entry")

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
	// When its slug is RunEntry, the filing is a family filing:
	// it turns the run's entry into the project, and only the entry's Record is read.
	ProjectEntry *RaddleEntry
	// Parts are the filed tasks, in dependency order: a part depends only on entries on the board or filed before it.
	Parts []RaddleEntry
	// Proposal, ColdProposal and SketchRecord are the staged files.
	// They are stored as proposal.yaml, cold-proposal.yaml and decision-record.md.
	Proposal     []byte
	ColdProposal []byte
	SketchRecord []byte
	// Check is the re-check run inside the write over the entries and the store as they stand; a refusal stops the write.
	Check func(tasks []Task, store RaddleStore) error
}

// FileRaddle files the entries of f and its raddle store in one write under the board lock.
// It writes nothing when the store already holds a directory for the Sketch with the filing's created stamp and hash.
// That record is the done marker of the filing.
// When the board already holds every filed entry of the Sketch, it skips the check and the entry writes.
// That is the state a crash or a failed store write leaves after board.json was saved.
// The write then only writes the store.
// Otherwise it runs f.Check, wrapping a refusal in ErrRaddleRecheck.
// It then adds the project and the parts as discussed entries.
// A family filing turns the run's entry into the project instead, the change the run lock admits for it.
// Every open entry that depends on the run's entry is moved onto the filed parts no other part depends on;
// a refusal of that move, such as a run lock on the entry, is wrapped in ErrRaddleRepointRefused.
// Each entry's decision-<slug>.md is written before board.json is saved.
// When the write fails, those record files are removed and the run's entry's former record is written back.
// After the save it writes the store directory, store.json last, so the write leaves its own done marker.
// The parts are filed draft, so a release stays the operator's; the run's entry stays open.
func (b *Board) FileRaddle(f RaddleFiling) error {
	var recordFiles []string
	formerRecords := map[string][]byte{}
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
			return nil, nil
		}
		if f.Check != nil {
			if err := f.Check(store.Tasks(), held); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrRaddleRecheck, err)
			}
		}

		partProject := f.Project
		if f.ProjectEntry != nil {
			if f.ProjectEntry.Slug == f.RunEntry {
				err = b.convertRunEntry(store, f, formerRecords, &recordFiles)
			} else {
				err = b.fileRaddleEntry(store, f, *f.ProjectEntry, f.Project, &recordFiles)
			}
			if err != nil {
				return nil, err
			}
			filed = append(filed, f.ProjectEntry.Slug)
			partProject = f.ProjectEntry.Slug
		}
		for _, part := range f.Parts {
			if err := b.fileRaddleEntry(store, f, part, partProject, &recordFiles); err != nil {
				return nil, err
			}
			filed = append(filed, part.Slug)
		}
		return nil, repointDependents(store, f.RunEntry, raddleSinkParts(f.Parts))
	}, func() error {
		saved = true
		return b.writeRaddleStore(f, filed)
	})
	if err != nil && !saved {
		for _, path := range recordFiles {
			if former, held := formerRecords[path]; held {
				_ = fsx.AtomicWriteBytes(path, former)
				continue
			}
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

// convertRunEntry turns the run's entry in store into the project of f's family filing, in place.
// It writes the project's decision record, appending the file's path to recordFiles.
// The project's order and part list name f's parts in filed order.
// The record file the entry held before is kept in formerRecords under its path, so a failed write restores it.
func (b *Board) convertRunEntry(store *Store, f RaddleFiling, formerRecords map[string][]byte, recordFiles *[]string) error {
	i, found := store.index(f.RunEntry)
	if !found {
		return fmt.Errorf("turn the run's entry %q into the family's project: the board holds no such entry", f.RunEntry)
	}
	parts := make([]string, len(f.Parts))
	for j, part := range f.Parts {
		parts[j] = part.Slug
	}

	name := DecisionFileName(f.RunEntry)
	path := filepath.Join(b.boardPath, name)
	former, err := os.ReadFile(path)
	switch {
	case err == nil:
		formerRecords[path] = former
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("read decision record %s: %w", name, err)
	}
	if err := fsx.AtomicWrite(b.boardPath, name, string(f.ProjectEntry.Record)); err != nil {
		return fmt.Errorf("write decision record %s: %w", name, err)
	}
	*recordFiles = append(*recordFiles, path)
	store.tasks[i] = arrasProjectOf(store.tasks[i], parts)
	return nil
}

// arrasProjectOf returns entry turned into the project of the family filed from it, whose parts are parts in filed order.
// The project has no status and none of the fields a run, a landing or the dependency graph reads.
// Its order lists the parts.
// Its body opens with the raddle project body prefix and the part list, followed by its former body.
// Every other field, the slug, title, labels and discussed flag included, stays.
func arrasProjectOf(entry Task, parts []string) Task {
	project := withoutRunFields(entry)
	project.Kind = KindProject
	project.Order = slices.Clone(parts)
	project.Body = fmt.Sprintf("%s: %s.\n", RaddleProjectBodyPrefix(entry.Slug), strings.Join(parts, ", "))
	if entry.Body != "" {
		project.Body += "\n" + entry.Body
	}
	return project
}

// endsAsFamilyProject reports whether the change from before to now makes the project of a family filing.
// That project is a raddle run's entry turned into the project of the parts the filing adds.
// before must be a task whose run status has RaddleFileProducer as its producer half, whatever its state half.
// The family's parts are the entries of after absent from loaded whose project is the entry, in their sequence in after;
// each must be a discussed draft task that follows every part it depends on.
// now must have no status, profile, run settings or notify_on_land, and the discussed flag of before,
// and it must equal arrasProjectOf of before and the parts in every field the scope comparison reads.
// A change to the title, labels, depends_on, body or any other scope field alongside it is therefore no such change.
func endsAsFamilyProject(before, now Task, loadedSlugs map[string]bool, after []Task, removedDone map[string]bool) bool {
	producer, _ := RunStatusProducer(before.Status)
	if producer != RaddleFileProducer || before.Kind != KindTask || now.Status != nil || now.Discussed != before.Discussed ||
		now.Profile != "" || len(now.RunSettings) > 0 || now.NotifyOnLand {
		return false
	}
	family := map[string]bool{}
	for _, task := range after {
		if !loadedSlugs[task.Slug] && task.Project == before.Slug {
			family[task.Slug] = true
		}
	}
	var parts []string
	for _, task := range after {
		if !family[task.Slug] {
			continue
		}
		if task.Kind != KindTask || !task.Discussed || !isDraft(task) {
			return false
		}
		for _, dep := range task.DependsOn {
			if family[dep] && !slices.Contains(parts, dep) {
				return false
			}
		}
		parts = append(parts, task.Slug)
	}
	return len(parts) > 0 && !scopeChanged(arrasProjectOf(before, parts), now, removedDone)
}

// raddleSinkParts returns, in filed order, the slug of every part no other part depends on.
func raddleSinkParts(parts []RaddleEntry) []string {
	var sinks []string
	for _, part := range parts {
		dependedOn := slices.ContainsFunc(parts, func(other RaddleEntry) bool { return slices.Contains(other.DependsOn, part.Slug) })
		if !dependedOn {
			sinks = append(sinks, part.Slug)
		}
	}
	return sinks
}

// repointDependents moves every open entry's dependency on runEntry onto sinks, in place.
// Each entry keeps its other dependencies in their order and names each slug once.
// A dependent a run holds, or one the board refuses as written, refuses the move with ErrRaddleRepointRefused naming it.
func repointDependents(store *Store, runEntry string, sinks []string) error {
	for i, task := range store.tasks {
		if task.Slug == runEntry || isDone(task) || !slices.Contains(task.DependsOn, runEntry) {
			continue
		}
		if RunHolds(task) {
			return fmt.Errorf("%w: %w", ErrRaddleRepointRefused, &RunLockedError{Slug: task.Slug, Status: *task.Status})
		}
		incoming := task
		incoming.DependsOn = repointedDependencies(task.DependsOn, runEntry, sinks)
		if err := store.validateWrite(store.tasks, incoming); err != nil {
			return fmt.Errorf("%w: entry %q: %w", ErrRaddleRepointRefused, task.Slug, err)
		}
		store.tasks[i] = incoming
	}
	return nil
}

// repointedDependencies returns dependsOn with runEntry replaced, in its place, by sinks, naming each slug once.
func repointedDependencies(dependsOn []string, runEntry string, sinks []string) []string {
	var repointed []string
	for _, dep := range dependsOn {
		replacement := []string{dep}
		if dep == runEntry {
			replacement = sinks
		}
		for _, slug := range replacement {
			if !slices.Contains(repointed, slug) {
				repointed = append(repointed, slug)
			}
		}
	}
	return repointed
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

// ClearProjectRunStatus clears the run status of the project slug under the write lock.
// A raddle run's repair of its family filing writes it, so no status the run wrote stays on the family's project.
// It writes nothing when the entry is absent, no project, or carries no run status.
// The cleared status releases no member: only a release of the project itself does.
func (b *Board) ClearProjectRunStatus(slug string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		i, found := store.index(slug)
		if !found || store.tasks[i].Kind != KindProject || !IsRunStatus(store.tasks[i].Status) {
			return noWrite{}, nil
		}
		store.tasks[i].Status = nil
		return nil, nil
	}, nil)
	return err
}
