// round_test.go covers the round row decorator and the round archive.

package hardenshed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeProducer is a shedengine.ShedProducer whose call is the function.
type fakeProducer func() (shedengine.Outcome, shedengine.OutputPointer, error)

func (f fakeProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return f()
}

// rowFixture is a run directory and worktree with recording seams for one decorated row.
type rowFixture struct {
	runDir, anchor string
	log            []string
	commitPaths    [][]string
	reapErr        error
	commitErr      error
	deps           RowDeps
}

func newRowFixture(t *testing.T, kind RowKind) *rowFixture {
	t.Helper()
	fixture := &rowFixture{runDir: t.TempDir(), anchor: t.TempDir()}
	fixture.deps = RowDeps{
		Row:    "harden-" + string(kind),
		Kind:   kind,
		RunDir: fixture.runDir,
		Reap: func(row string, point ReapPoint) ([]StrayEntry, error) {
			fixture.log = append(fixture.log, "reap-"+string(point))
			if point == ReapEnd && fixture.reapErr != nil {
				return nil, fixture.reapErr
			}
			return []StrayEntry{{Row: row, Point: point, Kind: StrayProcess, PID: len(fixture.log)}}, nil
		},
		Models: func(round int) (string, string) {
			return fmt.Sprintf("reviewer-%d", round), fmt.Sprintf("fixer-%d", round)
		},
		Head: func() (string, error) { return "head0", nil },
		Commits: func(from string) ([]string, error) {
			fixture.log = append(fixture.log, "commits-from-"+from)
			return []string{"c1", "c2"}, nil
		},
		Archive: ArchiveDeps{
			RunDir:     fixture.runDir,
			AnchorRoot: fixture.anchor,
			Commit: func(relPaths []string, _ string) error {
				fixture.commitPaths = append(fixture.commitPaths, relPaths)
				return fixture.commitErr
			},
		},
	}
	return fixture
}

// writeFile writes content as the named file of dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readRecord decodes the named JSON file of dir into a T.
func readRecord[T any](t *testing.T, dir, name string) T {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	var record T
	if err := json.Unmarshal(content, &record); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return record
}

