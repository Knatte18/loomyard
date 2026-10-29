// counters.go declares the counters a tier names: writers a tier command's output goes through, which pass it on as text and count its tests.

package gatecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

const (
	counterGoJSON = "go-json"
	counterVSTest = "vstest"
)

// vstestSummary matches a VSTest summary line, whose `Passed!` and `Failed!` forms carry the same four counts.
var vstestSummary = regexp.MustCompile(`(?:Passed|Failed)!\s+-\s+Failed:\s*(\d+),\s*Passed:\s*(\d+),\s*Skipped:\s*(\d+),\s*Total:\s*(\d+)`)

// tierCounts is the number of tests a tier run discovered and how each ended.
type tierCounts struct {
	discovered int
	passed     int
	failed     int
	skipped    int
}

// counter is the writer a tier command's stdout and stderr go through.
// It passes the output on as text and counts the tests it reports; the totals decide nothing about the run's outcome, while the per-package counts let a go-json tier fail a package that ran none of its tests.
type counter interface {
	io.Writer
	// Flush emits a trailing partial line that no newline ended.
	Flush() error
	// Counts returns the counts so far.
	Counts() tierCounts
	// TestsRun returns, per package the events named, how many tests ran in it, zero for a package that sent events but ran none; nil for a counter that does not read packages.
	TestsRun() map[string]int
	// Launches returns, per package whose test binary ran, how long the binary took to launch and how long the package took, in the order the packages started; nil for a counter that does not read packages.
	Launches() []gateslot.PackageLaunch
	// Failures returns the failed tests so far, leaves only, in the order they failed; nil for a counter that does not read tests.
	Failures() []gateslot.TestFailure
}

// newCounter returns the counter kind names, writing the tier's text output to out.
// Any kind but `go-json` and `vstest` is an error.
func newCounter(kind string, out io.Writer) (counter, error) {
	switch kind {
	case counterGoJSON:
		c := &goJSONCounter{}
		c.lines = lineReader{out: out, handle: c.handle}
		return c, nil
	case counterVSTest:
		c := &vstestCounter{}
		c.lines = lineReader{out: out, handle: c.handle}
		return c, nil
	}
	return nil, fmt.Errorf("gate counter %q is neither %q nor %q", kind, counterGoJSON, counterVSTest)
}

// lineReader splits written bytes into lines and hands each to handle, serialising concurrent writers.
// A line is handed over without its newline, and terminated says whether a newline ended it.
type lineReader struct {
	mu      sync.Mutex
	out     io.Writer
	pending []byte
	handle  func(line []byte, terminated bool) error
}

func (r *lineReader) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, p...)
	for {
		end := bytes.IndexByte(r.pending, '\n')
		if end < 0 {
			return len(p), nil
		}
		line := r.pending[:end]
		err := r.handle(line, true)
		r.pending = r.pending[end+1:]
		if err != nil {
			return len(p), err
		}
	}
}

func (r *lineReader) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		return nil
	}
	line := r.pending
	r.pending = nil
	return r.handle(line, false)
}

// passThrough writes line to out, with its newline when terminated.
func (r *lineReader) passThrough(line []byte, terminated bool) error {
	if terminated {
		line = append(line[:len(line):len(line)], '\n')
	}
	_, err := r.out.Write(line)
	return err
}

// goJSONCounter reads `go test -json` events and writes their output text, so failures, panics and build errors read as plain `go test` prints them.
// A line that is no JSON event, such as a `go build` or `go vet` step's output, passes through verbatim.
type goJSONCounter struct {
	lines  lineReader
	counts tierCounts
	// packages holds the launch timing of each package that sent a start event, and order the packages in the order they started.
	packages map[string]*packageTiming
	order    []string
	// testsRun holds, per package any event named, the number of tests that sent a run event.
	testsRun map[string]int
	// firstLines holds, per test in flight, the first line it printed beside go test's status lines; a test leaves it when it ends.
	firstLines map[testKey]string
	// failures holds every failed test, parents of failed subtests included, in the order they failed.
	failures []gateslot.TestFailure
}

// testKey names one test of one package.
type testKey struct {
	pkg, test string
}

// packageTiming is what the counter has seen of one package's test binary launching.
type packageTiming struct {
	// started is the time of the package's start event, valid when hasStart is set.
	started  time.Time
	hasStart bool
	// gapSeconds is the time from started to the package's next event, valid when hasGap is set.
	gapSeconds float64
	hasGap     bool
	// elapsedSeconds is the elapsed time of the package's closing pass or fail event, valid when closed is set.
	elapsedSeconds float64
	closed         bool
	// noBinary is set when the package's output reports a cached result or no test files, so no test binary ran.
	noBinary bool
}

// goTestEvent is the part of a `go test -json` event the counter reads.
type goTestEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
	// Time is the event's RFC 3339 timestamp, kept as text so a missing or odd one never turns the line into plain output.
	Time    string
	Elapsed float64
}

func (c *goJSONCounter) Write(p []byte) (int, error) { return c.lines.Write(p) }

func (c *goJSONCounter) Flush() error { return c.lines.Flush() }

func (c *goJSONCounter) Counts() tierCounts {
	c.lines.mu.Lock()
	defer c.lines.mu.Unlock()
	return c.counts
}

// TestsRun returns a copy of the per-package count of tests that sent a run event; a package whose events carry no test, such as one with only a `TestMain`, counts zero.
func (c *goJSONCounter) TestsRun() map[string]int {
	c.lines.mu.Lock()
	defer c.lines.mu.Unlock()
	return maps.Clone(c.testsRun)
}

