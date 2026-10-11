package parentreview

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const testCap = 3

func newGates(t *testing.T, s Store, reviewer string) (gate, final shuttleengine.Gate) {
	t.Helper()
	return NewGate(GateConfig{
		Store:          s,
		Slug:           "x",
		Reviewer:       reviewer,
		DecisionRecord: "d.md",
		SupportLog:     "s.md",
		WaitBound:      time.Hour,
		Cap:            testCap,
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
	if !strings.Contains(res.Send, r.BriefPath()) {
		t.Fatalf("send %q lacks brief path", res.Send)
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

// rejectRound records a reject on the latest round.
func rejectRound(t *testing.T, s Store) {
	t.Helper()
	if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix this")); err != nil {
		t.Fatal(err)
	}
}

func wantTerminal(t *testing.T, res shuttleengine.GateResult) {
	t.Helper()
	if !res.Terminal || res.Passed || res.Pending {
		t.Fatalf("want a terminal failure, got %+v", res)
	}
}

func TestGate_RejectConsumedThenNextRoundOpens(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	rejectRound(t, s)
	res := mustEval(t, gate)
	if res.Passed || res.Pending || res.Terminal || !strings.Contains(res.Findings, latest(t, s).ReviewPath()) {
		t.Fatalf("reject result = %+v", res)
	}
	if !latest(t, s).Verdict.Consumed {
		t.Fatal("reject not consumed")
	}
	next := mustEval(t, gate)
	wantCarry(t, next)
	r := latest(t, s)
	if r.Number != 2 || r.Request == nil || r.Request.Cap != testCap || !strings.Contains(next.Send, r.BriefPath()) {
		t.Fatalf("round = %+v, send = %q", r, next.Send)
	}
}

func TestGate_ApproveOnRoundTwoPasses(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	rejectRound(t, s)
	mustEval(t, gate)
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
		t.Fatal(err)
	}
	if !mustEval(t, gate).Passed {
		t.Fatal("approve on round 2 must pass")
	}
}

// rejectRounds drives gate through n rejected rounds, leaving the nth reject unconsumed.
func rejectRounds(t *testing.T, s Store, gate shuttleengine.Gate, n int) {
	t.Helper()
	wantCarry(t, mustEval(t, gate))
	for i := 1; i <= n; i++ {
		rejectRound(t, s)
		if i == n {
			return
		}
		mustEval(t, gate)
		wantCarry(t, mustEval(t, gate))
	}
}

func TestGate_CapFailsTerminalNamingWayOut(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	rejectRounds(t, s, gate, testCap)
	res := mustEval(t, gate)
	wantTerminal(t, res)
	r := latest(t, s)
	for _, want := range []string{"x", "3", r.ReviewPath(), "lyx loom review approve x", "lyx loom start"} {
		if !strings.Contains(res.Findings, want) {
			t.Fatalf("findings %q lack %q", res.Findings, want)
		}
	}
	if r.Verdict.Consumed || r.Number != testCap {
		t.Fatalf("cap's reject consumed or round opened: %+v", r)
	}
}

func TestGate_CountSurvivesFreshGates(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	rejectRounds(t, s, gate, 2)
	attached, _ := newGates(t, s, "hub:orch")
	if res := mustEval(t, attached); res.Terminal || res.Passed || res.Pending {
		t.Fatalf("a fresh gate between rejects must consume, got %+v", res)
	}
	wantCarry(t, mustEval(t, attached))
	rejectRound(t, s)
	fresh, _ := newGates(t, s, "hub:orch")
	wantTerminal(t, mustEval(t, fresh))
}

func TestGate_ResumeAtCapRejectFailsWithoutOpening(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	rejectRounds(t, s, gate, testCap)
	resumed, _ := newGates(t, s, "hub:orch")
	wantTerminal(t, mustEval(t, resumed))
	if latest(t, s).Number != testCap {
		t.Fatal("a resume's arrival opened a round")
	}
}

func TestGate_ConsumedRejectOnAttachOpensNextRound(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	rejectRound(t, s)
	mustEval(t, gate)
	attached, _ := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, attached))
	if latest(t, s).Number != 2 {
		t.Fatal("expected round 2")
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

func TestFinal_OpenWithoutVerdictExpiresAndPasses(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	if !mustEval(t, final).Passed {
		t.Fatal("an open request with no verdict must expire and pass")
	}
	if got := latest(t, s).Request.State; got != StateExpired {
		t.Fatalf("state = %q", got)
	}
	if !mustEval(t, final).Passed {
		t.Fatal("expired round must pass on the next final")
	}
}

func TestFinal_UnconsumedRejectFailsTerminalWithoutConsuming(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	rejectRound(t, s)
	res := mustEval(t, final)
	wantTerminal(t, res)
	if !strings.Contains(res.Findings, latest(t, s).ReviewPath()) || !strings.Contains(res.Findings, "re-prompts it with that review") {
		t.Fatalf("findings = %q", res.Findings)
	}
	if latest(t, s).Verdict.Consumed {
		t.Fatal("final consumed the verdict")
	}
}

func TestFinal_ConsumedRejectFailsTerminalWithNextRoundReason(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	wantCarry(t, mustEval(t, gate))
	rejectRound(t, s)
	mustEval(t, gate)
	res := mustEval(t, final)
	wantTerminal(t, res)
	if !strings.Contains(res.Findings, "next round") {
		t.Fatalf("findings = %q", res.Findings)
	}
	if latest(t, s).Number != 1 {
		t.Fatal("final opened a round")
	}
}

func TestFinal_RejectAtCapNamesWayOut(t *testing.T) {
	s, _ := newStore(t)
	gate, final := newGates(t, s, "hub:orch")
	rejectRounds(t, s, gate, testCap)
	res := mustEval(t, final)
	wantTerminal(t, res)
	if !strings.Contains(res.Findings, "lyx loom review approve x") {
		t.Fatalf("findings = %q", res.Findings)
	}
}

func newLiveGates(t *testing.T, s Store, live func() (bool, error)) (gate, final shuttleengine.Gate) {
	t.Helper()
	return NewGate(GateConfig{
		Store:          s,
		Slug:           "x",
		Reviewer:       "hub:orch",
		DecisionRecord: "d.md",
		SupportLog:     "s.md",
		WaitBound:      time.Hour,
		Cap:            testCap,
		ReviewerLive:   live,
		RenderDelivery: func(p string) (string, error) { return "review " + p, nil },
		RenderBrief:    func() (string, error) { return "brief", nil },
	})
}

func TestGate_NotLiveHoldsWithoutSpendingPrompts(t *testing.T) {
	s, c := newStore(t)
	live := false
	gate, _ := newLiveGates(t, s, func() (bool, error) { return live, nil })
	wantHold(t, mustEval(t, gate))
	r := latest(t, s)
	if r.Request == nil || r.Delivery.Prompts != 0 {
		t.Fatalf("round = %+v, want an open request with no prompt", r)
	}
	for i := 0; i < 5; i++ {
		c.t = c.t.Add(time.Minute)
		wantHold(t, mustEval(t, gate))
	}
	if got := latest(t, s).Delivery.Prompts; got != 0 {
		t.Fatalf("prompts = %d, want 0", got)
	}
	live = true
	wantCarry(t, mustEval(t, gate))
	if got := latest(t, s).Delivery.Prompts; got != 1 {
		t.Fatalf("prompts = %d, want 1", got)
	}
}

func TestGate_NilLivenessSeamPromptsAtOnce(t *testing.T) {
	s, _ := newStore(t)
	gate, _ := newLiveGates(t, s, nil)
	wantCarry(t, mustEval(t, gate))
}

func TestGate_LivenessErrorHoldsAndWarnsOncePerError(t *testing.T) {
	s, c := newStore(t)
	var buf strings.Builder
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	gate, _ := newLiveGates(t, s, func() (bool, error) { return false, errors.New("reed unreachable") })
	for i := 0; i < 3; i++ {
		wantHold(t, mustEval(t, gate))
		c.t = c.t.Add(time.Minute)
	}
	if got := strings.Count(buf.String(), "reed unreachable"); got != 1 {
		t.Fatalf("error Warn count = %d, want 1; log = %q", got, buf.String())
	}
	if got := latest(t, s).Delivery.Prompts; got != 0 {
		t.Fatalf("prompts = %d, want 0", got)
	}
}

func TestGate_HeldRequestStillExpiresAtWaitBound(t *testing.T) {
	s, c := newStore(t)
	gate, _ := newLiveGates(t, s, func() (bool, error) { return false, nil })
	wantHold(t, mustEval(t, gate))
	c.t = c.t.Add(time.Hour)
	if !mustEval(t, gate).Passed {
		t.Fatal("a held request must still pass at the wait bound")
	}
	if latest(t, s).Request.State != StateExpired {
		t.Fatal("request not marked expired")
	}
}

func TestGate_HeldGateKeepsWaitingNotifyUntilLive(t *testing.T) {
	s, _ := newStore(t)
	live := true
	gate, _ := newLiveGates(t, s, func() (bool, error) { return live, nil })
	wantCarry(t, mustEval(t, gate))
	live = false
	if err := s.AddNotify(); err != nil {
		t.Fatal(err)
	}
	wantHold(t, mustEval(t, gate))
	if got := latest(t, s).Delivery.WaitingNotifys; got != 1 {
		t.Fatalf("waiting notifies = %d, want 1 while held", got)
	}
	live = true
	wantCarry(t, mustEval(t, gate))
	if got := latest(t, s).Delivery.WaitingNotifys; got != 0 {
		t.Fatalf("waiting notifies = %d, want 0 once carried", got)
	}
}

// TestGate_ResendAfterDelivery covers the delivery record's effect on the throttled re-prompt: a failed delivery does not stop resends, and a recorded delivery does.
func TestGate_ResendAfterDelivery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		failedReason string
		advance      time.Duration
		wantResend   bool
	}{
		{name: "failed delivery does not stop resends", failedReason: "no such agent", advance: time.Minute, wantResend: true},
		{name: "delivered stops resends", failedReason: "", advance: 10 * time.Minute, wantResend: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, c := newStore(t)
			gate, _ := newGates(t, s, "hub:orch")
			wantCarry(t, mustEval(t, gate))
			if err := s.RecordDelivered(tt.failedReason); err != nil {
				t.Fatal(err)
			}
			c.t = c.t.Add(tt.advance)
			if tt.wantResend {
				wantCarry(t, mustEval(t, gate))
			} else {
				wantHold(t, mustEval(t, gate))
			}
		})
	}
}

