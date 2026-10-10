package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func reviewStore(t *testing.T) parentreview.Store {
	t.Helper()
	d := t.TempDir()
	return parentreview.Store{Root: filepath.Join(d, "reviews"), LockDir: filepath.Join(d, "locks")}
}

func openReview(t *testing.T, s parentreview.Store) {
	t.Helper()
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	spec := parentreview.OpenSpec{Slug: "x", Reviewer: "hub:orch", DecisionRecord: "d.md", SupportLog: "s.md", Brief: "brief"}
	if _, err := s.OpenRequest(spec); err != nil {
		t.Fatal(err)
	}
}

func waitingNote(t *testing.T, s parentreview.Store) string {
	t.Helper()
	note, err := reviewWaiting(s.Root, s.LockDir)()
	if err != nil {
		t.Fatal(err)
	}
	return note
}

// TestReviewWaiting asserts the waiting note names an open request and whether its notice was delivered, and is empty when there is no review, a settled round or a superseded one.
func TestReviewWaiting(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, s parentreview.Store)
		wantEmpty  bool
		wantPrefix string
		wantIn     string
	}{
		{name: "absent dir is empty", wantEmpty: true},
		{
			name:       "open undelivered",
			setup:      openReview,
			wantPrefix: "parent review: waiting on reviewer hub:orch",
			wantIn:     "notice not delivered",
		},
		{
			name: "open delivered",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if err := s.RecordDelivered(""); err != nil {
					t.Fatal(err)
				}
			},
			wantIn: "notice delivered at",
		},
		{
			name: "verdicted round reports nothing",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if err := s.RecordVerdict(parentreview.VerdictApprove, ""); err != nil {
					t.Fatal(err)
				}
			},
			wantEmpty: true,
		},
		{
			name: "expired round reports nothing",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if err := s.MarkExpired(); err != nil {
					t.Fatal(err)
				}
			},
			wantEmpty: true,
		},
		{
			// Only the latest round counts: the superseded round's open request must not leak.
			name: "superseded round reports nothing",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if _, err := s.BeginRound(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(s.Root, "round-2")); err != nil {
					t.Fatal(err)
				}
			},
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := reviewStore(t)
			if tt.setup != nil {
				tt.setup(t, s)
			}
			note := waitingNote(t, s)
			if tt.wantEmpty && note != "" {
				t.Errorf("note = %q, want empty", note)
			}
			if !strings.HasPrefix(note, tt.wantPrefix) || !strings.Contains(note, tt.wantIn) {
				t.Errorf("note = %q; want prefix %q containing %q", note, tt.wantPrefix, tt.wantIn)
			}
		})
	}
}

// reviewAsNext adapts the review store's waiting note to the hook chain's next step.
func reviewAsNext(s parentreview.Store) func(shedengine.Status) (string, error) {
	note := reviewWaiting(s.Root, s.LockDir)
	return func(shedengine.Status) (string, error) { return note() }
}

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

