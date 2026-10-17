// pralign_test.go covers PRAlign row by row over newPRGateFixture's fake GitHub server and a scripted alignment session.
// None of its tests runs in parallel: the fixture swaps the package-level NewGitHubClient.

package landingshed

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakeAlignmentSession records begins, polls, closes and applied marks, and answers Poll from a script whose last outcome repeats.
type fakeAlignmentSession struct {
	dir      string
	attempts []alignment.Attempt

	beginErr error
	pollErr  error
	script   []alignment.Outcome
	// onPoll runs at the start of each Poll.
	onPoll func()
	// onMarkApplied runs inside MarkApplied, before the mark is recorded.
	onMarkApplied func(applied string)

	begins  []alignment.SpawnInput
	spawned int
	polls   int
	closes  []string
	marks   []string
}

// Begin returns the attempt that holds in.Key, or starts a new one whose report file already holds text.
func (f *fakeAlignmentSession) Begin(in alignment.SpawnInput) (alignment.Attempt, error) {
	f.begins = append(f.begins, in)
	if f.beginErr != nil {
		return alignment.Attempt{}, f.beginErr
	}
	for _, existing := range f.attempts {
		if existing.Key == in.Key {
			return existing, nil
		}
	}
	number := len(f.attempts) + 1
	attempt := alignment.Attempt{
		Number: number, Key: in.Key, Point: in.Prompt.Point, Token: "tok", Snapshot: in.Snapshot,
		PRNumber: in.PRNumber, HeadSHA: in.HeadSHA, Dir: filepath.Join(f.dir, "attempt-"+strconv.Itoa(number)),
	}
	if err := os.MkdirAll(attempt.Dir, 0o755); err != nil {
		return alignment.Attempt{}, err
	}
	if err := os.WriteFile(attempt.ReportPath(), []byte("report text\n"), 0o644); err != nil {
		return alignment.Attempt{}, err
	}
	f.attempts = append(f.attempts, attempt)
	f.spawned++
	return attempt, nil
}

func (f *fakeAlignmentSession) Poll(alignment.Attempt) (alignment.Outcome, error) {
	f.polls++
	if f.onPoll != nil {
		f.onPoll()
	}
	if f.pollErr != nil {
		return alignment.Outcome{}, f.pollErr
	}
	return f.script[min(f.polls, len(f.script))-1], nil
}

func (f *fakeAlignmentSession) Close(a alignment.Attempt, reason string) error {
	f.closes = append(f.closes, reason)
	return nil
}

func (f *fakeAlignmentSession) MarkApplied(a alignment.Attempt, applied string, notified bool) error {
	if f.onMarkApplied != nil {
		f.onMarkApplied(applied)
	}
	f.marks = append(f.marks, applied)
	for i := range f.attempts {
		if f.attempts[i].Number == a.Number {
			f.attempts[i].Applied, f.attempts[i].Notified = applied, notified
		}
	}
	return nil
}

func (f *fakeAlignmentSession) Attempts() ([]alignment.Attempt, error) {
	return append([]alignment.Attempt(nil), f.attempts...), nil
}

// alignFixture is a PRAlign over the PR-Gate fixture's server, with the alignment seams faked.
type alignFixture struct {
	*prGateFixture
	session  *fakeAlignmentSession
	cfg      AlignmentConfig
	cfgErr   error
	snapshot string
	snapErr  error
	taskHead string
	notified []string
}

func newAlignFixture(t *testing.T, listBody string) *alignFixture {
	t.Helper()
	f := &alignFixture{prGateFixture: newPRGateFixture(t, listBody), cfg: AlignmentConfig{PRApprove: true}, snapshot: "snap", taskHead: prGateHead}
	dir := filepath.Join(t.TempDir(), "pr-gate")
	f.session = &fakeAlignmentSession{dir: dir}
	f.deps.TaskHead = func() (string, error) { return f.taskHead, nil }
	f.deps.Notify = func(line string) error { f.notified = append(f.notified, line); return nil }
	f.deps.Alignment = AlignmentDeps{
		Config:   func() (AlignmentConfig, error) { return f.cfg, f.cfgErr },
		Session:  f.session,
		Snapshot: func() (string, error) { return f.snapshot, f.snapErr },
		Prompt:   alignment.PromptInput{Slug: "x"},
		NotePath: filepath.Join(dir, "note.json"),
		Tick:     time.Millisecond,
	}
	return f
}

