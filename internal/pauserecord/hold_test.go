//go:build tier0

// hold_test.go pins the shared wait at entry and the process network gate.
// TestNetwork is not parallel: the gate is process-wide state.

package pauserecord

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// setLive replaces the record's live entries with entries.
func setLive(t *testing.T, path string, entries ...Entry) {
	t.Helper()
	_, err := Update(path, epoch, func(r *Record) error {
		r.Live = entries
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// resumeLive moves every live entry to the resumed list.
func resumeLive(t *testing.T, path string) {
	t.Helper()
	_, err := Update(path, epoch, func(r *Record) error {
		for _, entry := range r.Live {
			entry.ResumedAt = epoch
			r.Resumed = append(r.Resumed, entry)
		}
		r.Live = nil
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWaitUncovered(t *testing.T) {
	t.Parallel()
	hub := Entry{Scope: ScopeHub, PausedAt: epoch}
	slugEntry := Entry{Scope: "a", PausedAt: epoch}
	replaced := Entry{Scope: ScopeHub, PausedAt: epoch.Add(time.Hour)}
	second := time.Second

	tests := []struct {
		name       string
		live       []Entry
		slug       string
		exempt     bool
		corrupt    bool
		cancelled  bool
		onSleep    func(t *testing.T, call int, path string)
		wantDelays []time.Duration
		wantLines  int
		wantLine   string
		wantErr    error
	}{
		{name: "no entry returns at once with no line", slug: "a"},
		{
			name: "a hub entry holds until resumed, with one line naming lyx resume",
			live: []Entry{hub}, slug: "a",
			onSleep:    func(t *testing.T, call int, path string) { resumeLive(t, path) },
			wantDelays: []time.Duration{second},
			wantLines:  1,
			wantLine:   `lyx: held by a hub-wide pause since 2026-01-10T12:00:00Z; it continues after "lyx resume"`,
		},
		{
			name: "a slug entry holds its slug and names both resumes",
			live: []Entry{slugEntry}, slug: "a",
			onSleep:    func(t *testing.T, call int, path string) { resumeLive(t, path) },
			wantDelays: []time.Duration{second},
			wantLines:  1,
			wantLine:   `lyx: held by the pause of a since 2026-01-10T12:00:00Z; it continues after "lyx resume" or "lyx resume a"`,
		},
		{name: "a slug entry does not hold another slug", live: []Entry{slugEntry}, slug: "b"},
		{name: "an exempt caller passes a slug entry", live: []Entry{slugEntry}, slug: "a", exempt: true},
		{
			name: "an exempt caller is held by a hub entry",
			live: []Entry{hub}, slug: "a", exempt: true,
			onSleep:    func(t *testing.T, call int, path string) { resumeLive(t, path) },
			wantDelays: []time.Duration{second},
			wantLines:  1,
		},
		{
			name: "wake delays double to the ceiling",
			live: []Entry{hub}, slug: "a",
			onSleep: func(t *testing.T, call int, path string) {
				if call == 6 {
					resumeLive(t, path)
				}
			},
			wantDelays: []time.Duration{1 * second, 2 * second, 4 * second, 5 * second, 5 * second, 5 * second},
			wantLines:  1,
		},
		{
			name: "a changed covering entry returns the delay to the floor",
			live: []Entry{hub}, slug: "a",
			onSleep: func(t *testing.T, call int, path string) {
				switch call {
				case 2:
					setLive(t, path, replaced)
				case 3:
					resumeLive(t, path)
				}
			},
			wantDelays: []time.Duration{1 * second, 2 * second, 1 * second},
			wantLines:  1,
		},
		{name: "an undecodable record returns at once with one line", slug: "a", corrupt: true, wantLines: 1, wantLine: "lyx: pause record unreadable, not holding: decode pause record"},
		{name: "a cancelled context returns its error", live: []Entry{hub}, slug: "a", cancelled: true, wantLines: 1, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), RecordFileName)
			if tt.corrupt {
				if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if tt.live != nil {
				setLive(t, path, tt.live...)
			}

			var delays []time.Duration
			var lines []string
			waiter := Waiter{
				Notify:          func(line string) { lines = append(lines, line) },
				SlugEntryExempt: func() bool { return tt.exempt },
			}
			if !tt.cancelled {
				waiter.Sleep = func(ctx context.Context, d time.Duration) error {
					delays = append(delays, d)
					if tt.onSleep != nil {
						tt.onSleep(t, len(delays), path)
					}
					return nil
				}
			}
			ctx := context.Background()
			if tt.cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			err := WaitUncovered(ctx, path, tt.slug, waiter)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("WaitUncovered error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(delays, tt.wantDelays) {
				t.Fatalf("wake delays = %v, want %v", delays, tt.wantDelays)
			}
			if len(lines) != tt.wantLines {
				t.Fatalf("lines = %q, want %d", lines, tt.wantLines)
			}
			if tt.wantLine != "" && !strings.HasPrefix(lines[0], tt.wantLine) {
				t.Fatalf("line = %q, want prefix %q", lines[0], tt.wantLine)
			}
		})
	}
}

func TestNetwork(t *testing.T) {
	alive := func(int, string) bool { return true }
	dead := func(int, string) bool { return false }

	t.Run("no gate is a no-op writing no marker", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		held, done := Network("fetch")
		done()
		if held != 0 {
			t.Fatalf("held = %v, want zero with no gate", held)
		}
		if _, err := os.Stat(NetDir(recordPath)); !os.IsNotExist(err) {
			t.Fatalf("net dir stat error = %v, want not exist", err)
		}
	})

	t.Run("an installed gate waits under a live entry, then marks the operation", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		setLive(t, recordPath, Entry{Scope: ScopeHub, PausedAt: epoch})
		sleeps := 0
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			Strand:   "s1", PID: 42, StartTime: "t0",
			Notify: func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		_, done := Network("push")
		if sleeps != 1 {
			t.Fatalf("sleeps = %d, want the gate to wait once", sleeps)
		}
		ops, err := ListNetOps(recordPath, alive)
		if err != nil || len(ops) != 1 {
			t.Fatalf("ListNetOps = %+v, %v; want the one marker", ops, err)
		}
		if op := ops[0]; op.PID != 42 || op.Slug != "a" || op.Strand != "s1" || op.Op != "push" || op.UnderParentLock {
			t.Fatalf("marker = %+v, want pid 42, slug a, strand s1, op push, not under a parent lock", op)
		}
		done()
		if ops, _ := ListNetOps(recordPath, alive); len(ops) != 0 {
			t.Fatalf("markers after done = %+v, want none", ops)
		}
	})

	t.Run("a gate whose sleep advances the clock reports the held time", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		setLive(t, recordPath, Entry{Scope: ScopeHub, PausedAt: epoch})
		clock := epoch
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			Notify:   func(string) {},
			Now:      func() time.Time { return clock },
			Sleep: func(ctx context.Context, d time.Duration) error {
				clock = clock.Add(3 * time.Second)
				resumeLive(t, recordPath)
				return nil
			},
		})()

		held, done := Network("push")
		done()
		if held != 3*time.Second {
			t.Fatalf("held = %v, want 3s", held)
		}
	})

	t.Run("a marker of a dead process is removed and not listed", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		defer InstallGate(Gate{Geometry: Geometry{RecordPath: recordPath}, PID: 7})()

		Network("fetch")
		ops, err := ListNetOps(recordPath, dead)
		if err != nil || len(ops) != 0 {
			t.Fatalf("ListNetOps = %+v, %v; want none", ops, err)
		}
		files, err := os.ReadDir(NetDir(recordPath))
		if err != nil || len(files) != 0 {
			t.Fatalf("net dir = %v, %v; want the dead marker removed", files, err)
		}
	})

	t.Run("a slug given through SetGateSlug is held by a live entry for it", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		setLive(t, recordPath, Entry{Scope: "a", PausedAt: epoch})
		sleeps := 0
		clock := epoch
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath},
			Notify:   func(string) {},
			Now: func() time.Time {
				clock = clock.Add(time.Second)
				return clock
			},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		held, done := Network("fetch")
		done()
		if sleeps != 0 {
			t.Fatalf("sleeps = %d with an empty slug, want no hold", sleeps)
		}
		if held != 0 {
			t.Fatalf("held = %v with an empty slug, want zero", held)
		}
		setLive(t, recordPath, Entry{Scope: "a", PausedAt: epoch})
		SetGateSlug("a")
		_, done = Network("fetch")
		done()
		if sleeps != 1 {
			t.Fatalf("sleeps = %d after SetGateSlug, want the gate held once", sleeps)
		}
	})

	t.Run("an exempt entry passes alone, holds beside another, and holds again once restored", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		owned := Entry{Scope: ScopeHub, PausedAt: epoch}
		other := Entry{Scope: "a", PausedAt: epoch.Add(time.Minute)}
		setLive(t, recordPath, owned)
		sleeps := 0
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		restore := SetGateExempt(ScopeHub, epoch)
		_, done := Network("fetch")
		done()
		if sleeps != 0 {
			t.Fatalf("sleeps = %d with the exempt entry alone, want the call to pass at once", sleeps)
		}

		setLive(t, recordPath, owned, other)
		_, done = Network("fetch")
		done()
		if sleeps != 1 {
			t.Fatalf("sleeps = %d beside a second covering entry, want the call held until it is lifted", sleeps)
		}

		restore()
		setLive(t, recordPath, owned)
		_, done = Network("fetch")
		done()
		if sleeps != 2 {
			t.Fatalf("sleeps = %d after the restore, want the entry to hold again", sleeps)
		}
	})

	t.Run("an exemption of another pause time does not pass the entry", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		setLive(t, recordPath, Entry{Scope: ScopeHub, PausedAt: epoch.Add(time.Hour)})
		sleeps := 0
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		defer SetGateExempt(ScopeHub, epoch)()
		_, done := Network("fetch")
		done()
		if sleeps != 1 {
			t.Fatalf("sleeps = %d for a newer hub entry, want the call held", sleeps)
		}
	})

	t.Run("a waiting call leaves a waiting marker that is gone once the wait returns", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		setLive(t, recordPath, Entry{Scope: ScopeHub, PausedAt: epoch})
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			PID:      42, StartTime: "t0",
			Notify: func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				waiting, err := ListWaiting(recordPath, alive)
				if want := []WaitingMarker{{PID: 42, StartTime: "t0", Slug: "a"}}; err != nil || !reflect.DeepEqual(waiting, want) {
					t.Errorf("ListWaiting while held = %+v, %v; want %+v", waiting, err, want)
				}
				if ops, err := ListNetOps(recordPath, alive); err != nil || len(ops) != 0 {
					t.Errorf("ListNetOps while held = %+v, %v; want none", ops, err)
				}
				if gone, _ := ListWaiting(recordPath, dead); len(gone) != 0 {
					t.Errorf("ListWaiting with a dead process = %+v; want none", gone)
				}
				if files, err := os.ReadDir(WaitingDir(recordPath)); err != nil || len(files) != 0 {
					t.Errorf("waiting dir = %v, %v; want the dead marker removed", files, err)
				}
				resumeLive(t, recordPath)
				return nil
			},
		})()

		_, done := Network("push")

		if waiting, err := ListWaiting(recordPath, alive); err != nil || len(waiting) != 0 {
			t.Fatalf("ListWaiting after the wait = %+v, %v; want none", waiting, err)
		}
		if ops, err := ListNetOps(recordPath, alive); err != nil || len(ops) != 1 {
			t.Fatalf("ListNetOps after the wait = %+v, %v; want the in-flight marker", ops, err)
		}
		done()
	})

	t.Run("a waiting directory path taken by a file leaves the call waiting and then passing", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		setLive(t, recordPath, Entry{Scope: ScopeHub, PausedAt: epoch})
		if err := os.WriteFile(WaitingDir(recordPath), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		sleeps := 0
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		_, done := Network("push")

		if sleeps != 1 {
			t.Fatalf("sleeps = %d, want the call held once with no marker written", sleeps)
		}
		if ops, err := ListNetOps(recordPath, alive); err != nil || len(ops) != 1 {
			t.Fatalf("ListNetOps = %+v, %v; want the in-flight marker", ops, err)
		}
		done()
	})

	t.Run("exempt all passes a hub and a slug entry at once, and holds again once restored", func(t *testing.T) {
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		live := []Entry{{Scope: ScopeHub, PausedAt: epoch}, {Scope: "a", PausedAt: epoch.Add(time.Minute)}}
		setLive(t, recordPath, live...)
		sleeps := 0
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath, Slug: "a"},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		restore := SetGateExemptAll()
		_, done := Network("fetch")
		done()
		if sleeps != 0 {
			t.Fatalf("sleeps = %d with exempt all, want the call to pass at once", sleeps)
		}

		restore()
		_, done = Network("fetch")
		done()
		if sleeps != 1 {
			t.Fatalf("sleeps = %d after the restore, want the entries to hold again", sleeps)
		}
	})

	t.Run("SetGateExemptAll with no gate installed installs none", func(t *testing.T) {
		restore := SetGateExemptAll()
		restore()
		if held, done := Network("fetch"); held != 0 || installedGate.Load() != nil {
			t.Fatalf("held = %v, gate installed = %v; want zero and no gate", held, installedGate.Load() != nil)
		} else {
			done()
		}
	})

	t.Run("SetGateExempt with no gate installed is a no-op", func(t *testing.T) {
		restore := SetGateExempt(ScopeHub, epoch)
		restore()
		if held, done := Network("fetch"); held != 0 {
			t.Fatalf("held = %v with no gate, want zero", held)
		} else {
			done()
		}
	})

	// networkInGoroutine runs Network on its own goroutine, so a deferral that ends it with runtime.Goexit stands in for a child's exit.
	networkInGoroutine := func(t *testing.T) (inFlight bool) {
		t.Helper()
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			_, done := Network("push")
			inFlight = true
			done()
		}()
		<-finished
		return inFlight
	}

	deferralRows := []struct {
		name          string
		live          []Entry
		gate          Gate
		deferral      func(calls *int) func()
		wantCalls     int
		wantSleeps    int
		wantInFlight  bool
		wantCoveredAt bool
	}{
		{
			name:         "a Defer gate covered by a hub entry calls the deferral once and never sleeps",
			live:         []Entry{{Scope: ScopeHub, PausedAt: epoch}},
			gate:         Gate{Defer: true},
			deferral:     func(calls *int) func() { return func() { *calls++; runtime.Goexit() } },
			wantCalls:    1,
			wantSleeps:   0,
			wantInFlight: false, wantCoveredAt: true,
		},
		{
			name:         "a Defer gate with nothing covering calls no deferral and marks its operation",
			gate:         Gate{Defer: true},
			deferral:     func(calls *int) func() { return func() { *calls++; runtime.Goexit() } },
			wantInFlight: true,
		},
		{
			name:          "a Defer gate with no deferral stored waits until the entry is lifted",
			live:          []Entry{{Scope: ScopeHub, PausedAt: epoch}},
			gate:          Gate{Defer: true},
			wantSleeps:    1,
			wantInFlight:  true,
			wantCoveredAt: true,
		},
		{
			name:          "a Defer gate whose deferral returns waits until the entry is lifted",
			live:          []Entry{{Scope: ScopeHub, PausedAt: epoch}},
			gate:          Gate{Defer: true},
			deferral:      func(calls *int) func() { return func() { *calls++ } },
			wantCalls:     1,
			wantSleeps:    1,
			wantInFlight:  true,
			wantCoveredAt: true,
		},
		{
			name:         "an OwnerEntryExempt gate passes its own slug's owner entry at once",
			live:         []Entry{{Scope: "a", Caller: OwnerCaller, PausedAt: epoch}},
			gate:         Gate{OwnerEntryExempt: true},
			wantInFlight: true,
		},
		{
			name:          "an OwnerEntryExempt gate still waits on a hub entry",
			live:          []Entry{{Scope: ScopeHub, Caller: OwnerCaller, PausedAt: epoch}},
			gate:          Gate{OwnerEntryExempt: true},
			wantSleeps:    1,
			wantInFlight:  true,
			wantCoveredAt: true,
		},
		{
			name:          "an OwnerEntryExempt gate still waits on an operator's entry of its own slug",
			live:          []Entry{{Scope: "a", Caller: "EXTERNAL", PausedAt: epoch}},
			gate:          Gate{OwnerEntryExempt: true},
			wantSleeps:    1,
			wantInFlight:  true,
			wantCoveredAt: true,
		},
	}
	for _, tt := range deferralRows {
		t.Run(tt.name, func(t *testing.T) {
			recordPath := filepath.Join(t.TempDir(), RecordFileName)
			setLive(t, recordPath, tt.live...)
			sleeps, calls := 0, 0
			gate := tt.gate
			gate.Geometry = Geometry{RecordPath: recordPath, Slug: "a"}
			gate.Notify = func(string) {}
			gate.Sleep = func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			}
			defer InstallGate(gate)()
			if tt.deferral != nil {
				defer SetGateDeferral(tt.deferral(&calls))()
			}

			if got := GateCovered(); got != tt.wantCoveredAt {
				t.Errorf("GateCovered() = %v; want %v", got, tt.wantCoveredAt)
			}
			inFlight := networkInGoroutine(t)

			if calls != tt.wantCalls || sleeps != tt.wantSleeps || inFlight != tt.wantInFlight {
				t.Errorf("deferral calls, sleeps, in flight = %d, %d, %v; want %d, %d, %v", calls, sleeps, inFlight, tt.wantCalls, tt.wantSleeps, tt.wantInFlight)
			}
		})
	}

	t.Run("GateCovered reads false with no gate installed and over a record that does not decode", func(t *testing.T) {
		if GateCovered() {
			t.Fatal("GateCovered() = true with no gate installed; want false")
		}
		recordPath := filepath.Join(t.TempDir(), RecordFileName)
		if err := os.WriteFile(recordPath, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer InstallGate(Gate{Geometry: Geometry{RecordPath: recordPath, Slug: "a"}, Defer: true})()
		if GateCovered() {
			t.Fatal("GateCovered() = true over an undecodable record; want false")
		}
	})
}
