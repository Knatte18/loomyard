// poll_test.go covers PollUntilTerminal's long-poll loop against a fake
// clock: a terminal mid-wait result short-circuits, a deadline returns the
// last running digest, and a gather error propagates. This file lives in
// package websterengine (not websterengine_test) because the clock/
// realClock seam is unexported. Tier 1: no git, fake clock only.

package websterengine

import (
	"errors"
	"testing"
	"time"
)

// fakeClock is a package-local, scriptable clock double for
// PollUntilTerminal: Now starts at a fixed base and only advances when
// Sleep is called, so a test controls exactly how many ticks elapse before
// a fixed wait budget is exceeded, without ever blocking for real.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(d time.Duration) {
	c.now = c.now.Add(d)
}

var _ clock = (*fakeClock)(nil)

func TestPollUntilTerminal_TerminalMidWaitReturnsEarly(t *testing.T) {
	t.Parallel()

	calls := 0
	gather := func() (Digest, bool, error) {
		calls++
		if calls < 3 {
			return Digest{Batch: "01-x", Status: DigestStatusRunning}, false, nil
		}
		return Digest{Batch: "01-x", Status: DigestStatusDone}, true, nil
	}

	digest, err := PollUntilTerminal(gather, time.Hour, &fakeClock{now: time.Unix(0, 0)})
	if err != nil {
		t.Fatalf("PollUntilTerminal() error = %v; want nil", err)
	}
	if digest.Status != DigestStatusDone {
		t.Errorf("PollUntilTerminal().Status = %q; want %q", digest.Status, DigestStatusDone)
	}
	if calls != 3 {
		t.Errorf("gather called %d times; want exactly 3 (short-circuit on terminal)", calls)
	}
}

func TestPollUntilTerminal_DeadlineReturnsRunning(t *testing.T) {
	t.Parallel()

	running := Digest{Batch: "01-x", Status: DigestStatusRunning, ElapsedS: 99}
	gather := func() (Digest, bool, error) {
		return running, false, nil
	}

	digest, err := PollUntilTerminal(gather, 3*time.Second, &fakeClock{now: time.Unix(0, 0)})
	if err != nil {
		t.Fatalf("PollUntilTerminal() error = %v; want nil", err)
	}
	if digest.Status != DigestStatusRunning {
		t.Errorf("PollUntilTerminal().Status = %q; want %q", digest.Status, DigestStatusRunning)
	}
	if digest.ElapsedS != 99 {
		t.Errorf("PollUntilTerminal().ElapsedS = %d; want 99", digest.ElapsedS)
	}
}

func TestPollUntilTerminal_GatherErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("gather failed")
	gather := func() (Digest, bool, error) {
		return Digest{}, false, wantErr
	}

	_, err := PollUntilTerminal(gather, time.Hour, &fakeClock{now: time.Unix(0, 0)})
	if err == nil {
		t.Fatalf("PollUntilTerminal() error = nil; want a propagated error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("PollUntilTerminal() error = %v; want it to wrap %v", err, wantErr)
	}
}
