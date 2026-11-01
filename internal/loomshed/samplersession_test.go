// samplersession_test.go covers the Sampler-Session row: how it starts, reuses and resumes its session, how its wait spaces wakes on an injected clock, and what each done flag, pause and failure ends it with.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeSamplerSessions is a SamplerSessions that records every start with its resume id and every stop, and keeps a live set.
type fakeSamplerSessions struct {
	mu       sync.Mutex
	live     map[string]bool
	resumes  []string
	stops    []string
	started  int
	startErr error
	onStart  func()
}

var _ SamplerSessions = (*fakeSamplerSessions)(nil)

func (f *fakeSamplerSessions) StartSession(spec shuttleengine.Spec) (shuttleengine.OpenResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumes = append(f.resumes, spec.ResumeSessionID)
	if f.startErr != nil {
		return shuttleengine.OpenResult{}, f.startErr
	}
	f.started++
	suffix := string(rune('0' + f.started))
	result := shuttleengine.OpenResult{StrandGUID: "guid-" + suffix, StrandName: "strand-" + suffix, SessionID: "session-" + suffix, Engine: "claude"}
	f.live[result.StrandGUID] = true
	if f.onStart != nil {
		f.onStart()
	}
	return result, nil
}

func (f *fakeSamplerSessions) StopStrand(guid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, guid)
	f.live[guid] = false
	return nil
}

func (f *fakeSamplerSessions) isLive(guid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live[guid]
}

// samplerWorld is the world a Sampler-Session call runs in: the fake sessions, the record paths and scripted seams.
// onWake runs at the start of each wake with its 1-based count, so a test changes the world between two wakes.
// The Sleep seam advances the clock by its interval and cancels the call's context once sleepBudget sleeps are spent.
type samplerWorld struct {
	t          *testing.T
	sessions   *fakeSamplerSessions
	recordPath string
	lockPath   string
	cancel     context.CancelFunc

	mu          sync.Mutex
	now         time.Time
	sleeps      []time.Duration
	sleepBudget int
	stamp       int
	// covered answers PauseCovered at the n-th (1-based) wake; nil covers nothing.
	covered     func(n int) bool
	coveredErr  error
	dirty       [][]string
	dirtyCalls  int
	findings    []discussionparser.Finding
	mails       []string
	mailNames   []string
	specErr     error
	registerErr error
	wakes       int
	onWake      func(w *samplerWorld, n int)
	// sleepFn, when set, replaces the default Sleep.
	sleepFn func(w *samplerWorld, ctx context.Context, d time.Duration) error
	watch   chan struct{}
}

func newSamplerWorld(t *testing.T) (*samplerWorld, context.Context) {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &samplerWorld{
		t:           t,
		sessions:    &fakeSamplerSessions{live: map[string]bool{}},
		recordPath:  filepath.Join(dir, "scratch", "sampler.json"),
		lockPath:    filepath.Join(dir, "scratch", "sampler.json.lock"),
		cancel:      cancel,
		now:         time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		sleepBudget: 3,
		stamp:       2,
	}, ctx
}

// writeRecord stores rec as the sampler record.
func (w *samplerWorld) writeRecord(rec samplerengine.Record) {
	w.t.Helper()
	if err := samplerengine.UpdateRecord(w.recordPath, w.lockPath, func(stored *samplerengine.Record) { *stored = rec }); err != nil {
		w.t.Fatalf("UpdateRecord() = %v; want nil", err)
	}
}

// record reads the sampler record back.
func (w *samplerWorld) record() samplerengine.Record {
	w.t.Helper()
	rec, _, err := samplerengine.ReadRecord(w.recordPath, w.lockPath)
	if err != nil {
		w.t.Fatalf("ReadRecord() = _, _, %v; want nil", err)
	}
	return rec
}

// raiseFlag writes a done flag at the world's current stamp.
func (w *samplerWorld) raiseFlag() {
	w.t.Helper()
	if err := samplerengine.UpdateRecord(w.recordPath, w.lockPath, func(stored *samplerengine.Record) { stored.DoneStamp = w.stamp }); err != nil {
		w.t.Fatalf("UpdateRecord() = %v; want nil", err)
	}
}

