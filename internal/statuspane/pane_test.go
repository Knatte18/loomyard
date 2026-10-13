// pane_test.go pins the pane loop's redraw, uncut drawing, binary check, back-off and resize behavior against fake size and wait seams, and FormatMinutes' format boundaries.

package statuspane

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// paneStep is one scripted poll: the frame it yields and the terminal size the loop then reads.
type paneStep struct {
	frame   Frame
	cols    int
	rows    int
	sizeErr bool
	// resize makes the wait after this step's poll end only through Resized.
	resize bool
}

// rendered is the bytes one redraw of lines writes: cursor home, erase screen, erase history, then the lines.
func rendered(lines ...string) string {
	return "\x1b[H\x1b[2J\x1b[3J" + strings.Join(lines, "\n")
}

func frameOf(lines ...string) Frame { return Frame{Lines: lines} }

func TestPane_Run(t *testing.T) {
	t.Parallel()

	const cols, rows = 80, 24
	same := func(n int, frame Frame) []paneStep {
		steps := make([]paneStep, n)
		for i := range steps {
			steps[i] = paneStep{frame: frame, cols: cols, rows: rows}
		}
		return steps
	}

	tests := []struct {
		name        string
		steps       []paneStep
		wantRedraws int
		wantWaits   []time.Duration
		// wantChecks is the number of Check calls: one per poll.
		wantChecks int
		wantLast   string
	}{
		{
			name:        "UnchangedFrameRedrawsOnceAndTheWaitsDoubleToTheCeiling",
			steps:       same(6, frameOf("a", "b")),
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second},
			wantChecks:  6,
			wantLast:    rendered("a", "b"),
		},
		{
			name:        "ChangedFrameRedrawsAndTheWaitReturnsToOneSecond",
			steps:       append(same(2, frameOf("a", "b")), same(1, frameOf("a", "b", "c"))...),
			wantRedraws: 2,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second, time.Second},
			wantChecks:  3,
			wantLast:    rendered("a", "b", "c"),
		},
		{
			name:        "FrameTallerThanTheTermRowsIsDrawnWhole",
			steps:       []paneStep{{frame: frameOf("l1", "l2", "l3", "l4", "l5"), cols: cols, rows: 3}},
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second},
			wantChecks:  1,
			wantLast:    rendered("l1", "l2", "l3", "l4", "l5"),
		},
		{
			name: "SizeChangeAloneRedraws",
			steps: []paneStep{
				{frame: frameOf("a"), cols: cols, rows: rows},
				{frame: frameOf("a"), cols: cols, rows: rows + 1},
			},
			wantRedraws: 2,
			wantWaits:   []time.Duration{time.Second, time.Second},
			wantChecks:  2,
			wantLast:    rendered("a"),
		},
		{
			name: "ResizeRedrawsWithoutWaitingOutTheBackOff",
			steps: append(same(3, frameOf("a", "b")),
				paneStep{frame: frameOf("a", "b"), cols: cols, rows: rows, resize: true},
				paneStep{frame: frameOf("a", "b"), cols: cols + 20, rows: rows},
			),
			wantRedraws: 2,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, time.Second},
			wantChecks:  5,
			wantLast:    rendered("a", "b"),
		},
		{
			name:        "LineWiderThanTheColumnsIsDrawnUncut",
			steps:       []paneStep{{frame: frameOf("abcdefghij"), cols: 4, rows: rows}},
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second},
			wantChecks:  1,
			wantLast:    rendered("abcdefghij"),
		},
		{
			name:        "FailedSizeKeepsEveryLineUncut",
			steps:       []paneStep{{frame: frameOf("l1", "l2", "l3", "l4", "l5"), sizeErr: true}},
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second},
			wantChecks:  1,
			wantLast:    rendered("l1", "l2", "l3", "l4", "l5"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The timeout bounds a loop that ignores Resized, which fails on its redraw count instead of hanging the test binary.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var (
				out    bytes.Buffer
				polls  int
				checks int
				waits  []time.Duration
			)
			resized := make(chan struct{}, 1)
			pane := Pane{
				Out: &out,
				Size: func() (int, int, error) {
					step := tt.steps[polls-1]
					if step.sizeErr {
						return 0, 0, errors.New("no terminal")
					}
					return step.cols, step.rows, nil
				},
				Check: func() {
					checks++
					if checks != polls+1 {
						t.Errorf("Check call %d ran with %d polls done, want it before poll %d", checks, polls, checks)
					}
				},
				After: func(d time.Duration) <-chan time.Time {
					waits = append(waits, d)
					if polls == len(tt.steps) {
						cancel()
					}
					if tt.steps[polls-1].resize {
						resized <- struct{}{}
						return make(chan time.Time)
					}
					fired := make(chan time.Time, 1)
					fired <- time.Time{}
					return fired
				},
				Resized: resized,
			}

			pane.Run(ctx, func() Frame {
				polls++
				return tt.steps[polls-1].frame
			})

			got := out.String()
			if !strings.HasPrefix(got, hideCursor) || strings.Count(got, hideCursor) != 1 {
				t.Errorf("output %q, want the cursor hidden exactly once at the start", got)
			}
			if redraws := strings.Count(got, cursorHome); redraws != tt.wantRedraws {
				t.Errorf("redraws = %d, want %d", redraws, tt.wantRedraws)
			}
			if last := got[strings.LastIndex(got, cursorHome):]; last != tt.wantLast {
				t.Errorf("last redraw = %q, want %q", last, tt.wantLast)
			}
			if !slices.Equal(waits, tt.wantWaits) {
				t.Errorf("waits = %v, want %v", waits, tt.wantWaits)
			}
			if checks != tt.wantChecks {
				t.Errorf("Check calls = %d, want %d", checks, tt.wantChecks)
			}
		})
	}
}

func TestFormatMinutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "0m"},
		{12 * time.Minute, "12m"},
		{59 * time.Minute, "59m"},
		{60 * time.Minute, "1h00m"},
		{65 * time.Minute, "1h05m"},
		{23*time.Hour + 59*time.Minute, "23h59m"},
		{24 * time.Hour, "1d00h"},
		{51 * time.Hour, "2d03h"},
	}
	for _, tt := range tests {
		if got := FormatMinutes(tt.d); got != tt.want {
			t.Errorf("FormatMinutes(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
