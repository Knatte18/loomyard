// trx.go reads the VSTest results files a vstest tier's invocation writes into a directory the gate names.
// The tier's counts, failures and run messages then come from a file the gate asked for.
// They never come from console text whose shape a logger among the call's own args changes.

package gatecli

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gateslot"
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
	// runMessages are the texts of the run-level messages of outcome Error, such as a test host crash and its running test.
	runMessages []string
}

// trxFile is the part of a TRX results file the gate reads.
type trxFile struct {
	Results  []trxTestResult `xml:"Results>UnitTestResult"`
	RunInfos []trxRunInfo    `xml:"ResultSummary>RunInfos>RunInfo"`
}

// trxRunInfo is one run-level message in a TRX results file, with the outcome that says how serious it is.
type trxRunInfo struct {
	Outcome string `xml:"outcome,attr"`
	Text    string `xml:"Text"`
}

// resultsArgsRefusal returns the refusal of a call whose own args hold a word the gate owns for the tier numbered number.
// Only a tier with results: trx owns such words.
// It returns "" for a call that holds none.
func resultsArgsRefusal(number int, tier gateslot.Tier, args []string) string {
	if tier.Results != gateslot.ResultsTRX {
		return ""
	}
	word := conflictingResultsWord(args)
	if word == "" {
		return ""
	}
	return fmt.Sprintf("gate test: the words after \"--\" hold %s, which the gate sets itself because %s reads its results file; way forward: drop %s from the words after \"--\", then re-run the same command", word, tierName(number), word)
}

// conflictingResultsWord returns the first word of args that sets the results directory or a trx logger, or "" for none.
// The gate owns those words for a tier that reads a results file,
// and a call's own copy could move the file out of the gate's directory.
// A logger word keeps its value, as `--logger trx;LogFileName=a.trx`.
func conflictingResultsWord(args []string) string {
	for i, arg := range args {
		name, value, joined := arg, "", false
		if cut := strings.IndexAny(arg, "=:"); cut >= 0 {
			name, value, joined = arg[:cut], arg[cut+1:], true
		}
		switch name {
		case "--results-directory":
			return arg
		case "--logger", "-l":
			word := arg
			if !joined && i+1 < len(args) {
				value, word = args[i+1], arg+" "+args[i+1]
			}
			if strings.HasPrefix(strings.ToLower(value), "trx") {
				return word
			}
		}
	}
	return ""
}

// trxTestResult is one test's result in a TRX results file.
type trxTestResult struct {
	TestName   string `xml:"testName,attr"`
	Outcome    string `xml:"outcome,attr"`
	Message    string `xml:"Output>ErrorInfo>Message"`
	StackTrace string `xml:"Output>ErrorInfo>StackTrace"`
}

// readTRXResults sums the `.trx` files directly under dir and reports whether any of them could be read.
// A test counts as passed for the outcome Passed.
// It counts as failed for Failed, Error, Timeout and Aborted, and as skipped otherwise.
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
	for _, info := range file.RunInfos {
		if info.Outcome != "Error" {
			continue
		}
		if lines := firstLines(-1, info.Text); len(lines) > 0 {
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