func (w *samplerWorld) deps() SamplerDeps {
	deps := SamplerDeps{
		RecordPath:     w.recordPath,
		RecordLockPath: w.lockPath,
		Spec: func() (shuttleengine.Spec, error) {
			if w.specErr != nil {
				return shuttleengine.Spec{}, w.specErr
			}
			return shuttleengine.Spec{Prompt: "prompt"}, nil
		},
		Sessions:        w.sessions,
		SessionLive:     func(guid string) (bool, error) { return w.sessions.isLive(guid), nil },
		RegisterSession: func(string) error { return w.registerErr },
		Stamp:           func() (int, error) { return w.stamp, nil },
		DirtyPaths: func() ([]string, error) {
			w.dirtyCalls++
			if len(w.dirty) == 0 {
				return nil, nil
			}
			return w.dirty[min(w.dirtyCalls, len(w.dirty))-1], nil
		},
		CheckRecord: func() ([]discussionparser.Finding, error) { return w.findings, nil },
		MailAgent: func(strandName, line string) error {
			w.mailNames = append(w.mailNames, strandName)
			w.mails = append(w.mails, line)
			return nil
		},
		PauseCovered: func() (bool, error) {
			w.wakes++
			if w.onWake != nil {
				w.onWake(w, w.wakes)
			}
			if w.coveredErr != nil {
				return false, w.coveredErr
			}
			return w.covered != nil && w.covered(w.wakes), nil
		},
		Now: func() time.Time {
			w.mu.Lock()
			defer w.mu.Unlock()
			return w.now
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			if w.sleepFn != nil {
				return w.sleepFn(w, ctx, d)
			}
			return w.advance(d)
		},
	}
	if w.watch != nil {
		deps.Watch = w.watch
	}
	return deps
}

// advance records a sleep of d and moves the clock by it, cancelling the call once the sleep budget is spent.
func (w *samplerWorld) advance(d time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sleeps = append(w.sleeps, d)
	w.now = w.now.Add(d)
	if len(w.sleeps) >= w.sleepBudget {
		w.cancel()
		return context.Canceled
	}
	return nil
}

