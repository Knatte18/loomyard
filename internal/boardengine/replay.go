// replay.go — FabricReplay, the board's Bolt replayer and run ledger: the by-entry merge of a local board commit onto origin's board.
//
// It is the board's implementation of the registry fabricengine declares for a records replay: a commit's entry-level changes are re-applied one by one onto the board checked out at origin's tip, each through the board's own write checks, so a change only one side made never conflicts.
// The by-entry merge itself is internal/entrymerge's.

package boardengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/entrymerge"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// The board's file names a replay recognises: the rendered overview and the per-entry design files.
// The per-project overview files take projectFilePrefix, and the entry store boardFile.
const (
	overviewFile     = "README.md"
	designFilePrefix = "design-"
	designFileSuffix = ".md"
)

// FabricReplay is the board's Bolt replayer, named board, and fabricengine's run ledger over this package's store.
// It holds no state: Apply takes the board config from the request's directory.
type FabricReplay struct{}

// Name returns the replayer's registry name.
func (FabricReplay) Name() string { return "board" }

// Claims reports whether path, a Bolt-root-relative slash path, is a file of the board: the entry store, a rendered overview file or a design file.
func (FabricReplay) Claims(path string) bool {
	if path == boardFile || isBoardOverview(path) {
		return true
	}
	_, design := boardDesignSlug(path)
	return design
}

// File returns the Bolt file holding the board's entries, the run ledger's file.
func (FabricReplay) File() string { return boardFile }

// boardDesignSlug returns the slug of the entry a design file belongs to, and reports whether path is a design file:
// `design-<slug>.md` at the Bolt root.
func boardDesignSlug(path string) (string, bool) {
	name, found := strings.CutPrefix(path, designFilePrefix)
	if !found || strings.Contains(path, "/") {
		return "", false
	}
	slug, found := strings.CutSuffix(name, designFileSuffix)
	return slug, found && slug != ""
}

// isBoardOverview reports whether path is a rendered overview file at the Bolt root: `README.md` or a project's `project-<slug>.md`.
// The board re-renders every overview after a replay, so none names an entry change.
func isBoardOverview(path string) bool {
	if path == overviewFile {
		return true
	}
	name, found := strings.CutPrefix(path, projectFilePrefix)
	slug, isMarkdown := strings.CutSuffix(name, designFileSuffix)
	return found && isMarkdown && slug != "" && !strings.Contains(path, "/")
}

// entriesBySlug decodes the raw entries of a `board.json`, keyed by slug and compacted so two writes of the same entry compare equal.
// An absent or empty file holds none.
// It reads only the entries' slugs, never their meaning.
func entriesBySlug(data []byte) (map[string]string, error) {
	entries := map[string]string{}
	if len(bytes.TrimSpace(data)) == 0 {
		return entries, nil
	}
	var file struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	for _, raw := range file.Entries {
		var key struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(raw, &key); err != nil {
			return nil, err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return nil, err
		}
		entries[key.Slug] = compact.String()
	}
	return entries, nil
}

// CommitRuns returns the slugs of the entries a commit of paths changes, sorted:
// each entry of `board.json` that differs between before and after, and each design file's entry.
// before and after are the entry store as the commit's parent and the commit hold it, read only when paths names the entry store.
// An overview file names no entry.
func (FabricReplay) CommitRuns(before, after []byte, paths []string) ([]string, error) {
	slugs := map[string]bool{}
	for _, path := range paths {
		if slug, design := boardDesignSlug(path); design {
			slugs[slug] = true
		}
	}
	if slices.Contains(paths, boardFile) {
		beforeEntries, err := entriesBySlug(before)
		if err != nil {
			return nil, fmt.Errorf("parent %s: %w", boardFile, err)
		}
		afterEntries, err := entriesBySlug(after)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", boardFile, err)
		}
		for slug, entry := range afterEntries {
			if beforeEntries[slug] != entry {
				slugs[slug] = true
			}
		}
		for slug := range beforeEntries {
			if _, kept := afterEntries[slug]; !kept {
				slugs[slug] = true
			}
		}
	}
	result := make([]string, 0, len(slugs))
	for slug := range slugs {
		result = append(result, slug)
	}
	slices.Sort(result)
	return result, nil
}