// TestApprovePasses covers an approve verdict on the opened round: the gate passes, and the final passes.
func TestApprovePasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		newGates func(t *testing.T, s Store) (gate, final shuttleengine.Gate)
		useFinal bool
	}{
		{name: "gate", newGates: func(t *testing.T, s Store) (shuttleengine.Gate, shuttleengine.Gate) {
			return newGates(t, s, "hub:orch")
		}},
		{name: "final", newGates: func(t *testing.T, s Store) (shuttleengine.Gate, shuttleengine.Gate) {
			return newGates(t, s, "hub:orch")
		}, useFinal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newStore(t)
			gate, final := tt.newGates(t, s)
			wantCarry(t, mustEval(t, gate))
			if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
				t.Fatal(err)
			}
			closure := gate
			if tt.useFinal {
				closure = final
			}
			if !mustEval(t, closure).Passed {
				t.Fatal("approve must pass")
			}
		})
	}
}

// TestSupersedingApprovePasses covers a superseding approve recorded over the cap's reject: both the gate, which first fails terminal at the cap, and the final pass.
func TestSupersedingApprovePasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		useFinal bool
	}{
		{name: "gate"},
		{name: "final", useFinal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newStore(t)
			gate, final := newGates(t, s, "hub:orch")
			rejectRounds(t, s, gate, testCap)
			closure := gate
			if tt.useFinal {
				closure = final
			} else {
				wantTerminal(t, mustEval(t, gate))
			}
			if err := s.SupersedeCapReject(); err != nil {
				t.Fatal(err)
			}
			if !mustEval(t, closure).Passed {
				t.Fatal("a superseding approve must pass")
			}
		})
	}
}

