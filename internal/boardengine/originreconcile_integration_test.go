// originreconcile_integration_test.go covers the origin reconcile's flows that rest on the board:
// which runs origin's board has ended, and which entries it gives to another machine.
// Its hubs are hubforge hubs, and the other machine is a second clone of a hub's bare repositories.

package boardengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/fabrickit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// titledEntryJSON returns the board entry of the task slug under title, with extra appended to its fields.
func titledEntryJSON(slug, title, extra string) string {
	return fmt.Sprintf(`{"slug":%q,"title":%q,"kind":"task","labels":["bug"],"issues":[],"depends_on":[]%s}`, slug, title, extra)
}

// entryJSON is titledEntryJSON for an entry titled by its slug.
func entryJSON(slug, extra string) string {
	return titledEntryJSON(slug, slug, extra)
}

// addPublishedPair adds the pair slug under the pair parent, where "" names the prime.
// It sets the code branch's upstream and puts the records branch on origin.
func addPublishedPair(t *testing.T, h *hubforge.Hub, slug, parent string) {
	t.Helper()
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{Parent: parent})
	code, records := h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)
	gitkit.Git(t, code, "branch", "--set-upstream-to=origin/"+gitkit.CurrentBranch(t, code))
	gitkit.Git(t, records, "push", "--set-upstream", "origin", gitkit.CurrentBranch(t, records))
}

// newReconcileHub is a hub with the prime's records branch on origin and the pairs added under the prime.
func newReconcileHub(t *testing.T, pairs ...string) *hubforge.Hub {
	t.Helper()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.Git(t, h.PrimeRecords(), "push", "--set-upstream", "origin", gitkit.CurrentBranch(t, h.PrimeRecords()))
	for _, slug := range pairs {
		addPublishedPair(t, h, slug, "")
	}
	return h
}

// publishBoard commits content as the local board, pushes it and returns the board's branch.
func publishBoard(t *testing.T, h *hubforge.Hub, content string) string {
	t.Helper()
	branch := gitkit.CurrentBranch(t, h.BoardDir())
	gitkit.CommitFile(t, h.BoardDir(), boardengine.BoardFileRel(), content, "base board")
	gitkit.Git(t, h.BoardDir(), "push", "origin", branch)
	return branch
}

// reconcileHub runs the origin reconcile over h, failing the test on error.
func reconcileHub(t *testing.T, h *hubforge.Hub, scope fabriclayout.ReconcileScope, seams fabricengine.OriginSeams, opts fabricengine.ReconcileOptions) fabriclayout.OriginReport {
	t.Helper()
	report, err := fabricengine.ReconcileOrigin(h.Location, scope, seams, opts)
	if err != nil {
		t.Fatalf("ReconcileOrigin(%+v) error = %v; want nil", scope, err)
	}
	return report
}

// entryOf returns the entry of slug, failing the test when the report holds none.
func entryOf(t *testing.T, report fabriclayout.OriginReport, slug string) fabriclayout.EntryOutcome {
	t.Helper()
	outcome, found := fabrickit.EntryOf(report, slug)
	if !found {
		t.Fatalf("report holds no entry %q; entries = %+v", slug, report.Entries)
	}
	return outcome
}

// branchOf returns the outcome of branch on the repository side repo, failing the test when the report holds none.
func branchOf(t *testing.T, report fabriclayout.OriginReport, repo, branch string) fabriclayout.BranchOutcome {
	t.Helper()
	outcome, found := fabrickit.BranchOf(report, repo, branch)
	if !found {
		t.Fatalf("report holds no %s branch %q; branches = %+v", repo, branch, report.Branches)
	}
	return outcome
}

// writeRunStatus writes the run record of slug at the worktree prime, standing at producer.
// Its history holds one finished row per name in history.
func writeRunStatus(tb testing.TB, prime *lymcwd.Location, slug, producer string, history ...string) {
	tb.Helper()
	rows := make([]shedengine.HistoryEntry, len(history))
	for i, row := range history {
		rows[i] = shedengine.HistoryEntry{Producer: row, Outcome: shedengine.Done}
	}
	err := state.UpdateJSON(shedrun.StatusFile(prime, slug), shedrun.StatusLock(prime, slug), func(shedengine.Status, bool) (shedengine.Status, error) {
		return shedengine.Status{CurrentProducer: producer, State: shedengine.StateRunning, History: rows}, nil
	})
	if err != nil {
		tb.Fatalf("write the run record of %s: %v", slug, err)
	}
}

// teardownRecorder records each teardown the reconcile asks for and passes it to the real teardown.
type teardownRecorder struct {
	calls []fabriclayout.EndedTeardown
}