// TestShuttleWaiting asserts the hook chain's order.
// Gate wait records are reported ahead of a live verify marker, and a live verify marker ahead of a live shuttle marker.
// A dead shuttle marker falls through to the review note.
func TestShuttleWaiting(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
	otherHolder := gateslot.Holder{Worktree: "/hub/other", Site: "lyx gate test ./z"}
	tests := []struct {
		name         string
		kind         string
		shuttlePID   int
		verifyMarker func(t *testing.T) string
		// waits are the live gate wait records, written in the order listed.
		waits      []gateslot.Wait
		holders    []gateslot.Holder
		want       string
		wantReview bool
	}{
		{
			name:         "two live wait records render both sites oldest first, then the holders once, then the next note",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			waits: []gateslot.Wait{
				{Site: "card verify 01-x", PID: os.Getpid(), Started: started.Add(3 * time.Minute)},
				{Site: "lyx gate test ./a", PID: os.Getpid(), Started: started},
			},
			holders: []gateslot.Holder{otherHolder},
			want: "Plan-Write: lyx gate test ./a waiting for a gate slot 6m; Plan-Write: card verify 01-x waiting for a gate slot 3m" +
				" (holders: /hub/other lyx gate test ./z); Plan-Write: gate validate-plan running 6m",
		},
		{
			name:         "a wait record beside a live verify marker renders the gate wait then the verify note",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid(), 0) },
			waits:        []gateslot.Wait{{Site: "lyx gate test ./a", PID: os.Getpid(), Started: started}},
			want:         "Plan-Write: lyx gate test ./a waiting for a gate slot 6m; Plan-Write: verify running 6m (go test ./...)",
		},
		{
			name:         "a wait record of a dead process is not reported",
			kind:         "gate validate-plan",
			shuttlePID:   os.Getpid(),
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			waits:        []gateslot.Wait{{Site: "lyx gate test ./a", PID: 2147483646, Started: started}},
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
			name:         "dead shuttle marker falls to review",
			kind:         "gate validate-plan",
			shuttlePID:   2147483646,
			verifyMarker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			wantReview:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRoot := t.TempDir()
			writeShuttleMarker(t, runRoot, tt.kind, tt.shuttlePID)
			s := reviewStore(t)
			openReview(t, s)
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
			verify := verifyWaiting(tt.verifyMarker(t), holders, now, shuttleWaiting(readMarker, now, reviewAsNext(s)))
			hook := gateWaiting(waitDir, holders, now, verify)
			note, err := hook(shedengine.Status{CurrentProducer: "Plan-Write"})
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantReview {
				if !strings.HasPrefix(note, "parent review: ") {
					t.Errorf("note = %q; want the parent-review note", note)
				}
				return
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

// TestVerifyWaiting asserts a live verify marker is reported ahead of an open review, a dead marker falls through to the review note, and no marker with no review is empty.
func TestVerifyWaiting(t *testing.T) {
	tests := []struct {
		name       string
		openReview bool
		marker     func(t *testing.T) string
		// holders is the told holders read; nil reads no holder.
		holders func() ([]gateslot.Holder, error)
		check   func(t *testing.T, note string)
	}{
		{
			name:   "waiting marker names its site and the holders",
			marker: func(t *testing.T) string { return writeWaitingVerifyMarker(t, os.Getpid()) },
			holders: func() ([]gateslot.Holder, error) {
				return []gateslot.Holder{{Worktree: "/hub/a", Site: "lyx gate test ./x"}, {Worktree: "/hub/b", Site: "card verify 01-x"}}, nil
			},
			check: func(t *testing.T, note string) {
				want := "Webster-Burler: verify waiting for a gate slot 6m (Publish; holders: /hub/a lyx gate test ./x, /hub/b card verify 01-x)"
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
			name:       "live marker ahead of review",
			openReview: true,
			marker:     func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid(), 2) },
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
			name:       "dead marker falls to review",
			openReview: true,
			marker:     func(t *testing.T) string { return writeVerifyMarker(t, 2147483646, 2) },
			check: func(t *testing.T, note string) {
				if !strings.HasPrefix(note, "parent review: ") {
					t.Errorf("note = %q", note)
				}
			},
		},
		{
			name:   "no marker no review is empty",
			marker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			check: func(t *testing.T, note string) {
				if note != "" {
					t.Errorf("note = %q, want empty", note)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := reviewStore(t)
			if tt.openReview {
				openReview(t, s)
			}
			started := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
			now := func() time.Time { return started.Add(6*time.Minute + 20*time.Second) }
			holders := tt.holders
			if holders == nil {
				holders = func() ([]gateslot.Holder, error) { return nil, nil }
			}
			note, err := verifyWaiting(tt.marker(t), holders, now, reviewAsNext(s))(shedengine.Status{CurrentProducer: "Webster-Burler"})
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

//testtiming:keep pins the status spec carrying no waiting hook when the CLI has no location; its covering tests build the spec without asserting its hooks
func TestSpecFor_WaitingHookNilWithoutLocation(t *testing.T) {
	c := &loomCLI{}
	if got := c.specFor("status").Hooks.Waiting; got != nil {
		t.Errorf("Waiting set with no location")
	}
	var _ shedverbs.Spec = c.specFor("status")
}
