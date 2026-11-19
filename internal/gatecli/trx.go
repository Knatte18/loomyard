// trx.go reads the VSTest results files a vstest tier's invocation writes into a directory the gate names.
// The tier's counts, failures and run messages then come from a file the gate asked for,
// never from console text whose shape a logger among the call's own args changes.

package gatecli

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// trxLogFilePrefix opens the name of every results file the gate asks for.
// VSTest adds the target framework and a timestamp to it, so the projects of one invocation never share a file.
const trxLogFilePrefix = "gate"

// vstestResultsArgs returns the words a vstest invocation's {args} opens with, so the run writes its results to dir:
// a TRX results file per test project, and the blame collector's report of the test a crashed test host was running.
func vstestResultsArgs(dir string) []string {
	return []string{"--results-directory", dir, "--logger", "trx;LogFilePrefix=" + trxLogFilePrefix, "--blame"}
}

// trxResults is what the results files of one invocation report.
type trxResults struct {
	counts   tierCounts
	failures []runFailure
	// runMessages are the texts of the run-level messages, such as a test host crash and the test it was running.
	runMessages []string
}

// trxFile is the part of a TRX results file the gate reads.
type trxFile struct {
	Results     []trxTestResult `xml:"Results>UnitTestResult"`
	RunMessages []string        `xml:"ResultSummary>RunInfos>RunInfo>Text"`
}

// trxTestResult is one test's result in a TRX results file.
type trxTestResult struct {
	TestName   string `xml:"testName,attr"`
	Outcome    string `xml:"outcome,attr"`
	Message    string `xml:"Output>ErrorInfo>Message"`
	StackTrace string `xml:"Output>ErrorInfo>StackTrace"`
}

// readTRXResults sums the `.trx` files directly under dir and reports whether any of them could be read.
// A test counts as passed for the outcome Passed, as failed for Failed, Error, Timeout and Aborted, and as skipped otherwise.
// A failed test keeps the first failureLines non-empty lines of its error message and stack trace.
// A file that cannot be read or parsed is left out with a warning, so the console counts stand when none is read.
func readTRXResults(dir string) (trxResults, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("gate test: could not list the vstest results directory; the console counts stand", "dir", dir, "cause", err)
		return trxResults{}, false
	}
	var results trxResults
	read := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".trx" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parseTRXFile(path)
		if err != nil {
			logger.Warn("gate test: could not read a vstest results file; it is left out", "path", path, "cause", err)
			continue
		}
		read = true
		results.add(file)
	}
	return results, read
}

// parseTRXFile reads and decodes the TRX results file at path.
func parseTRXFile(path string) (trxFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return trxFile{}, err
	}
	var file trxFile
	if err := xml.Unmarshal(data, &file); err != nil {
		return trxFile{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return file, nil
}

// add folds one results file into r.
func (r *trxResults) add(file trxFile) {
	for _, result := range file.Results {
		r.counts.discovered++
		switch result.Outcome {
		case "Passed":
			r.counts.passed++
		case "Failed", "Error", "Timeout", "Aborted":
			r.counts.failed++
			r.failures = append(r.failures, runFailure{test: result.TestName, lines: firstLines(failureLines, result.Message, result.StackTrace)})
		default:
			r.counts.skipped++
		}
	}
	for _, message := range file.RunMessages {
		if lines := firstLines(-1, message); len(lines) > 0 {
			r.runMessages = append(r.runMessages, strings.Join(lines, "; "))
		}
	}
}

// firstLines returns the trimmed non-empty lines of texts in order, at most limit of them, or all when limit is negative.
func firstLines(limit int, texts ...string) []string {
	var lines []string
	for _, text := range texts {
		for _, line := range strings.Split(text, "\n") {
			if limit >= 0 && len(lines) == limit {
				return lines
			}
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

// resultsFileLines returns one line per run-level message the results files of c's run reported, naming tier.
// It returns nil for a counter that reads no results file.
func resultsFileLines(c counter, tier string) []string {
	vstest, ok := c.(*vstestCounter)
	if !ok {
		return nil
	}
	var lines []string
	for _, message := range vstest.RunMessages() {
		lines = append(lines, fmt.Sprintf("gate test: the test results file of %s reports: %s", tier, message))
	}
	return lines
}
