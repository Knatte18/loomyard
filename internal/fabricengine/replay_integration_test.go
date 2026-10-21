//go:build integration

// replay_integration_test.go proves the records replay and the ended-set rule on hubforge hubs.
// The other machine is a second clone of the hub's records bare repository.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// recordsFixture is a hub whose records bare repository a second clone moves.
type recordsFixture struct {
	t   *testing.T
	hub *hubforge.Hub
}

func newRecordsFixture(t *testing.T) *recordsFixture {
	t.Helper()
	boardkit.InstallReplay(t)
	return &recordsFixture{t: t, hub: hubforge.NewHub(t, ".")}
}

// primeWeft publishes the prime weft branch with an upstream, as a machine that pushed it earlier holds it, and returns the worktree and its branch.
func (f *recordsFixture) primeWeft() (worktree, branch string) {
	f.t.Helper()

	worktree = f.hub.PrimeRecords()
	branch = gitkit.CurrentBranch(f.t, worktree)
	gitkit.Git(f.t, worktree, "push", "--set-upstream", "origin", branch)
	return worktree, branch
}

// originCommit commits rel on branch from a second clone of the records bare repository, pushes it and returns the new origin tip.
func (f *recordsFixture) originCommit(branch, rel, content, message string) string {
	f.t.Helper()

	other := filepath.Join(f.t.TempDir(), "other")
	gitkit.Git(f.t, filepath.Dir(other), "clone", "--branch", branch, f.hub.RecordsBare, other)
	sha := gitkit.CommitFile(f.t, other, rel, content, message)
	gitkit.Git(f.t, other, "push", "origin", branch)
	return sha
}

// requireClean fails the test when dir has an uncommitted change.
// scribbleThenFailReplay is the real board replay for every board but one.
// For that one it writes an uncommitted `board.json` and then fails, as an apply that dies halfway leaves it.
type scribbleThenFailReplay struct {
	boardengine.FabricReplay
	boardDir string
}

func (r scribbleThenFailReplay) Apply(req fabricengine.BoardReplayRequest) (fabricengine.BoardReplayResult, error) {
	if req.BoardDir != r.boardDir {
		return r.FabricReplay.Apply(req)
	}
	if err := os.WriteFile(filepath.Join(req.BoardDir, "board.json"), []byte("{}\n"), 0o644); err != nil {
		return fabricengine.BoardReplayResult{}, err
	}
	return fabricengine.BoardReplayResult{}, errors.New("apply died after writing board.json")
}

func requireClean(t *testing.T, dir string) {
	t.Helper()
	if status := gitkit.GitStatusPorcelain(t, dir); strings.TrimSpace(status) != "" {
		t.Errorf("%s status = %q, want a clean tree", dir, status)
	}
}

// tagsOf lists the local tags of dir matching pattern.
func tagsOf(t *testing.T, dir, pattern string) []string {
	t.Helper()
	return strings.Fields(gitkit.Git(t, dir, "tag", "--list", pattern))
}

// boardEntry builds a board entry with the type label bug, the options applied after the defaults.
func boardEntry(slug string, options ...func(*boardengine.Task)) boardengine.Task {
	task := boardengine.Task{Slug: slug, Title: slug, Kind: boardengine.KindTask, Labels: []string{"bug"}, Issues: []int{}, DependsOn: []string{}}
	for _, option := range options {
		option(&task)
	}
	return task
}

func withBoardTitle(title string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Title = title }
}

func withBoardBody(body string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Body = body }
}

func withBoardStatus(status string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Status = &status }
}

func withBoardCreated(created time.Time) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Created = created }
}

// boardFileOf encodes entries as a board.json, numbering them by position.
func boardFileOf(t *testing.T, entries ...boardengine.Task) string {
	t.Helper()
	for i := range entries {
		entries[i].ID = i
	}
	data, err := json.Marshal(struct {
		Version int                `json:"version"`
		Entries []boardengine.Task `json:"entries"`
	}{1, entries})
	if err != nil {
		t.Fatalf("marshal board.json: %v", err)
	}
	return string(data)
}

