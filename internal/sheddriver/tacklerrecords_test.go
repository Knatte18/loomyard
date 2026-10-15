// tackler_test.go — untagged Tier-1 tests for the tackler's record readers and writers, over temp directories.

package sheddriver

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTacklerRecords pins the worktree snapshot comparison (the tamper cases), attempt, envelope and marker round-trips, attempt listing in order from disk, the applied mark, and the verdict reader's refusals.
func TestTacklerRecords(t *testing.T) {
	t.Parallel()

	t.Run("snapshot equality", func(t *testing.T) {
		t.Parallel()
		base := func() WorktreeSnapshot {
			return WorktreeSnapshot{Head: "abc", Branch: "task", Dirty: map[string]string{"a.go": "h1", "b.go": "h2"}}
		}
		tests := []struct {
			name   string
			other  func(s *WorktreeSnapshot)
			wantEq bool
		}{
			{"equal", func(*WorktreeSnapshot) {}, true},
			{"moved HEAD", func(s *WorktreeSnapshot) { s.Head = "def" }, false},
			{"switched branch", func(s *WorktreeSnapshot) { s.Branch = "other" }, false},
			{"a new dirty path", func(s *WorktreeSnapshot) { s.Dirty["c.go"] = "h3" }, false},
			{"a dirty path gone", func(s *WorktreeSnapshot) { delete(s.Dirty, "b.go") }, false},
			{"an already-dirty path edited", func(s *WorktreeSnapshot) { s.Dirty["a.go"] = "h9" }, false},
		}
		for _, tt := range tests {
			other := base()
			tt.other(&other)
			if got := base().Equal(other); got != tt.wantEq {
				t.Errorf("%s: Equal() = %v; want %v", tt.name, got, tt.wantEq)
			}
		}
		if !(WorktreeSnapshot{Head: "a"}).Equal(WorktreeSnapshot{Head: "a", Dirty: map[string]string{}}) {
			t.Error("Equal() of a nil and an empty dirty set = false; want true")
		}
	})

	t.Run("attempts round-trip, order by number and count from disk", func(t *testing.T) {
		t.Parallel()
		tacklerDir := filepath.Join(t.TempDir(), "tackler")
		listed, err := ListTacklerAttempts(tacklerDir)
		if err != nil || listed != nil {
			t.Fatalf("ListTacklerAttempts(absent) = %v, %v; want nil, nil", listed, err)
		}
		attempt := func(n int) TacklerAttempt {
			return TacklerAttempt{
				Number:       n,
				StopKey:      StopKey{CurrentProducer: "B", HistoryLength: n, State: "paused"},
				PauseBefore:  "C",
				PauseAfter:   "A",
				Snapshot:     WorktreeSnapshot{Head: "abc", Branch: "task", Dirty: map[string]string{"a.go": "h1"}},
				SpawnedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				Deadline:     time.Date(2026, 1, 2, 3, 14, 5, 0, time.UTC),
				EnvelopePath: "/e",
				ReportPath:   "/r",
				StrandGUID:   "guid",
			}
		}
		for _, n := range []int{2, 1} {
			if err := WriteTacklerAttempt(filepath.Join(tacklerDir, "attempt-"+strconv.Itoa(n)), attempt(n)); err != nil {
				t.Fatalf("WriteTacklerAttempt(%d) = %v; want nil", n, err)
			}
		}
		listed, err = ListTacklerAttempts(tacklerDir)
		if err != nil {
			t.Fatalf("ListTacklerAttempts() = _, %v; want nil", err)
		}
		if len(listed) != 2 || listed[0].Number != 1 || listed[1].Number != 2 {
			t.Fatalf("ListTacklerAttempts() numbers = %v; want [1 2]", listed)
		}
		want := attempt(1)
		want.Dir = filepath.Join(tacklerDir, "attempt-1")
		if !reflect.DeepEqual(listed[0], want) {
			t.Errorf("attempt 1 = %+v; want %+v", listed[0], want)
		}
		if err := os.MkdirAll(filepath.Join(tacklerDir, "attempt-3"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ListTacklerAttempts(tacklerDir); err == nil || !strings.Contains(err.Error(), "attempt-3") {
			t.Errorf("ListTacklerAttempts() with an attempt-less directory error = %v; want it to name attempt-3", err)
		}
	})

	t.Run("envelope and marker round-trip, absent files read not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		envelopePath := TacklerEnvelopePath(dir)
		if _, found, err := ReadTacklerEnvelope(envelopePath); found || err != nil {
			t.Fatalf("ReadTacklerEnvelope(absent) found = %v, err = %v; want false, nil", found, err)
		}
		envelope := TacklerEnvelope{Verdict: TacklerVerdictHandBack, Reason: "r", Question: "q", Goto: "G", Cause: "c", Attempt: 2, AttemptDir: dir, VerdictPath: TacklerVerdictPath(dir), CaseBriefPath: TacklerCaseBriefPath(dir)}
		if err := WriteTacklerEnvelope(envelopePath, envelope); err != nil {
			t.Fatalf("WriteTacklerEnvelope() = %v; want nil", err)
		}
		if got, found, err := ReadTacklerEnvelope(envelopePath); !found || err != nil || got != envelope {
			t.Errorf("ReadTacklerEnvelope() = %+v, %v, %v; want %+v, true, nil", got, found, err, envelope)
		}

		markerPath := filepath.Join(dir, "scratch", "marker.json")
		if _, found, err := ReadTacklerMarker(markerPath); found || err != nil {
			t.Fatalf("ReadTacklerMarker(absent) found = %v, err = %v; want false, nil", found, err)
		}
		marker := TacklerMarker{Attempt: 3, StrandGUID: "g", Deadline: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
		if err := WriteTacklerMarker(markerPath, marker); err != nil {
			t.Fatalf("WriteTacklerMarker() = %v; want nil", err)
		}
		if got, found, err := ReadTacklerMarker(markerPath); !found || err != nil || got != marker {
			t.Errorf("ReadTacklerMarker() = %+v, %v, %v; want %+v, true, nil", got, found, err, marker)
		}
		if err := RemoveTacklerMarker(markerPath); err != nil {
			t.Fatalf("RemoveTacklerMarker() = %v; want nil", err)
		}
		if err := RemoveTacklerMarker(markerPath); err != nil {
			t.Errorf("RemoveTacklerMarker(absent) = %v; want nil", err)
		}
	})

	t.Run("applied mark", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "attempt-1")
		if applied, err := TacklerApplied(dir); applied || err != nil {
			t.Fatalf("TacklerApplied(unmarked) = %v, %v; want false, nil", applied, err)
		}
		if err := MarkTacklerApplied(dir); err != nil {
			t.Fatalf("MarkTacklerApplied() = %v; want nil", err)
		}
		if applied, err := TacklerApplied(dir); !applied || err != nil {
			t.Errorf("TacklerApplied(marked) = %v, %v; want true, nil", applied, err)
		}
	})

	t.Run("verdict reader", func(t *testing.T) {
		t.Parallel()
		write := func(t *testing.T, content string) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "verdict.yaml")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}
		got, found, err := ReadTacklerVerdict(write(t, "verdict: hand-back\nreason: why\nquestion: which?\n"))
		if err != nil || !found {
			t.Fatalf("ReadTacklerVerdict(valid) = _, %v, %v; want found, nil", found, err)
		}
		if want := (TacklerVerdict{Verdict: TacklerVerdictHandBack, Reason: "why", Question: "which?"}); got != want {
			t.Errorf("ReadTacklerVerdict(valid) = %+v; want %+v", got, want)
		}
		if _, found, err := ReadTacklerVerdict(filepath.Join(t.TempDir(), "absent.yaml")); found || err != nil {
			t.Errorf("ReadTacklerVerdict(absent) found = %v, err = %v; want false, nil", found, err)
		}
		for name, content := range map[string]string{
			"unknown verdict word": "verdict: ponder\nreason: x\n",
			"undecodable YAML":     "verdict: [unclosed\n",
		} {
			if _, _, err := ReadTacklerVerdict(write(t, content)); err == nil {
				t.Errorf("ReadTacklerVerdict(%s) error = nil; want a refusal", name)
			}
		}
	})
}
