// replay.go — FabricReplay, the by-entry merge of a local board commit onto origin's board.
//
// It is the implementation of the seam fabricengine declares for a records replay: a commit's entry-level changes are re-applied one by one onto the board checked out at origin's tip, each through the board's own write checks, so a change only one side made never conflicts.

package boardengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// FabricReplay is fabricengine's board replay seam over this package's store.
// It holds no state: Apply takes the board config from the request's board directory.
type FabricReplay struct{}

// replayField is one comparable field of an entry: its JSON name, a canonical getter that treats nil and empty alike, and the setter that copies it from another entry.
type replayField struct {
	name string
	get  func(Task) any
	set  func(dst *Task, src Task)
}

// replayFields lists every entry field a replay merges, in the order a change reports them.
// Id, handle, created, revised and unknown keys are not merged as fields: the id and handle are origin's, the dates follow the changes, and an unknown key stays as origin holds it.
var replayFields = []replayField{
	{"title", func(t Task) any { return t.Title }, func(dst *Task, src Task) { dst.Title = src.Title }},
	{"kind", func(t Task) any { return t.Kind }, func(dst *Task, src Task) { dst.Kind = src.Kind }},
	{"labels", func(t Task) any { return orEmpty(t.Labels) }, func(dst *Task, src Task) { dst.Labels = slices.Clone(src.Labels) }},
	{"issues", func(t Task) any { return orEmpty(t.Issues) }, func(dst *Task, src Task) { dst.Issues = slices.Clone(src.Issues) }},
	{"recipe", func(t Task) any { return t.Recipe }, func(dst *Task, src Task) { dst.Recipe = src.Recipe }},
	{"priority", func(t Task) any { return t.Priority }, func(dst *Task, src Task) { dst.Priority = src.Priority }},
	{"depends_on", func(t Task) any { return orEmpty(t.DependsOn) }, func(dst *Task, src Task) { dst.DependsOn = slices.Clone(src.DependsOn) }},
	{"isolated", func(t Task) any { return t.Isolated }, func(dst *Task, src Task) { dst.Isolated = src.Isolated }},
	{"brief", func(t Task) any { return t.Brief }, func(dst *Task, src Task) { dst.Brief = src.Brief }},
	{"body", func(t Task) any { return t.Body }, func(dst *Task, src Task) { dst.Body = src.Body }},
	{"status", func(t Task) any { return statusText(t.Status) }, func(dst *Task, src Task) { dst.Status = cloneTasks([]Task{src})[0].Status }},
	{"short_name", func(t Task) any { return t.ShortName }, func(dst *Task, src Task) { dst.ShortName = src.ShortName }},
	{"run_settings", func(t Task) any { return orNilSettings(t.RunSettings) }, func(dst *Task, src Task) { dst.RunSettings = cloneSettings(src.RunSettings) }},
	{"profile", func(t Task) any { return t.Profile }, func(dst *Task, src Task) { dst.Profile = src.Profile }},
	{"owner_machine", func(t Task) any { return t.OwnerMachine }, func(dst *Task, src Task) { dst.OwnerMachine = src.OwnerMachine }},
	{"project", func(t Task) any { return t.Project }, func(dst *Task, src Task) { dst.Project = src.Project }},
	{"order", func(t Task) any { return orEmpty(t.Order) }, func(dst *Task, src Task) { dst.Order = slices.Clone(src.Order) }},
	{"beam", func(t Task) any { return t.Beam }, func(dst *Task, src Task) { dst.Beam = src.Beam }},
}

