package parentreview

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newStore(t *testing.T) (Store, *clock) {
	t.Helper()
	d := t.TempDir()
	c := &clock{t: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	return Store{Root: filepath.Join(d, "reviews"), LockDir: filepath.Join(d, "locks"), Now: c.now}, c
}

func openOne(t *testing.T, s Store) Round {
	t.Helper()
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	r, err := s.OpenRequest(OpenSpec{Slug: "x", Reviewer: "hub:orch", DecisionRecord: "d.md", SupportLog: "s.md", Brief: "brief"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLatest_NoRounds(t *testing.T) {
	s, _ := newStore(t)
	if _, ok, err := s.Latest(); ok || err != nil {
		t.Fatalf("Latest = %v, %v; want none", ok, err)
	}
}

func TestOpenRequest_WritesRequestAndBrief(t *testing.T) {
	s, _ := newStore(t)
	r := openOne(t, s)
	if r.Number != 1 || r.Request == nil || r.Request.State != StateOpen {
		t.Fatalf("round = %+v", r)
	}
	b, err := os.ReadFile(filepath.Join(r.Dir, "brief.md"))
	if err != nil || string(b) != "brief" {
		t.Fatalf("brief = %q, %v", b, err)
	}
	got, ok, _ := s.Latest()
	if !ok || got.Request == nil || got.Request.Reviewer != "hub:orch" {
		t.Fatalf("Latest = %+v", got)
	}
}

func TestBeginRound_SupersedesOpenAndNumbers(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	r2, err := s.BeginRound()
	if err != nil || r2.Number != 2 {
		t.Fatalf("BeginRound = %+v, %v", r2, err)
	}
	r1, err := s.load(1)
	if err != nil || r1.Request.State != StateSuperseded {
		t.Fatalf("round 1 = %+v, %v", r1.Request, err)
	}
	if err := s.AddNotify(); !errors.Is(err, ErrNoOpenRequest) {
		t.Fatalf("AddNotify on request-less latest = %v", err)
	}
}

func TestBeginRound_CompletedRoundNotSuperseded(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	r1, _ := s.load(1)
	if r1.Request.State != StateOpen {
		t.Fatalf("completed round state = %s", r1.Request.State)
	}
}

func TestRecordDelivered(t *testing.T) {
	s, c := newStore(t)
	if err := s.RecordDelivered(""); !errors.Is(err, ErrNoOpenRequest) {
		t.Fatalf("no round: %v", err)
	}
	openOne(t, s)
	if err := s.RecordDelivered("tmux gone"); err != nil {
		t.Fatal(err)
	}
	r, _, _ := s.Latest()
	if !r.Delivery.DeliveredAt.IsZero() || r.Delivery.FailedReason != "tmux gone" {
		t.Fatalf("delivery = %+v", r.Delivery)
	}
	if err := s.RecordDelivered(""); err != nil {
		t.Fatal(err)
	}
	r, _, _ = s.Latest()
	if !r.Delivery.DeliveredAt.Equal(c.t) || r.Delivery.FailedReason != "" {
		t.Fatalf("delivery = %+v", r.Delivery)
	}
	if err := s.MarkExpired(); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDelivered(""); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
}

func TestRecordDelivered_SupersededRefused(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	s.BeginRound()
	s.OpenRequest(OpenSpec{Slug: "x"})
	if err := s.setRequestState(1, StateSuperseded); err != nil {
		t.Fatal(err)
	}
	// latest is round 2, open; supersede it to hit the refusal.
	if err := s.setRequestState(2, StateSuperseded); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDelivered(""); !errors.Is(err, ErrNoOpenRequest) {
		t.Fatalf("superseded: %v", err)
	}
}

func TestAddNotify(t *testing.T) {
	s, _ := newStore(t)
	if err := s.AddNotify(); !errors.Is(err, ErrNoOpenRequest) {
		t.Fatalf("no round: %v", err)
	}
	openOne(t, s)
	if err := s.AddNotify(); err != nil {
		t.Fatal(err)
	}
	if err := s.AddNotify(); err != nil {
		t.Fatal(err)
	}
	r, _, _ := s.Latest()
	if r.Delivery.WaitingNotifys != 2 {
		t.Fatalf("notifies = %d", r.Delivery.WaitingNotifys)
	}
	if err := s.RecordPrompt(true); err != nil {
		t.Fatal(err)
	}
	r, _, _ = s.Latest()
	if r.Delivery.WaitingNotifys != 1 || r.Delivery.Prompts != 0 || r.Delivery.LastPromptAt.IsZero() {
		t.Fatalf("delivery = %+v", r.Delivery)
	}
	if err := s.RecordPrompt(false); err != nil {
		t.Fatal(err)
	}
	r, _, _ = s.Latest()
	if r.Delivery.Prompts != 1 {
		t.Fatalf("prompts = %d", r.Delivery.Prompts)
	}
}

func TestAddNotify_VerdictAndExpired(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AddNotify(); !errors.Is(err, ErrVerdictRecorded) {
		t.Fatalf("verdict: %v", err)
	}
	s2, _ := newStore(t)
	openOne(t, s2)
	s2.MarkExpired()
	if err := s2.AddNotify(); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
}

func TestRecordVerdict(t *testing.T) {
	s, c := newStore(t)
	if err := s.RecordVerdict(VerdictApprove, ""); !errors.Is(err, ErrNoOpenRequest) {
		t.Fatalf("no round: %v", err)
	}
	openOne(t, s)
	if err := s.RecordVerdict(VerdictReject, ""); !errors.Is(err, ErrEmptyReviewFile) {
		t.Fatalf("reject no file: %v", err)
	}
	if err := s.RecordVerdict(VerdictReject, filepath.Join(t.TempDir(), "missing.md")); !errors.Is(err, ErrEmptyReviewFile) {
		t.Fatalf("reject missing file: %v", err)
	}
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "  \n")); !errors.Is(err, ErrEmptyReviewFile) {
		t.Fatalf("reject empty file: %v", err)
	}
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix it")); err != nil {
		t.Fatal(err)
	}
	r, _, _ := s.Latest()
	if r.Verdict == nil || r.Verdict.Kind != VerdictReject || r.Verdict.Consumed || !r.Verdict.RecordedAt.Equal(c.t) {
		t.Fatalf("verdict = %+v", r.Verdict)
	}
	if b, _ := os.ReadFile(r.ReviewPath()); string(b) != "fix it" {
		t.Fatalf("review.md = %q", b)
	}
	if err := s.RecordVerdict(VerdictApprove, ""); !errors.Is(err, ErrVerdictRecorded) {
		t.Fatalf("second verdict: %v", err)
	}
	if err := s.MarkConsumed(); err != nil {
		t.Fatal(err)
	}
	r, _, _ = s.Latest()
	if !r.Verdict.Consumed {
		t.Fatal("verdict not consumed")
	}
}

func TestRecordVerdict_Expired(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	s.MarkExpired()
	if err := s.RecordVerdict(VerdictApprove, ""); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
}

// TestRecordVerdict_ExpiredSinceRead asserts a request the gate expires between RecordVerdict's refusal checks and its write refuses the verdict and leaves review.md unwritten.
// The store's clock runs between the two, so it stands in for the gate's concurrent MarkExpired.
func TestRecordVerdict_ExpiredSinceRead(t *testing.T) {
	s, c := newStore(t)
	r := openOne(t, s)
	s.Now = func() time.Time {
		req := `{"slug":"x","round":1,"reviewer":"hub:orch","state":"expired"}`
		if err := os.WriteFile(r.RequestPath(), []byte(req), 0o644); err != nil {
			t.Fatal(err)
		}
		return c.t
	}
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix it")); !errors.Is(err, ErrExpired) {
		t.Fatalf("RecordVerdict = %v; want ErrExpired", err)
	}
	if got := latest(t, s); got.Verdict != nil {
		t.Fatalf("verdict = %+v; want none", got.Verdict)
	}
	if _, err := os.Stat(r.ReviewPath()); !os.IsNotExist(err) {
		t.Fatalf("review.md stat = %v; want not written", err)
	}
}

