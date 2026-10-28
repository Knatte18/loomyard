//go:build tier0

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// fallthroughNote is the fixed note the fake next step returns, so a row can tell a fall-through from a reported wait.
const fallthroughNote = "next step note"

// fixedNextStep is the hook chain's next step in place of a real wait: it always returns fallthroughNote.
func fixedNextStep(shedengine.Status) (string, error) { return fallthroughNote, nil }

// writeShuttleMarker writes a wait marker of the given kind held by pid into a run directory under runRoot.
func writeShuttleMarker(t *testing.T, runRoot, kind string, pid int) {
	t.Helper()
	runDir := filepath.Join(runRoot, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "kind: " + kind + "\nstarted: 2026-10-03T09:15:00Z\npid: " + strconv.Itoa(pid) + "\n"
	if err := os.WriteFile(filepath.Join(runDir, "wait.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeRunningShuttleRun writes a run record that reads running, under strandName and waited on by pid, into a run directory of loc's anchor.
// Its creation time lies an hour ahead, so a reading of it never predates a probe started after the call.
func writeRunningShuttleRun(t *testing.T, loc *lymcwd.Location, strandName string, pid int) {
	t.Helper()
	runDir := filepath.Join(loc.AnchorPath(), ".lym", "shuttle", strandName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	body := `{"strandName":"` + strandName + `","outcome":"running","pid":` + strconv.Itoa(pid) + `,"createdAt":"` + createdAt + `"}`
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLiveRunStrands_SkipsRunWithGoneWaiter asserts a running run whose waiting process is gone is left out of the live strands.
func TestLiveRunStrands_SkipsRunWithGoneWaiter(t *testing.T) {
	t.Parallel()
	loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}
	writeRunningShuttleRun(t, loc, "live-run", os.Getpid())
	writeRunningShuttleRun(t, loc, "waiterless-run", 1<<30)

	got, err := liveRunStrands(loc)

	if err != nil {
		t.Fatalf("liveRunStrands() error = %v", err)
	}
	if want := []string{"live-run"}; !reflect.DeepEqual(got, want) {
		t.Errorf("liveRunStrands() = %v; want %v", got, want)
	}
}

// TestShuttleWaiting asserts the hook chain's order.
// Gate wait records are reported ahead of a live verify marker, and a live verify marker ahead of a live shuttle marker.
// A dead shuttle marker falls through to the next step.
func TestShuttleWaiting(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
	otherHolder := gateslot.Holder{Worktree: "/hub/other", Site: "lym gate test ./z"}
	tests := []struct {
		name         string
		kind         string
		shuttlePID   int
		verifyMarker func(t *testing.T) string
		// waits are the live gate wait records, written in the order listed.
		waits   []gateslot.Wait
		holders []gateslot.Holder
		want    string
	}{
		{
			name:         "two live wait records render both sites oldest first, then the holders once, then the next note",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			waits: []gateslot.Wait{
				{Site: "card verify 01-x", PID: os.Getpid(), Started: started.Add(3 * time.Minute)},
				{Site: "lym gate test ./a", PID: os.Getpid(), Started: started},
			},
			holders: []gateslot.Holder{otherHolder},
			want: "Plan-Write: lym gate test ./a waiting for a gate slot 6m; Plan-Write: card verify 01-x waiting for a gate slot 3m" +
				" (holders: /hub/other lym gate test ./z); Plan-Write: gate validate-plan running 6m",
		},
		{
			name:         "a wait record beside a live verify marker renders the gate wait then the verify note",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid(), 0) },
			waits:        []gateslot.Wait{{Site: "lym gate test ./a", PID: os.Getpid(), Started: started}},
			want:         "Plan-Write: lym gate test ./a waiting for a gate slot 6m; Plan-Write: verify running 6m (go test ./...)",
		},
		{
			name:         "a wait record of a dead process is not reported",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			waits:        []gateslot.Wait{{Site: "lym gate test ./a", PID: 2147483646, Started: started}},
			want:         "Plan-Write: gate validate-plan running 6m",
		},
		{
			name:         "live shuttle marker when no verify runs",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			want:         "Plan-Write: gate validate-plan running 6m",
		},
		{
			name:         "live held marker names its label",
			kind:         "held",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			want:         "Plan-Write: held running 6m",
		},
		{
			name:         "live verify marker ahead of the shuttle marker",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid(), 0) },
			want:         "Plan-Write: verify running 6m (go test ./...)",
		},
		{
			name:         "dead shuttle marker falls to the next step",
			kind:         "gate validate-plan",
			shuttlePID:   2147483646,
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			want:         fallthroughNote,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRoot := t.TempDir()
			writeShuttleMarker(t, runRoot, tt.kind, tt.shuttlePID)
			now := func() time.Time { return started.Add(6*time.Minute + 20*time.Second) }
			readMarker := func() (shuttleengine.WaitMarker, bool, error) {
				return shuttleengine.ReadWaitMarker(shuttleengine.Config{RunDir: runRoot}, t.TempDir())
			}
			waitDir := t.TempDir()
			for _, wait := range tt.waits {
				if _, err := gateslot.WriteWait(waitDir, wait); err != nil {
					t.Fatal(err)
				}
			}
			holders := func() ([]gateslot.Holder, error) { return tt.holders, nil }
			verify := verifyWaiting(tt.verifyMarker(t), holders, now, shuttleWaiting(readMarker, now, fixedNextStep))
			hook := gateWaiting(waitDir, holders, now, verify)
			note, err := hook(shedengine.Status{CurrentProducer: "Plan-Write"})
			if err != nil {
				t.Fatal(err)
			}
			if note != tt.want {
				t.Errorf("note = %q; want %q", note, tt.want)
			}
		})
	}
}