// seams returns the hub's real origin seams with the teardown recorded.
// LivePair is left unset, so no pair reads live.
func (r *teardownRecorder) seams(tb testing.TB, h *hubforge.Hub) fabricengine.OriginSeams {
	tb.Helper()
	teardown, err := pairteardown.New(h.Location)
	if err != nil {
		tb.Fatalf("pairteardown.New: %v", err)
	}
	seams := teardown.OriginSeams(pairteardown.HubSeamOptions(h.Location))
	seams.LivePair = nil
	remove := seams.Teardown
	seams.Teardown = func(request fabriclayout.EndedTeardown) (fabriclayout.TeardownReport, error) {
		r.calls = append(r.calls, request)
		return remove(request)
	}
	return seams
}

func (r *teardownRecorder) slugsCalled() []string {
	var slugs []string
	for _, call := range r.calls {
		slugs = append(slugs, call.Slug)
	}
	return slugs
}

// TestReconcileOrigin_EndedRuns covers which runs the origin reconcile reads as ended on origin's board.
// It also covers which of their pairs the reconcile tears down.
func TestReconcileOrigin_EndedRuns(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	const done = `,"status":"done"`
	originBoard := func(t *testing.T, h *hubforge.Hub, entries ...string) string {
		t.Helper()
		return gitkit.CommitOnBare(t, h.RecordsBare, gitkit.CurrentBranch(t, h.BoardDir()), boardengine.BoardFileRel(), boardkit.BoardFile(entries...), "origin board")
	}
	// endedHub holds the pair "task", its run recorded before Finalize, and an origin board marking it done.
	endedHub := func(t *testing.T) *hubforge.Hub {
		t.Helper()
		h := newReconcileHub(t, "task")
		writeRunStatus(t, h.Location, "task", "Implementer")
		publishBoard(t, h, boardkit.BoardFile(entryJSON("task", "")))
		originBoard(t, h, entryJSON("task", done))
		return h
	}
	pairStatus := func(t *testing.T, h *hubforge.Hub, slug, producer string, history ...string) {
		t.Helper()
		location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		writeRunStatus(t, location, slug, producer, history...)
	}

	t.Run("A teardown that refuses the pair's uncommitted work reports its error on the entry", func(t *testing.T) {
		t.Parallel()
		h := endedHub(t)
		if err := os.WriteFile(filepath.Join(h.PairCodeWorktree("task"), "uncommitted.txt"), []byte("work\n"), 0o644); err != nil {
			t.Fatalf("write the uncommitted file: %v", err)
		}
		recorder := &teardownRecorder{}

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		outcome := entryOf(t, report, "task")
		if outcome.Removed || outcome.Busy || outcome.Error != "worktree has uncommitted changes; use --force" {
			t.Errorf("entry = %+v; want nothing removed, no busy flag and the removal's uncommitted-changes refusal", outcome)
		}
	})

	t.Run("An entry done on origin with a local pair is torn down once and reported ended as landed", func(t *testing.T) {
		t.Parallel()
		h := endedHub(t)
		originHead := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+gitkit.CurrentBranch(t, h.BoardDir()))
		code, records := h.PairCodeWorktree("task"), h.PairRecordsSibling("task")
		codeBranch, recordsBranch := gitkit.CurrentBranch(t, code), gitkit.CurrentBranch(t, records)
		recordsTip := gitkit.RevParse(t, records, "HEAD")
		recorder := &teardownRecorder{}

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task"}) {
			t.Fatalf("teardown calls = %v; want one call for task", got)
		}
		if recorder.calls[0].EarlierSnapshot || recorder.calls[0].Force {
			t.Errorf("teardown request = %+v; want no earlier snapshot and no force", recorder.calls[0])
		}
		outcome := entryOf(t, report, "task")
		if outcome.State != fabriclayout.EntryEnded || outcome.How != "done" || !outcome.Local || !outcome.Removed || outcome.Busy || outcome.Error != "" {
			t.Errorf("entry = %+v; want ended as done, local and removed", outcome)
		}
		for _, branch := range report.Branches {
			if branch.Slug == "task" {
				t.Errorf("branch %q of the removed pair is still reported", branch.Branch)
			}
		}
		if !strings.Contains(report.Summary(), "ended: ") {
			t.Errorf("Summary = %q; want the ended entries listed", report.Summary())
		}
		pairDir, recordsDir := filepath.Base(code), filepath.Base(records)
		want := []fabriclayout.Mutation{
			{Kind: fabriclayout.KindTagCreated, Target: filepath.Base(h.PrimeRecords()), Detail: "set-aside/task/" + recordsTip[:12]},
			{Kind: fabriclayout.KindLinkRemoved, Target: pairDir + "/.lym"},
			{Kind: fabriclayout.KindLinkRemoved, Target: pairDir + "/_lym"},
			{Kind: fabriclayout.KindLinkRemoved, Target: pairDir + "/.scratch"},
			{Kind: fabriclayout.KindWorktreeRemoved, Target: pairDir},
			{Kind: fabriclayout.KindWorktreeRemoved, Target: recordsDir},
			{Kind: fabriclayout.KindBranchDeleted, Target: recordsBranch},
			{Kind: fabriclayout.KindBranchDeleted, Target: codeBranch},
			{Kind: fabriclayout.KindFileWritten, Target: h.Location.RepoName + ".code-workspace"},
			{Kind: fabriclayout.KindRepoAdvanced, Target: filepath.Base(h.BoardDir()), Detail: originHead},
		}
		if got := report.Mutations.Entries(); !slices.Equal(got, want) {
			t.Errorf("Mutations = %+v; want %+v", got, want)
		}
	})

	t.Run("An ended entry with a run record and no local pair is ended with no teardown and no error", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t)
		writeRunStatus(t, h.Location, "ghost", "Implementer")
		publishBoard(t, h, boardkit.BoardFile(entryJSON("ghost", "")))
		originBoard(t, h, entryJSON("ghost", done))
		recorder := &teardownRecorder{}
		lsRemotes := 0

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{CallHook: func(args []string) {
			if args[0] == "ls-remote" {
				lsRemotes++
			}
		}})

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none for a run with no local pair", recorder.slugsCalled())
		}
		outcome := entryOf(t, report, "ghost")
		if outcome.State != fabriclayout.EntryEnded || outcome.Local || outcome.Removed || outcome.Busy || outcome.Error != "" {
			t.Errorf("entry = %+v; want ended with nothing local, nothing removed and no error", outcome)
		}
		if lsRemotes != 2 {
			t.Errorf("ls-remote calls = %d; want 2", lsRemotes)
		}
		if strings.Contains(report.Summary(), "ghost") {
			t.Errorf("Summary = %q; want an entry with no local pair left out", report.Summary())
		}
	})

	t.Run("A hub reconcile ends a pair at Finalize before its parent merge although it fast-forwards the parent", func(t *testing.T) {
		t.Parallel()
		h := endedHub(t)
		pairStatus(t, h, "task", "Finalize")
		gitkit.CommitFile(t, h.PairCodeWorktree("task"), "feature.txt", "task work\n", "task work")
		primeBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
		gitkit.CommitOnBare(t, h.CodeBare, primeBranch, "moved.txt", "moved", "origin moved")
		recorder := &teardownRecorder{}

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task"}) {
			t.Errorf("teardown calls = %v; want one call for task", got)
		}
		moved := slices.ContainsFunc(report.Branches, func(branch fabriclayout.BranchOutcome) bool {
			return branch.Repo == "code" && branch.Branch == primeBranch && branch.Moved
		})
		if !moved {
			t.Errorf("branches = %+v; want the prime code branch fast-forwarded in the same reconcile", report.Branches)
		}
	})

	t.Run("A pair's own run record decides whether a done entry ends it", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			producer  string
			history   []string
			wantEnded bool
		}{
			{"PastItsParentMerge", "Land", []string{"Finalize"}, false},
			{"RaddleStandingAtRaddleFile", "Raddle-File", []string{"Raddle-Write"}, false},
			{"RaddleSentBackToRaddleWrite", "Raddle-Write", []string{"Raddle-File"}, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := endedHub(t)
				pairStatus(t, h, "task", tt.producer, tt.history...)
				recorder := &teardownRecorder{}

				report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

				if got := len(recorder.calls) == 1; got != tt.wantEnded {
					t.Errorf("teardown calls = %v; want a call = %v", recorder.slugsCalled(), tt.wantEnded)
				}
				if got := entryOf(t, report, "task").State; (got == fabriclayout.EntryEnded) != tt.wantEnded {
					t.Errorf("entry state = %q; want ended = %v", got, tt.wantEnded)
				}
			})
		}
	})

	t.Run("A raddle run standing at Raddle-File whose done entry origin then pruned is not ended", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t, "task")
		writeRunStatus(t, h.Location, "task", "Implementer")
		pairStatus(t, h, "task", "Raddle-File", "Raddle-Write")
		publishBoard(t, h, boardkit.BoardFile(entryJSON("task", ""), entryJSON("keep", "")))
		originBoard(t, h, entryJSON("task", done), entryJSON("keep", ""))
		originBoard(t, h, entryJSON("keep", ""))
		recorder := &teardownRecorder{}

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none for a run still carried to its teardown", recorder.slugsCalled())
		}
		if outcome := entryOf(t, report, "task"); outcome.State != fabriclayout.EntryActive {
			t.Errorf("entry = %+v; want active", outcome)
		}
	})

	t.Run("An entry removed from origin's board is ended as removed, a raddle run at Raddle-File included", func(t *testing.T) {
		t.Parallel()
		for _, raddle := range []bool{false, true} {
			h := newReconcileHub(t, "task")
			writeRunStatus(t, h.Location, "task", "Implementer")
			if raddle {
				pairStatus(t, h, "task", "Raddle-File", "Raddle-Write")
			}
			publishBoard(t, h, boardkit.BoardFile(entryJSON("task", ""), entryJSON("keep", "")))
			originBoard(t, h, entryJSON("keep", ""))
			recorder := &teardownRecorder{}

			report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

			outcome := entryOf(t, report, "task")
			if outcome.State != fabriclayout.EntryEnded || outcome.How != "removed" || !outcome.Removed {
				t.Errorf("raddle %v: entry = %+v; want ended as removed and torn down", raddle, outcome)
			}
			if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task"}) {
				t.Errorf("raddle %v: teardown calls = %v; want one call for task", raddle, got)
			}
		}
	})

	t.Run("An entry only local Bolt holds is not ended and its pair is not torn down", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t, "mine")
		pairStatus(t, h, "mine", "Implementer")
		writeRunStatus(t, h.Location, "mine", "Implementer")
		publishBoard(t, h, boardkit.BoardFile(entryJSON("keep", "")))
		gitkit.CommitFile(t, h.BoardDir(), boardengine.BoardFileRel(), boardkit.BoardFile(entryJSON("keep", ""), entryJSON("mine", "")), "mine, local only")
		originBoard(t, h, entryJSON("keep", `,"brief":"origin"`))
		recorder := &teardownRecorder{}

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none", recorder.slugsCalled())
		}
		if outcome := entryOf(t, report, "mine"); outcome.State == fabriclayout.EntryEnded {
			t.Errorf("entry mine = %+v; want it not ended", outcome)
		}
	})

	t.Run("A pair with no run record and no board entry is never torn down", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t, "plain")
		publishBoard(t, h, boardkit.BoardFile(entryJSON("keep", "")))
		originBoard(t, h, entryJSON("keep", `,"brief":"origin"`))
		recorder := &teardownRecorder{}

		report := reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none", recorder.slugsCalled())
		}
		if len(report.Entries) != 0 {
			t.Errorf("entries = %+v; want none for a pair with no run record", report.Entries)
		}
	})

	t.Run("The run scope ends its own run like any other and calls no teardown", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			merged    bool
			wantEnded bool
		}{
			{"a parent lacking the task's work is the landing another machine made", false, true},
			{"a parent the run's own Finalize merged into is the re-entry after its done mark", true, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := endedHub(t)
				pairStatus(t, h, "task", "Finalize")
				gitkit.CommitFile(t, h.PairCodeWorktree("task"), "feature.txt", "task work\n", "task work")
				if tt.merged {
					gitkit.Git(t, h.PrimeWorktree(), "merge", "--no-edit", gitkit.CurrentBranch(t, h.PairCodeWorktree("task")))
				}
				recorder := &teardownRecorder{}

				report := reconcileHub(t, h, fabriclayout.ReconcileScope{Run: "task"}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

				if len(recorder.calls) != 0 {
					t.Errorf("teardown calls = %v; want none, since the run's driver is the caller", recorder.slugsCalled())
				}
				if got := entryOf(t, report, "task").State; (got == fabriclayout.EntryEnded) != tt.wantEnded {
					t.Errorf("entry state = %q; want ended = %v", got, tt.wantEnded)
				}
			})
		}
	})

	t.Run("An earlier snapshot tag sets EarlierSnapshot on the request", func(t *testing.T) {
		t.Parallel()
		h := endedHub(t)
		gitkit.Git(t, h.PrimeRecords(), "tag", "-a", "-m", "snapshot", "set-aside/task/abc123")
		recorder := &teardownRecorder{}

		reconcileHub(t, h, fabriclayout.ReconcileScope{}, recorder.seams(t, h), fabricengine.ReconcileOptions{})

		if len(recorder.calls) != 1 || !recorder.calls[0].EarlierSnapshot {
			t.Errorf("teardown calls = %+v; want one request with EarlierSnapshot set", recorder.calls)
		}
	})

	t.Run("A failed-push record of a branch no longer local is removed by a moving reconcile only", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t)
		hubPath := h.Location.HubPath
		primeBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
		seen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		for _, branch := range []string{"vanished", primeBranch} {
			failure := fabriclayout.FailedPush{Repo: "code", RepoPath: h.PrimeWorktree(), Branch: branch, Class: fabriclayout.PushFailureTransport}
			if err := fabricengine.RecordPushFailure(hubPath, failure, seen); err != nil {
				t.Fatalf("RecordPushFailure(%s): %v", branch, err)
			}
		}
		both := []string{primeBranch, "vanished"}
		slices.Sort(both)
		for _, tt := range []struct {
			judgeOnly bool
			want      []string
		}{{true, both}, {false, []string{primeBranch}}} {
			reconcileHub(t, h, fabriclayout.ReconcileScope{JudgeOnly: tt.judgeOnly}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})

			failures, err := fabricengine.ListFailedPushes(hubPath)
			if err != nil {
				t.Fatalf("ListFailedPushes: %v", err)
			}
			var got []string
			for _, failure := range failures {
				got = append(got, failure.Branch)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("judge-only %v: failed-push branches = %v; want %v", tt.judgeOnly, got, tt.want)
			}
		}
	})
}