func TestWrapRow(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ReviewRowReapsAtBothEndsAndWritesTheModels", func(t *testing.T) {
		t.Parallel()
		fixture := newRowFixture(t, RowReview)
		inner := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			fixture.log = append(fixture.log, "call")
			writeFile(t, fixture.runDir, "round-1-review.md", "review")
			return shedengine.Done, shedengine.OutputPointer{Path: "review"}, nil
		})

		outcome, pointer, err := WrapRow(inner, fixture.deps).Call(context.Background())
		if err != nil || outcome != shedengine.Done || pointer.Path != "review" {
			t.Fatalf("Call() = %q, %+v, %v; want done, the inner pointer, nil", outcome, pointer, err)
		}
		if want := []string{"reap-start", "call", "reap-end"}; !slices.Equal(fixture.log, want) {
			t.Errorf("calls = %v; want %v", fixture.log, want)
		}
		models := readRecord[RoundModels](t, fixture.runDir, "harden-models-1.json")
		if want := (RoundModels{Round: 1, Review: "reviewer-1", Fix: "fixer-1"}); models != want {
			t.Errorf("models file = %+v; want %+v", models, want)
		}
		strays := readRecord[StrayReport](t, fixture.runDir, "harden-strays-1.json")
		want := StrayReport{Round: 1, Entries: []StrayEntry{
			{Row: "harden-review", Point: ReapStart, Kind: StrayProcess, PID: 1, Interrupted: true},
			{Row: "harden-review", Point: ReapEnd, Kind: StrayProcess, PID: 3},
		}}
		if !reflect.DeepEqual(strays, want) {
			t.Errorf("stray report = %+v; want %+v", strays, want)
		}
	})

	t.Run("ReviewRowKeepsTheRoundOfItsWrittenReview", func(t *testing.T) {
		t.Parallel()
		fixture := newRowFixture(t, RowReview)
		writeFile(t, fixture.runDir, "round-1-review.md", "review")
		inner := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			return shedengine.Done, shedengine.OutputPointer{}, nil
		})

		if _, _, err := WrapRow(inner, fixture.deps).Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(fixture.runDir, "harden-models-1.json")); err != nil {
			t.Errorf("models file of round 1: %v; want it written", err)
		}
		if _, err := os.Stat(filepath.Join(fixture.runDir, "harden-models-2.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("models file of round 2: %v; want it absent", err)
		}
	})

	t.Run("FixRowWritesTheCommitsReapsAndArchives", func(t *testing.T) {
		t.Parallel()
		fixture := newRowFixture(t, RowFix)
		writeFile(t, fixture.runDir, "round-1-review.md", "review")
		writeFile(t, fixture.runDir, "round-1-bouncer-ledger.md", "ledger")
		inner := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			fixture.log = append(fixture.log, "call")
			writeFile(t, fixture.runDir, "round-1-fixer-report.md", "report")
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		})

		outcome, _, err := WrapRow(inner, fixture.deps).Call(context.Background())
		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, _, %v; want stuck, nil", outcome, err)
		}
		if want := []string{"reap-start", "call", "commits-from-head0", "reap-end"}; !slices.Equal(fixture.log, want) {
			t.Errorf("calls = %v; want %v", fixture.log, want)
		}
		commits := readRecord[RoundCommits](t, fixture.runDir, "harden-commits-1.json")
		if want := (RoundCommits{Round: 1, Head: "head0", Commits: []string{"c1", "c2"}}); !reflect.DeepEqual(commits, want) {
			t.Errorf("commits file = %+v; want %+v", commits, want)
		}
		if want := [][]string{{"_lym/harden/round-1"}}; !reflect.DeepEqual(fixture.commitPaths, want) {
			t.Errorf("archive commits = %v; want %v", fixture.commitPaths, want)
		}
		archived := filepath.Join(fixture.anchor, "_lym", "harden", "round-1")
		for _, name := range []string{"round-1-review.md", "round-1-fixer-report.md", "harden-commits-1.json", "harden-head-1.json", "round-1-bouncer-ledger.md"} {
			if _, err := os.Stat(filepath.Join(archived, name)); err != nil {
				t.Errorf("archived %s: %v; want it present", name, err)
			}
		}
		head := readRecord[RoundHead](t, fixture.runDir, "harden-head-1.json")
		if want := (RoundHead{Round: 1, Head: "head0"}); head != want {
			t.Errorf("head file = %+v; want %+v", head, want)
		}
		strays := readRecord[StrayReport](t, archived, "harden-strays-1.json")
		wantStrays := StrayReport{Round: 1, Entries: []StrayEntry{
			{Row: "harden-fix", Point: ReapStart, Kind: StrayProcess, PID: 1, Interrupted: true},
			{Row: "harden-fix", Point: ReapEnd, Kind: StrayProcess, PID: 4},
		}}
		if !reflect.DeepEqual(strays, wantStrays) {
			t.Errorf("archived stray report = %+v; want the start entry and the end-of-row reap's %+v", strays, wantStrays)
		}
	})

	t.Run("ResumedFixRowKeepsTheHeadItFirstRecorded", func(t *testing.T) {
		t.Parallel()
		fixture := newRowFixture(t, RowFix)
		writeFile(t, fixture.runDir, "round-1-review.md", "review")
		writeFile(t, fixture.runDir, "harden-head-1.json", `{"round":1,"head":"head-first"}`)
		inner := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		})

		if _, _, err := WrapRow(inner, fixture.deps).Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		commits := readRecord[RoundCommits](t, fixture.runDir, "harden-commits-1.json")
		if want := (RoundCommits{Round: 1, Head: "head-first", Commits: []string{"c1", "c2"}}); !reflect.DeepEqual(commits, want) {
			t.Errorf("commits file = %+v; want %+v, listing the commits past the first call's head", commits, want)
		}
		if want := []string{"reap-start", "commits-from-head-first", "reap-end"}; !slices.Equal(fixture.log, want) {
			t.Errorf("calls = %v; want %v", fixture.log, want)
		}
	})

	t.Run("InnerErrorStillReapsAndArchivesNothing", func(t *testing.T) {
		t.Parallel()
		fixture := newRowFixture(t, RowFix)
		writeFile(t, fixture.runDir, "round-1-review.md", "review")
		innerErr := errors.New("fixer died")
		inner := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			return "", shedengine.OutputPointer{}, innerErr
		})

		_, _, err := WrapRow(inner, fixture.deps).Call(context.Background())
		if !errors.Is(err, innerErr) {
			t.Errorf("Call() error = %v; want it to wrap %v", err, innerErr)
		}
		if want := []string{"reap-start", "reap-end"}; !slices.Equal(fixture.log, want) {
			t.Errorf("calls = %v; want %v", fixture.log, want)
		}
		if _, statErr := os.Stat(filepath.Join(fixture.runDir, "harden-commits-1.json")); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("commits file: %v; want it absent", statErr)
		}
		if len(fixture.commitPaths) != 0 {
			t.Errorf("archive commits = %v; want none", fixture.commitPaths)
		}
	})

	t.Run("FailingReapAndFailingArchiveAreJoinedWithTheirWayForward", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("inner failed")
		reapErr := errors.New("reap failed")
		commitErr := errors.New("commit failed")

		reaping := newRowFixture(t, RowReview)
		reaping.reapErr = reapErr
		inner := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			return "", shedengine.OutputPointer{}, innerErr
		})
		_, _, err := WrapRow(inner, reaping.deps).Call(context.Background())
		const clear = "run lym harden sandbox clear in the task worktree, then lym loom resume"
		if !errors.Is(err, innerErr) || !errors.Is(err, reapErr) || !strings.Contains(err.Error(), clear) {
			t.Errorf("Call() error = %v; want it to wrap the inner and reap errors and name %q", err, clear)
		}

		archiving := newRowFixture(t, RowFix)
		archiving.commitErr = commitErr
		writeFile(t, archiving.runDir, "round-1-review.md", "review")
		ok := fakeProducer(func() (shedengine.Outcome, shedengine.OutputPointer, error) {
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		})
		_, _, err = WrapRow(ok, archiving.deps).Call(context.Background())
		if !errors.Is(err, commitErr) || !strings.HasSuffix(err.Error(), "way forward: run lym loom resume") || strings.Contains(err.Error(), "sandbox clear") {
			t.Errorf("Call() error = %v; want it to wrap the commit error and name only lym loom resume", err)
		}
	})
}

