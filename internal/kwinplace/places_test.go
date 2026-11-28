// places_test.go pins the rules that update remembered places and the places file's reads and writes.

package kwinplace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remembered is the part of an entry the update rules decide.
type remembered struct {
	home    string
	key     int
	last    string
	settled bool
}

// at is a settled window homed and last seen on home.
func at(home string, key int) remembered {
	return remembered{home: home, key: key, last: home, settled: true}
}

func unsettled(r remembered) remembered {
	r.settled = false
	return r
}

func placesOf(entries map[string]remembered) Places {
	places := Places{}
	for worktree, r := range entries {
		entry := Entry{Worktree: worktree, PID: 1, Home: r.home, OrderKey: r.key, LastDesktop: r.last}
		if r.settled {
			entry.SettledBy = 1
		}
		places[worktree] = entry
	}
	return places
}

func rememberedOf(places Places) map[string]remembered {
	got := map[string]remembered{}
	for worktree, entry := range places {
		got[worktree] = remembered{home: entry.Home, key: entry.OrderKey, last: entry.LastDesktop, settled: entry.Settled()}
	}
	return got
}

// listing is a worktree list that answers names.
func listing(names ...string) func() ([]string, error) {
	return func() ([]string, error) { return names, nil }
}

// seenAt is a window that stands on desktop at position order, not minimized.
func seenAt(worktree, desktop string, order int) SeenWindow {
	return SeenWindow{Worktree: worktree, Desktop: desktop, Order: order}
}

