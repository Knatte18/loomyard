// failurereport.go builds the failure report a failing `lym gate test` call prints before its closing line:
// every failure of its tier, vet, guard and build runs and of its gate key's lint steps.
// It also names the files the change touches and each failed run's log.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/patternlint"
)

const (
	// failureReportHeader opens the failure report.
	failureReportHeader = "gate test: failure report, every failure of this call:"
	// failureReportLines caps the lines below the header; the closing line of a cut report says where the rest is.
	failureReportLines = 80
	// failureReportLineBytes caps one report line so a long panic or diff line cannot crowd out later failures.
	failureReportLineBytes = 300
	// touchedFilesListed caps the files the report's touched-files line names.
	touchedFilesListed = 20
	// lintLabelPrefix opens the label of a lint step's failed run; the rest of the label is the lint's name.
	lintLabelPrefix = "lint "
)

// lintWayForward is the fix each lint step's findings call for.
var lintWayForward = map[string]string{
	gateslot.LintComments: "break each wrapped comment at its sentence boundaries, one sentence per line, and split each over-long one into shorter sentences",
	gateslot.LintPattern:  patternlint.WayForward,
}

// bareGoFile matches a line that opens with a Go file's bare name and a line number.
// A test's own error lines print that form, so the report can prefix the package's directory.
var bareGoFile = regexp.MustCompile(`^[A-Za-z0-9_.\-]+\.go:[0-9]+`)

// runRelativeGoFile matches a line that opens with a Go file's path relative to the run directory.
// The path starts with `./` or has at least one directory, as the go command prints a compile error.
// Either path separator is accepted, since the go command prints the platform's own.
var runRelativeGoFile = regexp.MustCompile(`^(\.[/\\]|[A-Za-z0-9_.\-]+[/\\])([A-Za-z0-9_.\-]+[/\\])*[A-Za-z0-9_.\-]+\.go:[0-9]+`)

// failedRun is one failed run of a call as the failure report lists it: a tier, vet, guard or build run, or a lint step.
type failedRun struct {
	// label names the run as the closing line of failed runs does, such as `tier0`, `vet tier1`, `build` or `lint comments`.
	label string
	// outcome says how the run failed, such as `exit 1`, `timed out` or `2 findings`.
	outcome string
	// wayForward is the fix a lint step's findings call for, empty for any other run.
	wayForward string
	// log is the run's log path, empty for a lint step.
	log string
	// modulePath and moduleDir are the run's module path and worktree-relative directory; both empty when unknown.
	modulePath, moduleDir string
	failures              []runFailure
	// causes are the lines the gate states the run's failure with, such as a timeout, listed even without a read failure.
	causes []string
	// unparsed are the first lines of the run's output no test event carried, listed when no failure was read.
	unparsed []string
}

// tierFailedRun returns the failed run the report lists for the tier, vet or guard run that ended as result.
// logPath is its log, and root is the worktree root its run directory resolves against.
func tierFailedRun(run tierRun, result tierResult, logPath, root string) failedRun {
	outcome := fmt.Sprintf("exit %d", result.record.ExitCode)
	if result.record.TimedOut {
		outcome = "timed out"
	}
	failed := failedRun{label: runLabel(run), outcome: outcome, log: logPath, failures: result.failures, causes: result.causes, unparsed: result.unparsed}
	if result.record.RunDir != "" {
		if rel, err := filepath.Rel(root, result.record.RunDir); err == nil {
			failed.modulePath, failed.moduleDir = modulePathOf(result.record.RunDir), filepath.ToSlash(rel)
		}
	}
	return failed
}

// buildFailedRun returns the failed run the report lists for the build step that exited with code, or timed out.
// logPath is its log. Of output, it keeps the last non-empty lines, where a build states its errors.
func buildFailedRun(code int, timedOut bool, logPath string, output []byte) failedRun {
	outcome := fmt.Sprintf("exit %d", code)
	if timedOut {
		outcome = "timed out"
	}
	var lines []string
	for _, line := range strings.Split(string(output), "\n") {
		if text := strings.TrimSpace(line); text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) > unparsedLines {
		lines = lines[len(lines)-unparsedLines:]
	}
	return failedRun{label: "build", outcome: outcome, log: logPath, unparsed: lines}
}

