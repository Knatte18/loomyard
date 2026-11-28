// queue_test.go covers the landing queue store and its backstop, with a fake liveness check.

package landingqueue

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the package's tests under tmuxkit.Main.
// Their temp files then live in the process run directory and go with it.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}

const livePID, deadPID = 100, 200

// newTestStore returns a store of one branch in which only a process with livePID lives.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{
		Dir:    Dir(t.TempDir()),
		Branch: "feature/parent",
		Alive:  func(pid int, _ string) bool { return pid == livePID },
	}
}

// testEntry returns an entry of slug joined second seconds into a fixed day.
func testEntry(slug string, second int) Entry {
	return Entry{
		Slug:        slug,
		Pair:        "/pairs/" + slug,
		RunID:       "run-" + slug,
		PID:         livePID,
		StartTime:   "t1",
		TaskHead:    "head-1",
		RecordsHead: "records-1",
		Message:     "land " + slug,
		TrueMerge:   true,
		JoinedAt:    time.Date(2026, 1, 1, 0, 0, second, 0, time.UTC),
	}
}

func join(t *testing.T, store *Store, entry Entry) {
	t.Helper()
	if err := store.Join(entry); err != nil {
		t.Fatal(err)
	}
}

func setOutcome(t *testing.T, store *Store, slug string, outcome Outcome, commit string) {
	t.Helper()
	if err := store.SetOutcome(slug, outcome, commit); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, store *Store, slug string) (Entry, bool) {
	t.Helper()
	entry, found, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}
	return entry, found
}

func slugs(t *testing.T, store *Store) []string {
	t.Helper()
	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Slug)
	}
	return names
}

