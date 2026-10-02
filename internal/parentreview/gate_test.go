package parentreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func newGates(t *testing.T, s Store, reviewer string) (gate, final shuttleengine.Gate) {
	t.Helper()
	return NewGate(GateConfig{
		Store:          s,
		Slug:           "x",
		Reviewer:       reviewer,
		DecisionRecord: "d.md",
		SupportLog:     "s.md",
		WaitBound:      time.Hour,
		RenderDelivery: func(p string) (string, error) { return "review " + p, nil },
		RenderBrief:    func() (string, error) { return "brief", nil },
	})
}

func mustEval(t *testing.T, g shuttleengine.Gate) shuttleengine.GateResult {
	t.Helper()
	res, err := g()
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func wantCarry(t *testing.T, res shuttleengine.GateResult) {
	t.Helper()
	if !res.Pending || !strings.HasPrefix(res.Send, "review ") {
		t.Fatalf("want pending carrying a prompt, got %+v", res)
	}
	if !strings.Contains(res.SendFailedWayForward, "lyx loom review notify x") {
		t.Fatalf("way-forward = %q", res.SendFailedWayForward)
	}
}

func wantHold(t *testing.T, res shuttleengine.GateResult) {
	t.Helper()
	if !res.Pending || res.Send != "" {
		t.Fatalf("want pending with no prompt, got %+v", res)
	}
}

func latest(t *testing.T, s Store) Round {
	t.Helper()
	r, ok, err := s.Latest()
	if err != nil || !ok {
		t.Fatalf("Latest = %v, %v", ok, err)
	}
	return r
}

func TestGate_NoReviewerPasses(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "")
	if !mustEval(t, gate).Passed || !mustEval(t, final).Passed {
		t.Fatal("no reviewer must pass both closures")
	}
	if _, ok, _ := s.Latest(); ok {
		t.Fatal("no reviewer must not open a round")
	}
}

func TestGate_OpensAndPrompts(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	res := mustEval(t, gate)
	wantCarry(t, res)
	r := latest(t, s)
	if r.Request == nil || r.Request.Reviewer != "hub:orch" || r.Delivery.Prompts != 1 {
		t.Fatalf("round = %+v", r)
	}
	if !strings.Contains(res.Send, r.RequestPath()) {
		t.Fatalf("send %q lacks request path", res.Send)
	}
}

func TestGate_ThrottleAndCap(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	wantHold(t, mustEval(t, gate))
	c.t = c.t.Add(time.Minute)
	wantCarry(t, mustEval(t, gate))
	c.t = c.t.Add(time.Minute)
	wantCarry(t, mustEval(t, gate))
	c.t = c.t.Add(time.Minute)
	wantHold(t, mustEval(t, gate))
	if !latest(t, s).Delivery.CapWarned {
		t.Fatal("cap Warn not recorded")
	}
	c.t = c.t.Add(time.Minute)
	wantHold(t, mustEval(t, gate))
	if got := latest(t, s).Delivery.Prompts; got != 3 {
		t.Fatalf("prompts = %d, want 3", got)
	}
}

func TestGate_NotifyPastCap(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	for i := 0; i < 3; i++ {
		wantCarry(t, mustEval(t, gate))
		c.t = c.t.Add(time.Minute)
	}
	wantHold(t, mustEval(t, gate))
	if err := s.AddNotify(); err != nil {
		t.Fatal(err)
	}
	wantCarry(t, mustEval(t, gate))
	wantHold(t, mustEval(t, gate))
	if r := latest(t, s); r.Delivery.WaitingNotifys != 0 || r.Delivery.Prompts != 3 {
		t.Fatalf("delivery = %+v", r.Delivery)
	}
}

func TestGate_NotifyBetweenClosuresCarriedByFirstCallOfSecond(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.AddNotify(); err != nil {
		t.Fatal(err)
	}
	// Still inside the throttle, so only the notify can carry.
	wantCarry(t, mustEval(t, gate))
	wantHold(t, mustEval(t, gate))
}

func TestGate_FailedDeliveryDoesNotStopResends(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordDelivered("no such agent"); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(time.Minute)
	wantCarry(t, mustEval(t, gate))
}

func TestGate_DeliveredStopsResends(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordDelivered(""); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(10 * time.Minute)
	wantHold(t, mustEval(t, gate))
}

