// poll_test.go covers PollUntilTerminal's long-poll loop against a fake clock: a terminal mid-wait
// result short-circuits, a deadline returns the last running digest, and a gather error propagates.
// This file lives in package websterengine (not websterengine_test) because the clock/ realClock
// seam is unexported.
// Tier 1: no git, fake clock only.

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

func TestPollUntilTerminal(t *testing.T) {
	t.Parallel()

	gatherErr := errors.New("gather failed")
	tests := []struct {
		name string
		wait time.Duration
		// gather is built per row so call counting stays row-local; calls reports how often it ran.
		gather       func(calls *int) (Digest, bool, error)
		wantStatus   string
		wantElapsedS int
		wantCalls    int
		wantErr      error
	}{
		{
			name: "a terminal result mid-wait returns early",
			wait: time.Hour,
			gather: func(calls *int) (Digest, bool, error) {
				*calls++
				if *calls < 3 {
					return Digest{Batch: "01-x", Status: DigestStatusRunning}, false, nil
				}
				return Digest{Batch: "01-x", Status: DigestStatusDone}, true, nil
			},
			wantStatus: DigestStatusDone,
			// Exactly three gathers: short-circuit on the terminal one.
			wantCalls: 3,
		},
		{
			name: "the deadline returns the last running digest",
			wait: 3 * time.Second,
			gather: func(*int) (Digest, bool, error) {
				return Digest{Batch: "01-x", Status: DigestStatusRunning, ElapsedS: 99}, false, nil
			},
			wantStatus:   DigestStatusRunning,
			wantElapsedS: 99,
		},
		{
			name: "a gather error propagates",
			wait: time.Hour,
			gather: func(*int) (Digest, bool, error) {
				return Digest{}, false, gatherErr
			},
			wantErr: gatherErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			digest, err := PollUntilTerminal(func() (Digest, bool, error) { return tt.gather(&calls) }, tt.wait, &fakeClock{now: time.Unix(0, 0)})

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("PollUntilTerminal() error = %v; want it to wrap %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PollUntilTerminal() error = %v; want nil", err)
			}
			if digest.Status != tt.wantStatus {
				t.Errorf("PollUntilTerminal().Status = %q; want %q", digest.Status, tt.wantStatus)
			}
			if digest.ElapsedS != tt.wantElapsedS {
				t.Errorf("PollUntilTerminal().ElapsedS = %d; want %d", digest.ElapsedS, tt.wantElapsedS)
			}
			if tt.wantCalls != 0 && calls != tt.wantCalls {
				t.Errorf("gather called %d times; want exactly %d", calls, tt.wantCalls)
			}
		})
	}
}