// orEmpty returns values, or an empty slice for nil, so a nil and an empty slice compare equal.
func orEmpty[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

// orNilSettings returns nil for an empty run settings map, so a nil and an empty map compare equal.
func orNilSettings(settings map[string]any) map[string]any {
	if len(settings) == 0 {
		return nil
	}
	return settings
}

// statusText is the status an entry carries, empty when it carries none.
func statusText(status *string) string {
	if status == nil {
		return ""
	}
	return *status
}

// replayValue renders a field value as JSON, the form board.json stores it in.
func replayValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

// parsePrimeOwner decodes the top-level prime_owner of a board.json read from a commit, empty for an absent or empty file or an absent key.
func parsePrimeOwner(data []byte) (string, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return "", nil
	}
	var decoded struct {
		PrimeOwner string `json:"prime_owner"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", err
	}
	return decoded.PrimeOwner, nil
}

// parseReplayBoard decodes a board.json read from a commit, an absent or empty file being a board with no entry.
func parseReplayBoard(data []byte) (map[string]Task, []Task, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]Task{}, nil, nil
	}
	tasks, err := ParseBoardTasks(data)
	if err != nil {
		return nil, nil, err
	}
	bySlug := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		bySlug[task.Slug] = task
	}
	return bySlug, tasks, nil
}

// Entries decodes boardJSON into each entry's slug, status and created stamp, in file order.
// An empty boardJSON holds no entry.
func (FabricReplay) Entries(boardJSON []byte) ([]fabricengine.BoardEntry, error) {
	_, tasks, err := parseReplayBoard(boardJSON)
	if err != nil {
		return nil, err
	}
	entries := make([]fabricengine.BoardEntry, len(tasks))
	for i, task := range tasks {
		entries[i] = fabricengine.BoardEntry{Slug: task.Slug, Status: statusText(task.Status), Created: task.Created}
	}
	return entries, nil
}

// replayRun is the state of one Apply: origin's store, the three sides of the commit and what has been applied or set aside so far.
type replayRun struct {
	store *Store
	// origin is the entries as origin's tip holds them, the baseline the run lock judges each change's net effect against.
	origin      []Task
	base        map[string]Task
	refusedAdds map[string]bool
	result      fabricengine.BoardReplayResult
	applied     bool
}

// Apply re-applies the request's commit onto the board directory by entry and design file, and reports what it kept and what it set aside.
// It loads the board config from the request's board directory and a store from origin's board.json there, and applies each change of the commit against its parent on its own, against the board as the kept changes before it left it.
// The hub's prime_owner record replays like an entry field, reported lost under the entry name prime.
// Where both sides changed the same field of an entry or the same design file since the merge base, or the change touches an entry origin removed, origin's side is kept for that change and it is reported lost.
// A change the board's own checks refuse, the run lock included, is reported refused and so is a later change that depends on an entry a refused change added; the rest of the commit still lands.
// An added entry takes a handle free on origin's board, and a changed entry keeps origin's handle unless its letter changed.
// It saves board.json and re-renders the board's output files when anything was applied, and writes nothing otherwise.
// It takes no lock and spawns no sync, since its caller holds the board and push locks.
func (FabricReplay) Apply(req fabricengine.BoardReplayRequest) (fabricengine.BoardReplayResult, error) {
	cfg, err := LoadConfig(configengine.Dirs{Board: req.BoardDir}, "board")
	if err != nil {
		return fabricengine.BoardReplayResult{}, err
	}
	vocabulary := cfg.Vocabulary()

	base, _, err := parseReplayBoard(req.BaseBoardJSON)
	if err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("merge base %s: %w", boardFile, err)
	}
	parent, parentTasks, err := parseReplayBoard(req.ParentBoardJSON)
	if err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("parent %s: %w", boardFile, err)
	}
	_, commitTasks, err := parseReplayBoard(req.CommitBoardJSON)
	if err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("commit %s: %w", boardFile, err)
	}

	basePrime, err := parsePrimeOwner(req.BaseBoardJSON)
	if err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("merge base %s: %w", boardFile, err)
	}
	parentPrime, err := parsePrimeOwner(req.ParentBoardJSON)
	if err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("parent %s: %w", boardFile, err)
	}
	commitPrime, err := parsePrimeOwner(req.CommitBoardJSON)
	if err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("commit %s: %w", boardFile, err)
	}

	store := NewStore(req.BoardDir)
	store.vocab = &vocabulary
	if err := store.Load(); err != nil {
		return fabricengine.BoardReplayResult{}, err
	}
	run := &replayRun{store: store, origin: cloneTasks(store.Tasks()), base: base, refusedAdds: map[string]bool{}}

	inCommit := make(map[string]bool, len(commitTasks))
	for _, commit := range commitTasks {
		inCommit[commit.Slug] = true
		if before, existed := parent[commit.Slug]; existed {
			run.changeEntry(before, commit)
		} else {
			run.addEntry(commit)
		}
	}
	for _, before := range parentTasks {
		if !inCommit[before.Slug] {
			run.removeEntry(before)
		}
	}

	run.changePrimeOwner(basePrime, parentPrime, commitPrime)

	designsApplied := false
	for _, file := range req.DesignFiles {
		applied, err := run.applyDesignFile(req.BoardDir, file)
		if err != nil {
			return fabricengine.BoardReplayResult{}, err
		}
		designsApplied = designsApplied || applied
	}

	run.result.Empty = !run.applied && !designsApplied
	if run.result.Empty {
		return run.result, nil
	}
	if run.applied {
		assignHandles(store.tasks, store.vocab)
		if err := store.Save(); err != nil {
			return fabricengine.BoardReplayResult{}, fmt.Errorf("save store: %w", err)
		}
	}
	if err := RenderToDisk(req.BoardDir, store.Tasks(), store.primeOwner, cfg.Outputs(), time.Now()); err != nil {
		return fabricengine.BoardReplayResult{}, fmt.Errorf("render: %w", err)
	}
	return run.result, nil
}

func (r *replayRun) lost(entry, field, reason, local string) {
	r.result.Lost = append(r.result.Lost, fabricengine.BoardReplayFinding{Entry: entry, Field: field, Reason: reason, Local: local})
}

func (r *replayRun) refuse(entry, field string, err error) {
	r.result.Refused = append(r.result.Refused, fabricengine.BoardReplayFinding{Entry: entry, Field: field, Reason: err.Error()})
}

// originEntry returns the store's current entry for slug and its index, or false when the store holds none.
func (r *replayRun) originEntry(slug string) (Task, int, bool) {
	for i, task := range r.store.tasks {
		if task.Slug == slug {
			return task, i, true
		}
	}
	return Task{}, -1, false
}

// refusedDependency returns the slug in entry's depends_on that a refused change added, and whether there is one.
func (r *replayRun) refusedDependency(entry Task) (string, bool) {
	for _, dep := range entry.DependsOn {
		if r.refusedAdds[dep] {
			return dep, true
		}
	}
	return "", false
}

// addEntry appends commit's entry with a fresh id and no handle when origin holds no entry of its slug.
// Where origin holds one, each field commit's entry carries differently is reported lost.
func (r *replayRun) addEntry(commit Task) {
	if origin, _, held := r.originEntry(commit.Slug); held {
		for _, field := range replayFields {
			if local := field.get(commit); !reflect.DeepEqual(field.get(origin), local) {
				r.lost(commit.Slug, field.name, "origin added an entry with this slug too", replayValue(local))
			}
		}
		return
	}
	incoming := cloneTasks([]Task{commit})[0]
	incoming.ID = r.store.nextID()
	// The local handle may already be origin's for another entry; Apply assigns one free on origin's board.
	incoming.Handle = ""
	incoming.Labels, incoming.Issues, incoming.DependsOn = orEmpty(incoming.Labels), orEmpty(incoming.Issues), orEmpty(incoming.DependsOn)
	if dep, refused := r.refusedDependency(incoming); refused {
		r.refusedAdds[incoming.Slug] = true
		r.refuse(incoming.Slug, "", fmt.Errorf("depends on %q, whose own change was refused", dep))
		return
	}
	if err := normalizeTask(&incoming); err != nil {
		r.refusedAdds[incoming.Slug] = true
		r.refuse(incoming.Slug, "", err)
		return
	}
	if err := r.store.validateWrite(r.store.tasks, incoming); err != nil {
		r.refusedAdds[incoming.Slug] = true
		r.refuse(incoming.Slug, "", err)
		return
	}
	r.store.tasks = append(r.store.tasks, incoming)
	r.applied = true
}

// changePrimeOwner applies the commit's prime_owner change against its parent onto origin's record.
// A record origin already holds is skipped, and one origin changed since the merge base to a third value stands and is reported lost.
func (r *replayRun) changePrimeOwner(base, before, commit string) {
	if before == commit {
		return
	}
	current := r.store.primeOwner
	if current == commit {
		return
	}
	if current != base && current != before {
		r.lost("prime", "prime_owner", "origin changed the field too", replayValue(commit))
		return
	}
	r.store.primeOwner = commit
	r.applied = true
}

// changeEntry applies the fields commit changed against before onto origin's entry as one change.
// A field origin already holds is skipped, a field origin changed since the merge base to a third value is reported lost and origin's value kept, and an entry origin removed reports every change lost.
func (r *replayRun) changeEntry(before, commit Task) {
	var changed []replayField
	for _, field := range replayFields {
		if !reflect.DeepEqual(field.get(before), field.get(commit)) {
			changed = append(changed, field)
		}
	}
	if len(changed) == 0 {
		return
	}

	origin, index, held := r.originEntry(commit.Slug)
	if !held {
		reason := "origin does not hold the entry"
		if _, inBase := r.base[commit.Slug]; inBase {
			reason = "origin removed the entry"
		}
		r.lost(commit.Slug, "", reason, "")
		return
	}

	incoming := cloneTasks([]Task{origin})[0]
	base := r.base[commit.Slug]
	var applied []string
	for _, field := range changed {
		current, local := field.get(origin), field.get(commit)
		if reflect.DeepEqual(current, local) {
			continue
		}
		// An origin value equal to the commit's parent is the state the commit was made on, such as an earlier commit of the same replay.
		if !reflect.DeepEqual(current, field.get(base)) && !reflect.DeepEqual(current, field.get(before)) {
			r.lost(commit.Slug, field.name, "origin changed the field too", replayValue(local))
			continue
		}
		field.set(&incoming, commit)
		applied = append(applied, field.name)
	}
	if len(applied) == 0 {
		return
	}
	if commit.Revised.After(incoming.Revised) {
		incoming.Revised = commit.Revised
	}
	label := strings.Join(applied, ", ")
	if dep, refused := r.refusedDependency(incoming); refused {
		r.refuse(commit.Slug, label, fmt.Errorf("depends on %q, whose own change was refused", dep))
		return
	}
	if err := normalizeTask(&incoming); err != nil {
		r.refuse(commit.Slug, label, err)
		return
	}
	if err := r.store.validateWrite(r.store.tasks, incoming); err != nil {
		r.refuse(commit.Slug, label, err)
		return
	}
	after := cloneTasks(r.store.tasks)
	after[index] = incoming
	if err := checkRunLocks(r.origin, after); err != nil {
		r.refuse(commit.Slug, label, err)
		return
	}
	r.store.tasks[index] = incoming
	r.applied = true
}

// removeEntry removes before's entry from origin's board when origin still holds it unchanged.
// An entry origin changed is kept and reported lost, and a removal the run lock or a dependent entry refuses is reported refused.
func (r *replayRun) removeEntry(before Task) {
	origin, index, held := r.originEntry(before.Slug)
	if !held {
		return
	}
	for _, field := range replayFields {
		if !reflect.DeepEqual(field.get(origin), field.get(before)) {
			r.lost(before.Slug, "", "origin changed the entry this commit removes", "")
			return
		}
	}
	for _, task := range r.store.tasks {
		if slices.Contains(task.DependsOn, before.Slug) {
			r.refuse(before.Slug, "", fmt.Errorf("dangling dependency: %q depends on the entry", task.Slug))
			return
		}
	}
	after := slices.Delete(cloneTasks(r.store.tasks), index, index+1)
	if err := checkRunLocks(r.origin, after); err != nil {
		r.refuse(before.Slug, "", err)
		return
	}
	r.store.tasks = slices.Delete(r.store.tasks, index, index+1)
	r.applied = true
}

// applyDesignFile writes the commit's side of one design file into boardDir when origin's file is still as the merge base or the commit's parent had it, and reports whether it wrote.
// A file origin changed too stays as origin holds it and is reported lost.
func (r *replayRun) applyDesignFile(boardDir string, file fabricengine.BoardDesignFile) (bool, error) {
	if file.Name == "" || file.Name != filepath.Base(file.Name) {
		r.refuse(file.Name, "design file", fmt.Errorf("design file name %q is not a plain file name", file.Name))
		return false, nil
	}
	path := filepath.Join(boardDir, file.Name)
	current, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		current, err = nil, nil
	}
	if err != nil {
		return false, fmt.Errorf("read design file: %w", err)
	}
	if sameFile(current, file.After) {
		return false, nil
	}
	if !sameFile(current, file.Base) && !sameFile(current, file.Before) {
		r.lost(file.Name, "design file", "origin changed the file too", "")
		return false, nil
	}
	if file.After == nil {
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("remove design file: %w", err)
		}
		return true, nil
	}
	if err := fsx.AtomicWrite(boardDir, file.Name, string(file.After)); err != nil {
		return false, fmt.Errorf("write design file: %w", err)
	}
	return true, nil
}

// sameFile reports whether two file contents are equal, an absent file (nil) differing from an empty one.
func sameFile(a, b []byte) bool {
	return (a == nil) == (b == nil) && bytes.Equal(a, b)
}