// Launches returns the packages whose test binary ran, with the gap from the package's start event to its next event and the elapsed time of its closing event.
// A package that reported a cached result or no test files, or that never closed, is left out.
func (c *goJSONCounter) Launches() []gateslot.PackageLaunch {
	c.lines.mu.Lock()
	defer c.lines.mu.Unlock()
	var launches []gateslot.PackageLaunch
	for _, name := range c.order {
		timing := c.packages[name]
		if timing.noBinary || !timing.hasGap || !timing.closed {
			continue
		}
		launches = append(launches, gateslot.PackageLaunch{Package: name, LaunchSeconds: timing.gapSeconds, ElapsedSeconds: timing.elapsedSeconds})
	}
	return launches
}

// Failures returns the failed tests, leaving out a test with a failed subtest in the same package, since its own failure carries no line of its own.
func (c *goJSONCounter) Failures() []gateslot.TestFailure {
	c.lines.mu.Lock()
	defer c.lines.mu.Unlock()
	var leaves []gateslot.TestFailure
	for _, failure := range c.failures {
		parent := slices.ContainsFunc(c.failures, func(other gateslot.TestFailure) bool {
			return other.Package == failure.Package && strings.HasPrefix(other.Test, failure.Test+"/")
		})
		if !parent {
			leaves = append(leaves, failure)
		}
	}
	return leaves
}

// observeTest folds a test's event into its first line and, on its fail event, into the failures.
func (c *goJSONCounter) observeTest(event goTestEvent) {
	key := testKey{event.Package, event.Test}
	switch event.Action {
	case "output":
		line := strings.TrimSpace(event.Output)
		if _, seen := c.firstLines[key]; seen || line == "" || strings.HasPrefix(line, "=== ") || strings.HasPrefix(line, "--- ") {
			return
		}
		if c.firstLines == nil {
			c.firstLines = map[testKey]string{}
		}
		c.firstLines[key] = line
	case "fail":
		c.failures = append(c.failures, gateslot.TestFailure{Package: event.Package, Test: event.Test, FirstLine: c.firstLines[key]})
		delete(c.firstLines, key)
	case "pass", "skip":
		delete(c.firstLines, key)
	}
}

// observePackage folds event into the launch timing of its package.
func (c *goJSONCounter) observePackage(event goTestEvent) {
	if event.Package == "" {
		return
	}
	at, timeErr := time.Parse(time.RFC3339Nano, event.Time)
	if event.Action == "start" && event.Test == "" {
		if c.packages == nil {
			c.packages = map[string]*packageTiming{}
		}
		if _, seen := c.packages[event.Package]; !seen {
			c.order = append(c.order, event.Package)
		}
		c.packages[event.Package] = &packageTiming{started: at, hasStart: timeErr == nil}
		return
	}
	timing := c.packages[event.Package]
	if timing == nil {
		return
	}
	if timing.hasStart && !timing.hasGap && timeErr == nil {
		timing.gapSeconds, timing.hasGap = at.Sub(timing.started).Seconds(), true
	}
	if event.Test != "" {
		return
	}
	switch event.Action {
	case "output":
		if strings.Contains(event.Output, "(cached)") || strings.Contains(event.Output, "[no test files]") {
			timing.noBinary = true
		}
	case "pass", "fail":
		timing.elapsedSeconds, timing.closed = event.Elapsed, true
	}
}

// handle runs under the line reader's lock.
func (c *goJSONCounter) handle(line []byte, terminated bool) error {
	var event goTestEvent
	if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &event) != nil || event.Action == "" {
		return c.lines.passThrough(line, terminated)
	}
	c.observePackage(event)
	if event.Package != "" {
		if c.testsRun == nil {
			c.testsRun = map[string]int{}
		}
		if _, seen := c.testsRun[event.Package]; !seen {
			c.testsRun[event.Package] = 0
		}
		if event.Test != "" && event.Action == "run" {
			c.testsRun[event.Package]++
		}
	}
	if event.Test != "" {
		c.observeTest(event)
		switch event.Action {
		case "run":
			c.counts.discovered++
		case "pass":
			c.counts.passed++
		case "fail":
			c.counts.failed++
		case "skip":
			c.counts.skipped++
		}
	}
	if event.Output == "" {
		return nil
	}
	_, err := io.WriteString(c.lines.out, event.Output)
	return err
}

// vstestCounter passes every line through and sums the counts of every VSTest summary line.
type vstestCounter struct {
	lines  lineReader
	counts tierCounts
}

func (c *vstestCounter) Write(p []byte) (int, error) { return c.lines.Write(p) }

func (c *vstestCounter) Flush() error { return c.lines.Flush() }

func (c *vstestCounter) Counts() tierCounts {
	c.lines.mu.Lock()
	defer c.lines.mu.Unlock()
	return c.counts
}

// TestsRun returns nil: a VSTest run reports no per-package counts.
func (c *vstestCounter) TestsRun() map[string]int { return nil }

// Launches returns nil: a VSTest run reports no per-package launch timing.
func (c *vstestCounter) Launches() []gateslot.PackageLaunch { return nil }

// Failures returns nil: a VSTest summary line names no test.
func (c *vstestCounter) Failures() []gateslot.TestFailure { return nil }

// handle runs under the line reader's lock.
func (c *vstestCounter) handle(line []byte, terminated bool) error {
	if match := vstestSummary.FindSubmatch(line); match != nil {
		// The pattern holds only digit groups, so the conversions cannot fail.
		failed, _ := strconv.Atoi(string(match[1]))
		passed, _ := strconv.Atoi(string(match[2]))
		skipped, _ := strconv.Atoi(string(match[3]))
		total, _ := strconv.Atoi(string(match[4]))
		c.counts.failed += failed
		c.counts.passed += passed
		c.counts.skipped += skipped
		c.counts.discovered += total
	}
	return c.lines.passThrough(line, terminated)
}