func TestArchiveRound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	runDir, anchor := t.TempDir(), t.TempDir()
	var committed [][]string
	deps := ArchiveDeps{
		RunDir:     runDir,
		AnchorRoot: anchor,
		Commit:     func(relPaths []string, _ string) error { committed = append(committed, relPaths); return nil },
	}
	writeFile(t, runDir, "round-1-review.md", "review one")
	writeFile(t, runDir, "round-2-review.md", "review two")
	writeFile(t, runDir, "round-1-bouncer-ledger.md", "ledger one")
	writeFile(t, runDir, "round-2-bouncer-ledger.md", "ledger two")
	writeFile(t, runDir, "harden-live-1.json", "{}")
	writeFile(t, runDir, "harden-live-12.json", "{}")

	if err := ArchiveRound(deps, 1); err != nil {
		t.Fatalf("ArchiveRound(1) error = %v", err)
	}
	archived := filepath.Join(anchor, "_lym", "harden", "round-1")
	entries, err := os.ReadDir(archived)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if want := []string{"harden-live-1.json", "round-1-bouncer-ledger.md", "round-1-review.md", "round-2-bouncer-ledger.md"}; !slices.Equal(names, want) {
		t.Errorf("archived files = %v; want %v", names, want)
	}
	if want := [][]string{{"_lym/harden/round-1"}}; !reflect.DeepEqual(committed, want) {
		t.Errorf("commits = %v; want %v", committed, want)
	}

	writeFile(t, runDir, "round-1-review.md", "review one, edited")
	if err := ArchiveRound(deps, 1); err != nil {
		t.Fatalf("second ArchiveRound(1) error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(archived, "round-1-review.md")); err != nil || string(content) != "review one, edited" {
		t.Errorf("archived review after a second archive = %q, %v; want the edited content", content, err)
	}

	if err := ArchiveFinalLedger(deps); err != nil {
		t.Fatalf("ArchiveFinalLedger() error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(anchor, "_lym", "harden", "round-2-bouncer-ledger.md")); err != nil || string(content) != "ledger two" {
		t.Errorf("final ledger = %q, %v; want the latest ledger", content, err)
	}
	if last := committed[len(committed)-1]; !reflect.DeepEqual(last, []string{"_lym/harden/round-2-bouncer-ledger.md"}) {
		t.Errorf("final ledger commit = %v; want its path", last)
	}

	if err := ArchiveFinalLedger(ArchiveDeps{RunDir: t.TempDir(), AnchorRoot: anchor, Commit: deps.Commit}); err == nil {
		t.Error("ArchiveFinalLedger() over a run directory with no ledger = nil; want an error")
	}
}

func TestCampaignArchivesUnderTheAnchorPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	runDir, anchor, worktree := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, runDir, "round-2-bouncer-ledger.md", "ledger two")
	var committed [][]string
	campaign := &Campaign{
		AnchorPath:   anchor,
		WorktreeRoot: worktree,
		Commit:       func(relPaths []string, _ string) error { committed = append(committed, relPaths); return nil },
	}

	if err := campaign.ArchiveFinal(runDir); err != nil {
		t.Fatalf("ArchiveFinal() error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(anchor, "_lym", "harden", "round-2-bouncer-ledger.md")); err != nil || string(content) != "ledger two" {
		t.Errorf("ledger under the anchor = %q, %v; want the run directory's ledger", content, err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "_lym")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(_lym under the worktree) = %v; want it absent, the archive sitting under the anchor alone", err)
	}
	if want := [][]string{{"_lym/harden/round-2-bouncer-ledger.md"}}; !reflect.DeepEqual(committed, want) {
		t.Errorf("commits = %v; want the anchor-relative path %v", committed, want)
	}
}