// TestSamplerSession covers the row's session start, reuse and resume, the done flag's judgement on entry and during the wait, the pause, the dead-strand halt, the failures of its seams, and the cancelled context.
func TestSamplerSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dirtyCheckout := [][]string{{"main.go"}}
	invalidRecord := []discussionparser.Finding{{Check: "decision-ids", Detail: "decision D2 is missing"}}
	const gone = "guid-old"
	recorded := samplerengine.Record{StrandGUID: gone, StrandName: "strand-old", SessionID: "session-old", Engine: "claude"}
	flagged := func(rec samplerengine.Record, stamp int) samplerengine.Record {
		rec.DoneStamp = stamp
		return rec
	}

	tests := []struct {
		name  string
		setup func(w *samplerWorld)
		// wantErr is a substring of the returned error; "cancelled" means the call's context was cancelled.
		wantErr     string
		wantOutcome shedengine.Outcome
		wantReason  []string
		wantResumes []string
		wantStops   []string
		wantMails   int
		check       func(t *testing.T, w *samplerWorld)
	}{
		{
			name:    "first entry starts one session and records it",
			wantErr: "cancelled", wantResumes: []string{""},
			check: func(t *testing.T, w *samplerWorld) {
				if got := w.record(); got.StrandGUID != "guid-1" || got.StrandName != "strand-1" || got.SessionID != "session-1" || got.Engine != "claude" {
					t.Errorf("record = %+v; want the started session recorded", got)
				}
			},
		},
		{
			name:    "a live recorded strand is reused with no start",
			setup:   func(w *samplerWorld) { w.writeRecord(recorded); w.sessions.live[gone] = true },
			wantErr: "cancelled",
		},
		{
			name:    "a recorded session id whose strand is gone is resumed and the new strand re-recorded",
			setup:   func(w *samplerWorld) { w.writeRecord(recorded) },
			wantErr: "cancelled", wantResumes: []string{"session-old"},
			check: func(t *testing.T, w *samplerWorld) {
				if got := w.record(); got.StrandGUID != "guid-1" || got.SessionID != "session-1" {
					t.Errorf("record = %+v; want the resumed strand recorded", got)
				}
			},
		},
		{
			name:        "a non-transient start failure is Stuck with Halt naming the ways forward",
			setup:       func(w *samplerWorld) { w.sessions.startErr = errors.New("pane refused") },
			wantOutcome: shedengine.Stuck, wantResumes: []string{""},
			wantReason: []string{"pane refused", "lym loom resume", "lym loom sampler-done", "lym batten cancel"},
		},
		{
			name: "a transient start failure is an error the driver re-steps",
			setup: func(w *samplerWorld) {
				w.sessions.startErr = shedengine.MarkTransient(shedengine.TransientAgentStart, errors.New("never ready"))
			},
			wantErr: "never ready", wantResumes: []string{""},
		},
		{
			name:        "a Spec error is Stuck with Halt naming the agent fix",
			setup:       func(w *samplerWorld) { w.specErr = errors.New("unknown alias") },
			wantOutcome: shedengine.Stuck,
			wantReason:  []string{"unknown alias", "lym config sampler"},
		},
		{
			name: "a flag written before the run halted ends Done on entry with no start, also when the session would not resume",
			setup: func(w *samplerWorld) {
				w.writeRecord(flagged(recorded, 2))
				w.sessions.startErr = errors.New("cannot resume")
			},
			wantOutcome: shedengine.Done,
		},
		{
			name:        "a flag on entry with a live reopened strand stops it and ends Done",
			setup:       func(w *samplerWorld) { w.writeRecord(flagged(recorded, 2)); w.sessions.live[gone] = true },
			wantOutcome: shedengine.Done, wantStops: []string{gone},
		},
		{
			name:    "a flag at an older stamp is ignored and the session started",
			setup:   func(w *samplerWorld) { w.writeRecord(flagged(recorded, 1)) },
			wantErr: "cancelled", wantResumes: []string{"session-old"},
		},
		{
			name: "a flag read with a dirty checkout clears the flag, keeps the session live and mails it once",
			setup: func(w *samplerWorld) {
				w.writeRecord(flagged(recorded, 2))
				w.sessions.live[gone] = true
				w.dirty = dirtyCheckout
			},
			wantErr: "cancelled", wantResumes: []string{"session-old"}, wantStops: []string{gone}, wantMails: 1,
			check: func(t *testing.T, w *samplerWorld) {
				if got := w.record(); got.DoneStamp != 0 {
					t.Errorf("record.DoneStamp = %d; want 0, the flag cleared", got.DoneStamp)
				}
				if !w.sessions.isLive("guid-1") {
					t.Error("session guid-1 is not live; want the session brought back")
				}
				if len(w.mails) != 1 || !strings.Contains(w.mails[0], "main.go") || !strings.Contains(w.mails[0], "lym loom sampler-done") || w.mailNames[0] != "strand-1" {
					t.Errorf("mails = %q to %q; want one line naming main.go and lym loom sampler-done, sent to strand-1", w.mails, w.mailNames)
				}
			},
		},
		{
			name:        "a flag with a damaged decision record is Stuck with Halt naming the restore steps",
			setup:       func(w *samplerWorld) { w.writeRecord(flagged(recorded, 2)); w.findings = invalidRecord },
			wantOutcome: shedengine.Stuck,
			wantReason:  []string{"decision D2 is missing", "git -C _lym checkout"},
		},
		{
			name: "a flag with a damaged decision record is Stuck on entry also when the session would not resume",
			setup: func(w *samplerWorld) {
				w.writeRecord(flagged(recorded, 2))
				w.findings = invalidRecord
				w.sessions.startErr = errors.New("cannot resume")
			},
			wantOutcome: shedengine.Stuck,
			wantReason:  []string{"decision D2 is missing"},
		},
		{
			name: "a flag written during the wait stops the session and ends Done",
			setup: func(w *samplerWorld) {
				w.onWake = func(w *samplerWorld, n int) {
					if n == 2 {
						w.raiseFlag()
					}
				}
			},
			wantOutcome: shedengine.Done, wantResumes: []string{""}, wantStops: []string{"guid-1"},
		},
		{
			name: "a flag written during the wait on a dirty checkout clears the flag, resumes the session and mails it once",
			setup: func(w *samplerWorld) {
				w.dirty = dirtyCheckout
				w.onWake = func(w *samplerWorld, n int) {
					if n == 2 {
						w.raiseFlag()
					}
				}
			},
			wantErr: "cancelled", wantResumes: []string{"", "session-1"}, wantStops: []string{"guid-1"}, wantMails: 1,
			check: func(t *testing.T, w *samplerWorld) {
				if got := w.record(); got.DoneStamp != 0 {
					t.Errorf("record.DoneStamp = %d; want 0, the flag cleared", got.DoneStamp)
				}
				if len(w.mails) != 1 || !strings.Contains(w.mails[0], "main.go") || w.mailNames[0] != "strand-2" {
					t.Errorf("mails = %q to %q; want one line naming main.go, sent to strand-2", w.mails, w.mailNames)
				}
			},
		},
		{
			name: "a flag written during the wait with a damaged decision record is Stuck with Halt naming the restore steps",
			setup: func(w *samplerWorld) {
				w.findings = invalidRecord
				w.onWake = func(w *samplerWorld, n int) {
					if n == 2 {
						w.raiseFlag()
					}
				}
			},
			wantOutcome: shedengine.Stuck, wantResumes: []string{""}, wantStops: []string{"guid-1"},
			wantReason: []string{"decision D2 is missing", "git -C _lym checkout"},
		},
		{
			name: "a strand that dies unstopped during the wait is Stuck with Halt naming the ways forward",
			setup: func(w *samplerWorld) {
				w.onWake = func(w *samplerWorld, n int) {
					if n == 2 {
						w.sessions.live["guid-1"] = false
					}
				}
			},
			wantOutcome: shedengine.Stuck, wantResumes: []string{""},
			wantReason: []string{"lym loom resume", "lym loom sampler-done", "lym batten cancel"},
		},
		{
			name: "a covered wake stops the session and the next uncovered wake resumes it with no halt",
			setup: func(w *samplerWorld) {
				w.sleepBudget = 4
				w.covered = func(n int) bool { return n == 2 }
			},
			wantErr: "cancelled", wantResumes: []string{"", "session-1"}, wantStops: []string{"guid-1"},
		},
		{
			name: "a covered wake with a current flag stops the session and ends no verdict",
			setup: func(w *samplerWorld) {
				w.covered = func(n int) bool { return n >= 2 }
				w.onWake = func(w *samplerWorld, n int) {
					if n == 2 {
						w.raiseFlag()
					}
				}
			},
			wantErr: "cancelled", wantResumes: []string{""}, wantStops: []string{"guid-1"},
		},
		{
			name:    "an unreadable pause record reads as uncovered",
			setup:   func(w *samplerWorld) { w.coveredErr = errors.New("pause record unreadable") },
			wantErr: "cancelled", wantResumes: []string{""},
		},
		{
			name:    "a RegisterSession failure keeps the session and the row waiting",
			setup:   func(w *samplerWorld) { w.registerErr = errors.New("registry full") },
			wantErr: "cancelled", wantResumes: []string{""},
			check: func(t *testing.T, w *samplerWorld) {
				if got := w.record(); got.StrandGUID != "guid-1" {
					t.Errorf("record.StrandGUID = %q; want guid-1", got.StrandGUID)
				}
			},
		},
		{
			name: "a failed record write after a start stops the new strand and is returned",
			setup: func(w *samplerWorld) {
				w.sessions.onStart = func() {
					if err := os.MkdirAll(w.recordPath, 0o755); err != nil {
						w.t.Errorf("MkdirAll(%q) = %v; want nil", w.recordPath, err)
					}
				}
			},
			wantErr: "record the sampler session", wantResumes: []string{""}, wantStops: []string{"guid-1"},
		},
		{
			name: "a record read error ends the call with the error and the live session stopped",
			setup: func(w *samplerWorld) {
				w.onWake = func(w *samplerWorld, n int) {
					if n == 2 {
						if err := os.Remove(w.recordPath); err != nil {
							w.t.Errorf("Remove(%q) = %v; want nil", w.recordPath, err)
						}
						if err := os.Mkdir(w.recordPath, 0o755); err != nil {
							w.t.Errorf("Mkdir(%q) = %v; want nil", w.recordPath, err)
						}
					}
				}
			},
			wantErr: "read the sampler record", wantResumes: []string{""}, wantStops: []string{"guid-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w, ctx := newSamplerWorld(t)
			if tt.setup != nil {
				tt.setup(w)
			}
			outcome, pointer, err := NewSamplerSession(NameSamplerSession, w.deps()).Call(ctx)

			switch {
			case tt.wantErr == "cancelled":
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Call() error = %v; want the cancelled context", err)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || errors.Is(err, context.Canceled) {
					t.Errorf("Call() error = %v; want an error containing %q", err, tt.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != tt.wantOutcome {
					t.Errorf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
				}
			}
			if outcome == shedengine.Stuck && !pointer.Halt {
				t.Errorf("Call() Stuck without Halt; want the run halted, reason %q", pointer.Reason)
			}
			for _, want := range tt.wantReason {
				if !strings.Contains(pointer.Reason, want) {
					t.Errorf("Call() reason = %q; want it to contain %q", pointer.Reason, want)
				}
			}
			if !slices.Equal(w.sessions.resumes, tt.wantResumes) {
				t.Errorf("session starts with resume ids %q; want %q", w.sessions.resumes, tt.wantResumes)
			}
			if !slices.Equal(w.sessions.stops, tt.wantStops) {
				t.Errorf("session stops = %q; want %q", w.sessions.stops, tt.wantStops)
			}
			if len(w.mails) != tt.wantMails {
				t.Errorf("mails = %q; want %d", w.mails, tt.wantMails)
			}
			if tt.check != nil {
				tt.check(t, w)
			}
		})
	}
}

