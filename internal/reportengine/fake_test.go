// fake_test.go holds fakeRepo, an in-memory Reader, and the builders that put tags, loom records and batten snapshots into it.
// A fake holds one main line of commits, which Log walks, and any number of commits off it, which only a tag names, as an archive tag names a commit off the records branch.

package reportengine

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabricvocab"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/haltnote"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// fakeCommit is one commit: its committer time, message, parents and the whole tree it holds.
// The first parent is the commit a first-parent walk follows.
type fakeCommit struct {
	committed time.Time
	message   string
	parents   []string
	tree      string
	files     map[string]string
}

// blameCall is one BlameLines call a fakeRepo received.
type blameCall struct {
	rev, path string
	lines     []int
}

// fakeRepo is an in-memory Reader.
type fakeRepo struct {
	commits map[string]fakeCommit
	// chain lists the main line's commits, oldest first; its last commit is the tip Log is asked for.
	chain []string
	// preImages and blames stand in for PreImageLines and BlameLines, keyed by commit and by revision and path.
	preImages map[string]map[string][]int
	blames    map[string]map[int]string
	// blameCalls logs every BlameLines call in order.
	blameCalls []blameCall
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{commits: map[string]fakeCommit{}, preImages: map[string]map[string][]int{}, blames: map[string]map[int]string{}}
}

// commitOnChain appends a commit to the main line whose tree is the previous commit's with set written and remove deleted.
func (f *fakeRepo) commitOnChain(sha string, at time.Time, set map[string]string, remove ...string) {
	f.commitOnChainWith(sha, at, "", nil, set, remove...)
}

// commitOnChainWith is commitOnChain with a message and further parents, the first parent being the previous commit.
func (f *fakeRepo) commitOnChainWith(sha string, at time.Time, message string, otherParents []string, set map[string]string, remove ...string) {
	files := map[string]string{}
	var parents []string
	if len(f.chain) > 0 {
		last := f.chain[len(f.chain)-1]
		parents = append(parents, last)
		for name, content := range f.commits[last].files {
			files[name] = content
		}
	}
	parents = append(parents, otherParents...)
	for name, content := range set {
		files[name] = content
	}
	for _, name := range remove {
		delete(files, name)
	}
	f.commits[sha] = fakeCommit{committed: at, message: message, parents: parents, tree: "tree-" + sha, files: files}
	f.chain = append(f.chain, sha)
}

// commitOffChain adds a commit outside the main line holding exactly files, optionally on top of parent.
func (f *fakeRepo) commitOffChain(sha string, at time.Time, files map[string]string) {
	f.commitOffChainOn(sha, "", at, "", files)
}

// commitOffChainOn is commitOffChain with a parent and a message.
func (f *fakeRepo) commitOffChainOn(sha, parent string, at time.Time, message string, files map[string]string) {
	var parents []string
	if parent != "" {
		parents = []string{parent}
	}
	f.commits[sha] = fakeCommit{committed: at, message: message, parents: parents, tree: "tree-" + sha, files: files}
}

// setTree gives sha the tree name tree, so two commits can hold one tree.
func (f *fakeRepo) setTree(sha, tree string) {
	commit := f.commits[sha]
	commit.tree = tree
	f.commits[sha] = commit
}

func (f *fakeRepo) tree(rev string) (fakeCommit, error) {
	commit, ok := f.commits[rev]
	if !ok {
		return fakeCommit{}, fmt.Errorf("fakeRepo: unknown commit %q", rev)
	}
	return commit, nil
}

func (f *fakeRepo) FileAtRevision(rev, relPath string) ([]byte, error) {
	commit, err := f.tree(rev)
	if err != nil {
		return nil, err
	}
	content, ok := commit.files[relPath]
	if !ok {
		return nil, gitrepo.ErrPathNotAtRevision
	}
	return []byte(content), nil
}

