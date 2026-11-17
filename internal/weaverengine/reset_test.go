// reset_test.go pins Reset over fake seams.
// It covers the refusals Reset makes in order.
// It covers what a reset moves, restores, archives and commits.
// It covers how an interrupted reset is finished.

package weaverengine_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// resetMove is one call of the ResetCode seam.
type resetMove struct {
	sha string
	own []string
}

// resetHarness is ResetDeps over the run fixture's fakes, recording what Reset asks of each seam.
type resetHarness struct {
	t        *testing.T
	fx       *runFixture
	deps     weaverengine.ResetDeps
	original map[string][]byte
	moves    []resetMove
	commits  []string

	commitErr error
	pending   bool
	dirty     []string
	history   []weaverengine.StateVersion
}

// newResetHarness wires a harness over a two-card plan whose files it remembers as the run's start held them.
func newResetHarness(t *testing.T) *resetHarness {
	t.Helper()
	fx := newRunFixture(t, 1, "one", "two")
	h := &resetHarness{t: t, fx: fx, original: map[string][]byte{}}
	entries, err := os.ReadDir(fx.deps.Geom.PlanDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		h.original[entry.Name()] = []byte(readFile(t, filepath.Join(fx.deps.Geom.PlanDir, entry.Name())))
	}
	h.deps = weaverengine.ResetDeps{
		Geom:         fx.deps.Geom,
		Runner:       fx.runner,
		Reed:         fx.reed,
		StateHistory: func() ([]weaverengine.StateVersion, error) { return h.history, nil },
		ReadPlanAt: func(_, name string) ([]byte, bool, error) {
			data, found := h.original[name]
			return data, found, nil
		},
		ResetCode: func(sha string, own []string) error {
			h.moves = append(h.moves, resetMove{sha: sha, own: own})
			return nil
		},
		RecordPending: func() (bool, error) { return h.pending, nil },
		Commit: func(msg string) error {
			h.commits = append(h.commits, msg)
			return h.commitErr
		},
		DirtyTracked: func() ([]string, error) { return h.dirty, nil },
		Now:          func() time.Time { return time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC) },
	}
	return h
}

// seed saves a begun state for batch 1 recorded over plan commit "plan-commit", holding the given attempts.
func (h *resetHarness) seed(attempts ...*weaverengine.AttemptState) *weaverengine.State {
	h.t.Helper()
	st := h.fx.seedBegun(1, "one", attempts...)
	st.PlanRecordsCommit = "plan-commit"
	h.fx.saveSeed(st)
	return st
}

// marker returns the reset-pending marker's path.
func (h *resetHarness) marker() string {
	return filepath.Join(h.deps.Geom.ScratchDir, "reset-pending")
}

// start returns the start commit batch 1 recorded.
func (h *resetHarness) start(st *weaverengine.State) string { return st.Batches[1].StartSHA }