func (f *alignFixture) callAlign(t *testing.T, ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	t.Helper()
	p, err := NewPRAlign(f.deps)
	if err != nil {
		t.Fatalf("NewPRAlign() error = %v", err)
	}
	return p.Call(ctx)
}

func approveOutcome(superseded ...string) alignment.Outcome {
	return alignment.Outcome{Done: true, Verdict: alignment.Verdict{Verdict: alignment.VerdictApprove, Reason: "in scope", Superseded: superseded}}
}

func rejectOutcome() alignment.Outcome {
	return alignment.Outcome{Done: true, Verdict: alignment.Verdict{Verdict: alignment.VerdictReject, Reason: "out of scope"}}
}

func pendingOutcome() alignment.Outcome { return alignment.Outcome{} }

// TestPRAlign walks the producer row by row: what spawns nothing, what is applied as a record, and what is left to the run's parent as a note.
// It stays serial because the fixture swaps the package-level NewGitHubClient.
func TestPRAlign(t *testing.T) {
	closedPR := `[{"number":3,"state":"closed","head":{"sha":"` + prGateHead + `"}}]`
	tests := []struct {
		name     string
		listBody string
		setup    func(t *testing.T, f *alignFixture)
		script   []alignment.Outcome
		// wantErr is a substring of the error Call returns; empty means none.
		wantErr      string
		wantBegins   int
		wantSpawned  int
		wantNote     string
		wantApproval bool
		wantReject   bool
		extra        func(t *testing.T, f *alignFixture)
	}{
		{
			name: "a parent outside require_pr_to_base spawns nothing and makes no GitHub call", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *alignFixture) { f.deps.Config.RequirePRToBase = []string{"other"} },
			extra: func(t *testing.T, f *alignFixture) {
				if len(f.order) != 0 {
					t.Errorf("GitHub calls = %v; want none", f.order)
				}
			},
		},
		{
			name: "off spawns nothing", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *alignFixture) { f.cfg.Off = true },
		},
		{
			name: "a config error is returned and spawns nothing", listBody: openPR(3, prGateHead),
			setup:   func(t *testing.T, f *alignFixture) { f.cfgErr = errors.New("boom") },
			wantErr: "boom",
		},
		{
			name: "a 5xx answer is the transient error and spawns nothing", listBody: "[]",
			setup:   func(t *testing.T, f *alignFixture) { f.srv.listStatus = http.StatusServiceUnavailable },
			wantErr: "query existing pull request",
		},
		{
			name: "a 404 answer ends Done and spawns nothing", listBody: "[]",
			setup: func(t *testing.T, f *alignFixture) { f.srv.listStatus = http.StatusNotFound },
		},
		{
			name: "a 422 answer ends Done and spawns nothing", listBody: "[]",
			setup: func(t *testing.T, f *alignFixture) { f.srv.listStatus = http.StatusUnprocessableEntity },
		},
		{name: "no pull request ends Done and spawns nothing", listBody: "[]"},
		{name: "a closed pull request ends Done and spawns nothing", listBody: closedPR},
		{
			name: "an older open pull request spawns nothing and records nothing", listBody: `[{"number":3,"state":"open","created_at":"2026-01-01T00:00:00Z","head":{"sha":"` + prGateHead + `"}}]`,
			setup: func(t *testing.T, f *alignFixture) { f.deps.RunStart = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
		},
		{
			name: "an open pull request created after the run started is aligned and its approve applied", listBody: `[{"number":3,"state":"open","created_at":"2026-07-01T00:00:00Z","head":{"sha":"` + prGateHead + `"}}]`,
			setup:  func(t *testing.T, f *alignFixture) { f.deps.RunStart = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantApproval: true,
		},
		{
			name: "a snapshot error spawns nothing and leaves a note", listBody: openPR(3, prGateHead),
			setup:    func(t *testing.T, f *alignFixture) { f.snapErr = errors.New("board down") },
			wantNote: "board down",
		},
		{
			name: "a failed begin leaves a note and no record", listBody: openPR(3, prGateHead),
			setup:      func(t *testing.T, f *alignFixture) { f.session.beginErr = errors.New("no tmux") },
			wantBegins: 1, wantNote: "no tmux",
		},
		{
			name: "a matching approval spawns nothing", listBody: openPR(3, prGateHead),
			setup:        func(t *testing.T, f *alignFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) },
			wantApproval: true,
		},
		{
			name: "a matching rejection spawns nothing", listBody: openPR(3, prGateHead),
			setup:      func(t *testing.T, f *alignFixture) { writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead) },
			wantReject: true,
		},
		{
			name: "an open attempt for the key is reused through Begin and nothing new spawns", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *alignFixture) {
				dir := filepath.Join(f.session.dir, "attempt-1")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				f.session.attempts = []alignment.Attempt{{Number: 1, Key: "pr-3-" + prGateHead, Token: "tok", Snapshot: "snap", Dir: dir}}
				if err := os.WriteFile(f.session.attempts[0].ReportPath(), []byte("report text\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantApproval: true,
		},
		{
			name: "a closed attempt for the same key spawns nothing", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *alignFixture) {
				f.session.attempts = []alignment.Attempt{{Number: 1, Key: "pr-3-" + prGateHead, Closed: true}}
			},
		},
		{
			name: "a poll error leaves a note", listBody: openPR(3, prGateHead),
			setup:      func(t *testing.T, f *alignFixture) { f.session.pollErr = errors.New("probe broke") },
			wantBegins: 1, wantSpawned: 1, wantNote: "probe broke",
		},
		{
			name: "an approve records the approval, clears a pending rejection and leaves no note", listBody: openPR(3, prGateHead),
			setup:  func(t *testing.T, f *alignFixture) { writeRejectionAt(t, f.deps.RejectionPath, 3, "old") },
			script: []alignment.Outcome{pendingOutcome(), approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantApproval: true,
			extra: func(t *testing.T, f *alignFixture) {
				if _, found, _ := ReadRejection(f.deps.RejectionPath); found {
					t.Error("the pending rejection survived the approval")
				}
				if got, _, _ := ReadApproval(f.deps.ApprovalPath); got.PRNumber != 3 || got.HeadSHA != prGateHead {
					t.Errorf("approval = %+v; want #3 at %s", got, prGateHead)
				}
				if len(f.session.closes) != 1 || f.session.marks[0] != alignment.VerdictApprove {
					t.Errorf("closes = %v, marks = %v; want one close after an approve mark", f.session.closes, f.session.marks)
				}
			},
		},
		{
			name: "an approve with alignment_pr_approve false leaves a note and no record", listBody: openPR(3, prGateHead),
			setup:  func(t *testing.T, f *alignFixture) { f.cfg.PRApprove = false },
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantNote: approveNotGivenReason,
		},
		{
			name: "an approve naming superseded entries queues one notice and marks the attempt notified", listBody: openPR(3, prGateHead),
			script: []alignment.Outcome{approveOutcome("old-a", "old-b")}, wantBegins: 1, wantSpawned: 1, wantApproval: true,
			extra: func(t *testing.T, f *alignFixture) {
				if len(f.notified) != 1 || !strings.Contains(f.notified[0], "old-a, old-b") || !strings.Contains(f.notified[0], "#3") || !strings.Contains(f.notified[0], "x") {
					t.Errorf("notices = %q; want one naming the slug, the pull request and both entries", f.notified)
				}
				if !f.session.attempts[0].Notified {
					t.Error("the attempt was not marked notified")
				}
			},
		},
		{
			name: "an approve naming none queues no notice", listBody: openPR(3, prGateHead),
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantApproval: true,
			extra: func(t *testing.T, f *alignFixture) {
				if len(f.notified) != 0 {
					t.Errorf("notices = %q; want none", f.notified)
				}
			},
		},
		{
			name: "an approve over a changed snapshot leaves a note and no record", listBody: openPR(3, prGateHead),
			setup:  func(t *testing.T, f *alignFixture) { f.session.onPoll = func() { f.snapshot = "changed" } },
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantNote: alignment.DecisionRecordedReason,
		},
		{
			name: "a first reject records the rejection with the report as findings, after the count", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *alignFixture) {
				f.session.onMarkApplied = func(string) {
					if _, found, _ := ReadRejection(f.deps.RejectionPath); found {
						t.Error("a rejection file existed when MarkApplied(reject) ran")
					}
				}
			},
			script: []alignment.Outcome{rejectOutcome()}, wantBegins: 1, wantSpawned: 1, wantReject: true,
			extra: func(t *testing.T, f *alignFixture) {
				got, _, _ := ReadRejection(f.deps.RejectionPath)
				if got.Findings != "report text\n" || got.PRNumber != 3 || got.HeadSHA != prGateHead {
					t.Errorf("rejection = %+v; want the report as findings for #3 at %s", got, prGateHead)
				}
			},
		},
		{
			name: "a reject after an applied-reject attempt leaves a note and no record", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *alignFixture) {
				f.session.attempts = []alignment.Attempt{{Number: 1, Key: "pr-3-old", Closed: true, Applied: alignment.VerdictReject}}
			},
			script: []alignment.Outcome{rejectOutcome()}, wantBegins: 1, wantSpawned: 1, wantNote: secondRejectReason,
		},
		{
			name: "an escalate leaves a note naming the report and the question", listBody: openPR(3, prGateHead),
			script:     []alignment.Outcome{{Done: true, Verdict: alignment.Verdict{Verdict: alignment.VerdictEscalate, Reason: "overlap", Question: "which lands first?"}}},
			wantBegins: 1, wantSpawned: 1, wantNote: "which lands first?",
			extra: func(t *testing.T, f *alignFixture) {
				if note, _, _ := ReadAlignmentNote(f.deps.Alignment.NotePath); !strings.HasSuffix(note.Report, "report-tok.md") {
					t.Errorf("note report = %q; want the attempt's report path", note.Report)
				}
			},
		},
		{
			name: "a timeout cause leaves a note naming the cause", listBody: openPR(3, prGateHead),
			script: []alignment.Outcome{{Done: true, Cause: alignment.CauseTimeout, Detail: "ran past its deadline"}}, wantBegins: 1, wantSpawned: 1, wantNote: "timeout",
		},
		{
			name: "a head that moved during the wait applies nothing and the note carries the new head", listBody: openPR(3, prGateHead),
			setup:  func(t *testing.T, f *alignFixture) { f.session.onPoll = func() { f.srv.listBody = openPR(3, "bbb") } },
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantNote: "moved",
			extra: func(t *testing.T, f *alignFixture) {
				if note, _, _ := ReadAlignmentNote(f.deps.Alignment.NotePath); note.HeadSHA != "bbb" || note.PRNumber != 3 {
					t.Errorf("note = %+v; want #3 at bbb", note)
				}
			},
		},
		{
			name: "a task head that differs at apply time applies nothing", listBody: openPR(3, prGateHead),
			setup:  func(t *testing.T, f *alignFixture) { f.session.onPoll = func() { f.taskHead = "ccc" } },
			script: []alignment.Outcome{approveOutcome()}, wantBegins: 1, wantSpawned: 1, wantNote: "ccc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAlignFixture(t, tt.listBody)
			if tt.setup != nil {
				tt.setup(t, f)
			}
			f.session.script = tt.script
			outcome, _, err := f.callAlign(t, context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Call() error = %v; want one containing %q", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != shedengine.Done {
					t.Fatalf("Call() outcome = %q; want Done", outcome)
				}
			}
			if len(f.session.begins) != tt.wantBegins || f.session.spawned != tt.wantSpawned {
				t.Errorf("begins = %d, spawned = %d; want %d and %d", len(f.session.begins), f.session.spawned, tt.wantBegins, tt.wantSpawned)
			}
			note, noted, _ := ReadAlignmentNote(f.deps.Alignment.NotePath)
			if (tt.wantNote != "") != noted || !strings.Contains(note.Reason, tt.wantNote) {
				t.Errorf("note = %+v (found %v); want one containing %q", note, noted, tt.wantNote)
			}
			_, approved, _ := ReadApproval(f.deps.ApprovalPath)
			_, rejected, _ := ReadRejection(f.deps.RejectionPath)
			if approved != tt.wantApproval || rejected != tt.wantReject {
				t.Errorf("approval = %v, rejection = %v; want %v and %v", approved, rejected, tt.wantApproval, tt.wantReject)
			}
			if tt.extra != nil {
				tt.extra(t, f)
			}
		})
	}

	t.Run("a cancelled context closes the attempt and returns the cancellation error", func(t *testing.T) {
		f := newAlignFixture(t, openPR(3, prGateHead))
		ctx, cancel := context.WithCancel(context.Background())
		f.session.onPoll = cancel
		f.session.script = []alignment.Outcome{pendingOutcome()}
		_, _, err := f.callAlign(t, ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Call() error = %v; want the cancellation", err)
		}
		if len(f.session.closes) != 1 {
			t.Errorf("closes = %v; want one", f.session.closes)
		}
	})
}