// directChildren returns the sorted names directly in dir of rev's tree: the files, or when dirs is set the directories.
func (f *fakeRepo) directChildren(rev, dir string, dirs bool) ([]string, error) {
	commit, err := f.tree(rev)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for name := range commit.files {
		rest := name
		if dir != "" && dir != "." {
			var under bool
			if rest, under = strings.CutPrefix(name, dir+"/"); !under {
				continue
			}
		}
		first, deeper, nested := strings.Cut(rest, "/")
		if nested && dirs && deeper != "" {
			seen[first] = true
		}
		if !nested && !dirs {
			seen[first] = true
		}
	}
	var names []string
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (f *fakeRepo) FilesInDirAtRevision(rev, dir string) ([]string, error) {
	return f.directChildren(rev, dir, false)
}

func (f *fakeRepo) SubdirsAtRevision(rev, dir string) ([]string, error) {
	return f.directChildren(rev, dir, true)
}

// changedPaths returns the sorted paths whose content differs between sha and its first parent, every path for a root commit.
func (f *fakeRepo) changedPaths(sha string) []string {
	current := f.commits[sha].files
	var previous map[string]string
	if parents := f.commits[sha].parents; len(parents) > 0 {
		previous = f.commits[parents[0]].files
	}
	var changed []string
	for name, content := range current {
		if old, ok := previous[name]; !ok || old != content {
			changed = append(changed, name)
		}
	}
	for name := range previous {
		if _, ok := current[name]; !ok {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

func (f *fakeRepo) Log(tip string, q gitrepo.LogQuery) ([]gitrepo.LogCommit, error) {
	if _, known := f.commits[tip]; !known {
		return nil, fmt.Errorf("fakeRepo: unknown commit %q", tip)
	}
	var commits []gitrepo.LogCommit
	for sha := tip; sha != ""; {
		commit := f.commits[sha]
		next := ""
		if len(commit.parents) > 0 {
			next = commit.parents[0]
		}
		current := sha
		sha = next

		committed := commit.committed
		if !q.Since.IsZero() && committed.Before(q.Since) {
			if q.FirstParent {
				break
			}
			continue
		}
		entry := gitrepo.LogCommit{SHA: current, Parents: commit.parents, Tree: commit.tree, Committed: committed, Message: commit.message}
		if q.PathPrefix != "" {
			for _, name := range f.changedPaths(current) {
				if name == q.PathPrefix || strings.HasPrefix(name, q.PathPrefix+"/") {
					entry.Paths = append(entry.Paths, name)
				}
			}
			if len(entry.Paths) == 0 {
				continue
			}
		}
		commits = append(commits, entry)
		if q.Limit > 0 && len(commits) >= q.Limit {
			break
		}
	}
	return commits, nil
}

func (f *fakeRepo) PreImageLines(sha string) (map[string][]int, error) {
	return f.preImages[sha], nil
}

func (f *fakeRepo) BlameLines(rev, relPath string, lines []int) (map[int]string, error) {
	f.blameCalls = append(f.blameCalls, blameCall{rev: rev, path: relPath, lines: slices.Clone(lines)})
	attributed := map[int]string{}
	for _, line := range lines {
		if sha, ok := f.blames[rev+" "+relPath][line]; ok {
			attributed[line] = sha
		}
	}
	return attributed, nil
}

// at returns the UTC time hours after Monday 2026-01-05 00:00, the first day of an ISO week.
func at(hours int) time.Time {
	return time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC).Add(time.Duration(hours) * time.Hour)
}

// historyOf builds a history whose entries are done calls of the producers.
func historyOf(producers ...string) []shedengine.HistoryEntry {
	history := make([]shedengine.HistoryEntry, len(producers))
	for i, producer := range producers {
		history[i] = shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Done, At: at(i).Format(time.RFC3339)}
	}
	return history
}

// statusText encodes a status holding the history of producers.
func statusText(t *testing.T, producers ...string) string {
	t.Helper()

	return statusJSON(t, shedengine.Status{State: shedengine.StateDone, History: historyOf(producers...)})
}

// statusJSON encodes status.
func statusJSON(t *testing.T, status shedengine.Status) string {
	t.Helper()

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return string(data)
}

// stateChange is the history entry batten records for a child state change at hour.
func stateChange(from, to shedengine.State, producer string, hour int) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{
		Producer:     "Run-Shed",
		Outcome:      shedengine.Done,
		Output:       fmt.Sprintf("child %s → %s at %s", from, to, producer),
		At:           at(hour).Format(time.RFC3339),
		BudgetExempt: true,
	}
}

// battenStatusOf is a batten status that opens with the worktree-create row and then holds entries.
func battenStatusOf(state shedengine.State, current string, entries ...shedengine.HistoryEntry) shedengine.Status {
	history := append([]shedengine.HistoryEntry{{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done, At: at(0).Format(time.RFC3339)}}, entries...)
	return shedengine.Status{State: state, CurrentProducer: current, History: history}
}

// mergeFiles returns the union of the file sets.
func mergeFiles(sets ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, set := range sets {
		for name, content := range set {
			merged[name] = content
		}
	}
	return merged
}

// reviewFiles returns empty review files named names in the review segment directory segment.
func reviewFiles(segment string, names ...string) map[string]string {
	files := map[string]string{}
	for _, name := range names {
		files[path.Join(filepath.ToSlash(loomengine.LoomReviewsDirRel()), segment, name)] = ""
	}
	return files
}

// parentRoundFiles returns the files of parent-review round number: a delivery stamped when delivered, with pings keep-alives, an alignment record in state alignmentState unless that is empty, and a verdict of kind verdictKind unless that is empty.
func parentRoundFiles(number int, delivered bool, pings int, alignmentState, verdictKind string) map[string]string {
	dir := path.Join(filepath.ToSlash(loomengine.LoomParentReviewDirRel()), fmt.Sprintf("round-%d", number))
	delivery := fmt.Sprintf(`{"pings":%d}`, pings)
	if delivered {
		delivery = fmt.Sprintf(`{"delivered_at":%q,"pings":%d}`, at(1).Format(time.RFC3339), pings)
	}
	files := map[string]string{
		path.Join(dir, "request.json"):  fmt.Sprintf(`{"round":%d,"state":"open"}`, number),
		path.Join(dir, "delivery.json"): delivery,
	}
	if alignmentState != "" {
		files[path.Join(dir, "alignment.json")] = fmt.Sprintf(`{"state":%q}`, alignmentState)
	}
	if verdictKind != "" {
		files[path.Join(dir, "verdict.json")] = fmt.Sprintf(`{"kind":%q}`, verdictKind)
	}
	return files
}

// arbiterAttemptFiles returns the files of attempt number of slug's arbiter: attempt.json holding attempt, a verdict file unless verdictYAML is empty, and the applied mark when applied.
func arbiterAttemptFiles(t *testing.T, slug string, number int, attempt loomengine.ArbiterAttempt, verdictYAML string, applied bool) map[string]string {
	t.Helper()

	attempt.Number = number
	data, err := json.Marshal(attempt)
	if err != nil {
		t.Fatalf("marshal arbiter attempt: %v", err)
	}
	dir := path.Join(recordPaths(t, slug).Arbiter, fmt.Sprintf("attempt-%d", number))
	files := map[string]string{path.Join(dir, "attempt.json"): string(data)}
	if verdictYAML != "" {
		files[path.Join(dir, "verdict.yaml")] = verdictYAML
	}
	if applied {
		files[path.Join(dir, "applied")] = ""
	}
	return files
}

// driveReportFiles returns count drive reports of slug.
func driveReportFiles(t *testing.T, slug string, count int) map[string]string {
	t.Helper()

	files := map[string]string{}
	for i := 1; i <= count; i++ {
		files[path.Join(recordPaths(t, slug).DriveReports, fmt.Sprintf("attempt-%d.md", i))] = "report"
	}
	return files
}

// decisionRecordFiles returns the discussion's decision record holding the given lines after its Decisions heading.
func decisionRecordFiles(lines ...string) map[string]string {
	text := "# Decision record\n\n## Decisions\n\n"
	for _, line := range lines {
		text += line + "\n\n"
	}
	return map[string]string{filepath.ToSlash(loomengine.DiscussionDecisionRecordRel()): text}
}

// addedDecision is the heading line of an entry added after the discussion by author.
func addedDecision(author, title string) string {
	return fmt.Sprintf("### Added after Discussion (%s, 2026-01-05): %s", author, title)
}

// fullRun adds the archive tag of a full loom run of slug at hour holding extra files, with the batten snapshots committed before it and entries in the loom history before its Finalize.
func (w *world) fullRun(slug, sha string, hour int, snapshots []shedengine.Status, entries []shedengine.HistoryEntry, extra map[string]string) {
	for i, snapshot := range snapshots {
		w.battenRecord(fmt.Sprintf("%s-b%d", slug, i), hour-len(snapshots)+i, slug, snapshot)
	}
	history := append(entries, shedengine.HistoryEntry{Producer: loomshed.NameFinalize, Outcome: shedengine.Done, At: at(hour - 10).Format(time.RFC3339)})
	status := statusJSON(w.t, shedengine.Status{State: shedengine.StateDone, History: history})
	w.tagWith(slug, sha, at(hour), seedText("loom"), status, extra)
}

// alignmentAttemptFiles returns the files of attempt number of slug's alignment agent at point: attempt.json recording applied, and a verdict file unless verdict is empty.
func alignmentAttemptFiles(t *testing.T, slug, point string, number int, applied, verdict string, superseded ...string) map[string]string {
	t.Helper()

	dir := path.Join(recordPaths(t, slug).Alignment, point, fmt.Sprintf("attempt-%d", number))
	token := fmt.Sprintf("tok%d", number)
	files := map[string]string{
		path.Join(dir, "attempt.json"): fmt.Sprintf(`{"number":%d,"point":%q,"token":%q,"applied":%q}`, number, point, token, applied),
	}
	if verdict != "" {
		text := fmt.Sprintf("verdict: %s\nreason: because\n", verdict)
		if len(superseded) > 0 {
			text += "superseded:\n"
			for _, entry := range superseded {
				text += "  - " + entry + "\n"
			}
		}
		files[path.Join(dir, "verdict-"+token+".yaml")] = text
	}
	return files
}

// haltNoteFiles returns the halt note in the friction directory dir (the live one when dir is empty, else the archive sibling named dir) as file name.
func haltNoteFiles(dir, name string, note haltnote.Note) map[string]string {
	folder := filepath.ToSlash(loomengine.LoomFrictionDirRel())
	if dir != "" {
		folder = path.Join(filepath.ToSlash(loomengine.LoomDurableDirRel()), dir)
	}
	return map[string]string{path.Join(folder, name): string(haltnote.Render(note))}
}

// seedText is the text of a loom seed naming recipe.
func seedText(recipe string) string {
	return fmt.Sprintf(`{"recipe":%q,"driver":"go"}`, recipe)
}

// recordPaths returns the tree-relative seed and status paths of slug.
func recordPaths(t *testing.T, slug string) shedrun.RunTreeRels {
	t.Helper()

	rels, err := shedrun.TreeRunRels(slug)
	if err != nil {
		t.Fatalf("TreeRunRels(%q): %v", slug, err)
	}
	return rels
}

// world is a fake records repository and the archive tags naming commits in it.
type world struct {
	t    *testing.T
	repo *fakeRepo
	tags []gitrepo.TagRef

	// code is the parent branch's repository and board the board's, each a main line.
	code  *fakeRepo
	board *fakeRepo
}

// newWorld returns a world whose records, parent and board main lines each hold one empty base commit, so every tip exists.
func newWorld(t *testing.T) *world {
	w := &world{t: t, repo: newFakeRepo(), code: newFakeRepo(), board: newFakeRepo()}
	w.repo.commitOnChain("base", at(-1000), nil)
	w.code.commitOnChain("code-base", at(-1000), nil)
	w.board.commitOnChain("board-base", at(-1000), nil)
	return w
}

// trailerKey is the task-commit trailer key, spelled from the side tokens so no file of this package names one.
func trailerKey() string {
	token := fabricvocab.SideTokens()[1]
	return strings.ToUpper(token[:1]) + token[1:] + "-SHA"
}

// recordsCommit commits to the records main line at hour; its message names task in the trailer when task is not empty.
func (w *world) recordsCommit(sha string, hour int, task string) {
	message := "records"
	if task != "" {
		message += "\n\n" + trailerKey() + ": " + task
	}
	w.repo.commitOnChainWith(sha, at(hour), message, nil, map[string]string{"records-" + sha: ""})
}

// checkpoint commits the landing checkpoint of slug to the records main line at hour.
func (w *world) checkpoint(sha string, hour int, slug string) {
	w.repo.commitOnChainWith(sha, at(hour), loomengine.LandingCheckpointSubject(slug), nil, map[string]string{recordPaths(w.t, slug).Dir + "/checkpoint-" + sha: ""})
}

// parentCommit commits to the parent branch's main line at hour with the given message.
func (w *world) parentCommit(sha string, hour int, message string) {
	w.code.commitOnChainWith(sha, at(hour), message, nil, nil)
}

// parentMerge commits a merge of the commit second to the parent branch's main line at hour.
func (w *world) parentMerge(sha string, hour int, second string) {
	w.code.commitOnChainWith(sha, at(hour), "merge", []string{second}, nil)
}

// taskCommit adds a commit outside the parent branch's main line, as a task branch's commit before it landed.
func (w *world) taskCommit(sha string, hour int) {
	w.code.commitOffChainOn(sha, "", at(hour), "task", nil)
}

// boardTask is a board entry of slug with the given status and labels.
func boardTask(slug, status string, labels ...string) boardengine.Task {
	task := boardengine.Task{Slug: slug, Kind: boardengine.KindTask, Labels: labels}
	if status != "" {
		task.Status = &status
	}
	return task
}

// boardCommit commits a board holding tasks to the board's main line at hour.
func (w *world) boardCommit(sha string, hour int, tasks ...boardengine.Task) {
	data, err := json.Marshal(map[string]any{"version": 1, "entries": tasks})
	if err != nil {
		w.t.Fatalf("marshal board: %v", err)
	}
	w.board.commitOnChain(sha, at(hour), map[string]string{boardengine.BoardFileRel(): string(data)})
}

// battenStatus commits slug's batten status holding the history of producers to the main line at hour.
func (w *world) battenStatus(sha string, hour int, slug string, producers ...string) {
	w.repo.commitOnChain(sha, at(hour), map[string]string{recordPaths(w.t, slug).Status: statusText(w.t, producers...)})
}

// battenRecord commits slug's batten status, exactly status, to the main line at hour.
func (w *world) battenRecord(sha string, hour int, slug string, status shedengine.Status) {
	w.repo.commitOnChain(sha, at(hour), map[string]string{recordPaths(w.t, slug).Status: statusJSON(w.t, status)})
}

// battenDeleted commits the removal of slug's batten status to the main line at hour.
func (w *world) battenDeleted(sha string, hour int, slug string) {
	w.repo.commitOnChain(sha, at(hour), nil, recordPaths(w.t, slug).Status)
}

// tag adds the archive tag of slug naming a commit off the main line at when, holding the given seed and status text; an empty text leaves that file out.
func (w *world) tag(slug, sha string, when time.Time, seed, status string) {
	w.tagWith(slug, sha, when, seed, status, nil)
}

// tagWith is tag with extra files, keyed by tree-relative path, in the tagged commit's tree.
func (w *world) tagWith(slug, sha string, when time.Time, seed, status string, extra map[string]string) {
	w.tagOver(slug, sha, "", when, seed, status, extra)
}

// tagOver is tagWith with the tagged commit on top of parent, a commit of the records main line, so the history below the tag is walkable.
func (w *world) tagOver(slug, sha, parent string, when time.Time, seed, status string, extra map[string]string) {
	rels := recordPaths(w.t, slug)
	files := map[string]string{}
	for name, content := range extra {
		files[name] = content
	}
	if seed != "" {
		files[rels.Seed] = seed
	}
	if status != "" {
		files[rels.Status] = status
	}
	w.repo.commitOffChainOn(sha, parent, when, "", files)
	w.tags = append(w.tags, gitrepo.TagRef{Name: fabricengine.ArchiveTagPrefix + slug + "/" + sha, Commit: sha, Committed: when})
}

// finishedRun adds the archive tag of a full loom run of slug at hour.
func (w *world) finishedRun(slug, sha string, hour int) {
	w.tag(slug, sha, at(hour), seedText("loom"), statusText(w.t, "Plan-Write", "Finalize"))
}

// sources returns the Sources the world presents, its tags ordered as an archive tag listing orders them.
func (w *world) sources() Sources {
	tags := slices.Clone(w.tags)
	sort.SliceStable(tags, func(i, j int) bool { return tags[i].Committed.Before(tags[j].Committed) })
	tipOf := func(repo *fakeRepo) string { return repo.chain[len(repo.chain)-1] }
	return Sources{
		Code: w.code, Records: w.repo, Board: w.board,
		ParentTip: tipOf(w.code), RecordsTip: tipOf(w.repo), BoardTip: tipOf(w.board),
		Tags: tags,
	}
}