// newKeepAliveGate returns a gate with a one-minute KeepAlive whose renderer echoes the ping number; a nil live means the reviewer is always live.
func newKeepAliveGate(t *testing.T, s Store, live func() (bool, error)) shuttleengine.Gate {
	t.Helper()
	gate, _ := NewGate(GateConfig{
		Store:           s,
		Slug:            "x",
		Reviewer:        "hub:orch",
		DecisionRecord:  "d.md",
		SupportLog:      "s.md",
		WaitBound:       time.Hour,
		Cap:             testCap,
		KeepAlive:       time.Minute,
		ReviewerLive:    live,
		RenderDelivery:  func(p string) (string, error) { return "review " + p, nil },
		RenderBrief:     func() (string, error) { return "brief", nil },
		RenderKeepAlive: func(n int) (string, error) { return "ping " + strconv.Itoa(n), nil },
	})
	return gate
}

// wantPing asserts res carries keep-alive number n with a way forward that asks for no re-send.
func wantPing(t *testing.T, res shuttleengine.GateResult, n int) {
	t.Helper()
	if !res.Pending || res.Send != "ping "+strconv.Itoa(n) {
		t.Fatalf("want pending carrying ping %d, got %+v", n, res)
	}
	if strings.Contains(res.SendFailedWayForward, "lyx loom review notify") {
		t.Fatalf("a ping's way-forward = %q; want none that re-sends the notice", res.SendFailedWayForward)
	}
}