// runLints runs the lint steps in names over the worktree at root, each against HEAD, in order.
// It returns the steps that found something or could not run, as failed runs.
func runLints(root string, names []string) []failedRun {
	var failed []failedRun
	for _, name := range names {
		var findings []string
		var err error
		switch name {
		case gateslot.LintComments:
			var found []commentlint.Finding
			found, err = commentlint.Lint(root, "")
			for _, finding := range found {
				findings = append(findings, finding.String())
			}
		case gateslot.LintPattern:
			finding, found, lintErr := patternlint.Lint(root, "")
			err = lintErr
			if found {
				findings = []string{"PATTERN.md: " + finding.Message}
			}
		}
		if run, ok := lintFailedRun(name, findings, err); ok {
			failed = append(failed, run)
		}
	}
	return failed
}

// lintFailedRun returns the failed run of the lint step name when it found findings or failed to run with err.
// It returns false when the step found nothing and ran.
func lintFailedRun(name string, findings []string, err error) (failedRun, bool) {
	label := lintLabelPrefix + name
	if err != nil {
		logger.Warn("gate test: a lint step could not run", "lint", name, "cause", err)
		return failedRun{label: label, outcome: "could not run", unparsed: []string{err.Error()}, wayForward: "re-run the same command"}, true
	}
	if len(findings) == 0 {
		return failedRun{}, false
	}
	outcome := fmt.Sprintf("%d findings", len(findings))
	if len(findings) == 1 {
		outcome = "1 finding"
	}
	failures := make([]runFailure, len(findings))
	for i, finding := range findings {
		failures[i] = runFailure{lines: []string{finding}}
	}
	return failedRun{label: label, outcome: outcome, wayForward: lintWayForward[name], failures: failures}, true
}