func TestStore_Entries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		check func(t *testing.T, store *Store)
	}{
		{"ReadsBackFieldForField", func(t *testing.T, store *Store) {
			join(t, store, testEntry("alpha", 1))
			got, found := read(t, store, "alpha")
			if want := testEntry("alpha", 1); !found || !reflect.DeepEqual(got, want) {
				t.Errorf("Read() = %+v, %v; want %+v, true", got, found, want)
			}
		}},
		{"RewriteKeepsAnUnknownKey", func(t *testing.T, store *Store) {
			join(t, store, testEntry("alpha", 1))
			path := filepath.Join(store.Dir, "666561747572652f706172656e74", "616c706861.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			newer := strings.Replace(string(data), "{", `{"newer_key": "kept",`, 1)
			if err := os.WriteFile(path, []byte(newer), 0o644); err != nil {
				t.Fatal(err)
			}
			setOutcome(t, store, "alpha", OutcomeLanded, "c1")
			rewritten, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(rewritten), `"newer_key": "kept"`) {
				t.Errorf("rewritten entry = %s; want it to keep newer_key", rewritten)
			}
		}},
		{"JoinKeepsAFailedOrConflictOutcome", func(t *testing.T, store *Store) {
			for _, outcome := range []Outcome{OutcomeFailed, OutcomeConflict} {
				join(t, store, testEntry("alpha", 1))
				setOutcome(t, store, "alpha", outcome, "c1")
				moved := testEntry("alpha", 2)
				moved.TaskHead = "head-2"
				join(t, store, moved)
				got, _ := read(t, store, "alpha")
				if got.Outcome != outcome || got.Commit != "c1" || got.TaskHead != "head-2" {
					t.Errorf("Join over %s = outcome %q, commit %q, head %q; want %q, c1, head-2",
						outcome, got.Outcome, got.Commit, got.TaskHead, outcome)
				}
				setOutcome(t, store, "alpha", "", "ignored")
				if got, _ := read(t, store, "alpha"); got.Outcome != "" || got.Commit != "" {
					t.Errorf("SetOutcome(\"\") left outcome %q, commit %q; want both empty", got.Outcome, got.Commit)
				}
			}
		}},
		{"JoinKeepsLandedOnlyForTheSameHead", func(t *testing.T, store *Store) {
			join(t, store, testEntry("alpha", 1))
			setOutcome(t, store, "alpha", OutcomeLanded, "c1")
			join(t, store, testEntry("alpha", 2))
			if got, _ := read(t, store, "alpha"); got.Outcome != OutcomeLanded || got.Commit != "c1" {
				t.Errorf("Join with the same head = outcome %q, commit %q; want landed, c1", got.Outcome, got.Commit)
			}
			moved := testEntry("alpha", 3)
			moved.TaskHead = "head-2"
			join(t, store, moved)
			if got, _ := read(t, store, "alpha"); got.Outcome != "" || got.Commit != "" {
				t.Errorf("Join with a moved head = outcome %q, commit %q; want both empty", got.Outcome, got.Commit)
			}
		}},
		{"LiveJudgesTheOwnerProcess", func(t *testing.T, store *Store) {
			if !store.Live(Entry{PID: livePID}) {
				t.Error("Live(live owner) = false; want true")
			}
			if store.Live(Entry{PID: deadPID}) {
				t.Error("Live(dead owner) = true; want false")
			}
		}},
		{"PruneRemovesOnlyADeadOwnerWithoutOutcome", func(t *testing.T, store *Store) {
			dead := testEntry("dead", 1)
			dead.PID = deadPID
			decided := testEntry("decided", 2)
			decided.PID = deadPID
			join(t, store, dead)
			join(t, store, decided)
			join(t, store, testEntry("live", 3))
			setOutcome(t, store, "decided", OutcomeFailed, "c1")
			pruned, err := store.Prune()
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{"dead"}; !slices.Equal(pruned, want) {
				t.Errorf("Prune() = %v; want %v", pruned, want)
			}
			if got, want := slugs(t, store), []string{"decided", "live"}; !slices.Equal(got, want) {
				t.Errorf("entries after Prune = %v; want %v", got, want)
			}
		}},
		{"EntriesAreInJoinOrder", func(t *testing.T, store *Store) {
			join(t, store, testEntry("late", 3))
			join(t, store, testEntry("bravo", 1))
			join(t, store, testEntry("alpha", 1))
			if got, want := slugs(t, store), []string{"alpha", "bravo", "late"}; !slices.Equal(got, want) {
				t.Errorf("Entries() = %v; want %v", got, want)
			}
		}},
		{"SetOutcomeOnAnAbsentSlugErrors", func(t *testing.T, store *Store) {
			err := store.SetOutcome("ghost", OutcomeLanded, "c1")
			if err == nil || !strings.Contains(err.Error(), "ghost") {
				t.Errorf("SetOutcome(ghost) = %v; want an error naming ghost", err)
			}
		}},
		{"RemoveDeletesAndIgnoresAnAbsentEntry", func(t *testing.T, store *Store) {
			if err := store.Remove("alpha"); err != nil {
				t.Errorf("Remove(absent) = %v; want nil", err)
			}
			join(t, store, testEntry("alpha", 1))
			if err := store.Remove("alpha"); err != nil {
				t.Fatal(err)
			}
			if _, found := read(t, store, "alpha"); found {
				t.Error("Read() after Remove found the entry")
			}
		}},
		{"ASecondMarkReplacesTheFirst", func(t *testing.T, store *Store) {
			for _, base := range []string{"base-1", "base-2"} {
				if err := store.WriteMark(MoveMark{Base: base, Members: []MarkMember{{Slug: base}}}); err != nil {
					t.Fatal(err)
				}
			}
			got, found, err := store.ReadMark()
			want := MoveMark{Base: "base-2", Members: []MarkMember{{Slug: "base-2"}}}
			if err != nil || !found || !reflect.DeepEqual(got, want) {
				t.Errorf("ReadMark() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.check(t, newTestStore(t))
		})
	}
}

func TestStore_Backstop(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	mark := &MoveMark{Base: "base", Members: []MarkMember{
		{Slug: "alpha", Commit: "c1", TaskHead: "head-1", Records: "r1"},
		{Slug: "bravo", Commit: "c2", TaskHead: "head-1", Records: "r2"},
	}}
	writeFailure := errors.New("ledger unwritable")
	tests := []struct {
		name   string
		mark   *MoveMark
		queued []string
		// rejoined are the queued slugs that join again with another task head after the mark.
		rejoined    []string
		parentHolds []string
		// upstreamHolds are the commits the parent's upstream already has.
		upstreamHolds []string
		writeErr      error
		wantWritten   int
		wantLandings  []string
		// wantLanded maps each queued slug to the commit it must read as landed with, empty for no outcome.
		wantLanded   map[string]string
		wantMarkKept bool
		wantSeamless bool
	}{
		{
			name: "WritesEachHeldUnpushedMember", mark: mark,
			queued: []string{"alpha", "bravo"}, parentHolds: []string{"c1", "c2"},
			wantWritten: 2, wantLandings: []string{"alpha c1 r1", "bravo c2 r2"},
			wantLanded: map[string]string{"alpha": "c1", "bravo": "c2"},
		},
		{
			name: "SkipsAMemberTheUpstreamHolds", mark: mark,
			queued: []string{"alpha", "bravo"}, parentHolds: []string{"c1", "c2"}, upstreamHolds: []string{"c1"},
			wantWritten: 1, wantLandings: []string{"bravo c2 r2"},
			wantLanded: map[string]string{"alpha": "", "bravo": "c2"},
		},
		{
			name: "WritesTheLedgerAloneForAMemberWithNoEntry", mark: mark,
			queued: []string{"alpha"}, parentHolds: []string{"c1", "c2"},
			wantWritten: 2, wantLandings: []string{"alpha c1 r1", "bravo c2 r2"},
			wantLanded: map[string]string{"alpha": "c1"},
		},
		{
			name: "GivesARejoinedEntryItsLedgerLandingAndNoOutcome", mark: mark,
			queued: []string{"alpha", "bravo"}, rejoined: []string{"alpha"}, parentHolds: []string{"c1", "c2"},
			wantWritten: 2, wantLandings: []string{"alpha c1 r1", "bravo c2 r2"},
			wantLanded: map[string]string{"alpha": "", "bravo": "c2"},
		},
		{
			name: "RemovesAMarkTheParentNeverMovedTo", mark: mark,
			queued:     []string{"alpha", "bravo"},
			wantLanded: map[string]string{"alpha": "", "bravo": ""},
		},
		{
			name: "KeepsTheMarkOnAWriteError", mark: mark,
			queued: []string{"alpha"}, parentHolds: []string{"c1", "c2"}, writeErr: writeFailure,
			wantLandings: []string{"alpha c1 r1"}, wantLanded: map[string]string{"alpha": ""}, wantMarkKept: true,
		},
		{name: "DoesNothingWithoutAMark", wantSeamless: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newTestStore(t)
			for i, slug := range tt.queued {
				join(t, store, testEntry(slug, i))
			}
			if tt.mark != nil {
				if err := store.WriteMark(*tt.mark); err != nil {
					t.Fatal(err)
				}
			}
			for _, slug := range tt.rejoined {
				entry, _ := read(t, store, slug)
				entry.TaskHead = "head-2"
				join(t, store, entry)
			}
			calls := 0
			var landings []string
			written, err := store.Backstop(BackstopSeams{
				ParentHolds: func(commit string) (bool, error) {
					calls++
					return slices.Contains(tt.parentHolds, commit), nil
				},
				UpstreamLacks: func(commit string) (bool, error) {
					calls++
					return !slices.Contains(tt.upstreamHolds, commit), nil
				},
				WriteLanding: func(slug, tip, records string) error {
					calls++
					landings = append(landings, slug+" "+tip+" "+records)
					return tt.writeErr
				},
			})

			if !errors.Is(err, tt.writeErr) || written != tt.wantWritten {
				t.Errorf("Backstop() = %d, %v; want %d, %v", written, err, tt.wantWritten, tt.writeErr)
			}
			if !slices.Equal(landings, tt.wantLandings) {
				t.Errorf("landings = %q; want %q", landings, tt.wantLandings)
			}
			if tt.wantSeamless && calls != 0 {
				t.Errorf("seam calls = %d; want 0", calls)
			}
			for slug, commit := range tt.wantLanded {
				entry, _ := read(t, store, slug)
				wantOutcome := OutcomeLanded
				if commit == "" {
					wantOutcome = ""
				}
				if entry.Outcome != wantOutcome || entry.Commit != commit {
					t.Errorf("entry %s = outcome %q, commit %q; want %q, %q", slug, entry.Outcome, entry.Commit, wantOutcome, commit)
				}
			}
			if got, want := slugs(t, store), tt.queued; !slices.Equal(got, want) {
				t.Errorf("entries after Backstop = %v; want %v", got, want)
			}
			if _, kept, err := store.ReadMark(); err != nil || kept != tt.wantMarkKept {
				t.Errorf("ReadMark() after Backstop = %v, %v; want %v, nil", kept, err, tt.wantMarkKept)
			}
		})
	}
}