// TestReconcileOrigin_OwnedElsewhere covers the entries origin's board gives to another machine.
// It also covers the entries a run record classifies, and what a Bolt that origin moved does to them.
func TestReconcileOrigin_OwnedElsewhere(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	// ownedByRoot is a board whose prime owner is another machine, which every root entry reads.
	ownedByRoot := func(entries ...string) string {
		return strings.Replace(boardkit.BoardFile(entries...), `{`, `{"prime_owner":"other-machine",`, 1)
	}
	// ownerOf is the real owner reader as the machine this-machine, naming each entry's parent as h records it.
	ownerOf := func(h *hubforge.Hub) fabricengine.OwnerReader {
		return pairteardown.OwnerReader(
			func() (machinename.Identity, error) { return machinename.Identity{Name: "this-machine"}, nil },
			func(_, slug string) (string, error) { return hubgeom.ParentOf(h.Location, slug) },
		)
	}
	// recordedHub holds the pair "task" and its run recorded.
	recordedHub := func(t *testing.T) *hubforge.Hub {
		t.Helper()
		h := newReconcileHub(t, "task")
		writeRunStatus(t, h.Location, "task", "Land")
		return h
	}
	reconcile := func(t *testing.T, h *hubforge.Hub, scope fabriclayout.ReconcileScope, seams fabricengine.OriginSeams) fabriclayout.OriginReport {
		t.Helper()
		return reconcileHub(t, h, scope, seams, fabricengine.ReconcileOptions{})
	}

	t.Run("An entry origin's board gives to another machine is owned_elsewhere with its owner, way forward and summary line, in a run and a judge-only hub scope", func(t *testing.T) {
		t.Parallel()
		beam := `{"slug":"beam","title":"beam","kind":"project","beam":true,"owner_machine":"other-machine","labels":[],"issues":[],"depends_on":[]}`
		tests := []struct {
			name  string
			board string
			// underBeam adds the pair "beam" and the task pair under it.
			underBeam   bool
			wantSubject string
		}{
			{"a task member of a beam names the beam", boardkit.BoardFile(entryJSON("task", ""), beam), true, "beam"},
			{"a root task names root", ownedByRoot(entryJSON("task", "")), false, "root"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var h *hubforge.Hub
				if tt.underBeam {
					h = newReconcileHub(t, "beam")
					addPublishedPair(t, h, "task", "beam")
					writeRunStatus(t, h.Location, "task", "Land")
				} else {
					h = recordedHub(t)
				}
				publishBoard(t, h, tt.board)
				wantWayForward := "`lym adopt " + tt.wantSubject + "` from this machine's prime takes it over"
				wantLine := "owned elsewhere: task by other-machine; `lym adopt " + tt.wantSubject + "` takes it over"
				for _, scope := range []fabriclayout.ReconcileScope{{Run: "task"}, {JudgeOnly: true}} {
					var asked []string
					read := ownerOf(h)
					report := reconcile(t, h, scope, fabricengine.OriginSeams{OwnerOf: func(hubPath string, boardJSON []byte, slug string) (string, string, bool, error) {
						asked = append(asked, slug)
						return read(hubPath, boardJSON, slug)
					}})

					if !slices.Equal(asked, []string{"task"}) {
						t.Errorf("scope %+v: OwnerOf asked about %v; want one question for task", scope, asked)
					}
					outcome := entryOf(t, report, "task")
					if outcome.State != fabriclayout.EntryOwnedElsewhere || outcome.Owner != "other-machine" || outcome.AdoptSubject != tt.wantSubject || outcome.WayForward != wantWayForward {
						t.Errorf("scope %+v: entry = %+v; want owned elsewhere by other-machine, subject %q and way forward %q", scope, outcome, tt.wantSubject, wantWayForward)
					}
					if !strings.Contains(report.Summary(), wantLine) {
						t.Errorf("scope %+v: Summary() = %q; want the line %q", scope, report.Summary(), wantLine)
					}
				}
			})
		}
	})

	t.Run("A records branch both sides moved stays unmoved for an entry owned elsewhere while its fast-forwards move, and its pair is never torn down", func(t *testing.T) {
		t.Parallel()
		h := recordedHub(t)
		publishBoard(t, h, ownedByRoot(entryJSON("task", "")))
		code, records := h.PairCodeWorktree("task"), h.PairRecordsSibling("task")
		codeBranch, recordsBranch := gitkit.CurrentBranch(t, code), gitkit.CurrentBranch(t, records)
		localRecords := gitkit.CommitFile(t, records, "notes/local.md", "local\n", "local note")
		gitkit.CommitOnBare(t, h.RecordsBare, recordsBranch, "notes/origin.md", "origin\n", "origin note")
		originCode := gitkit.CommitOnBare(t, h.CodeBare, codeBranch, "moved.txt", "moved", "origin moved")
		recorder := &teardownRecorder{}
		seams := recorder.seams(t, h)
		seams.OwnerOf = ownerOf(h)

		report := reconcile(t, h, fabriclayout.ReconcileScope{}, seams)

		outcome := branchOf(t, report, "records", recordsBranch)
		if outcome.State != fabriclayout.BranchNeedsReplay || outcome.Moved || gitkit.RevParse(t, records, "HEAD") != localRecords {
			t.Errorf("records outcome = %+v, HEAD = %q; want it unreplayed at %q", outcome, gitkit.RevParse(t, records, "HEAD"), localRecords)
		}
		if got := branchOf(t, report, "code", codeBranch); got.State != fabriclayout.BranchLevel || !got.Moved || gitkit.RevParse(t, code, "HEAD") != originCode {
			t.Errorf("code outcome = %+v; want the fast-forward moved to %q", got, originCode)
		}
		if got := entryOf(t, report, "task"); got.State != fabriclayout.EntryOwnedElsewhere {
			t.Errorf("entry = %+v; want owned elsewhere", got)
		}
		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none for an entry owned elsewhere", recorder.slugsCalled())
		}
	})

	t.Run("An ended entry owned elsewhere reports ended", func(t *testing.T) {
		t.Parallel()
		h := recordedHub(t)
		writeRunStatus(t, h.Location, "task", "Implementer")
		branch := publishBoard(t, h, ownedByRoot(entryJSON("task", "")))
		gitkit.CommitOnBare(t, h.RecordsBare, branch, boardengine.BoardFileRel(), ownedByRoot(entryJSON("task", `,"status":"done"`)), "task done")
		recorder := &teardownRecorder{}
		seams := recorder.seams(t, h)
		seams.OwnerOf = ownerOf(h)

		report := reconcile(t, h, fabriclayout.ReconcileScope{}, seams)

		if got := entryOf(t, report, "task"); got.State != fabriclayout.EntryEnded {
			t.Errorf("entry = %+v; want ended", got)
		}
	})

	t.Run("A nil OwnerOf reads the entry active and an owner read's error is the reconcile's error", func(t *testing.T) {
		t.Parallel()
		h := recordedHub(t)
		branch := publishBoard(t, h, ownedByRoot(entryJSON("task", "")))
		if got := entryOf(t, reconcile(t, h, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}), "task"); got.State != fabriclayout.EntryActive {
			t.Errorf("entry with a nil OwnerOf = %+v; want active", got)
		}

		gitkit.CommitOnBare(t, h.RecordsBare, branch, boardengine.BoardFileRel(), "not a board", "broken board")
		_, err := fabricengine.ReconcileOrigin(h.Location, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{OwnerOf: ownerOf(h)}, fabricengine.ReconcileOptions{})
		if err == nil || !strings.Contains(err.Error(), "read the board's owners") {
			t.Errorf("ReconcileOrigin error = %v; want the owner reader's error", err)
		}
	})

	t.Run("A hub with no Bolt origin head asks for no owner", func(t *testing.T) {
		t.Parallel()
		h := recordedHub(t)
		gitkit.Git(t, h.BoardDir(), "update-ref", "-d", "refs/remotes/origin/"+gitkit.CurrentBranch(t, h.BoardDir()))
		asked := 0
		read := ownerOf(h)

		reconcile(t, h, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{OwnerOf: func(hubPath string, boardJSON []byte, slug string) (string, string, bool, error) {
			asked++
			return read(hubPath, boardJSON, slug)
		}})

		if asked != 0 {
			t.Errorf("OwnerOf was asked %d times on a hub with no Bolt origin head; want no question", asked)
		}
	})

	t.Run("An entry is classified only for a pair with a run record", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t, "stopped", "active", "unrecorded")
		gitkit.Git(t, h.CodeBare, "branch", "-D", gitkit.CurrentBranch(t, h.PairCodeWorktree("stopped")))
		writeRunStatus(t, h.Location, "stopped", "Land")
		writeRunStatus(t, h.Location, "active", "Land")

		report := reconcile(t, h, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		got := map[string]fabriclayout.EntryState{}
		for _, outcome := range report.Entries {
			got[outcome.Slug] = outcome.State
		}
		want := map[string]fabriclayout.EntryState{"stopped": fabriclayout.EntryStopped, "active": fabriclayout.EntryActive}
		if len(got) != len(want) || got["stopped"] != want["stopped"] || got["active"] != want["active"] {
			t.Errorf("entries = %v; want %v, with none for the pair without a run record", got, want)
		}
	})

	t.Run("A run scope that may find its pair absent judges the run's entry from origin's board with no branch of its own, and refuses without the option", func(t *testing.T) {
		t.Parallel()
		// absentHub returns a hub whose task worktree is gone.
		// Without staleRegistration it is removed through git;
		// with it the directory is deleted and the registration is left for prune.
		absentHub := func(t *testing.T, staleRegistration bool) *hubforge.Hub {
			t.Helper()
			h := recordedHub(t)
			writeRunStatus(t, h.Location, "task", "Implementer")
			if staleRegistration {
				if err := os.RemoveAll(h.PairCodeWorktree("task")); err != nil {
					t.Fatalf("delete the task worktree directory: %v", err)
				}
				return h
			}
			gitkit.Git(t, h.PrimeWorktree(), "worktree", "remove", "--force", h.PairCodeWorktree("task"))
			return h
		}
		tests := []struct {
			name string
			// board is what the Bolt publishes, and originBoard what another machine then commits; "" skips either.
			board, originBoard string
			staleRegistration  bool
			want               fabriclayout.EntryState
		}{
			{"an active entry", "", "", false, fabriclayout.EntryActive},
			{"an entry origin's board gives to another machine", ownedByRoot(entryJSON("task", "")), "", false, fabriclayout.EntryOwnedElsewhere},
			{"an entry origin's board gives to another machine, its task worktree directory deleted without a prune", ownedByRoot(entryJSON("task", "")), "", true, fabriclayout.EntryOwnedElsewhere},
			{"an entry origin's board marks done", boardkit.BoardFile(entryJSON("task", "")), boardkit.BoardFile(entryJSON("task", `,"status":"done"`)), false, fabriclayout.EntryEnded},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := absentHub(t, tt.staleRegistration)
				if tt.board != "" {
					branch := publishBoard(t, h, tt.board)
					if tt.originBoard != "" {
						gitkit.CommitOnBare(t, h.RecordsBare, branch, boardengine.BoardFileRel(), tt.originBoard, "origin board")
					}
				}
				recorder := &teardownRecorder{}
				seams := recorder.seams(t, h)
				seams.OwnerOf = ownerOf(h)

				report := reconcile(t, h, fabriclayout.ReconcileScope{Run: "task", PairMayBeAbsent: true}, seams)

				if len(recorder.calls) != 0 {
					t.Errorf("teardown calls = %v; want none for the run scope's own run", recorder.slugsCalled())
				}

				if got := entryOf(t, report, "task"); got.State != tt.want || !got.PairAbsent {
					t.Errorf("entry = %+v; want state %q with PairAbsent", got, tt.want)
				}
				for _, branch := range report.Branches {
					if branch.Repo != "bolt" {
						t.Errorf("judged %s branch %q; want Bolt alone, with no branch of the absent pair or its parent", branch.Repo, branch.Branch)
					}
				}
				if !strings.Contains(report.Summary(), "pair absent: task") {
					t.Errorf("Summary() = %q; want the pair absent mark", report.Summary())
				}
			})
		}

		_, err := fabricengine.ReconcileOrigin(absentHub(t, false).Location, fabriclayout.ReconcileScope{Run: "task"}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
		if err == nil || !strings.Contains(err.Error(), "no pair worktree") {
			t.Errorf("ReconcileOrigin() without the option error = %v; want one naming no pair worktree", err)
		}
		reconcile(t, absentHub(t, true), fabriclayout.ReconcileScope{Run: "task"}, fabricengine.OriginSeams{})
	})

	t.Run("A diverged Bolt judged only reads needs_replay and stays out of the summary", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t)
		branch := publishBoard(t, h, boardkit.BoardFile(entryJSON("neve", "")))
		gitkit.CommitFile(t, h.BoardDir(), "notes/local.md", "local\n", "local note")
		gitkit.CommitOnBare(t, h.RecordsBare, branch, "notes/origin.md", "origin\n", "origin note")

		report := reconcile(t, h, fabriclayout.ReconcileScope{JudgeOnly: true}, fabricengine.OriginSeams{})

		if got := branchOf(t, report, "bolt", branch); got.State != fabriclayout.BranchNeedsReplay {
			t.Errorf("bolt outcome = %+v; want needs_replay", got)
		}
		if report.Summary() != "" {
			t.Errorf("Summary() = %q; want a branch that needs a replay left out", report.Summary())
		}
	})

	t.Run("A Bolt replay sets aside the local status of an entry origin ended, with two ls-remote calls", func(t *testing.T) {
		t.Parallel()
		h := newReconcileHub(t)
		board := h.BoardDir()
		branch := publishBoard(t, h, boardkit.BoardFile(entryJSON("neve", "")))
		running := boardkit.BoardFile(entryJSON("neve", fmt.Sprintf(`,"status":%q`, boardengine.RunStatus("running", "Implementer"))))
		local := gitkit.CommitFile(t, board, boardengine.BoardFileRel(), running, "neve runs")
		originHead := gitkit.CommitOnBare(t, h.RecordsBare, branch, boardengine.BoardFileRel(), boardkit.BoardFile(entryJSON("neve", `,"status":"done"`)), "neve done")
		lsRemotes := 0

		report, err := fabricengine.ReconcileOrigin(h.Location, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{CallHook: func(args []string) {
			if args[0] == "ls-remote" {
				lsRemotes++
			}
		}})

		if err != nil {
			t.Fatalf("ReconcileOrigin() error = %v; want nil", err)
		}
		if got := branchOf(t, report, "bolt", branch); got.State != fabriclayout.BranchLevel || !got.Moved || gitkit.RevParse(t, board, "HEAD") != originHead {
			t.Errorf("bolt outcome = %+v, HEAD = %q; want the local commit set aside and Bolt at origin's head %q", got, gitkit.RevParse(t, board, "HEAD"), originHead)
		}
		if got := gitkit.Git(t, board, "tag", "--list", "set-aside/neve/*"); got != "set-aside/neve/"+local[:12] {
			t.Errorf("set-aside tags = %q; want one for %s", got, local)
		}
		tagged := 0
		for _, mutation := range report.Mutations.Entries() {
			if mutation.Kind == fabriclayout.KindTagCreated {
				tagged++
			}
		}
		if tagged != 1 {
			t.Errorf("tag_created entries = %d; want one per set-aside tag", tagged)
		}
		if lsRemotes != 2 {
			t.Errorf("ls-remote calls = %d; want 2", lsRemotes)
		}
	})

	t.Run("A tracked change on a path Bolt's move touches stops every entry with Bolt's dirty line", func(t *testing.T) {
		t.Parallel()
		h := recordedHub(t)
		board := h.BoardDir()
		branch := publishBoard(t, h, boardkit.BoardFile(entryJSON("neve", "")))
		gitkit.CommitOnBare(t, h.RecordsBare, branch, boardengine.BoardFileRel(), boardkit.BoardFile(entryJSON("neve", `,"brief":"origin"`)), "origin board")
		if err := os.WriteFile(filepath.Join(board, boardengine.BoardFileRel()), []byte(boardkit.BoardFile(entryJSON("neve", `,"brief":"local"`))), 0o644); err != nil {
			t.Fatalf("write the uncommitted board: %v", err)
		}

		report := reconcile(t, h, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		want := "commit or restore the uncommitted paths (board.json) with `git commit` or `git checkout -- <path>` in " + board
		if got := branchOf(t, report, "bolt", branch); got.State != fabriclayout.BranchDirty || got.WayForward != want {
			t.Errorf("bolt outcome = %+v; want dirty with WayForward %q", got, want)
		}
		if got := entryOf(t, report, "task"); got.State != fabriclayout.EntryStopped || got.WayForward != want {
			t.Errorf("entry = %+v; want stopped with Bolt's line %q", got, want)
		}
	})

	t.Run("A both-sides change of the prime's records stops every entry with the conflict line", func(t *testing.T) {
		t.Parallel()
		h := recordedHub(t)
		primeRecords := gitkit.CurrentBranch(t, h.PrimeRecords())
		gitkit.CommitFile(t, h.PrimeRecords(), "notes/c.md", "local\n", "local note")
		gitkit.CommitOnBare(t, h.RecordsBare, primeRecords, "notes/c.md", "origin\n", "origin note")

		report := reconcile(t, h, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := branchOf(t, report, "records", primeRecords)
		if outcome.State != fabriclayout.BranchDiverged || !slices.Equal(outcome.Paths, []string{"notes/c.md"}) || !strings.Contains(outcome.WayForward, h.PrimeRecords()) {
			t.Errorf("prime records outcome = %+v; want diverged on notes/c.md with a way forward naming %s", outcome, h.PrimeRecords())
		}
		if got := entryOf(t, report, "task"); got.State != fabriclayout.EntryStopped || got.WayForward != outcome.WayForward {
			t.Errorf("entry = %+v; want stopped with the prime records' line %q", got, outcome.WayForward)
		}
	})
}