func TestReset(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	liveName, err := agentname.Format("sn", "slug", weaverengine.BatchStrandRole)
	if err != nil {
		t.Fatal(err)
	}

	refusals := []struct {
		name string
		// setup shapes the harness; it returns the strand-stop flag.
		setup func(h *resetHarness) (stopStrands bool)
		want  []string
	}{
		{"RunLockHeld", func(h *resetHarness) bool {
			h.seed()
			held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(h.deps.Geom.ScratchDir, "run.lock"))
			if err != nil || !acquired {
				h.t.Fatalf("acquire the run lock = %v, %v", acquired, err)
			}
			h.t.Cleanup(func() { _ = held.Release() })
			return false
		}, []string{"run.lock", "lym loom pause --after"}},
		{"AbsentStateWithNothingPending", func(h *resetHarness) bool { return false }, []string{"no Weaver run has begun"}},
		{"AbsentStateWithUncommittedPlanEditsAndNoMarker", func(h *resetHarness) bool {
			h.pending = true
			return false
		}, []string{"no Weaver run has begun"}},
		{"NoBegunBatch", func(h *resetHarness) bool {
			h.fx.saveSeed(&weaverengine.State{RunGUID: "guid"})
			return false
		}, []string{"no start commit is recorded"}},
		{"MergeInProgress", func(h *resetHarness) bool {
			h.seed()
			h.fx.git.merging = true
			return false
		}, []string{"merge in progress"}},
		{"ForeignUncommittedPath", func(h *resetHarness) bool {
			h.seed()
			h.dirty = []string{"other.go"}
			return false
		}, []string{"other.go", "git checkout --"}},
		{"FoundLiveStrand", func(h *resetHarness) bool {
			attempt := h.fx.seededAttempt(1, "one", 1, 0, "", true)
			h.seed(attempt)
			h.fx.runner.scripts = []attemptScript{{noReport: true}}
			h.fx.foundRun(h.fx.startStrand(attempt.ReportPath))
			return false
		}, []string{"--stop-strand", "lym loom pause --after"}},
		{"ProbeError", func(h *resetHarness) bool {
			h.seed(h.fx.seededAttempt(1, "one", 1, 0, "", true))
			h.fx.runner.probe = func(shuttleengine.Spec) (*shuttleengine.Run, bool, error) {
				return nil, false, errors.New("reed state is unreadable")
			}
			return false
		}, []string{"reed state is unreadable"}},
		{"StrandThatCannotBeStopped", func(h *resetHarness) bool {
			attempt := h.fx.seededAttempt(1, "one", 1, 0, "", true)
			h.seed(attempt)
			h.fx.runner.scripts = []attemptScript{{noReport: true}}
			run := h.fx.startStrand(attempt.ReportPath)
			h.fx.foundRun(run)
			h.fx.reed.RemoveErr = errors.New("reed is gone")
			return true
		}, []string{"lym reed remove fake-guid-1"}},
		{"CorruptStateWithNoCommittedVersion", func(h *resetHarness) bool {
			h.fx.saveSeed(&weaverengine.State{RunGUID: "guid"})
			if err := os.WriteFile(filepath.Join(h.deps.Geom.RunDir, "state.json"), []byte("{"), 0o644); err != nil {
				h.t.Fatal(err)
			}
			return false
		}, []string{"corrupt", "by hand with git"}},
		{"CorruptStateWhileAWeaverStrandIsLive", func(h *resetHarness) bool {
			good, _ := json.Marshal(h.seed())
			h.history = []weaverengine.StateVersion{{Commit: "c1", Data: good}}
			if err := os.WriteFile(filepath.Join(h.deps.Geom.RunDir, "state.json"), []byte("{"), 0o644); err != nil {
				h.t.Fatal(err)
			}
			h.fx.reed.Strands = []reedengine.StrandStatus{{GUID: "live-guid", Name: liveName, Live: true}}
			return false
		}, []string{"lym reed remove live-guid"}},
	}
	for _, tt := range refusals {
		t.Run("Refuses_"+tt.name, func(t *testing.T) {
			t.Parallel()
			h := newResetHarness(t)
			stop := tt.setup(h)

			_, err := weaverengine.Reset(h.deps, stop)

			if err == nil {
				t.Fatalf("Reset() error = nil; want a refusal holding %v", tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Reset() error lacks %q:\n%v", want, err)
				}
			}
			if len(h.moves) != 0 || len(h.commits) != 0 {
				t.Errorf("moves = %v, commits = %v; want a refusal to change nothing", h.moves, h.commits)
			}
			if _, err := os.Stat(h.marker()); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat marker = %v; want none written by a refusal", err)
			}
		})
	}

	t.Run("MovesRestoresArchivesAndCommitsOnce", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		geom := h.deps.Geom
		attempt := h.fx.seededAttempt(1, "one", 1, 0, "failed", false)
		attempt.Uncommitted = []string{"own.go"}
		st := h.seed(attempt)
		h.dirty = []string{"own.go"}
		card := filepath.Join(geom.PlanDir, "02-two.md")
		if err := os.WriteFile(card, []byte("edited since the run began\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		stray := filepath.Join(geom.PlanDir, "99-extra.md")
		if err := os.WriteFile(stray, []byte("not in the start's plan\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(weaverengine.JournalPath(geom.RunDir), []byte("## Batch 01-one, attempt 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(geom.PromptsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		websterDir := websterengine.Dir(geom.AnchorRoot)
		if err := os.MkdirAll(websterDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(websterDir, "state.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		result, err := weaverengine.Reset(h.deps, false)

		if err != nil {
			t.Fatalf("Reset() error = %v; want nil", err)
		}
		if want := []resetMove{{sha: h.start(st), own: []string{"own.go"}}}; !slices.EqualFunc(h.moves, want, func(a, b resetMove) bool { return a.sha == b.sha && slices.Equal(a.own, b.own) }) {
			t.Errorf("moves = %v; want the branch moved to the start with the attempt's own path %v", h.moves, want)
		}
		if got := readFile(t, card); got != string(h.original["02-two.md"]) {
			t.Errorf("card 02 = %q; want its text at the run's start", got)
		}
		if _, err := os.Stat(stray); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat stray plan file = %v; want it removed", err)
		}
		if _, err := os.Stat(filepath.Join(geom.RunDir, "state.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat state.json = %v; want it archived", err)
		}
		for _, prefix := range []string{"state-20261117T080000Z", "reports-20261117T080000Z", "journal-20261117T080000Z"} {
			if !slices.ContainsFunc(result.Archived, func(name string) bool { return strings.HasPrefix(name, prefix) }) {
				t.Errorf("archived = %v; want an entry starting %q", result.Archived, prefix)
			}
		}
		if _, err := os.Stat(geom.PromptsDir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat prompts dir = %v; want the scratch cleared", err)
		}
		if _, err := os.Stat(filepath.Join(geom.ScratchDir, "run.lock")); err != nil {
			t.Errorf("stat run.lock = %v; want the lock kept", err)
		}
		if got := readFile(t, filepath.Join(websterDir, "state.json")); got != "{}" {
			t.Errorf("webster state = %q; want Webster's run directory untouched", got)
		}
		if !slices.Equal(h.commits, []string{"weaver: reset"}) {
			t.Errorf("commits = %v; want one", h.commits)
		}
		if _, err := os.Stat(h.marker()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat marker = %v; want none after the commit landed", err)
		}
		if result.StartSHA != h.start(st) || result.PlanCommit != "plan-commit" {
			t.Errorf("result = %+v; want the start commit and the restored plan commit", result)
		}
	})

	t.Run("StopStrandStopsEachFoundRunAndResets", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		attempt := h.fx.seededAttempt(1, "one", 1, 0, "", true)
		h.seed(attempt)
		h.fx.runner.scripts = []attemptScript{{noReport: true}}
		run := h.fx.startStrand(attempt.ReportPath)
		h.fx.foundRun(run)

		if _, err := weaverengine.Reset(h.deps, true); err != nil {
			t.Fatalf("Reset() error = %v; want nil", err)
		}

		if !slices.Contains(h.fx.reed.RemovedGUIDs, run.StrandGUID()) || len(h.moves) != 1 {
			t.Errorf("removed = %v, moves = %v; want the found strand stopped and the reset done", h.fx.reed.RemovedGUIDs, h.moves)
		}
	})

	t.Run("LiveAttemptWithUncommittedEditsRefusesNamingTheStrandNotTheCheckout", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		attempt := h.fx.seededAttempt(1, "one", 1, 0, "", true)
		h.seed(attempt)
		h.fx.runner.scripts = []attemptScript{{noReport: true}}
		h.fx.foundRun(h.fx.startStrand(attempt.ReportPath))
		h.dirty = []string{"live.go"}

		_, err := weaverengine.Reset(h.deps, false)

		if err == nil || !strings.Contains(err.Error(), "--stop-strand") || strings.Contains(err.Error(), "git checkout --") {
			t.Errorf("Reset() error = %v; want the strand's way forward and no checkout advice", err)
		}
		if len(h.moves) != 0 {
			t.Errorf("moves = %v; want a refusal to move nothing", h.moves)
		}
	})

	t.Run("StopStrandCountsTheStoppedAttemptsEditsAsTheRunsOwn", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		attempt := h.fx.seededAttempt(1, "one", 1, 0, "", true)
		h.seed(attempt)
		h.fx.runner.scripts = []attemptScript{{noReport: true}}
		h.fx.foundRun(h.fx.startStrand(attempt.ReportPath))
		h.dirty = []string{"live.go"}

		if _, err := weaverengine.Reset(h.deps, true); err != nil {
			t.Fatalf("Reset() error = %v; want nil", err)
		}

		if len(h.moves) != 1 || !slices.Equal(h.moves[0].own, []string{"live.go"}) {
			t.Errorf("moves = %v; want one move discarding the stopped attempt's live.go", h.moves)
		}
	})

	t.Run("HoldsTheRunLockForItsWholeCall", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		h.seed()
		lockPath := filepath.Join(h.deps.Geom.ScratchDir, "run.lock")
		heldDuringMove := false
		h.deps.ResetCode = func(string, []string) error {
			probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
			if err != nil {
				t.Errorf("TryAcquireWriteLock() error = %v", err)
			}
			if acquired {
				_ = probe.Release()
			}
			heldDuringMove = !acquired
			return nil
		}

		if _, err := weaverengine.Reset(h.deps, false); err != nil {
			t.Fatalf("Reset() error = %v; want nil", err)
		}

		if !heldDuringMove {
			t.Errorf("run lock free while the branch moved; want it held")
		}
		probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock() after Reset = acquired %v, %v; want the lock released", acquired, err)
		}
		_ = probe.Release()
	})

	t.Run("CommitFailureNamesTheReRunWhichFinishesIt", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		h.seed()
		h.commitErr = errors.New("the records repository is locked")

		_, err := weaverengine.Reset(h.deps, false)

		if err == nil || !strings.Contains(err.Error(), "lym weaver reset") || !strings.Contains(err.Error(), "the records repository is locked") {
			t.Fatalf("Reset() error = %v; want one naming the re-run", err)
		}
		if _, err := os.Stat(h.marker()); err != nil {
			t.Fatalf("stat marker = %v; want it left by the failed commit", err)
		}

		h.commitErr, h.pending = nil, true
		result, err := weaverengine.Reset(h.deps, false)

		if err != nil || !result.Finished {
			t.Fatalf("Reset() = %+v, %v; want the earlier reset finished", result, err)
		}
		if len(h.commits) != 2 || len(h.moves) != 1 {
			t.Errorf("commits = %v, moves = %v; want the re-run to commit once more and move nothing", h.commits, h.moves)
		}
		if _, err := os.Stat(h.marker()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat marker = %v; want it removed", err)
		}
	})

	t.Run("AbsentStateWithTheMarkerAndNothingPendingRemovesTheMarkerAndCommitsNothing", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		if err := os.MkdirAll(h.deps.Geom.ScratchDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(h.marker(), []byte("sha\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		result, err := weaverengine.Reset(h.deps, false)

		if err != nil || !result.Finished || len(h.commits) != 0 {
			t.Fatalf("Reset() = %+v, %v, commits %v; want the marker removed with nothing committed", result, err, h.commits)
		}
		if _, err := os.Stat(h.marker()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat marker = %v; want it removed", err)
		}
	})

	t.Run("CorruptStateResetsFromTheLastCommittedVersion", func(t *testing.T) {
		t.Parallel()
		h := newResetHarness(t)
		st := h.seed()
		good, _ := json.Marshal(st)
		h.history = []weaverengine.StateVersion{{Commit: "c-torn", Data: []byte("{")}, {Commit: "c1", Data: good}}
		if err := os.WriteFile(filepath.Join(h.deps.Geom.RunDir, "state.json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}

		result, err := weaverengine.Reset(h.deps, false)

		if err != nil || result.StartSHA != h.start(st) || len(h.moves) != 1 {
			t.Fatalf("Reset() = %+v, %v, moves %v; want the reset from the newest version that parses", result, err, h.moves)
		}
	})
}
