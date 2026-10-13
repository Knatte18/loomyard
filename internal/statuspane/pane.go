// pane.go is the redraw-in-place pane loop the status panes share, the terminal-size and report seams it takes, and the panes' minute-resolution time format.

package statuspane

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// initialWait is the wait between polls after a change, and the floor of the back-off.
	initialWait = time.Second
	// maxWait is the ceiling the wait doubles up to while polls change nothing.
	maxWait = 8 * time.Second
)

const (
	hideCursor   = "\x1b[?25l"
	cursorHome   = "\x1b[H"
	eraseScreen  = "\x1b[2J"
	eraseHistory = "\x1b[3J"
)

// tmuxPaneEnv is the environment variable tmux exports to a pane's process with the pane's id.
const tmuxPaneEnv = "TMUX_PANE"

// Frame is one poll's result.
type Frame struct {
	// Lines holds every content line.
	Lines []string
	// Fit renders Lines into at most rows rows, its last row a summary line naming how many are not shown.
	// A nil Fit cuts Lines to the first rows.
	Fit func(rows int) []string
}

// Pane is a status pane's output and the seams it reads, so a test drives it without a terminal or a clock.
type Pane struct {
	// Out receives the redraws.
	Out io.Writer
	// Size reads the terminal's columns and rows.
	Size func() (cols, rows int, err error)
	// Report tells reed the pane's content line count; nil reports nothing.
	Report func(rows int) error
	// After returns a channel that fires once the wait between polls has passed; nil waits with time.After.
	After func(time.Duration) <-chan time.Time
	// Resized receives a value per terminal resize and ends the wait between polls at once; nil means no resize signal.
	Resized <-chan struct{}
}

// Run polls until ctx ends and redraws the pane in place whenever its text or size changes.
// Each poll it reports the line count to Report when it differs from the count last reported successfully; a failed report is logged at Debug and retried at the next poll.
// A frame taller than the terminal prints Fit's rows, a failed Size keeps every line, and each line is cut to the terminal's columns so none wraps.
// The wait between polls starts at one second, doubles while a poll changes nothing up to an eight-second ceiling, and returns to one second on a redraw.
// A value on Resized ends the wait at once, so the next poll redraws for the new size without waiting out the back-off.
// It reads and writes nothing but its seams.
func (p Pane) Run(ctx context.Context, poll func() Frame) {
	after := p.After
	if after == nil {
		after = time.After
	}

	fmt.Fprint(p.Out, hideCursor)

	var (
		reported = -1
		drawn    bool
		lastText string
		lastCols int
		lastRows int
		wait     = initialWait
	)
	for ctx.Err() == nil {
		frame := poll()

		if p.Report != nil && len(frame.Lines) != reported {
			if err := p.Report(len(frame.Lines)); err != nil {
				logger.Debug("statuspane: reporting the line count failed, retrying at the next poll", "rows", len(frame.Lines), "err", err)
			} else {
				reported = len(frame.Lines)
			}
		}

		cols, rows, err := p.Size()
		if err != nil {
			cols, rows = 0, 0
		}
		lines := fitLines(frame, rows, cols)

		text := strings.Join(lines, "\n")
		if drawn && text == lastText && cols == lastCols && rows == lastRows {
			wait = min(wait*2, maxWait)
		} else {
			p.redraw(lines)
			drawn, lastText, lastCols, lastRows = true, text, cols, rows
			wait = initialWait
		}
		select {
		case <-ctx.Done():
		case <-after(wait):
		case <-p.Resized:
		}
	}
}

// fitLines returns the lines to print for a terminal of rows by cols; a non-positive rows or cols means that dimension is unknown and unbounded.
func fitLines(frame Frame, rows, cols int) []string {
	lines := frame.Lines
	if rows > 0 && len(lines) > rows {
		if frame.Fit != nil {
			lines = frame.Fit(rows)
		}
		lines = lines[:min(len(lines), rows)]
	}
	if cols <= 0 {
		return lines
	}
	cut := make([]string, len(lines))
	for i, line := range lines {
		if runes := []rune(line); len(runes) > cols {
			line = string(runes[:cols])
		}
		cut[i] = line
	}
	return cut
}

// redraw writes lines over the pane from its home position after erasing the screen and the scrollback, so no line of an earlier, longer or wider frame survives on screen or in the history.
// A newline separates lines but does not follow the last.
func (p Pane) redraw(lines []string) {
	fmt.Fprint(p.Out, cursorHome+eraseScreen+eraseHistory+strings.Join(lines, "\n"))
}

// TerminalSize returns a size seam reading the columns and rows of the terminal on f.
func TerminalSize(f *os.File) func() (cols, rows int, err error) {
	return func() (int, int, error) {
		return term.GetSize(int(f.Fd()))
	}
}

// ReportFromEnv returns the report func that tells set the strand name and pane id getenv gives for agentname.StrandNameEnv and TMUX_PANE.
// It returns nil when either is empty, so a pane run by hand outside a strand reports nothing.
func ReportFromEnv(getenv func(string) string, set func(name, paneID string, rows int) error) func(rows int) error {
	name := getenv(agentname.StrandNameEnv)
	paneID := getenv(tmuxPaneEnv)
	if name == "" || paneID == "" {
		return nil
	}
	return func(rows int) error {
		return set(name, paneID, rows)
	}
}

// FormatMinutes renders d in whole minutes as `12m`, `1h05m` or `2d03h`, so a pane line changes at most once a minute.
func FormatMinutes(d time.Duration) string {
	minutes := max(int(d/time.Minute), 0)
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes < 24*60:
		return fmt.Sprintf("%dh%02dm", minutes/60, minutes%60)
	default:
		return fmt.Sprintf("%dd%02dh", minutes/(24*60), minutes/60%24)
	}
}
