// alignment_test.go covers the gate's alignment-agent path over a scripted fake Aligner.

package parentreview

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeAligner counts its calls and answers Poll from a script; the last scripted answer repeats, and an empty script answers "still running".
type fakeAligner struct {
	spawnErr error
	pollErr  error
	closeErr error
	script   []AlignmentPoll
	// onPoll runs at the start of each Poll, so a row can land a parent verdict mid-poll.
	onPoll func()

	spawns, polls, closes int
}

func (f *fakeAligner) Spawn(round int) (Alignment, error) {
	f.spawns++
	if f.spawnErr != nil {
		return Alignment{}, f.spawnErr
	}
	return Alignment{Attempt: "attempt-1", Strand: "agent"}, nil
}

func (f *fakeAligner) Poll(Alignment) (AlignmentPoll, error) {
	f.polls++
	if f.onPoll != nil {
		f.onPoll()
	}
	if f.pollErr != nil {
		return AlignmentPoll{}, f.pollErr
	}
	if len(f.script) == 0 {
		return AlignmentPoll{}, nil
	}
	return f.script[min(f.polls, len(f.script))-1], nil
}

func (f *fakeAligner) Close(Alignment, string) error {
	f.closes++
	return f.closeErr
}

type alignOpts struct {
	reviewer  string
	cap       int
	keepAlive time.Duration
	live      func() (bool, error)
}

func newAlignGates(t *testing.T, s Store, al *fakeAligner, o alignOpts) (gate, final shuttleengine.Gate) {
	t.Helper()
	return NewGate(GateConfig{
		Aligner: al,
		RenderAlignmentBrief: func(report, reason string) (string, error) {
			return "aligned report=" + report + " reason=" + reason, nil
		},
		Store:           s,
		Slug:            "x",
		Reviewer:        o.reviewer,
		DecisionRecord:  "d.md",
		SupportLog:      "s.md",
		WaitBound:       time.Hour,
		Cap:             o.cap,
		KeepAlive:       o.keepAlive,
		ReviewerLive:    o.live,
		RenderDelivery:  func(p string) (string, error) { return "review " + p, nil },
		RenderBrief:     func() (string, error) { return "brief", nil },
		RenderKeepAlive: func(n int) (string, error) { return "ping " + string(rune('0'+n)), nil },
	})
}