// deliveredKeepAliveGate opens a round, carries its prompt and records the delivery, so the gate is waiting on the review.
func deliveredKeepAliveGate(t *testing.T, live func() (bool, error)) (Store, *clock, shuttleengine.Gate) {
	t.Helper()
	s, c := newStore(t)
	gate := newKeepAliveGate(t, s, live)
	wantCarry(t, mustEval(t, gate))
	if err := s.RecordDelivered(""); err != nil {
		t.Fatal(err)
	}
	return s, c, gate
}

func TestGate_KeepAlive(t *testing.T) {
	t.Parallel()

	t.Run("pings once per interval, three per round, then stays quiet", func(t *testing.T) {
		t.Parallel()
		s, c, gate := deliveredKeepAliveGate(t, nil)
		before := latest(t, s).Delivery
		c.t = c.t.Add(30 * time.Second)
		wantHold(t, mustEval(t, gate))
		c.t = c.t.Add(30 * time.Second)
		for n := 1; n <= maxKeepAlives; n++ {
			wantPing(t, mustEval(t, gate), n)
			wantHold(t, mustEval(t, gate))
			c.t = c.t.Add(time.Minute)
		}
		c.t = c.t.Add(10 * time.Minute)
		wantHold(t, mustEval(t, gate))
		after := latest(t, s).Delivery
		if after.Pings != maxKeepAlives {
			t.Errorf("pings = %d, want %d", after.Pings, maxKeepAlives)
		}
		if after.Prompts != before.Prompts || !after.LastPromptAt.Equal(before.LastPromptAt) || after.WaitingNotifys != before.WaitingNotifys {
			t.Errorf("delivery after pings = %+v, want prompts, last-prompt time and waiting notifies as before (%+v)", after, before)
		}
	})

	t.Run("a fresh gate over the same store resumes the count and clock", func(t *testing.T) {
		t.Parallel()
		s, c, gate := deliveredKeepAliveGate(t, nil)
		for n := 1; n <= maxKeepAlives; n++ {
			c.t = c.t.Add(time.Minute)
			wantPing(t, mustEval(t, gate), n)
		}
		c.t = c.t.Add(time.Minute)
		wantHold(t, mustEval(t, newKeepAliveGate(t, s, nil)))
	})

	t.Run("waits on the stored clock when restarted mid-interval", func(t *testing.T) {
		t.Parallel()
		s, c, gate := deliveredKeepAliveGate(t, nil)
		c.t = c.t.Add(time.Minute)
		wantPing(t, mustEval(t, gate), 1)
		c.t = c.t.Add(30 * time.Second)
		restarted := newKeepAliveGate(t, s, nil)
		wantHold(t, mustEval(t, restarted))
		c.t = c.t.Add(30 * time.Second)
		wantPing(t, mustEval(t, restarted), 2)
	})

	t.Run("a waiting notify carries the delivery prompt ahead of a due ping", func(t *testing.T) {
		t.Parallel()
		s, c, gate := deliveredKeepAliveGate(t, nil)
		if err := s.AddNotify(); err != nil {
			t.Fatal(err)
		}
		c.t = c.t.Add(time.Minute)
		wantCarry(t, mustEval(t, gate))
		if got := latest(t, s).Delivery.Pings; got != 0 {
			t.Errorf("pings = %d after a notify prompt, want 0", got)
		}
	})

	t.Run("no ping while delivery is unrecorded", func(t *testing.T) {
		t.Parallel()
		s, c := newStore(t)
		gate := newKeepAliveGate(t, s, nil)
		wantCarry(t, mustEval(t, gate))
		c.t = c.t.Add(time.Minute)
		if res := mustEval(t, gate); strings.HasPrefix(res.Send, "ping") {
			t.Fatalf("got %+v before delivery was recorded; want no ping", res)
		}
		if got := latest(t, s).Delivery.Pings; got != 0 {
			t.Errorf("pings = %d, want 0", got)
		}
	})

	t.Run("no ping while the reviewer is not live", func(t *testing.T) {
		t.Parallel()
		live := true
		s, c, gate := deliveredKeepAliveGate(t, func() (bool, error) { return live, nil })
		live = false
		c.t = c.t.Add(time.Minute)
		wantHold(t, mustEval(t, gate))
		if got := latest(t, s).Delivery.Pings; got != 0 {
			t.Errorf("pings = %d, want 0", got)
		}
	})

	t.Run("no ping after a verdict", func(t *testing.T) {
		t.Parallel()
		s, c, gate := deliveredKeepAliveGate(t, nil)
		if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix this")); err != nil {
			t.Fatal(err)
		}
		c.t = c.t.Add(time.Minute)
		if res := mustEval(t, gate); strings.HasPrefix(res.Send, "ping") {
			t.Fatalf("got %+v after a verdict; want no ping", res)
		}
		if got := latest(t, s).Delivery.Pings; got != 0 {
			t.Errorf("pings = %d, want 0", got)
		}
	})

	t.Run("no ping after expiry", func(t *testing.T) {
		t.Parallel()
		s, c, gate := deliveredKeepAliveGate(t, nil)
		c.t = c.t.Add(2 * time.Hour)
		if !mustEval(t, gate).Passed {
			t.Fatal("an expired request must pass")
		}
		if got := latest(t, s).Delivery.Pings; got != 0 {
			t.Errorf("pings = %d, want 0", got)
		}
	})

	t.Run("a zero KeepAlive never pings", func(t *testing.T) {
		t.Parallel()
		s, c := newStore(t)
		gate, _ := newGates(t, s, "hub:orch")
		wantCarry(t, mustEval(t, gate))
		if err := s.RecordDelivered(""); err != nil {
			t.Fatal(err)
		}
		c.t = c.t.Add(30 * time.Minute)
		wantHold(t, mustEval(t, gate))
	})
}