// TestMarkExpired_RefusesRecordedVerdict asserts a round with a verdict is never expired, so the gate re-reads the verdict instead.
func TestMarkExpired_RefusesRecordedVerdict(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkExpired(); !errors.Is(err, ErrVerdictRecorded) {
		t.Fatalf("MarkExpired = %v; want ErrVerdictRecorded", err)
	}
	if got := latest(t, s).Request.State; got != StateOpen {
		t.Fatalf("state = %q; want open", got)
	}
}

func TestMarkCapWarned(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	if err := s.MarkCapWarned(); err != nil {
		t.Fatal(err)
	}
	r, _, _ := s.Latest()
	if !r.Delivery.CapWarned {
		t.Fatal("cap not warned")
	}
}

func TestWaitNote(t *testing.T) {
	s, _ := newStore(t)
	if n, err := s.WaitNote(); n != "" || err != nil {
		t.Fatalf("no round: %q, %v", n, err)
	}
	openOne(t, s)
	n, _ := s.WaitNote()
	if !strings.Contains(n, "hub:orch") || !strings.Contains(n, "notice not delivered") {
		t.Fatalf("open note = %q", n)
	}
	s.RecordDelivered("pane dead")
	n, _ = s.WaitNote()
	if !strings.Contains(n, "notice not delivered: pane dead") {
		t.Fatalf("failed note = %q", n)
	}
	s.RecordDelivered("")
	n, _ = s.WaitNote()
	if !strings.Contains(n, "notice delivered at 2026-10-02T12:00:00Z") {
		t.Fatalf("delivered note = %q", n)
	}
	s.RecordVerdict(VerdictApprove, "")
	if n, _ = s.WaitNote(); n != "" {
		t.Fatalf("verdicted note = %q", n)
	}
}