// touchedFiles returns the files the change under test touches, sorted and worktree-relative.
// They are the paths that differ between the working tree, untracked files included, and the base.
// The base is the merge base with the pair's parent branch inside a hub, or HEAD where no parent applies.
// A read that fails is logged and returns nil, since the list only orients the reader.
func touchedFiles(ctx context.Context, location *lymcwd.Location, root string, inHub bool) []string {
	base := "HEAD"
	if inHub {
		mergeBase, found, err := parentMergeBase(location)
		switch {
		case err != nil:
			logger.Warn("gate test: could not read the merge base for the failure report; it lists the files changed since HEAD", "cause", err)
		case found:
			base = mergeBase
		}
	}
	paths, err := impactset.ChangedPaths(ctx, root, base)
	if err != nil {
		logger.Warn("gate test: could not list the changed files for the failure report", "base", base, "cause", err)
		return nil
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// renderFailureReport returns the failure report's lines for runs, given the files the change touches.
// It returns nil when no run failed.
// Each run names its outcome and its log.
// Then each failure names its package and test, or its package alone for a failure outside any test.
// A path a run printed relative to its directory is prefixed with the directory's worktree-relative path.
// A run lists its cause lines, such as a timeout.
// Then, when no failure was read, it lists the first lines of output no test event carried,
// or it says that its log holds the cause.
// Past failureReportLines lines the report stops with a line saying how many lines each run lost and where to read them.
// The touched-files line closes it, naming at most touchedFilesListed files.
func renderFailureReport(runs []failedRun, touched []string) []string {
	if len(runs) == 0 {
		return nil
	}
	lines := []string{failureReportHeader}
	budget := failureReportLines
	var cut []cutRun
	for _, run := range runs {
		block := run.reportLines()
		kept := min(len(block), budget)
		for _, line := range block[:kept] {
			lines = append(lines, capLine(line))
		}
		budget -= kept
		if kept < len(block) {
			cut = append(cut, cutRun{label: run.label, lines: len(block) - kept})
		}
	}
	if len(cut) > 0 {
		// Uncapped: its hints are the only place a cut lint step's findings can be recovered from.
		lines = append(lines, cutLine(cut))
	}
	if len(touched) > 0 {
		named := touched
		suffix := ""
		if len(named) > touchedFilesListed {
			named = named[:touchedFilesListed]
			suffix = fmt.Sprintf(" and %d more", len(touched)-touchedFilesListed)
		}
		lines = append(lines, capLine("gate test: the change touches "+strings.Join(named, ", ")+suffix))
	}
	return lines
}

// cutRun is how many lines of the failed run labelled label the report cut.
type cutRun struct {
	label string
	lines int
}

// reportLines returns the report's lines for the run: its header, its failures, then its causes.
// The output no test event carried follows them.
func (run failedRun) reportLines() []string {
	header := fmt.Sprintf("%s (%s)", run.label, run.outcome)
	if run.wayForward != "" {
		header += "; way forward: " + run.wayForward
	}
	if run.log != "" {
		header += ", log " + run.log
	}
	body := []string{header + ":"}
	for _, failure := range run.failures {
		indent := "  "
		switch {
		case failure.test != "":
			body = append(body, fmt.Sprintf("  %s %s:", failure.pkg, failure.test))
			indent = "    "
		case failure.pkg != "":
			body = append(body, fmt.Sprintf("  %s, failed outside any test:", failure.pkg))
			indent = "    "
		}
		for _, line := range failure.lines {
			body = append(body, indent+run.qualify(failure, line))
		}
	}
	for _, cause := range run.causes {
		body = append(body, "  "+cause)
	}
	switch {
	case len(run.failures) > 0:
	case len(run.unparsed) > 0:
		body = append(body, "  its output:")
		for _, line := range run.unparsed {
			body = append(body, "    "+line)
		}
	case len(run.causes) == 0:
		body = append(body, "  no failing test or error line was read from its output; its log holds the cause")
	}
	return body
}

// cutLine returns the line that closes a report which lost the lines of the runs in cut.
// The line counts the lost lines of each run: a run with a log points to the logs.
// A lint step, which has no log, points to the lint command to re-run for its findings.
func cutLine(cut []cutRun) string {
	total := 0
	var counts, hints []string
	logged := false
	for _, run := range cut {
		total += run.lines
		counts = append(counts, fmt.Sprintf("%s: %d", run.label, run.lines))
		if lint, isLint := strings.CutPrefix(run.label, lintLabelPrefix); isLint {
			hints = append(hints, fmt.Sprintf("%s has no log, so re-run `lym loom lint-%s` for its findings", run.label, lint))
		} else {
			logged = true
		}
	}
	if logged {
		hints = slices.Insert(hints, 0, "the logs named above hold the cut lines of a run with a log")
	}
	return fmt.Sprintf("... %d more lines cut (%s); %s", total, strings.Join(counts, ", "), strings.Join(hints, "; "))
}

// qualify prefixes the Go file path line opens with by the worktree-relative module directory.
// It returns line unchanged when line opens with no Go file path.
// A bare file name is relative to the directory of the failure's package, which must lie inside the run's module.
// A failure outside any test prints a compile error's path relative to the run directory,
// so a path with a leading `./` or a directory is relative to that directory.
func (run failedRun) qualify(failure runFailure, line string) string {
	if run.modulePath == "" {
		return line
	}
	if failure.test == "" {
		if matched := runRelativeGoFile.FindString(line); matched != "" {
			if run.moduleDir == "." || run.moduleDir == "" {
				return line
			}
			located := strings.TrimPrefix(strings.ReplaceAll(matched, `\`, "/"), "./")
			return run.moduleDir + "/" + located + line[len(matched):]
		}
	}
	pkg := failure.pkg
	if pkg == "" || !bareGoFile.MatchString(line) {
		return line
	}
	rest, inModule := strings.CutPrefix(pkg, run.modulePath)
	if !inModule || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return line
	}
	dir := path.Join(run.moduleDir, strings.TrimPrefix(rest, "/"))
	if dir == "." {
		return line
	}
	return dir + "/" + line
}

// capLine returns line cut to failureReportLineBytes at a rune boundary, marked with an ellipsis when cut.
func capLine(line string) string {
	if len(line) <= failureReportLineBytes {
		return line
	}
	cut := failureReportLineBytes
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "..."
}

// printFailureReport writes the failure report of runs to stderr, with touched naming the files the change touches.
// It writes nothing when no run failed.
func printFailureReport(stderr io.Writer, runs []failedRun, touched []string) {
	for _, line := range renderFailureReport(runs, touched) {
		printLine(stderr, line)
	}
}