// TestSamplerSession_WaitSpacing covers the wait's wakes: they start a second apart and double to a minute while nothing changes, and a Watch delivery cuts a long sleep short and returns the interval to a second.
func TestSamplerSession_WaitSpacing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("wakes double up to a minute", func(t *testing.T) {
		t.Parallel()
		w, ctx := newSamplerWorld(t)
		w.sleepBudget = 9
		_, _, err := NewSamplerSession(NameSamplerSession, w.deps()).Call(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Call() error = %v; want the cancelled context", err)
		}
		want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
		for i := range want {
			want[i] *= time.Second
		}
		if !slices.Equal(w.sleeps, want) {
			t.Errorf("sleeps = %v; want %v", w.sleeps, want)
		}
	})

	t.Run("a Watch delivery cuts a long sleep short and the next wake reads the flag", func(t *testing.T) {
		t.Parallel()
		w, ctx := newSamplerWorld(t)
		w.watch = make(chan struct{}, 1)
		w.sleepBudget = 100
		var deliveredAt time.Time
		w.sleepFn = func(w *samplerWorld, ctx context.Context, d time.Duration) error {
			if d <= time.Second {
				return w.advance(d)
			}
			w.mu.Lock()
			w.sleeps = append(w.sleeps, d)
			deliveredAt = w.now
			w.mu.Unlock()
			w.raiseFlag()
			w.watch <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		}
		outcome, _, err := NewSamplerSession(NameSamplerSession, w.deps()).Call(ctx)
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		want := []time.Duration{time.Second, 2 * time.Second, time.Second}
		if !slices.Equal(w.sleeps, want) {
			t.Errorf("sleeps = %v; want %v", w.sleeps, want)
		}
		if waited := w.now.Sub(deliveredAt); waited > time.Second {
			t.Errorf("the flag was read %v after the delivery; want within a second", waited)
		}
	})
}

// TestSamplerSession_UnsetSeam covers the wiring guard: a seam left unset is an error naming it, and Watch alone may stay unset.
func TestSamplerSession_UnsetSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	w, ctx := newSamplerWorld(t)
	deps := w.deps()
	deps.Stamp = nil
	if _, _, err := NewSamplerSession(NameSamplerSession, deps).Call(ctx); err == nil || !strings.Contains(err.Error(), "Stamp") {
		t.Errorf("Call() error = %v; want an error naming Stamp", err)
	}
}
