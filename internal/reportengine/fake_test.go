// fake_test.go holds fakeRepo, an in-memory Reader, and the builders that put tags, loom records and batten snapshots into it.
// A fake holds one main line of commits, which Log walks, and any number of commits off it, which only a tag names, as an archive tag names a commit off the records branch.

package reportengine

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// fakeCommit is one commit: its committer time and the whole tree it holds.
type fakeCommit struct {
	committed time.Time
	files     map[string]string
}

// fakeRepo is an in-memory Reader.
type fakeRepo struct {
	commits map[string]fakeCommit
	// chain lists the main line's commits, oldest first; its last commit is the tip Log is asked for.
	chain []string
	// preImages and blames stand in for PreImageLines and BlameLines, keyed by commit and by revision and path.
	preImages map[string]map[string][]int
	blames    map[string]map[int]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{commits: map[string]fakeCommit{}, preImages: map[string]map[string][]int{}, blames: map[string]map[int]string{}}
}

// commitOnChain appends a commit to the main line whose tree is the previous commit's with set written and remove deleted.
func (f *fakeRepo) commitOnChain(sha string, at time.Time, set map[string]string, remove ...string) {
	files := map[string]string{}
	if len(f.chain) > 0 {
		for name, content := range f.commits[f.chain[len(f.chain)-1]].files {
			files[name] = content
		}
	}
	for name, content := range set {
		files[name] = content
	}
	for _, name := range remove {
		delete(files, name)
	}
	f.commits[sha] = fakeCommit{committed: at, files: files}
	f.chain = append(f.chain, sha)
}

// commitOffChain adds a commit outside the main line holding exactly files.
func (f *fakeRepo) commitOffChain(sha string, at time.Time, files map[string]string) {
	f.commits[sha] = fakeCommit{committed: at, files: files}
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

// changedPaths returns the sorted paths whose content differs between the chain commit at index and the one before it, every path for the first.
func (f *fakeRepo) changedPaths(index int) []string {
	current := f.commits[f.chain[index]].files
	var previous map[string]string
	if index > 0 {
		previous = f.commits[f.chain[index-1]].files
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
	start := slices.Index(f.chain, tip)
	if start < 0 {
		return nil, fmt.Errorf("fakeRepo: %q is not on the main line", tip)
	}
	var commits []gitrepo.LogCommit
	for i := start; i >= 0; i-- {
		sha := f.chain[i]
		committed := f.commits[sha].committed
		if !q.Since.IsZero() && committed.Before(q.Since) {
			continue
		}
		entry := gitrepo.LogCommit{SHA: sha, Committed: committed}
		if i > 0 {
			entry.Parents = []string{f.chain[i-1]}
		}
		if q.PathPrefix != "" {
			for _, name := range f.changedPaths(i) {
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

	data, err := json.Marshal(shedengine.Status{State: shedengine.StateDone, History: historyOf(producers...)})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return string(data)
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
}

// newWorld returns a world whose main line holds one empty base commit, so the records tip always exists.
func newWorld(t *testing.T) *world {
	w := &world{t: t, repo: newFakeRepo()}
	w.repo.commitOnChain("base", at(-1000), nil)
	return w
}

// battenStatus commits slug's batten status holding the history of producers to the main line at hour.
func (w *world) battenStatus(sha string, hour int, slug string, producers ...string) {
	w.repo.commitOnChain(sha, at(hour), map[string]string{recordPaths(w.t, slug).Status: statusText(w.t, producers...)})
}

// battenDeleted commits the removal of slug's batten status to the main line at hour.
func (w *world) battenDeleted(sha string, hour int, slug string) {
	w.repo.commitOnChain(sha, at(hour), nil, recordPaths(w.t, slug).Status)
}

// tag adds the archive tag of slug naming a commit off the main line at when, holding the given seed and status text; an empty text leaves that file out.
func (w *world) tag(slug, sha string, when time.Time, seed, status string) {
	rels := recordPaths(w.t, slug)
	files := map[string]string{}
	if seed != "" {
		files[rels.Seed] = seed
	}
	if status != "" {
		files[rels.Status] = status
	}
	w.repo.commitOffChain(sha, when, files)
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
	tip := ""
	if n := len(w.repo.chain); n > 0 {
		tip = w.repo.chain[n-1]
	}
	return Sources{Code: w.repo, Records: w.repo, Board: w.repo, RecordsTip: tip, Tags: tags}
}