// snapshotTree records every directory and file under each root, files with their content.
func snapshotTree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				snap[path] = "dir"
				return nil
			}
			data, err := os.ReadFile(path)
			snap[path] = string(data)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return snap
}

func TestExpiryProbe(t *testing.T) {
	tests := []struct {
		name     string
		reviewer string
		setup    func(t *testing.T, s Store, c *clock)
		want     bool
	}{
		{name: "no reviewer", setup: func(t *testing.T, s Store, c *clock) { openOne(t, s); c.t = c.t.Add(2 * time.Hour) }},
		{name: "no round", reviewer: "hub:orch", setup: func(*testing.T, Store, *clock) {}},
		{name: "open request before the bound", reviewer: "hub:orch", setup: func(t *testing.T, s Store, c *clock) {
			openOne(t, s)
			c.t = c.t.Add(59 * time.Minute)
		}},
		{name: "approve past the bound", reviewer: "hub:orch", setup: func(t *testing.T, s Store, c *clock) {
			openOne(t, s)
			if err := s.RecordVerdict(VerdictApprove, ""); err != nil {
				t.Fatal(err)
			}
			c.t = c.t.Add(2 * time.Hour)
		}},
		{name: "reject past the bound", reviewer: "hub:orch", setup: func(t *testing.T, s Store, c *clock) {
			openOne(t, s)
			if err := s.RecordVerdict(VerdictReject, writeFile(t, "fix it")); err != nil {
				t.Fatal(err)
			}
			c.t = c.t.Add(2 * time.Hour)
		}},
		{name: "expired request", reviewer: "hub:orch", setup: func(t *testing.T, s Store, c *clock) {
			openOne(t, s)
			if err := s.MarkExpired(); err != nil {
				t.Fatal(err)
			}
			c.t = c.t.Add(2 * time.Hour)
		}},
		{name: "superseded request", reviewer: "hub:orch", setup: func(t *testing.T, s Store, c *clock) {
			r := openOne(t, s)
			if err := s.setRequestState(r.Number, StateSuperseded); err != nil {
				t.Fatal(err)
			}
			c.t = c.t.Add(2 * time.Hour)
		}},
		{name: "open request with no verdict at the bound", reviewer: "hub:orch", want: true, setup: func(t *testing.T, s Store, c *clock) {
			openOne(t, s)
			c.t = c.t.Add(time.Hour)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, c := newStore(t)
			tt.setup(t, s, c)
			before := snapshotTree(t, s.Root, s.LockDir)

			expired, err := NewExpiryProbe(GateConfig{Store: s, Reviewer: tt.reviewer, WaitBound: time.Hour})()
			if err != nil {
				t.Fatal(err)
			}
			if expired != tt.want {
				t.Errorf("expired = %v, want %v", expired, tt.want)
			}
			after := snapshotTree(t, s.Root, s.LockDir)
			if !reflect.DeepEqual(before, after) {
				t.Errorf("probe changed the exchange's files:\nbefore %v\nafter  %v", before, after)
			}
			for path := range after {
				if filepath.Base(path) == deliveryFile {
					t.Errorf("probe left %s behind", path)
				}
			}
		})
	}
}