// replayFields lists every entry field a replay merges, in the order a change reports them.
// Id, handle, created, revised and unknown keys are not merged as fields: the id and handle are origin's, the dates follow the changes, and an unknown key stays as origin holds it.
var replayFields = entrymerge.Table[Task]{
	Key: func(t Task) string { return t.Slug },
	Fields: []entrymerge.Field[Task]{
		{Name: "title", Get: func(t Task) any { return t.Title }, Set: func(dst *Task, src Task) { dst.Title = src.Title }},
		{Name: "kind", Get: func(t Task) any { return t.Kind }, Set: func(dst *Task, src Task) { dst.Kind = src.Kind }},
		{Name: "labels", Get: func(t Task) any { return orEmpty(t.Labels) }, Set: func(dst *Task, src Task) { dst.Labels = slices.Clone(src.Labels) }},
		{Name: "issues", Get: func(t Task) any { return orEmpty(t.Issues) }, Set: func(dst *Task, src Task) { dst.Issues = slices.Clone(src.Issues) }},
		{Name: "recipe", Get: func(t Task) any { return t.Recipe }, Set: func(dst *Task, src Task) { dst.Recipe = src.Recipe }},
		{Name: "priority", Get: func(t Task) any { return t.Priority }, Set: func(dst *Task, src Task) { dst.Priority = src.Priority }},
		{Name: "depends_on", Get: func(t Task) any { return orEmpty(t.DependsOn) }, Set: func(dst *Task, src Task) { dst.DependsOn = slices.Clone(src.DependsOn) }},
		{Name: "isolated", Get: func(t Task) any { return t.Isolated }, Set: func(dst *Task, src Task) { dst.Isolated = src.Isolated }},
		{Name: "brief", Get: func(t Task) any { return t.Brief }, Set: func(dst *Task, src Task) { dst.Brief = src.Brief }},
		{Name: "body", Get: func(t Task) any { return t.Body }, Set: func(dst *Task, src Task) { dst.Body = src.Body }},
		{Name: "status", Get: func(t Task) any { return statusText(t.Status) }, Set: func(dst *Task, src Task) { dst.Status = cloneTasks([]Task{src})[0].Status }},
		{Name: "short_name", Get: func(t Task) any { return t.ShortName }, Set: func(dst *Task, src Task) { dst.ShortName = src.ShortName }},
		{Name: "run_settings", Get: func(t Task) any { return orNilSettings(t.RunSettings) }, Set: func(dst *Task, src Task) { dst.RunSettings = cloneSettings(src.RunSettings) }},
		{Name: "profile", Get: func(t Task) any { return t.Profile }, Set: func(dst *Task, src Task) { dst.Profile = src.Profile }},
		{Name: "pull_request", Get: func(t Task) any { return t.PullRequest }, Set: func(dst *Task, src Task) { dst.PullRequest = src.PullRequest }},
		{Name: "notify_on_land", Get: func(t Task) any { return t.NotifyOnLand }, Set: func(dst *Task, src Task) { dst.NotifyOnLand = src.NotifyOnLand }},
		{Name: "owner_machine", Get: func(t Task) any { return t.OwnerMachine }, Set: func(dst *Task, src Task) { dst.OwnerMachine = src.OwnerMachine }},
		{Name: "project", Get: func(t Task) any { return t.Project }, Set: func(dst *Task, src Task) { dst.Project = src.Project }},
		{Name: "order", Get: func(t Task) any { return orEmpty(t.Order) }, Set: func(dst *Task, src Task) { dst.Order = slices.Clone(src.Order) }},
		{Name: "beam", Get: func(t Task) any { return t.Beam }, Set: func(dst *Task, src Task) { dst.Beam = src.Beam }},
		{Name: "front", Get: func(t Task) any { return t.Front }, Set: func(dst *Task, src Task) { dst.Front = src.Front }},
		{Name: "on_land", Get: func(t Task) any { return t.OnLand }, Set: func(dst *Task, src Task) { dst.OnLand = src.OnLand }},
		{Name: "beam_color", Get: func(t Task) any { return t.BeamColor }, Set: func(dst *Task, src Task) { dst.BeamColor = src.BeamColor }},
	},
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

// parseDraftStatus decodes the top-level draft_status of a board.json read from a commit, false for an absent or empty file or an absent key.
func parseDraftStatus(data []byte) (bool, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return false, nil
	}
	var decoded struct {
		DraftStatus bool `json:"draft_status"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return false, err
	}
	return decoded.DraftStatus, nil
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

// Runs decodes data, a `board.json`, into each entry's slug, whether it is done and its created stamp, in file order.
// Empty data holds no entry.
func (FabricReplay) Runs(data []byte) ([]fabricengine.LedgerRun, error) {
	_, tasks, err := parseReplayBoard(data)
	if err != nil {
		return nil, err
	}
	runs := make([]fabricengine.LedgerRun, len(tasks))
	for i, task := range tasks {
		runs[i] = fabricengine.LedgerRun{Slug: task.Slug, Ended: isDone(task), Created: task.Created}
	}
	return runs, nil
}

// replayRun is the state of one Apply: origin's store, the three sides of the commit and what has been applied or set aside so far.
type replayRun struct {
	store *Store
	// origin is the entries as origin's tip holds them, the baseline the run lock judges each change's net effect against.
	origin      []Task
	base        map[string]Task
	refusedAdds map[string]bool
	result      fabricengine.ReplayResult
	applied     bool
}

// Apply re-applies the request's commit onto the board directory by entry and design file, reading the entry store and the design files out of its files by path, and reports what it kept and what it set aside.
// It loads the board config from the request's directory and a store from origin's board.json there, and applies each change of the commit against its parent on its own, against the board as the kept changes before it left it.
// The hub's prime_owner record replays like an entry field, reported lost under the entry name prime.
// Where both sides changed the same field of an entry or the same design file since the merge base, or the change touches an entry origin removed, origin's side is kept for that change and it is reported lost.
// A change the board's own checks refuse, the run lock included, is reported refused and so is a later change that depends on an entry a refused change added; the rest of the commit still lands.
// An added entry takes a handle free on origin's board, and a changed entry keeps origin's handle unless its letter changed.
// It saves board.json and re-renders the board's output files when anything was applied, and writes nothing otherwise.
// It takes no lock and spawns no sync, since its caller holds the board and push locks.
func (FabricReplay) Apply(req fabricengine.ReplayRequest) (fabricengine.ReplayResult, error) {
	cfg, err := LoadConfig(configengine.Dirs{Board: req.Dir}, "board")
	if err != nil {
		return fabricengine.ReplayResult{}, err
	}
	vocabulary := cfg.Vocabulary()

	var baseJSON, parentJSON, commitJSON []byte
	var designFiles []fabricengine.ReplayFile
	for _, file := range req.Files {
		if file.Path == boardFile {
			baseJSON, parentJSON, commitJSON = file.Base, file.Before, file.After
		} else if _, design := boardDesignSlug(file.Path); design {
			designFiles = append(designFiles, file)
		}
	}

	base, _, err := parseReplayBoard(baseJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("merge base %s: %w", boardFile, err)
	}
	parent, parentTasks, err := parseReplayBoard(parentJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("parent %s: %w", boardFile, err)
	}
	_, commitTasks, err := parseReplayBoard(commitJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("commit %s: %w", boardFile, err)
	}

	basePrime, err := parsePrimeOwner(baseJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("merge base %s: %w", boardFile, err)
	}
	parentPrime, err := parsePrimeOwner(parentJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("parent %s: %w", boardFile, err)
	}
	commitPrime, err := parsePrimeOwner(commitJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("commit %s: %w", boardFile, err)
	}

	commitDraft, err := parseDraftStatus(commitJSON)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("commit %s: %w", boardFile, err)
	}

	store := NewStore(req.Dir)
	store.vocab = &vocabulary
	if err := store.Load(); err != nil {
		return fabricengine.ReplayResult{}, err
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
	if commitDraft && !store.draftStatus {
		store.draftStatus = true
		run.applied = true
	}

	designsApplied := false
	for _, file := range designFiles {
		applied, err := run.applyDesignFile(req.Dir, file)
		if err != nil {
			return fabricengine.ReplayResult{}, err
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
			return fabricengine.ReplayResult{}, fmt.Errorf("save store: %w", err)
		}
	}
	if err := RenderToDisk(req.Dir, store.Tasks(), store.primeOwner, cfg.Outputs(), time.Now()); err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("render: %w", err)
	}
	return run.result, nil
}

func (r *replayRun) lost(entry, field, reason, local string) {
	r.result.Lost = append(r.result.Lost, fabricengine.ReplayFinding{Entry: entry, Field: field, Reason: reason, Local: local})
}

func (r *replayRun) refuse(entry, field string, err error) {
	r.result.Refused = append(r.result.Refused, fabricengine.ReplayFinding{Entry: entry, Field: field, Reason: err.Error()})
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
		for _, clash := range replayFields.AddClash(origin, commit) {
			r.lost(clash.Entry, clash.Field, clash.Reason, clash.Local)
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
		r.lost("prime", "prime_owner", "origin changed the field too", entrymerge.Render(commit))
		return
	}
	r.store.primeOwner = commit
	r.applied = true
}

// changeEntry applies the fields commit changed against before onto origin's entry as one change.
// A field origin already holds is skipped, a field origin changed since the merge base to a third value is reported lost and origin's value kept, and an entry origin removed reports every change lost.
func (r *replayRun) changeEntry(before, commit Task) {
	if len(replayFields.Changed(before, commit)) == 0 {
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

	incoming, applied, losses := replayFields.Change(r.base[commit.Slug], before, cloneTasks([]Task{origin})[0], commit)
	for _, loss := range losses {
		r.lost(loss.Entry, loss.Field, loss.Reason, loss.Local)
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
	if !replayFields.Same(origin, before) {
		r.lost(before.Slug, "", "origin changed the entry this commit removes", "")
		return
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
func (r *replayRun) applyDesignFile(boardDir string, file fabricengine.ReplayFile) (bool, error) {
	if file.Path == "" || file.Path != filepath.Base(file.Path) {
		r.refuse(file.Path, "design file", fmt.Errorf("design file name %q is not a plain file name", file.Path))
		return false, nil
	}
	path := filepath.Join(boardDir, file.Path)
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
		r.lost(file.Path, "design file", "origin changed the file too", "")
		return false, nil
	}
	if file.After == nil {
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("remove design file: %w", err)
		}
		return true, nil
	}
	if err := fsx.AtomicWrite(boardDir, file.Path, string(file.After)); err != nil {
		return false, fmt.Errorf("write design file: %w", err)
	}
	return true, nil
}

// sameFile reports whether two file contents are equal, an absent file (nil) differing from an empty one.
func sameFile(a, b []byte) bool {
	return (a == nil) == (b == nil) && bytes.Equal(a, b)
}
