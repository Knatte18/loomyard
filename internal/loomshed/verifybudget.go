// verifybudget.go holds the time budget of a verify gate and the clock that notices the run's parent once it is spent.

package loomshed

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyNoticeBytes caps the notice a spent budget sends, so it stays one short creel line.
const verifyNoticeBytes = 1000

// VerifyBudget is how long a verify gate may run before the run's parent hears of it.
// Its zero value is off.
type VerifyBudget struct {
	// Limit is the wall-clock time from the gate's first arrival after which the notice goes out; 0 turns the budget off.
	Limit time.Duration
	// Notify sends one line to the run's parent; nil turns the budget off.
	Notify func(line string) error
	// now reads the clock; nil reads the real one.
	now func() time.Time
	// after runs f once d has passed and returns the function that cancels it; nil uses time.AfterFunc.
	after func(d time.Duration, f func()) (cancel func())
}

// budgetClock counts one verify gate's time against its budget and sends the notice once.
// It is safe for concurrent use: its timer sends the notice while the gate's own call runs a verify command.
type budgetClock struct {
	budget VerifyBudget
	site   string

	mu      sync.Mutex
	start   time.Time
	attempt int
	// failures names each failed verify run so far, oldest first.
	failures []string
	noticed  bool
}

func (c *budgetClock) on() bool {
	return c.budget.Limit > 0 && c.budget.Notify != nil
}

func (c *budgetClock) now() time.Time {
	if c.budget.now != nil {
		return c.budget.now()
	}
	return time.Now()
}

// arrive starts the clock at the gate's first arrival and counts attempt as the current one.
// It sends the notice when the budget is already spent.
func (c *budgetClock) arrive(attempt int) {
	if !c.on() {
		return
	}
	c.mu.Lock()
	if c.start.IsZero() {
		c.start = c.now()
	}
	c.attempt = attempt
	c.mu.Unlock()
	c.noticeIfSpent()
}

// arm schedules the notice for the moment the budget runs out, and returns the function that cancels it.
// The gate arms it around each verify command and cancels it once the command returns,
// so no notice goes out after the gate stopped running.
func (c *budgetClock) arm() (disarm func()) {
	if !c.on() {
		return func() {}
	}
	c.mu.Lock()
	left := c.budget.Limit - c.now().Sub(c.start)
	c.mu.Unlock()
	if left <= 0 {
		return func() {}
	}
	if c.budget.after != nil {
		return c.budget.after(left, c.noticeIfSpent)
	}
	timer := time.AfterFunc(left, c.noticeIfSpent)
	return func() { timer.Stop() }
}

// failed keeps res, the failed verify run of attempt, among the failures the notice names.
// It names the runs the log's closing line names, or the exit code when the log holds no such line.
func (c *budgetClock) failed(attempt int, res verifytree.Result) {
	if !c.on() {
		return
	}
	runs := fmt.Sprintf("exit code %d", res.ExitCode)
	if log, err := os.ReadFile(res.Log); err == nil {
		if line := failedRunsLine(log); line != "" {
			runs = strings.TrimPrefix(line, gateslot.FailedRunsPrefix)
		}
	}
	c.mu.Lock()
	c.failures = append(c.failures, fmt.Sprintf("attempt %d, %s, log %s", attempt, runs, res.Log))
	c.mu.Unlock()
}

// noticeIfSpent sends the notice once the budget is spent, and only once.
// A notice that cannot be sent is logged and not retried.
func (c *budgetClock) noticeIfSpent() {
	c.mu.Lock()
	if c.noticed || c.now().Sub(c.start) < c.budget.Limit {
		c.mu.Unlock()
		return
	}
	c.noticed = true
	line := verifyBudgetNotice(c.site, c.budget.Limit, c.attempt, c.failures)
	c.mu.Unlock()
	if err := c.budget.Notify(line); err != nil {
		logger.Warn("loomshed: the verify gate's spent budget could not be noticed to the run's parent", "gate", c.site, "error", err)
	}
}

// verifyBudgetNotice renders the one line a spent budget sends.
// It names the gate, its budget, the current attempt and every failed run so far, the latest first.
// It is cut to verifyNoticeBytes, so the earliest failures are the ones a long list loses.
func verifyBudgetNotice(site string, limit time.Duration, attempt int, failures []string) string {
	failed := "no verify run has failed yet"
	if len(failures) > 0 {
		latestFirst := slices.Clone(failures)
		slices.Reverse(latestFirst)
		failed = "failed runs so far, latest first: " + strings.Join(latestFirst, "; ")
	}
	text := fmt.Sprintf("%s: the verify gate has run past its %s budget and is at attempt %d; it goes on until its attempts are spent; %s", site, limit, attempt, failed)
	line := strings.Join(strings.Fields(text), " ")
	if len(line) <= verifyNoticeBytes {
		return line
	}
	cut := verifyNoticeBytes
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "..."
}
