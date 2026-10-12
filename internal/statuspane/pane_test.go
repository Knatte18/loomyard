// pane_test.go pins the pane loop's redraw, fit, cut, report and back-off behavior against fake size and sleep seams, ReportFromEnv's gating, and FormatMinutes' format boundaries.

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
}

// rendered is the bytes one redraw of lines writes.
func rendered(lines ...string) string {
	var b strings.Builder
	b.WriteString(cursorHome)
	for i, line := range lines {
		b.WriteString(line + eraseToLineEnd)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString(eraseBelowCursor)
	return b.String()
}

func frameOf(lines ...string) Frame { return Frame{Lines: lines} }

func TestPane_RunAndReportFromEnv(t *testing.T) {
	t.Parallel()

	const cols, rows = 80, 24
	same := func(n int, frame Frame) []paneStep {
		steps := make([]paneStep, n)
		for i := range steps {
			steps[i] = paneStep{frame: frame, cols: cols, rows: rows}
		}
		return steps
	}
	fitTo := func(rows int) []string { return []string{"l1", "l2", "+3 not shown"}[:rows] }

	tests := []struct {
		name        string
		steps       []paneStep
		failReports int
		noReport    bool
		wantRedraws int
		wantWaits   []time.Duration
		wantReports []int
		wantLast    string
	}{
		{
			name:        "UnchangedFrameRedrawsOnceAndTheWaitsDoubleToTheCeiling",
			steps:       same(6, frameOf("a", "b")),
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second},
			wantReports: []int{2},
			wantLast:    rendered("a", "b"),
		},
		{
			name:        "ChangedFrameRedrawsAndTheWaitReturnsToOneSecond",
			steps:       append(same(2, frameOf("a", "b")), same(1, frameOf("a", "b", "c"))...),
			wantRedraws: 2,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second, time.Second},
			wantReports: []int{2, 3},
			wantLast:    rendered("a", "b", "c"),
		},
		{
			name:        "FrameTallerThanTheTermRowsPrintsFitRowsAndNeverMore",
			steps:       []paneStep{{frame: Frame{Lines: []string{"l1", "l2", "l3", "l4", "l5"}, Fit: fitTo}, cols: cols, rows: 3}},
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second},
			wantReports: []int{5},
			wantLast:    rendered("l1", "l2", "+3 not shown"),
		},
		{
			name: "SizeChangeAloneRedraws",
			steps: []paneStep{
				{frame: frameOf("a"), cols: cols, rows: rows},
				{frame: frameOf("a"), cols: cols, rows: rows + 1},
			},
			wantRedraws: 2,
			wantWaits:   []time.Duration{time.Second, time.Second},
			wantReports: []int{1},
			wantLast:    rendered("a"),
		},
		{
			name:        "LineWiderThanTheColumnsIsCut",
			steps:       []paneStep{{frame: frameOf("abcdefghij"), cols: 4, rows: rows}},
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second},
			wantReports: []int{1},
			wantLast:    rendered("abcd"),
		},
		{
			name:        "FailedSizeKeepsEveryLineUncut",
			steps:       []paneStep{{frame: Frame{Lines: []string{"l1", "l2", "l3", "l4", "l5"}, Fit: fitTo}, sizeErr: true}},
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second},
			wantReports: []int{5},
			wantLast:    rendered("l1", "l2", "l3", "l4", "l5"),
		},
		{
			name:        "FailedReportIsRetriedAtTheNextPollOnly",
			steps:       same(3, frameOf("a", "b")),
			failReports: 1,
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second, 4 * time.Second},
			wantReports: []int{2, 2},
			wantLast:    rendered("a", "b"),
		},
		{
			name:        "NilReportReportsNothing",
			steps:       same(2, frameOf("a")),
			noReport:    true,
			wantRedraws: 1,
			wantWaits:   []time.Duration{time.Second, 2 * time.Second},
			wantLast:    rendered("a"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var (
				out     bytes.Buffer
				polls   int
				waits   []time.Duration
				reports []int
			)
			pane := Pane{
				Out: &out,
				Size: func() (int, int, error) {
					step := tt.steps[polls-1]
					if step.sizeErr {
						return 0, 0, errors.New("no terminal")
					}
					return step.cols, step.rows, nil
				},
				Sleep: func(d time.Duration) {
					waits = append(waits, d)
					if polls == len(tt.steps) {
						cancel()
					}
				},
			}
			if !tt.noReport {
				pane.Report = func(rows int) error {
					reports = append(reports, rows)
					if len(reports) <= tt.failReports {
						return errors.New("pane not recorded yet")
					}
					return nil
				}
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
			if !slices.Equal(reports, tt.wantReports) {
				t.Errorf("reports = %v, want %v", reports, tt.wantReports)
			}
		})
	}

	t.Run("ReportFromEnvCallsSetWithTheStrandAndPaneOnlyWhenBothAreSet", func(t *testing.T) {
		t.Parallel()

		type call struct {
			name, paneID string
			rows         int
		}
		tests := []struct {
			name string
			env  map[string]string
			want *call
		}{
			{"BothSet", map[string]string{"LYX_STRAND_NAME": "ly:slug:runs", "TMUX_PANE": "%7"}, &call{"ly:slug:runs", "%7", 3}},
			{"NoStrandName", map[string]string{"TMUX_PANE": "%7"}, nil},
			{"NoPane", map[string]string{"LYX_STRAND_NAME": "ly:slug:runs"}, nil},
		}
		for _, tt := range tests {
			var got *call
			report := ReportFromEnv(func(key string) string { return tt.env[key] }, func(name, paneID string, rows int) error {
				got = &call{name, paneID, rows}
				return nil
			})
			if tt.want == nil {
				if report != nil {
					t.Errorf("%s: ReportFromEnv returned a func, want nil", tt.name)
				}
				continue
			}
			if report == nil {
				t.Fatalf("%s: ReportFromEnv returned nil, want a func", tt.name)
			}
			if err := report(3); err != nil {
				t.Fatalf("%s: report: %v", tt.name, err)
			}
			if *got != *tt.want {
				t.Errorf("%s: set called with %+v, want %+v", tt.name, *got, *tt.want)
			}
		}
	})
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
