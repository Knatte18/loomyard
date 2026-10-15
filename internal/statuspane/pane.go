// pane.go is the redraw-in-place pane loop the status panes share and the terminal-size and binary-check seams it takes.

package statuspane

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Knatte18/loomyard/internal/selfexec"
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

// Frame is one poll's result.
type Frame struct {
	// Lines holds every content line.
	Lines []string
}

// Pane is a status pane's output and the seams it reads, so a test drives it without a terminal or a clock.
type Pane struct {
	// Out receives the redraws.
	Out io.Writer
	// Size reads the terminal's columns and rows.
	Size func() (cols, rows int, err error)
	// Check runs once per poll, before the frame is read; nil means a selfexec.Watcher built when Run starts.
	Check func()
	// After returns a channel that fires once the wait between polls has passed; nil waits with time.After.
	After func(time.Duration) <-chan time.Time
	// Resized receives a value per terminal resize and ends the wait between polls at once; nil means no resize signal.
	Resized <-chan struct{}
}

// Run polls until ctx ends and redraws the pane in place whenever its text or size changes.
// Every line of the frame is drawn uncut: a frame taller than the terminal scrolls its first lines into the history, and a line wider than the terminal wraps.
// A size change alone redraws, so a wrapped frame is redrawn for the new width.
// Each poll calls Check before reading the frame, on the poll's own wait and adding no wake of its own.
// The wait between polls starts at one second, doubles while a poll changes nothing up to an eight-second ceiling, and returns to one second on a redraw.
// A value on Resized ends the wait at once, so the next poll redraws for the new size without waiting out the back-off.
// It reads and writes nothing but its seams.
func (p Pane) Run(ctx context.Context, poll func() Frame) {
	after := p.After
	if after == nil {
		after = time.After
	}
	check := p.Check
	if check == nil {
		check = selfexec.New().Check
	}

	fmt.Fprint(p.Out, hideCursor)

	var (
		drawn    bool
		lastText string
		lastCols int
		lastRows int
		wait     = initialWait
	)
	for ctx.Err() == nil {
		check()
		frame := poll()

		cols, rows, err := p.Size()
		if err != nil {
			cols, rows = 0, 0
		}

		text := strings.Join(frame.Lines, "\n")
		if drawn && text == lastText && cols == lastCols && rows == lastRows {
			wait = min(wait*2, maxWait)
		} else {
			p.redraw(frame.Lines)
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
