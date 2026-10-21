// counters.go declares the counters a tier names: writers a tier command's output goes through, which pass it on as text and count its tests.

package gatecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"sync"
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
// It passes the output on as text and counts the tests it reports; the counts decide nothing about the run's outcome.
type counter interface {
	io.Writer
	// Flush emits a trailing partial line that no newline ended.
	Flush() error
	// Counts returns the counts so far.
	Counts() tierCounts
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
}

// goTestEvent is the part of a `go test -json` event the counter reads.
type goTestEvent struct {
	Action string
	Test   string
	Output string
}

func (c *goJSONCounter) Write(p []byte) (int, error) { return c.lines.Write(p) }

func (c *goJSONCounter) Flush() error { return c.lines.Flush() }

func (c *goJSONCounter) Counts() tierCounts {
	c.lines.mu.Lock()
	defer c.lines.mu.Unlock()
	return c.counts
}

// handle runs under the line reader's lock.
func (c *goJSONCounter) handle(line []byte, terminated bool) error {
	var event goTestEvent
	if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &event) != nil || event.Action == "" {
		return c.lines.passThrough(line, terminated)
	}
	if event.Test != "" {
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