func TestGate_ExpiryFromOriginalOpenedAtAfterRestart(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	c.t = c.t.Add(59 * time.Minute)
	restarted, _ := newGates(t, s, "hub:orch")
	if res := mustEval(t, restarted); res.Passed {
		t.Fatalf("passed before the bound: %+v", res)
	}
	c.t = c.t.Add(time.Minute)
	if res := mustEval(t, restarted); !res.Passed {
		t.Fatalf("not passed at the bound: %+v", res)
	}
	if got := latest(t, s).Request.State; got != StateExpired {
		t.Fatalf("state = %q", got)
	}
	if !mustEval(t, gate).Passed {
		t.Fatal("expired round must pass")
	}
}

// TestGate_VerdictLandingBeforeExpiryIsRead asserts a reject recorded between the gate's read and its expiry is read and re-prompts, never expired unread.
// The store's clock runs between the two, so it stands in for the verb's concurrent RecordVerdict.
func TestGate_VerdictLandingBeforeExpiryIsRead(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	r := latest(t, s)
	landed := false
	s.Now = func() time.Time {
		if !landed {
			landed = true
			if err := os.WriteFile(r.ReviewPath(), []byte("fix it"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(r.Dir, verdictFile), []byte(`{"kind":"reject"}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return c.t.Add(time.Hour)
	}
	gate, _ = newGates(t, s, "hub:orch")
	res := mustEval(t, gate)
	if res.Passed || res.Pending || !strings.Contains(res.Findings, r.ReviewPath()) {
		t.Fatalf("gate = %+v; want the reject's findings naming %s", res, r.ReviewPath())
	}
	if got := latest(t, s).Request.State; got != StateOpen {
		t.Fatalf("state = %q; want open", got)
	}
}

func TestGate_CapAndThrottleAcrossRestart(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	c.t = c.t.Add(time.Minute)
	wantCarry(t, mustEval(t, gate))
	restarted, _ := newGates(t, s, "hub:orch")
	wantHold(t, mustEval(t, restarted))
	c.t = c.t.Add(time.Minute)
	wantCarry(t, mustEval(t, restarted))
	c.t = c.t.Add(time.Minute)
	wantHold(t, mustEval(t, restarted))
}

func TestGate_Approve(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
		t.Fatal(err)
	}
	if !mustEval(t, gate).Passed {
		t.Fatal("approve must pass")
	}
}

func TestGate_RejectConsumedOnceThenPasses(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix this")); err != nil {
		t.Fatal(err)
	}
	res := mustEval(t, gate)
	if res.Passed || res.Pending || !strings.Contains(res.Findings, latest(t, s).ReviewPath()) {
		t.Fatalf("reject result = %+v", res)
	}
	if !latest(t, s).Verdict.Consumed {
		t.Fatal("reject not consumed")
	}
	if !mustEval(t, gate).Passed {
		t.Fatal("consumed reject must pass")
	}
}

func TestGate_ConsumedRejectOnAttachPasses(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix this")); err != nil {
		t.Fatal(err)
	}
	mustEval(t, gate)
	attached, _ := newGates(t, s, "hub:orch")
	if !mustEval(t, attached).Passed {
		t.Fatal("attach over a consumed reject must pass")
	}
}

func TestGate_SupersededPasses(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	// The new round is empty, so the gate opens it; the superseded one is never consulted.
	wantCarry(t, mustEval(t, gate))
	if latest(t, s).Number != 2 {
		t.Fatal("expected round 2")
	}
}

func TestFinal_DoesNotOpen(t *testing.T) {
	s, _ := newStore(t)
	_, final := newGates(t, s, "hub:orch")
	if !mustEval(t, final).Passed {
		t.Fatal("final with no round must pass")
	}
	if _, ok, _ := s.Latest(); ok {
		t.Fatal("final opened a round")
	}
}

func TestFinal_OpenWithoutVerdictExpiresAndIsPending(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	wantHold(t, mustEval(t, final))
	if got := latest(t, s).Request.State; got != StateExpired {
		t.Fatalf("state = %q", got)
	}
	if !mustEval(t, final).Passed {
		t.Fatal("expired round must pass on the next final")
	}
}

func TestFinal_UnconsumedRejectWarnsAndPassesWithoutConsuming(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix this")); err != nil {
		t.Fatal(err)
	}
	if !mustEval(t, final).Passed {
		t.Fatal("final must pass an unconsumed reject")
	}
	if latest(t, s).Verdict.Consumed {
		t.Fatal("final consumed the verdict")
	}
}

func TestFinal_ApprovePasses(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
		t.Fatal(err)
	}
	if !mustEval(t, final).Passed {
		t.Fatal("final must pass an approve")
	}
}