func readBrief(t *testing.T, s Store) string {
	t.Helper()
	data, err := os.ReadFile(latest(t, s).BriefPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func wantAlignmentState(t *testing.T, s Store, want string) {
	t.Helper()
	a := latest(t, s).Alignment
	if a == nil || a.State != want {
		t.Fatalf("alignment = %+v, want state %s", a, want)
	}
}

func TestGate_Alignment(t *testing.T) {
	t.Parallel()
	const reviewer = "hub:orch"
	approve := func(t *testing.T) AlignmentPoll {
		return AlignmentPoll{Done: true, Verdict: VerdictApprove, Report: writeFile(t, "all aligned")}
	}
	reject := func(t *testing.T) AlignmentPoll {
		return AlignmentPoll{Done: true, Verdict: VerdictReject, Report: writeFile(t, "out of scope")}
	}

	t.Run("the first evaluation spawns once and carries nothing, live or not and with or without a reviewer", func(t *testing.T) {
		t.Parallel()
		for name, o := range map[string]alignOpts{
			"reviewer not live": {reviewer: reviewer, live: func() (bool, error) { return false, nil }},
			"empty reviewer":    {reviewer: ""},
		} {
			s, _ := newStore(t)
			al := &fakeAligner{}
			gate, _ := newAlignGates(t, s, al, o)
			wantHold(t, mustEval(t, gate))
			wantHold(t, mustEval(t, gate))
			if al.spawns != 1 || al.polls != 1 {
				t.Fatalf("%s: spawns = %d, polls = %d; want 1 and 1", name, al.spawns, al.polls)
			}
			wantAlignmentState(t, s, AlignmentRunning)
			if d := latest(t, s).Delivery; d.Prompts != 0 || !d.DeliveredAt.IsZero() {
				t.Fatalf("%s: delivery = %+v; want none", name, d)
			}
		}
	})

	t.Run("a spawn error falls back to delivery and the agent is never reached again", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{spawnErr: errors.New("tmux is gone")}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantCarry(t, mustEval(t, gate))
		if b := readBrief(t, s); !strings.Contains(b, "tmux is gone") {
			t.Fatalf("brief = %q; want the spawn error named", b)
		}
		wantAlignmentState(t, s, AlignmentFallback)
		mustEval(t, gate)
		if al.spawns != 1 || al.polls != 0 {
			t.Fatalf("spawns = %d, polls = %d; want 1 and 0", al.spawns, al.polls)
		}
	})

	t.Run("an approve passes with the report as review.md and no prompt", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{script: []AlignmentPoll{approve(t)}}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		if res := mustEval(t, gate); !res.Passed || res.Send != "" {
			t.Fatalf("want a pass carrying nothing, got %+v", res)
		}
		r := latest(t, s)
		if r.Verdict == nil || r.Verdict.Kind != VerdictApprove || r.Verdict.Superseding {
			t.Fatalf("verdict = %+v; want a plain approve", r.Verdict)
		}
		if data, err := os.ReadFile(r.ReviewPath()); err != nil || string(data) != "all aligned" {
			t.Fatalf("review.md = %q, %v", data, err)
		}
		wantAlignmentState(t, s, AlignmentApplied)
		if al.closes != 1 || r.Delivery.Prompts != 0 {
			t.Fatalf("closes = %d, prompts = %d; want 1 and 0", al.closes, r.Delivery.Prompts)
		}
	})

	t.Run("a reject returns findings and a second reject at the cap fails terminally", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{script: []AlignmentPoll{reject(t), reject(t)}}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer, cap: 2})
		wantHold(t, mustEval(t, gate))
		res := mustEval(t, gate)
		if res.Terminal || res.Pending || !strings.Contains(res.Findings, "review.md") {
			t.Fatalf("want findings naming the review, got %+v", res)
		}
		if !latest(t, s).Verdict.Consumed {
			t.Fatal("the reject must be consumed")
		}
		wantHold(t, mustEval(t, gate))
		res = mustEval(t, gate)
		wantTerminal(t, res)
		if !strings.Contains(res.Findings, "lyx loom review approve x") {
			t.Fatalf("findings = %q; want the cap's way out", res.Findings)
		}
	})

	for _, row := range []struct {
		name  string
		al    func(t *testing.T) *fakeAligner
		brief string
	}{
		{"an escalate", func(t *testing.T) *fakeAligner {
			return &fakeAligner{script: []AlignmentPoll{{Done: true, Verdict: "escalate", Report: "r.md", Reason: "overlaps peer y"}}}
		}, "report=r.md reason=overlaps peer y"},
		{"a done poll with only a reason", func(t *testing.T) *fakeAligner {
			return &fakeAligner{script: []AlignmentPoll{{Done: true, Reason: "timed out"}}}
		}, "reason=timed out"},
		{"a poll error", func(t *testing.T) *fakeAligner {
			return &fakeAligner{pollErr: errors.New("probe broke")}
		}, "probe broke"},
		{"a close error on the fallback", func(t *testing.T) *fakeAligner {
			return &fakeAligner{closeErr: errors.New("kill failed"), script: []AlignmentPoll{{Done: true, Reason: "died"}}}
		}, "reason=died"},
	} {
		t.Run(row.name+" rewrites the brief and delivers, and the agent is not polled again", func(t *testing.T) {
			t.Parallel()
			s, _ := newStore(t)
			al := row.al(t)
			gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
			wantHold(t, mustEval(t, gate))
			wantCarry(t, mustEval(t, gate))
			if b := readBrief(t, s); !strings.Contains(b, row.brief) {
				t.Fatalf("brief = %q; want it to contain %q", b, row.brief)
			}
			wantAlignmentState(t, s, AlignmentFallback)
			polls := al.polls
			al.script, al.pollErr = []AlignmentPoll{approve(t)}, nil
			mustEval(t, gate)
			if al.polls != polls {
				t.Fatalf("polls went from %d to %d after the fallback", polls, al.polls)
			}
		})
	}

	t.Run("a parent verdict stops the agent and stands, also when it lands inside Poll", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
			t.Fatal(err)
		}
		if !mustEval(t, gate).Passed {
			t.Fatal("a parent approve must pass")
		}
		wantAlignmentState(t, s, AlignmentStopped)
		if al.closes != 1 || al.polls != 0 {
			t.Fatalf("closes = %d, polls = %d; want 1 and 0", al.closes, al.polls)
		}

		s, _ = newStore(t)
		al = &fakeAligner{script: []AlignmentPoll{reject(t)}}
		al.onPoll = func() {
			if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
				t.Fatal(err)
			}
		}
		gate, _ = newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		if !mustEval(t, gate).Passed {
			t.Fatal("the parent's approve must stand over the agent's reject")
		}
		if r := latest(t, s); r.Verdict.Kind != VerdictApprove {
			t.Fatalf("verdict = %+v; want the parent's approve", r.Verdict)
		}
		wantAlignmentState(t, s, AlignmentStopped)
	})

	t.Run("a notify while running is held and carried after the fallback", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		if err := s.AddNotify(); err != nil {
			t.Fatal(err)
		}
		wantHold(t, mustEval(t, gate))
		if n := latest(t, s).Delivery.WaitingNotifys; n != 1 {
			t.Fatalf("waiting notifies = %d; want 1 held", n)
		}
		al.script = []AlignmentPoll{{Done: true, Verdict: "escalate"}}
		al.polls = 0
		wantCarry(t, mustEval(t, gate))
		if n := latest(t, s).Delivery.WaitingNotifys; n != 0 {
			t.Fatalf("waiting notifies = %d; want the notify carried", n)
		}
	})

	t.Run("fresh closures after a restart do not respawn and apply the next approve", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		al.script = []AlignmentPoll{approve(t)}
		al.polls = 0
		restarted, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		if !mustEval(t, restarted).Passed || al.spawns != 1 {
			t.Fatalf("spawns = %d; want the approve applied after 1 spawn", al.spawns)
		}
	})

	t.Run("a keep-alive runs from the opening while the agent works", func(t *testing.T) {
		t.Parallel()
		s, c := newStore(t)
		al := &fakeAligner{}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer, keepAlive: time.Minute})
		wantHold(t, mustEval(t, gate))
		wantHold(t, mustEval(t, gate))
		c.t = c.t.Add(2 * time.Minute)
		wantPing(t, mustEval(t, gate), 1)
	})

	t.Run("the wait bound and the writer's end each stop a running agent and pass", func(t *testing.T) {
		t.Parallel()
		s, c := newStore(t)
		al := &fakeAligner{}
		gate, _ := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		c.t = c.t.Add(2 * time.Hour)
		if !mustEval(t, gate).Passed {
			t.Fatal("the wait bound must pass")
		}
		wantAlignmentState(t, s, AlignmentStopped)
		if al.closes != 1 {
			t.Fatalf("closes = %d; want 1", al.closes)
		}

		s, _ = newStore(t)
		al = &fakeAligner{}
		gate, final := newAlignGates(t, s, al, alignOpts{reviewer: reviewer})
		wantHold(t, mustEval(t, gate))
		if !mustEval(t, final).Passed {
			t.Fatal("final must pass")
		}
		wantAlignmentState(t, s, AlignmentStopped)
		if al.closes != 1 {
			t.Fatalf("closes = %d; want 1", al.closes)
		}
	})

	t.Run("a gate rebuilt with no Aligner settles a running record as stopped without panicking", func(t *testing.T) {
		t.Parallel()
		for name, settle := range map[string]func(t *testing.T, s Store, c *clock, gate, final shuttleengine.Gate){
			"a parent verdict": func(t *testing.T, s Store, _ *clock, gate, _ shuttleengine.Gate) {
				if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
					t.Fatal(err)
				}
				if !mustEval(t, gate).Passed {
					t.Fatal("a parent approve must pass")
				}
			},
			"the wait bound": func(t *testing.T, _ Store, c *clock, gate, _ shuttleengine.Gate) {
				c.t = c.t.Add(2 * time.Hour)
				if !mustEval(t, gate).Passed {
					t.Fatal("the wait bound must pass")
				}
			},
			"the writer's end": func(t *testing.T, _ Store, _ *clock, _, final shuttleengine.Gate) {
				if !mustEval(t, final).Passed {
					t.Fatal("final must pass")
				}
			},
		} {
			s, c := newStore(t)
			gate, _ := newAlignGates(t, s, &fakeAligner{}, alignOpts{reviewer: reviewer})
			wantHold(t, mustEval(t, gate))
			offGate, offFinal := NewGate(GateConfig{
				Store:           s,
				Slug:            "x",
				Reviewer:        reviewer,
				DecisionRecord:  "d.md",
				SupportLog:      "s.md",
				WaitBound:       time.Hour,
				RenderDelivery:  func(p string) (string, error) { return "review " + p, nil },
				RenderBrief:     func() (string, error) { return "brief", nil },
				RenderKeepAlive: func(n int) (string, error) { return "ping", nil },
			})
			settle(t, s, c, offGate, offFinal)
			if a := latest(t, s).Alignment; a == nil || a.State != AlignmentStopped {
				t.Fatalf("%s: alignment = %+v, want state %s", name, a, AlignmentStopped)
			}
		}
	})

	t.Run("final fails terminally on an agent reject even with no reviewer", func(t *testing.T) {
		t.Parallel()
		s, _ := newStore(t)
		al := &fakeAligner{script: []AlignmentPoll{reject(t)}}
		gate, final := newAlignGates(t, s, al, alignOpts{reviewer: ""})
		wantHold(t, mustEval(t, gate))
		if res := mustEval(t, gate); res.Findings == "" {
			t.Fatalf("want findings, got %+v", res)
		}
		wantTerminal(t, mustEval(t, final))
	})
}
