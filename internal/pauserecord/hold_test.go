// hold_test.go pins the shared wait at entry and the process network gate.
// TestNetwork is not parallel: the gate is process-wide state.

package pauserecord

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		Network("fetch")()
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

		done := Network("push")
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
		defer InstallGate(Gate{
			Geometry: Geometry{RecordPath: recordPath},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps++
				resumeLive(t, recordPath)
				return nil
			},
		})()

		Network("fetch")()
		if sleeps != 0 {
			t.Fatalf("sleeps = %d with an empty slug, want no hold", sleeps)
		}
		setLive(t, recordPath, Entry{Scope: "a", PausedAt: epoch})
		SetGateSlug("a")
		Network("fetch")()
		if sleeps != 1 {
			t.Fatalf("sleeps = %d after SetGateSlug, want the gate held once", sleeps)
		}
	})
}