func TestWaitNote_ExpiredAndSuperseded(t *testing.T) {
	s, _ := newStore(t)
	openOne(t, s)
	s.MarkExpired()
	if n, _ := s.WaitNote(); n != "" {
		t.Fatalf("expired note = %q", n)
	}
	s2, _ := newStore(t)
	openOne(t, s2)
	s2.BeginRound()
	if n, _ := s2.WaitNote(); n != "" {
		t.Fatalf("superseded note = %q", n)
	}
}

// writeVerdict writes a verdict.json straight into round n's directory, creating it.
func writeVerdict(t *testing.T, s Store, n int, body string) {
	t.Helper()
	if err := os.MkdirAll(s.roundDir(n), 0o755); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(s.roundDir(n), verdictFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRejectedRounds_CountsOnlyRejects(t *testing.T) {
	s, _ := newStore(t)
	if n, err := s.RejectedRounds(); n != 0 || err != nil {
		t.Fatalf("no rounds = %d, %v", n, err)
	}
	writeVerdict(t, s, 1, `{"kind":"reject"}`)
	writeVerdict(t, s, 2, `{"kind":"approve"}`)
	writeVerdict(t, s, 3, `{"kind":"reject"}`)
	writeVerdict(t, s, 4, "")
	writeVerdict(t, s, 5, `{"kind":"reject"}`)
	if err := os.WriteFile(filepath.Join(s.roundDir(5), requestFile), []byte(`{"state":"expired"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RejectedRounds(); n != 3 || err != nil {
		t.Fatalf("RejectedRounds = %d, %v; want 3", n, err)
	}
}

func TestPrepareRound(t *testing.T) {
	tests := []struct {
		name      string
		verdict   string
		expire    bool
		noRound   bool
		wantRound int
	}{
		{name: "no round", noRound: true, wantRound: 1},
		{name: "reject kept", verdict: `{"kind":"reject"}`, wantRound: 1},
		{name: "superseding approve kept", verdict: `{"kind":"approve","superseding":true}`, wantRound: 1},
		{name: "plain approve begins", verdict: `{"kind":"approve"}`, wantRound: 2},
		{name: "expired begins", expire: true, wantRound: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newStore(t)
			if !tt.noRound {
				openOne(t, s)
				if tt.verdict != "" {
					writeVerdict(t, s, 1, tt.verdict)
				}
				if tt.expire {
					if err := s.MarkExpired(); err != nil {
						t.Fatal(err)
					}
				}
			}
			got, err := s.PrepareRound()
			if err != nil || got.Number != tt.wantRound {
				t.Fatalf("PrepareRound = %d, %v; want round %d", got.Number, err, tt.wantRound)
			}
		})
	}
}

func TestOpenRequest_RecordsCap(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	r, err := s.OpenRequest(OpenSpec{Slug: "x", Reviewer: "hub:orch", Brief: "b", Cap: 3})
	if err != nil || r.Request.Cap != 3 {
		t.Fatalf("OpenRequest = %+v, %v", r.Request, err)
	}
	if got := latest(t, s); got.Request.Cap != 3 {
		t.Fatalf("stored cap = %d", got.Request.Cap)
	}
}

// openCapped opens a round with the given cap and records a reject verdict carrying review text.
func openCapped(t *testing.T, s Store, cap int) {
	t.Helper()
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenRequest(OpenSpec{Slug: "x", Reviewer: "hub:orch", Brief: "b", Cap: cap}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix it")); err != nil {
		t.Fatal(err)
	}
}

func TestSupersedeCapReject_AtCap(t *testing.T) {
	s, c := newStore(t)
	openCapped(t, s, 2)
	openCapped(t, s, 2)
	if err := s.SupersedeCapReject(); err != nil {
		t.Fatal(err)
	}
	r := latest(t, s)
	if r.Verdict == nil || r.Verdict.Kind != VerdictApprove || !r.Verdict.Superseding || !r.Verdict.RecordedAt.Equal(c.t) {
		t.Fatalf("verdict = %+v", r.Verdict)
	}
	if b, _ := os.ReadFile(r.ReviewPath()); string(b) != "fix it" {
		t.Fatalf("review.md = %q", b)
	}
	if n, _ := s.RejectedRounds(); n != 1 {
		t.Fatalf("RejectedRounds = %d; want the earlier round only", n)
	}
	if err := s.SupersedeCapReject(); !errors.Is(err, ErrNotAtCap) {
		t.Fatalf("second supersede = %v; want ErrNotAtCap", err)
	}
}

func TestSupersedeCapReject_Refusals(t *testing.T) {
	t.Run("below cap", func(t *testing.T) {
		s, _ := newStore(t)
		openCapped(t, s, 3)
		openCapped(t, s, 3)
		if err := s.SupersedeCapReject(); !errors.Is(err, ErrNotAtCap) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("latest approve", func(t *testing.T) {
		s, _ := newStore(t)
		openOne(t, s)
		if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
			t.Fatal(err)
		}
		if err := s.SupersedeCapReject(); !errors.Is(err, ErrNotAtCap) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("open round without verdict", func(t *testing.T) {
		s, _ := newStore(t)
		openOne(t, s)
		if err := s.SupersedeCapReject(); !errors.Is(err, ErrNotAtCap) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("cap zero", func(t *testing.T) {
		s, _ := newStore(t)
		openCapped(t, s, 0)
		if err := s.SupersedeCapReject(); !errors.Is(err, ErrNotAtCap) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no round", func(t *testing.T) {
		s, _ := newStore(t)
		if err := s.SupersedeCapReject(); !errors.Is(err, ErrNotAtCap) {
			t.Fatalf("err = %v", err)
		}
	})
}