func TestUpdatePlaces(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		cur  map[string]remembered
		// placed is the placed window's worktree, and landed the desktop it landed on.
		placed     string
		landed     string
		overflowed bool
		fallback   bool
		homeGone   bool
		gone       []string
		listErr    error
		seen       []SeenWindow
		want       map[string]remembered
	}{
		{
			name:   "EveryReportedWindowIsRecorded",
			cur:    map[string]remembered{"p": {}, "w": at("d1", 0)},
			placed: "p", landed: "d1",
			seen: []SeenWindow{seenAt("p", "d1", 0), seenAt("w", "d2", 0)},
			want: map[string]remembered{"p": at("d1", 1), "w": at("d2", 0)},
		},
		{
			name:   "ReopenInKeyOrderKeepsKeys",
			cur:    map[string]remembered{"a": unsettled(at("d1", 0)), "b": at("d1", 1), "c": at("d1", 2)},
			placed: "a", landed: "d1",
			seen: []SeenWindow{seenAt("a", "d1", 0), seenAt("b", "d1", 1), seenAt("c", "d1", 2)},
			want: map[string]remembered{"a": at("d1", 0), "b": at("d1", 1), "c": at("d1", 2)},
		},
		{
			name:   "HandMoveRenumbersTheDesktop",
			cur:    map[string]remembered{"p": {}, "a": at("d1", 0), "b": at("d2", 0), "c": at("d2", 1)},
			placed: "p", landed: "d3",
			seen: []SeenWindow{seenAt("a", "d2", 0), seenAt("b", "d2", 1), seenAt("c", "d2", 2)},
			want: map[string]remembered{"p": at("d3", 0), "a": at("d2", 0), "b": at("d2", 1), "c": at("d2", 2)},
		},
		{
			name:   "RemovedWorktreeIsDropped",
			cur:    map[string]remembered{"p": at("d1", 0), "old": at("d1", 1)},
			placed: "p", landed: "d1", gone: []string{"old"},
			want: map[string]remembered{"p": at("d1", 0)},
		},
		{
			name:   "UnlistedWorktreesDropNothing",
			cur:    map[string]remembered{"p": at("d1", 0), "old": at("d1", 1)},
			placed: "p", landed: "d1", gone: []string{"old"}, listErr: errors.New("no list"),
			want: map[string]remembered{"p": at("d1", 0), "old": at("d1", 1)},
		},
		{
			name:   "GoneHomeIsReplaced",
			cur:    map[string]remembered{"p": unsettled(at("d9", 2))},
			placed: "p", landed: "d1", homeGone: true,
			want: map[string]remembered{"p": at("d1", 0)},
		},
		{
			name:   "MinimizedWindowKeepsItsKey",
			cur:    map[string]remembered{"a": at("d1", 0), "b": at("d1", 1), "c": at("d1", 2)},
			placed: "c", landed: "d1",
			seen: []SeenWindow{seenAt("b", "d1", 0), seenAt("c", "d1", 1), {Worktree: "a", Desktop: "d1", Order: 2, Minimized: true}},
			want: map[string]remembered{"a": at("d1", 0), "b": at("d1", 1), "c": at("d1", 2)},
		},
		{
			name:   "UnsettledWindowKeepsItsHome",
			cur:    map[string]remembered{"p": at("d3", 0), "u": unsettled(at("d1", 0))},
			placed: "p", landed: "d3",
			seen: []SeenWindow{seenAt("u", "d2", 0)},
			want: map[string]remembered{"p": at("d3", 0), "u": unsettled(at("d1", 0))},
		},
		{
			name:   "ApplyElsewhereKeepsAnUnsettledKey",
			cur:    map[string]remembered{"p": {}, "x": unsettled(at("d1", 0)), "y": at("d1", 1)},
			placed: "p", landed: "d2",
			seen: []SeenWindow{seenAt("y", "d1", 0), seenAt("x", "d1", 1), seenAt("p", "d2", 0)},
			want: map[string]remembered{"p": at("d2", 0), "x": unsettled(at("d1", 0)), "y": at("d1", 1)},
		},
		{
			name:   "ReopeningOnTheHomeEndsInKeyOrder",
			cur:    map[string]remembered{"x": unsettled(at("d1", 0)), "y": at("d1", 1)},
			placed: "x", landed: "d1",
			seen: []SeenWindow{seenAt("x", "d1", 0), seenAt("y", "d1", 1)},
			want: map[string]remembered{"x": at("d1", 0), "y": at("d1", 1)},
		},
		{
			name:   "OverflowKeepsHomeAndKey",
			cur:    map[string]remembered{"p": at("d1", 3)},
			placed: "p", landed: "d2", overflowed: true,
			seen: []SeenWindow{seenAt("p", "d2", 0)},
			want: map[string]remembered{"p": {home: "d1", key: 3, last: "d2", settled: true}},
		},
		{
			name:   "RenumberKeepsAnAbsentEntryInPlace",
			cur:    map[string]remembered{"a": at("d1", 0), "absent": unsettled(at("d1", 1)), "c": at("d1", 2)},
			placed: "c", landed: "d1",
			seen: []SeenWindow{seenAt("c", "d1", 0), seenAt("a", "d1", 1)},
			want: map[string]remembered{"c": at("d1", 0), "absent": unsettled(at("d1", 1)), "a": at("d1", 2)},
		},
		{
			name: "RenumberLeavesAnOverflowedWindowHomed",
			cur: map[string]remembered{
				"o": {home: "d1", key: 0, last: "d2", settled: true},
				"a": at("d2", 0),
				"b": at("d2", 1),
			},
			placed: "b", landed: "d2",
			seen: []SeenWindow{seenAt("b", "d2", 0), seenAt("o", "d2", 1), seenAt("a", "d2", 2)},
			want: map[string]remembered{"o": {home: "d1", key: 0, last: "d2", settled: true}, "b": at("d2", 0), "a": at("d2", 1)},
		},
		{
			name: "RenumberOfTheHomeKeepsAnOverflowedPosition",
			cur: map[string]remembered{
				"a": at("d1", 0),
				"o": {home: "d1", key: 1, last: "d2", settled: true},
				"c": at("d1", 2),
			},
			placed: "c", landed: "d1",
			seen: []SeenWindow{seenAt("c", "d1", 0), seenAt("a", "d1", 1), seenAt("o", "d2", 0)},
			want: map[string]remembered{"c": at("d1", 0), "o": {home: "d1", key: 1, last: "d2", settled: true}, "a": at("d1", 2)},
		},
		{
			name:   "FallbackAnchoredWindowTakesNoHome",
			cur:    map[string]remembered{"p": {}},
			placed: "p", landed: "d1", fallback: true,
			seen: []SeenWindow{seenAt("p", "d1", 0)},
			want: map[string]remembered{"p": {last: "d1", settled: true}},
		},
		{
			name:   "EmptyLandedDesktopSettlesAndKeepsThePlace",
			cur:    map[string]remembered{"p": unsettled(at("d1", 2))},
			placed: "p",
			want:   map[string]remembered{"p": at("d1", 2)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var worktrees []string
			for worktree := range tt.cur {
				if !slices.Contains(tt.gone, worktree) {
					worktrees = append(worktrees, worktree)
				}
			}
			got := rememberedOf(updatePlaces(placesOf(tt.cur), ReportUpdate{
				Worktree:   tt.placed,
				PID:        1,
				Landed:     tt.landed,
				Overflowed: tt.overflowed,
				Fallback:   tt.fallback,
				HomeGone:   tt.homeGone,
				Worktrees:  func() ([]string, error) { return worktrees, tt.listErr },
				Windows:    tt.seen,
			}))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("updatePlaces() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestPlacesFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	newPath := func(t *testing.T) string { return PlacesPath(t.TempDir()) }
	// seed writes entries straight to the places file.
	seed := func(t *testing.T, path string, entries map[string]remembered) {
		t.Helper()
		if err := writePlaces(path, func(Places) Places { return placesOf(entries) }); err != nil {
			t.Fatal(err)
		}
	}
	check := func(t *testing.T, path string, want map[string]remembered) {
		t.Helper()
		if got := rememberedOf(ReadPlaces(path)); !reflect.DeepEqual(got, want) {
			t.Errorf("places = %+v; want %+v", got, want)
		}
	}

	t.Run("AbsentReadsAsEmpty", func(t *testing.T) {
		t.Parallel()
		check(t, newPath(t), map[string]remembered{})
	})

	t.Run("UndecodableReadsAsEmptyAndHeals", func(t *testing.T) {
		t.Parallel()
		path := newPath(t)
		seed(t, path, nil)
		if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		check(t, path, map[string]remembered{})

		var group sync.WaitGroup
		for _, worktree := range []string{"a", "b"} {
			group.Go(func() {
				if err := RecordOpen(path, worktree, 5, "title", now); err != nil {
					t.Errorf("RecordOpen(%s) error = %v", worktree, err)
				}
			})
		}
		group.Wait()
		check(t, path, map[string]remembered{"a": {}, "b": {}})
	})

	t.Run("RecordOpenKeepsThePlaceAndUnsettles", func(t *testing.T) {
		t.Parallel()
		path := newPath(t)
		seed(t, path, map[string]remembered{"a": {home: "d1", key: 2, last: "d2", settled: true}})
		// The entry's own previous process id, as after a reboot.
		if err := RecordOpen(path, "a", 1, "title", now); err != nil {
			t.Fatal(err)
		}
		check(t, path, map[string]remembered{"a": {home: "d1", key: 2, last: "d2"}})
		want := Entry{Worktree: "a", PID: 1, Title: "title", Home: "d1", OrderKey: 2, LastDesktop: "d2", RecordedAt: now}
		if got := ReadPlaces(path)["a"]; got != want {
			t.Errorf("entry = %+v; want %+v", got, want)
		}
	})

	t.Run("SettleKeepsTheHome", func(t *testing.T) {
		t.Parallel()
		path := newPath(t)
		seed(t, path, map[string]remembered{"a": unsettled(at("d1", 2))})
		if err := Settle(path, "a", 1, "d3", now); err != nil {
			t.Fatal(err)
		}
		check(t, path, map[string]remembered{"a": {home: "d1", key: 2, last: "d3", settled: true}})
		if err := Settle(path, "a", 1, "", now); err != nil {
			t.Fatal(err)
		}
		check(t, path, map[string]remembered{"a": {home: "d1", key: 2, last: "d3", settled: true}})
	})

	t.Run("AnOlderWindowChangesNothing", func(t *testing.T) {
		t.Parallel()
		path := newPath(t)
		seed(t, path, map[string]remembered{"a": unsettled(at("d1", 0))})
		if err := RecordOpen(path, "a", 2, "newer", now); err != nil {
			t.Fatal(err)
		}
		want := ReadPlaces(path)["a"]

		later := now.Add(time.Minute)
		if err := Settle(path, "a", 1, "d3", later); err != nil {
			t.Fatal(err)
		}
		update := ReportUpdate{
			Worktree: "a", PID: 1, Landed: "d3", Output: "DP", Now: later,
			Worktrees: listing("a"), Windows: []SeenWindow{seenAt("a", "d3", 0)},
		}
		if err := RecordReport(path, update); err != nil {
			t.Fatal(err)
		}
		if got := ReadPlaces(path)["a"]; got != want {
			t.Errorf("entry = %+v; want it as the newer window left it, %+v", got, want)
		}
	})

	t.Run("ConcurrentWritersKeepEachOthersEntries", func(t *testing.T) {
		t.Parallel()
		for range 20 {
			path := newPath(t)
			seed(t, path, map[string]remembered{"p": {}})
			var group sync.WaitGroup
			group.Go(func() {
				if err := RecordOpen(path, "o", 1, "title", now); err != nil {
					t.Error(err)
				}
			})
			group.Go(func() {
				update := ReportUpdate{Worktree: "p", PID: 1, Landed: "d1", Worktrees: listing("p", "o")}
				if err := RecordReport(path, update); err != nil {
					t.Error(err)
				}
			})
			group.Wait()
			got := ReadPlaces(path)
			if got["p"].Home != "d1" || got["o"].PID != 1 {
				t.Fatalf("places in %s = %+v; want both writes", filepath.Base(path), got)
			}
		}
	})
}