// writeVerifyMarker writes a running marker held by pid and returns its path.
func writeVerifyMarker(t *testing.T, pid int, attempt int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "running.yaml")
	body := "site: Publish\nattempt: " + strconv.Itoa(attempt) + "\ncommand: go test ./...\nstarted: 2026-10-03T09:15:00Z\npid: " + strconv.Itoa(pid) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeWaitingVerifyMarker writes a marker held by pid that still waits for a gate slot, since the same start time, and returns its path.
func writeWaitingVerifyMarker(t *testing.T, pid int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "running.yaml")
	body := "site: Publish\nattempt: 0\ncommand: go test ./...\nstarted: 2026-10-03T09:15:00Z\npid: " + strconv.Itoa(pid) + "\nstate: waiting\nwait_started: 2026-10-03T09:15:00Z\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestVerifyWaiting asserts a live verify marker is reported ahead of the next step, and a dead or absent marker falls through to it.
func TestVerifyWaiting(t *testing.T) {
	tests := []struct {
		name   string
		marker func(t *testing.T) string
		// holders is the told holders read; nil reads no holder.
		holders func() ([]gateslot.Holder, error)
		check   func(t *testing.T, note string)
	}{
		{
			name:   "waiting marker names its site and the holders",
			marker: func(t *testing.T) string { return writeWaitingVerifyMarker(t, os.Getpid()) },
			holders: func() ([]gateslot.Holder, error) {
				return []gateslot.Holder{{Worktree: "/hub/a", Site: "lym gate test ./x"}, {Worktree: "/hub/b", Site: "card verify 01-x"}}, nil
			},
			check: func(t *testing.T, note string) {
				want := "Webster-Burler: verify waiting for a gate slot 6m (Publish; holders: /hub/a lym gate test ./x, /hub/b card verify 01-x)"
				if note != want {
					t.Errorf("note = %q; want %q", note, want)
				}
			},
		},
		{
			name:    "waiting marker with a failing holders read renders without the holders clause",
			marker:  func(t *testing.T) string { return writeWaitingVerifyMarker(t, os.Getpid()) },
			holders: func() ([]gateslot.Holder, error) { return nil, errors.New("unreadable") },
			check: func(t *testing.T, note string) {
				if want := "Webster-Burler: verify waiting for a gate slot 6m (Publish)"; note != want {
					t.Errorf("note = %q; want %q", note, want)
				}
			},
		},
		{
			name: "paused marker names the hold, its site and the elapsed time",
			marker: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "running.yaml")
				body := "site: Publish\nattempt: 0\ncommand: go test ./...\nstarted: 2026-10-03T09:15:00Z\npid: " + strconv.Itoa(os.Getpid()) + "\nstate: paused\nwait_started: 2026-10-03T09:15:00Z\n"
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			check: func(t *testing.T, note string) {
				if want := "Webster-Burler: verify held by a pause 6m (Publish)"; note != want {
					t.Errorf("note = %q; want %q", note, want)
				}
			},
		},
		{
			name:   "live marker ahead of the next step",
			marker: func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid(), 2) },
			check: func(t *testing.T, note string) {
				if want := "Webster-Burler: verify running 6m (attempt 2; go test ./...)"; note != want {
					t.Errorf("note = %q; want %q", note, want)
				}
			},
		},
		{
			name:   "live marker without an attempt drops the attempt clause",
			marker: func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid(), 0) },
			check: func(t *testing.T, note string) {
				if want := "Webster-Burler: verify running 6m (go test ./...)"; note != want {
					t.Errorf("note = %q; want %q", note, want)
				}
			},
		},
		{
			name:   "dead marker falls to the next step",
			marker: func(t *testing.T) string { return writeVerifyMarker(t, 2147483646, 2) },
			check: func(t *testing.T, note string) {
				if note != fallthroughNote {
					t.Errorf("note = %q; want %q", note, fallthroughNote)
				}
			},
		},
		{
			name:   "no marker falls to the next step",
			marker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			check: func(t *testing.T, note string) {
				if note != fallthroughNote {
					t.Errorf("note = %q; want %q", note, fallthroughNote)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
			now := func() time.Time { return started.Add(6*time.Minute + 20*time.Second) }
			holders := tt.holders
			if holders == nil {
				holders = func() ([]gateslot.Holder, error) { return nil, nil }
			}
			note, err := verifyWaiting(tt.marker(t), holders, now, fixedNextStep)(shedengine.Status{CurrentProducer: "Webster-Burler"})
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, note)
		})
	}
}