// boardTitles reads the board.json in dir as a slug to title map.
func boardTitles(t *testing.T, dir string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	var file struct {
		Entries []boardengine.Task `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("decode board.json: %v", err)
	}
	titles := map[string]string{}
	for _, entry := range file.Entries {
		titles[entry.Slug] = entry.Title
	}
	return titles
}

// statusJSON is a run's status.json at the given current producer and history rows.
func statusJSON(producer string, history ...string) string {
	rows := make([]map[string]string, len(history))
	for i, row := range history {
		rows[i] = map[string]string{"producer": row}
	}
	data, err := json.Marshal(map[string]any{"current_producer": producer, "state": "running", "history": rows})
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestRecordsReplay(t *testing.T) {
	t.Parallel()

	t.Run("Weft", func(t *testing.T) {
		t.Parallel()

		t.Run("A local commit under newer unrelated origin commits replays and an empty commit is dropped", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			weft, branch := f.primeWeft()
			local := gitkit.CommitFile(t, weft, "notes/local.md", "local\n", "local work")
			gitkit.Git(t, weft, "commit", "--allow-empty", "-m", "empty")
			empty := gitkit.RevParse(t, weft, "HEAD")
			upstream := f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, weft, "fetch")

			outcome, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, nil)
			if err != nil {
				t.Fatalf("ReplayWeftForTest: %v", err)
			}
			if !slices.Equal(outcome.Replayed, []string{local}) || !slices.Equal(outcome.Dropped, []string{empty}) || len(outcome.SetAside) != 0 {
				t.Errorf("outcome = %+v, want only %s replayed and %s dropped", outcome, local, empty)
			}
			if !gitkit.IsAncestor(t, weft, upstream, "HEAD") {
				t.Errorf("upstream %s is not under HEAD", upstream)
			}
			if got := gitkit.RevListCount(t, weft, upstream+"..HEAD"); got != 1 {
				t.Errorf("HEAD is %d commits past upstream, want only the replayed commit", got)
			}
			if _, err := os.Stat(filepath.Join(weft, "notes", "local.md")); err != nil {
				t.Errorf("the replayed file is missing: %v", err)
			}
			requireClean(t, weft)
		})

		t.Run("A status.json changed on both sides stops with ErrReplayConflict and restores the tip", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			weft, branch := f.primeWeft()
			const statusRel = "_lyx/shed/run1/status.json"
			gitkit.CommitFile(t, weft, statusRel, "base\n", "base status")
			gitkit.Git(t, weft, "push", "origin", branch)
			tip := gitkit.CommitFile(t, weft, statusRel, "local\n", "local status")
			f.originCommit(branch, statusRel, "remote\n", "remote status")
			gitkit.Git(t, weft, "fetch")

			outcome, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, nil)
			if !errors.Is(err, fabricengine.ErrReplayConflict) {
				t.Fatalf("ReplayWeftForTest error = %v, want ErrReplayConflict", err)
			}
			if !strings.Contains(err.Error(), statusRel) || !slices.Equal(outcome.Conflicts, []string{statusRel}) {
				t.Errorf("error %q and conflicts %v, want both to name %s", err, outcome.Conflicts, statusRel)
			}
			if got := gitkit.RevParse(t, weft, "HEAD"); got != tip {
				t.Errorf("HEAD = %s, want the original tip %s", got, tip)
			}
			requireClean(t, weft)
		})

		t.Run("A tracked change on a touched path refuses with ErrReplayDirty and moves nothing", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			weft, branch := f.primeWeft()
			gitkit.CommitFile(t, weft, "notes/a.md", "base\n", "base")
			gitkit.Git(t, weft, "push", "origin", branch)
			tip := gitkit.CommitFile(t, weft, "notes/a.md", "local\n", "local edit")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, weft, "fetch")
			dirty := filepath.Join(weft, "notes", "a.md")
			if err := os.WriteFile(dirty, []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatalf("write %s: %v", dirty, err)
			}

			_, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, nil)
			if !errors.Is(err, fabricengine.ErrReplayDirty) || !strings.Contains(err.Error(), "notes/a.md") {
				t.Fatalf("ReplayWeftForTest error = %v, want ErrReplayDirty naming notes/a.md", err)
			}
			if got := gitkit.RevParse(t, weft, "HEAD"); got != tip {
				t.Errorf("HEAD = %s, want the original tip %s", got, tip)
			}
			if data, err := os.ReadFile(dirty); err != nil || string(data) != "uncommitted\n" {
				t.Errorf("uncommitted file = %q, %v; want it untouched", data, err)
			}
		})

		t.Run("A set-aside tag already at another commit stops the replay with the branch still on its starting tip", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			weft, branch := f.primeWeft()
			status := gitkit.CommitFile(t, weft, "_lyx/shed/neve/status.json", statusJSON("Run-Shed"), "neve status")
			tip := gitkit.CommitFile(t, weft, "notes/keep.md", "keep\n", "keep")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, weft, "fetch")
			tag := "set-aside/neve/" + status[:12]
			gitkit.Git(t, weft, "tag", tag, tip)
			reflogBefore := gitkit.Git(t, weft, "reflog", "show", "HEAD")

			_, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, map[string]string{"neve": "done"})
			if err == nil || !strings.Contains(err.Error(), tag) {
				t.Fatalf("ReplayWeftForTest error = %v, want the conflicting tag %s named", err, tag)
			}
			// A branch that moved and was put back would show in the reflog; here it never moved.
			if got := gitkit.Git(t, weft, "reflog", "show", "HEAD"); got != reflogBefore {
				t.Errorf("HEAD reflog changed to %q from %q, want the branch never moved", got, reflogBefore)
			}
			if got := gitkit.RevParse(t, weft, "HEAD"); got != tip {
				t.Errorf("HEAD = %s, want the starting tip %s", got, tip)
			}
		})

		t.Run("A reset that is no dirty refusal stops without ErrReplayDirty and names the git failure", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			weft, branch := f.primeWeft()
			tip := gitkit.CommitFile(t, weft, "notes/local.md", "local\n", "local work")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, weft, "fetch")
			lock := strings.TrimSpace(gitkit.Git(t, weft, "rev-parse", "--path-format=absolute", "--git-path", "index.lock"))
			if err := os.WriteFile(lock, nil, 0o644); err != nil {
				t.Fatalf("hold %s: %v", lock, err)
			}

			_, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, nil)
			if err == nil || errors.Is(err, fabricengine.ErrReplayDirty) || !strings.Contains(err.Error(), "index.lock") {
				t.Fatalf("ReplayWeftForTest error = %v, want a git failure naming index.lock and not ErrReplayDirty", err)
			}
			if got := gitkit.RevParse(t, weft, "HEAD"); got != tip {
				t.Errorf("HEAD = %s, want the starting tip %s", got, tip)
			}
		})

		t.Run("A run's commits are set aside under tags when the entry is done on origin and the rest replays", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			weft, branch := f.primeWeft()
			const statusRel = "_lyx/shed/neve/status.json"
			status := gitkit.CommitFile(t, weft, statusRel, statusJSON("Run-Shed"), "neve status")
			keep := gitkit.CommitFile(t, weft, "notes/keep.md", "keep\n", "keep")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, weft, "fetch")

			outcome, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, map[string]string{"neve": "done"})
			if err != nil {
				t.Fatalf("ReplayWeftForTest: %v", err)
			}
			wantTag := "set-aside/neve/" + status[:12]
			if len(outcome.SetAside) != 1 || outcome.SetAside[0].Commit != status || outcome.SetAside[0].Tag != wantTag || !slices.Equal(outcome.Replayed, []string{keep}) {
				t.Errorf("outcome = %+v, want %s set aside as %s and %s replayed", outcome, status, wantTag, keep)
			}
			if got := tagsOf(t, weft, "set-aside/*"); !slices.Equal(got, []string{wantTag}) {
				t.Errorf("set-aside tags = %v, want [%s]", got, wantTag)
			}
			if got := gitkit.RevParse(t, weft, wantTag+"^{commit}"); got != status {
				t.Errorf("tag %s names %s, want %s", wantTag, got, status)
			}
			if got := tagsOf(t, weft, fabricengine.ArchiveTagPrefix+"*"); len(got) != 0 {
				t.Errorf("archive tags = %v, want none, so no run listing reads the set-aside commit", got)
			}
			if got := gitkit.LsFiles(t, weft, "_lyx/shed/neve"); len(got) != 0 {
				t.Errorf("replayed tip still tracks the run directory: %v", got)
			}
			requireClean(t, weft)
		})

		finalizeRows := []struct {
			name         string
			withPair     bool
			pairStatus   string
			primeStatus  string
			taskWork     bool
			wantSetAside bool
		}{
			{name: "A pair status holding a Finalize row is not set aside", withPair: true, pairStatus: statusJSON("Land", "Finalize")},
			{name: "A pair status at Finalize with no row, its local parent holding the task's work, is not set aside", withPair: true, pairStatus: statusJSON("Finalize")},
			{name: "A pair status at Finalize with no row, its local parent lacking the task's work, is set aside", withPair: true, pairStatus: statusJSON("Finalize"), taskWork: true, wantSetAside: true},
			{name: "A gone pair whose prime batten status stands at Land is not set aside", primeStatus: statusJSON("Land")},
		}
		for _, row := range finalizeRows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				f := newRecordsFixture(t)
				weft, branch := f.primeWeft()
				if row.withPair {
					hubforge.AddPair(t, f.hub, "neve")
					statusPath := filepath.Join(f.hub.PairCodeWorktree("neve"), "_lyx", "shed", "neve", "status.json")
					if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
						t.Fatalf("mkdir for %s: %v", statusPath, err)
					}
					if err := os.WriteFile(statusPath, []byte(row.pairStatus), 0o644); err != nil {
						t.Fatalf("write %s: %v", statusPath, err)
					}
					if row.taskWork {
						gitkit.CommitFile(t, f.hub.PairCodeWorktree("neve"), "feature.txt", "task work\n", "task work")
					}
				}
				primeStatus := row.primeStatus
				if primeStatus == "" {
					primeStatus = statusJSON("Run-Shed")
				}
				local := gitkit.CommitFile(t, weft, "_lyx/shed/neve/status.json", primeStatus, "neve prime status")
				f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
				gitkit.Git(t, weft, "fetch")

				outcome, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, map[string]string{"neve": "done"})
				if err != nil {
					t.Fatalf("ReplayWeftForTest: %v", err)
				}
				if row.wantSetAside {
					if len(outcome.SetAside) != 1 || outcome.SetAside[0].Commit != local || len(outcome.Replayed) != 0 {
						t.Errorf("outcome = %+v, want %s set aside", outcome, local)
					}
					return
				}
				if !slices.Equal(outcome.Replayed, []string{local}) || len(outcome.SetAside) != 0 {
					t.Errorf("outcome = %+v, want %s replayed and nothing set aside", outcome, local)
				}
			})
		}
	})

	t.Run("EndedSet", func(t *testing.T) {
		t.Parallel()

		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		const stamp = "2026-01-02T03:04:05Z"

		t.Run("An entry done on origin's tip is ended", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardStatus("done")), boardEntry("b")), "a done")
			gitkit.Git(t, board, "fetch")

			got, err := fabricengine.ReadEndedSetForTest(board, "refs/remotes/origin/"+branch, []string{"a", "b"}, nil)
			if err != nil {
				t.Fatalf("ReadEndedSetForTest: %v", err)
			}
			if want := map[string]string{"a": "done"}; !maps.Equal(got, want) {
				t.Errorf("ended set = %v, want %v", got, want)
			}
		})

		t.Run("An entry pruned on origin is ended even after local Bolt fast-forwarded past the removal", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardCreated(created)), boardEntry("b")), "add a")
			f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("b")), "prune a")
			gitkit.Git(t, board, "pull", "--ff-only")

			got, err := fabricengine.ReadEndedSetForTest(board, "refs/remotes/origin/"+branch, []string{"a", "b"}, map[string]string{"a": stamp})
			if err != nil {
				t.Fatalf("ReadEndedSetForTest: %v", err)
			}
			if want := map[string]string{"a": "removed"}; !maps.Equal(got, want) {
				t.Errorf("ended set = %v, want %v", got, want)
			}
		})

		t.Run("An entry only local Bolt holds is not ended", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("mine", withBoardCreated(created))), "add mine")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, board, "fetch")

			got, err := fabricengine.ReadEndedSetForTest(board, "refs/remotes/origin/"+branch, []string{"mine"}, map[string]string{"mine": stamp})
			if err != nil {
				t.Fatalf("ReadEndedSetForTest: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("ended set = %v, want none", got)
			}
		})

		t.Run("A reused slug with a different created stamp is not ended", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardCreated(created), withBoardStatus("done"))), "a done")
			f.originCommit(branch, "board.json", boardFileOf(t), "prune a")
			gitkit.Git(t, board, "fetch")

			got, err := fabricengine.ReadEndedSetForTest(board, "refs/remotes/origin/"+branch, []string{"a"}, map[string]string{"a": "2026-06-07T08:09:10Z"})
			if err != nil {
				t.Fatalf("ReadEndedSetForTest: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("ended set = %v, want none for a slug reused with another created stamp", got)
			}
		})
	})

	t.Run("Bolt", func(t *testing.T) {
		t.Parallel()

		// pushBase commits a board.json of entries on the board and pushes it, so both sides share it as the merge base.
		pushBase := func(t *testing.T, board, branch string, entries ...boardengine.Task) {
			t.Helper()
			gitkit.CommitFile(t, board, "board.json", boardFileOf(t, entries...), "base board")
			gitkit.Git(t, board, "push", "origin", branch)
		}

		t.Run("Status commits of different entries from two machines replay with no stop and README.md re-rendered", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			pushBase(t, board, branch, boardEntry("a", withBoardTitle("A")), boardEntry("b", withBoardTitle("B")))
			local := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("A")), boardEntry("b", withBoardTitle("B2"))), "retitle b")
			upstream := f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("A2")), boardEntry("b", withBoardTitle("B"))), "retitle a")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			if err != nil {
				t.Fatalf("ReplayBoltForTest: %v", err)
			}
			if !slices.Equal(outcome.Replayed, []string{local}) || len(outcome.Lost) != 0 || len(outcome.Refused) != 0 {
				t.Errorf("outcome = %+v, want only %s replayed", outcome, local)
			}
			if got := boardTitles(t, board); got["a"] != "A2" || got["b"] != "B2" {
				t.Errorf("board titles = %v, want a=A2 and b=B2", got)
			}
			readme, err := os.ReadFile(filepath.Join(board, "README.md"))
			if err != nil || !strings.Contains(string(readme), "B2") {
				t.Errorf("README.md = %q, %v; want it re-rendered with B2", readme, err)
			}
			if !gitkit.IsAncestor(t, board, upstream, "HEAD") {
				t.Errorf("upstream %s is not under HEAD", upstream)
			}
			requireClean(t, board)
		})

		t.Run("One field changed on both sides keeps origin's value and tags the local commit", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			pushBase(t, board, branch, boardEntry("a", withBoardTitle("A")))
			local := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("local"))), "retitle a locally")
			upstream := f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("origin"))), "retitle a on origin")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			if err != nil {
				t.Fatalf("ReplayBoltForTest: %v", err)
			}
			if len(outcome.Lost) != 1 || outcome.Lost[0].Entry != "a" || outcome.Lost[0].Field != "title" || outcome.Lost[0].Local != `"local"` {
				t.Errorf("lost = %+v, want the local title of a", outcome.Lost)
			}
			wantTag := "set-aside/a/" + local[:12]
			if len(outcome.SetAside) != 1 || outcome.SetAside[0].Tag != wantTag {
				t.Errorf("set aside = %+v, want %s under %s", outcome.SetAside, local, wantTag)
			}
			if got := boardTitles(t, board); got["a"] != "origin" {
				t.Errorf("board titles = %v, want origin's title", got)
			}
			if got := gitkit.RevParse(t, board, "HEAD"); got != upstream {
				t.Errorf("HEAD = %s, want upstream %s, since nothing of the commit survived", got, upstream)
			}
		})

		t.Run("A body edit of an entry origin holds under a run status is set aside with the run lock's refusal", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			pushBase(t, board, branch, boardEntry("a"), boardEntry("b", withBoardTitle("B")))
			local := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("a", withBoardBody("local body")), boardEntry("b", withBoardTitle("B"))), "edit body of a")
			f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardStatus(boardengine.RunStatus("running", "Implementer"))), boardEntry("b", withBoardTitle("B"))), "a runs")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			if err != nil {
				t.Fatalf("ReplayBoltForTest: %v", err)
			}
			if len(outcome.Refused) != 1 || outcome.Refused[0].Entry != "a" || !strings.Contains(outcome.Refused[0].Reason, "held by a run") {
				t.Errorf("refused = %+v, want a's body edit refused for the run lock", outcome.Refused)
			}
			if len(outcome.SetAside) != 1 || outcome.SetAside[0].Commit != local || len(tagsOf(t, board, "set-aside/*")) != 1 {
				t.Errorf("set aside = %+v, want %s under a set-aside tag", outcome.SetAside, local)
			}
		})

		t.Run("A commit touching a stencil file origin left alone is replayed by cherry-pick", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			stencilRel := fabricengine.StencilsSubtreeRel() + "/loom/s.md"
			local := gitkit.CommitFile(t, board, stencilRel, "local\n", "edit a stencil")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			if err != nil {
				t.Fatalf("ReplayBoltForTest: %v", err)
			}
			if !slices.Equal(outcome.Replayed, []string{local}) {
				t.Errorf("replayed = %v, want [%s]", outcome.Replayed, local)
			}
			if data, err := os.ReadFile(filepath.Join(board, filepath.FromSlash(stencilRel))); err != nil || string(data) != "local\n" {
				t.Errorf("stencil = %q, %v; want the local edit", data, err)
			}
		})

		t.Run("A seed commit conflicting with origin's change of the same stencil file is dropped", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			stencilRel := fabricengine.StencilsSubtreeRel() + "/loom/s.md"
			seed := gitkit.CommitFile(t, board, stencilRel, "seeded\n", fabricengine.SeedCommitMessage("stencils", "(0123456789ab production /bin/lyx)"))
			upstream := f.originCommit(branch, stencilRel, "remote\n", "operator edit")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			if err != nil {
				t.Fatalf("ReplayBoltForTest: %v", err)
			}
			if !slices.Equal(outcome.Dropped, []string{seed}) || len(outcome.Replayed) != 0 {
				t.Errorf("outcome = %+v, want only %s dropped", outcome, seed)
			}
			if got := gitkit.RevParse(t, board, "HEAD"); got != upstream {
				t.Errorf("HEAD = %s, want upstream %s", got, upstream)
			}
			requireClean(t, board)
		})

		t.Run("A non-board commit conflicting with origin stops with ErrReplayConflict and restores the tip", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			gitkit.CommitFile(t, board, "notes/c.md", "base\n", "base")
			gitkit.Git(t, board, "push", "origin", branch)
			tip := gitkit.CommitFile(t, board, "notes/c.md", "local\n", "local edit")
			f.originCommit(branch, "notes/c.md", "remote\n", "remote edit")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			if !errors.Is(err, fabricengine.ErrReplayConflict) || !slices.Equal(outcome.Conflicts, []string{"notes/c.md"}) {
				t.Fatalf("ReplayBoltForTest = %+v, %v; want ErrReplayConflict naming notes/c.md", outcome, err)
			}
			if got := gitkit.RevParse(t, board, "HEAD"); got != tip {
				t.Errorf("HEAD = %s, want the original tip %s", got, tip)
			}
			requireClean(t, board)
		})

		t.Run("A failed by-entry apply restores the starting tip and keeps every classified set-aside commit tagged", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			// The merge base's entry has a mistyped title, which the entry replay cannot parse but classification never reads.
			boardOf := func(neveTitle, otherTitle string) string {
				return `{"version":1,"entries":[{"slug":"neve","title":"` + neveTitle + `"},{"slug":"other","title":` + otherTitle + `}]}`
			}
			gitkit.CommitFile(t, board, "board.json", boardOf("n", "5"), "base board")
			gitkit.Git(t, board, "push", "origin", branch)
			neve := gitkit.CommitFile(t, board, "board.json", boardOf("n2", "5"), "retitle neve")
			tip := gitkit.CommitFile(t, board, "board.json", boardOf("n2", "6"), "retitle other")
			f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, board, "fetch")

			outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, map[string]string{"neve": "done"})
			if err == nil || !strings.Contains(err.Error(), "by entry") {
				t.Fatalf("ReplayBoltForTest = %+v, %v; want the by-entry apply error", outcome, err)
			}
			if got := gitkit.RevParse(t, board, "HEAD"); got != tip {
				t.Errorf("HEAD = %s, want the starting tip %s", got, tip)
			}
			if got := gitkit.RevParse(t, board, "set-aside/neve/"+neve[:12]+"^{commit}"); got != neve {
				t.Errorf("set-aside tag names %s, want %s", got, neve)
			}
		})

		t.Run("A refused restore after a failed by-entry apply tags the starting tip and names the way back", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			branch := gitkit.CurrentBranch(t, board)
			pushBase(t, board, branch, boardEntry("neve"), boardEntry("other"))
			tip := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("neve"), boardEntry("other", withBoardTitle("O2"))), "retitle other")
			upstream := f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, board, "fetch")
			t.Cleanup(fabricengine.InstallBoardReplay(scribbleThenFailReplay{boardDir: board}))

			_, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
			wantTag := "set-aside/restore-failed/" + tip[:12]
			if err == nil || !strings.Contains(err.Error(), "tagged "+wantTag) || !strings.Contains(err.Error(), "git reset --keep "+wantTag) {
				t.Fatalf("ReplayBoltForTest error = %v, want the refused restore naming tag %s and its reset --keep", err, wantTag)
			}
			if got := gitkit.RevParse(t, board, wantTag+"^{commit}"); got != tip {
				t.Errorf("tag %s names %s, want the starting tip %s", wantTag, got, tip)
			}
			if got := gitkit.RevParse(t, board, "HEAD"); got != upstream {
				t.Errorf("HEAD = %s, want upstream %s the refused restore left it on", got, upstream)
			}
		})

		t.Run("An ended entry's local Bolt and prime-weft commits are set aside and the rest replays", func(t *testing.T) {
			t.Parallel()
			f := newRecordsFixture(t)
			board := f.hub.BoardDir()
			boardBranch := gitkit.CurrentBranch(t, board)
			weft, weftBranch := f.primeWeft()
			pushBase(t, board, boardBranch, boardEntry("neve"), boardEntry("other", withBoardTitle("O")))

			neveBoard := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("neve", withBoardStatus(boardengine.RunStatus("running", "Implementer"))), boardEntry("other", withBoardTitle("O"))), "neve runs")
			otherBoard := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("neve", withBoardStatus(boardengine.RunStatus("running", "Implementer"))), boardEntry("other", withBoardTitle("O2"))), "retitle other")
			neveWeft := gitkit.CommitFile(t, weft, "_lyx/shed/neve/status.json", statusJSON("Run-Shed"), "neve status")
			keepWeft := gitkit.CommitFile(t, weft, "notes/keep.md", "keep\n", "keep")
			f.originCommit(boardBranch, "board.json", boardFileOf(t, boardEntry("neve", withBoardStatus("done")), boardEntry("other", withBoardTitle("O"))), "neve done")
			f.originCommit(weftBranch, "notes/up.md", "up\n", "upstream work")
			gitkit.Git(t, board, "fetch")
			gitkit.Git(t, weft, "fetch")
			ended := map[string]string{"neve": "done"}

			boardOutcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, ended)
			if err != nil {
				t.Fatalf("ReplayBoltForTest: %v", err)
			}
			if len(boardOutcome.SetAside) != 1 || boardOutcome.SetAside[0].Commit != neveBoard || boardOutcome.SetAside[0].Tag != "set-aside/neve/"+neveBoard[:12] {
				t.Errorf("board set aside = %+v, want %s under a neve tag", boardOutcome.SetAside, neveBoard)
			}
			if !slices.Contains(boardOutcome.Replayed, otherBoard) {
				t.Errorf("board replayed = %v, want %s among them", boardOutcome.Replayed, otherBoard)
			}
			if got := boardTitles(t, board); got["other"] != "O2" {
				t.Errorf("board titles = %v, want other retitled", got)
			}

			weftOutcome, err := fabricengine.ReplayWeftForTest(f.hub.Location, weft, ended)
			if err != nil {
				t.Fatalf("ReplayWeftForTest: %v", err)
			}
			if len(weftOutcome.SetAside) != 1 || weftOutcome.SetAside[0].Commit != neveWeft || !slices.Equal(weftOutcome.Replayed, []string{keepWeft}) {
				t.Errorf("weft outcome = %+v, want %s set aside and %s replayed", weftOutcome, neveWeft, keepWeft)
			}
			if got := gitkit.LsFiles(t, weft, "_lyx/shed/neve"); len(got) != 0 {
				t.Errorf("replayed prime-weft tip still tracks the run directory: %v", got)
			}
			for _, dir := range []string{board, weft} {
				if got := tagsOf(t, dir, fabricengine.ArchiveTagPrefix+"*"); len(got) != 0 {
					t.Errorf("archive tags in %s = %v, want none", dir, got)
				}
			}
		})
	})
}