// TestFormatElapsed pins the elapsed-time formatter at each unit boundary.
func TestFormatElapsed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45*time.Second + 900*time.Millisecond, "45s"},
		{time.Minute, "1m"},
		{6*time.Minute + 59*time.Second, "6m"},
		{time.Hour, "1h0m"},
		{time.Hour + 12*time.Minute + 30*time.Second, "1h12m"},
	}
	for _, tt := range tests {
		if got := formatElapsed(tt.d); got != tt.want {
			t.Errorf("formatElapsed(%v) = %q; want %q", tt.d, got, tt.want)
		}
	}
}

// TestParentLockWaiting asserts a live lock wait record renders the waiting note with its elapsed time, and an absent record, a dead writer's record and an unreadable record fall through to next or fail.
func TestParentLockWaiting(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	now := func() time.Time { return started.Add(6 * time.Minute) }
	next := func(shedengine.Status) (string, error) { return "next note", nil }
	alive := func(int, string) bool { return true }
	dead := func(int, string) bool { return false }
	writeWait := func(t *testing.T, path string) {
		t.Helper()
		if err := parentlock.WriteWait(path, parentlock.Wait{Branch: "main", Holder: "other-task", PID: 42, StartTime: "42-start", Started: started}); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name    string
		prepare func(t *testing.T, path string)
		alive   func(int, string) bool
		want    string
		wantErr bool
	}{
		{"live record names the branch, holder and elapsed time", writeWait, alive, "Parent-Align: waiting for the parent lock on main held by other-task 6m", false},
		{"absent record falls through", func(*testing.T, string) {}, alive, "next note", false},
		{"dead writer falls through", writeWait, dead, "next note", false},
		{"unreadable record is an error", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, alive, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := parentlock.WaitRecordPath(t.TempDir())
			tt.prepare(t, path)

			got, err := parentLockWaiting(path, tt.alive, now, next)(shedengine.Status{CurrentProducer: "Parent-Align"})

			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("note, err = %q, %v; want %q, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// TestAgentActivity_LiveLockWaitReadsAsActivity asserts a live parent lock wait record, a live running verify marker, a held drift capture lock, a held compare marker and a running agent run with a live waiter read as activity now, and a dead writer's record, a dead marker, a verify marker held by a pause, a lock file with no holder and a running agent run whose waiter is gone do not.
// A capture lock path the probe cannot take returns the probe's error.
func TestAgentActivity_LiveLockWaitReadsAsActivity(t *testing.T) {
	t.Parallel()
	writeLockWait := func(wait parentlock.Wait) func(t *testing.T, loc *lymcwd.Location) {
		return func(t *testing.T, loc *lymcwd.Location) {
			scratch := loomengine.LoomScratchDir(loc)
			if err := os.MkdirAll(scratch, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := parentlock.WriteWait(parentlock.WaitRecordPath(scratch), wait); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeVerifyMarkerState := func(pid int, state string) func(t *testing.T, loc *lymcwd.Location) {
		return func(t *testing.T, loc *lymcwd.Location) {
			path := verifytree.NewPaths(loc.WorktreePath(), verifytree.Dir(loc.AnchorPath())).Marker
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			body := "site: Publish\nattempt: 0\ncommand: go test ./...\nstarted: 2026-10-03T09:15:00Z\npid: " + strconv.Itoa(pid) + "\nstate: " + state + "\n"
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	holdDriftLock := func(lockPath func(driftengine.Paths) string) func(t *testing.T, loc *lymcwd.Location) {
		return func(t *testing.T, loc *lymcwd.Location) {
			path := lockPath(driftengine.NewPaths(loc.AnchorPath()))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			held, taken, err := lock.TryAcquireWriteLock(path)
			if err != nil || !taken {
				t.Fatalf("hold %s: taken = %v, error = %v", filepath.Base(path), taken, err)
			}
			t.Cleanup(func() { _ = held.Release() })
		}
	}
	captureLock := func(p driftengine.Paths) string { return p.CaptureLock }
	tests := []struct {
		name         string
		setup        func(t *testing.T, loc *lymcwd.Location)
		wantActivity bool
		wantErr      bool
	}{
		{"held capture lock", holdDriftLock(captureLock), true, false},
		{"held compare marker", holdDriftLock(func(p driftengine.Paths) string { return p.CompareMarker }), true, false},
		{"capture lock file with no holder", func(t *testing.T, loc *lymcwd.Location) {
			path := captureLock(driftengine.NewPaths(loc.AnchorPath()))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, false, false},
		{"capture lock path occupied by a directory", func(t *testing.T, loc *lymcwd.Location) {
			if err := os.MkdirAll(captureLock(driftengine.NewPaths(loc.AnchorPath())), 0o755); err != nil {
				t.Fatal(err)
			}
		}, false, true},
		{"live writer", writeLockWait(parentlock.NewWait("main", "other-task")), true, false},
		{"dead writer", writeLockWait(parentlock.Wait{Branch: "main", Holder: "other-task", PID: 1 << 30, StartTime: "gone", Started: time.Now()}), false, false},
		{"live running verify marker", writeVerifyMarkerState(os.Getpid(), verifytree.MarkerStateRunning), true, false},
		{"live paused verify marker", writeVerifyMarkerState(os.Getpid(), verifytree.MarkerStatePaused), false, false},
		{"dead verify marker", writeVerifyMarkerState(1<<30, verifytree.MarkerStateRunning), false, false},
		{"running agent run with a live waiter", func(t *testing.T, loc *lymcwd.Location) {
			writeRunningShuttleRun(t, loc, "live-run", os.Getpid())
		}, true, false},
		{"running agent run with a gone waiter", func(t *testing.T, loc *lymcwd.Location) {
			writeRunningShuttleRun(t, loc, "waiterless-run", 1<<30)
		}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}
			tt.setup(t, loc)

			before := time.Now()
			last, live, err := agentActivityFor(loc, shuttleengine.Config{Engine: "claude"})()

			if tt.wantErr {
				if err == nil {
					t.Fatal("agentActivityFor() error = nil; want the probe's error")
				}
				return
			}
			if err != nil {
				t.Fatalf("agentActivityFor() error = %v", err)
			}
			if live != tt.wantActivity || (tt.wantActivity && last.Before(before)) {
				t.Errorf("activity = %v at %v; want %v at or after %v", live, last, tt.wantActivity, before)
			}
		})
	}
}

//testtiming:keep pins the status spec carrying no waiting hook when the CLI has no location; its covering tests build the spec without asserting its hooks
func TestSpecFor_WaitingHookNilWithoutLocation(t *testing.T) {
	c := &loomCLI{}
	if got := c.specFor("status").Hooks.Waiting; got != nil {
		t.Errorf("Waiting set with no location")
	}
	var _ shedverbs.Spec = c.specFor("status")
}
