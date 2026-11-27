// redfirstdotnet.go runs `lym gate red-first` under the dotnet scoper.
// It runs the C# test methods the diff adds or changes, by fully qualified name, in a checkout of the diff's base.
// In that checkout the changed files of the tests' projects keep their working-tree text.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotnetimpact"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/output"
)

// csharpCompileError matches an MSBuild line that reports a C# compile error, capturing its file and its text.
// The line reads `<file>.cs(<line>,<column>): error CS<number>: <text>`, often followed by ` [<project>]`.
var csharpCompileError = regexp.MustCompile(`^\s*(.+?\.cs)\(\d+,\d+\): error (CS\d+: .*?)(?:\s+\[[^\]]*\])?\s*$`)

// dotnetRedFirstCall is one run of a C# diff's changed tests on its base, once the diff is resolved.
type dotnetRedFirstCall struct {
	request redFirstRequest
	cfg     gateslot.Config
	// tiers are the tier numbers of the gate key.
	tiers []int
	// root is the worktree root.
	root string
	// base is the commit whose code the tests run on.
	base string
	// changed are the worktree-relative slash paths that differ between base and the working tree.
	changed []string
	// baseTree reads the files of the base commit.
	baseTree sourceTree
	// exportBase writes every file of the base commit under its directory.
	// A test replaces it to stand in for the git read.
	exportBase func(dest string) error
	// runTiers runs one test project's tests through the tiers and returns the exit code.
	// A test replaces it to stand in for the real tier runs.
	runTiers func(ctx context.Context, out io.Writer, request testRequest) int
}

// projectTests are the changed tests of one test project.
type projectTests struct {
	// project is the worktree-relative slash path of the project's `.csproj`.
	project string
	tests   []dotnetimpact.ChangedTest
}

// run selects the changed test methods and runs them once per test project in a checkout of the base.
// It prints the verdict and returns the verb's exit code.
// A diff with no changed test method exits gateslot.RedFirstNoTestExit.
// It exits gateslot.RedFirstUnjudgedExit when no changed test can be selected by name, as one of an abstract class.
// It exits so too when a test file's project cannot be told.
// It exits so too when no tier of the key takes {targets}.
// It exits so too when a project's tier run discovered no test, as its filter selected none or a restore or build failed.
// A run that started tests and judged none of them exits 1.
// The tier runs' output and failure reports go to the verb's stderr as they come.
func (c dotnetRedFirstCall) run(ctx context.Context, out io.Writer) int {
	tests, err := c.changedTests()
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: find the tests the diff changes: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	selectable := slices.DeleteFunc(slices.Clone(tests), func(test dotnetimpact.ChangedTest) bool { return test.Abstract })
	if len(selectable) == 0 {
		if len(tests) > 0 {
			return redFirstCannotJudge(out, fmt.Sprintf("the tests the diff changes, %s, are methods of abstract classes, which no filter selects by name", strings.Join(dotnetLabels(tests), ", ")))
		}
		return redFirstNoTest(out, c.base)
	}
	tiers := c.targetTiers()
	if len(tiers) == 0 {
		return redFirstCannotJudge(out, fmt.Sprintf("no tier of --gate %q holds {targets}, so no tier can run a test project of the base's checkout", c.request.gate))
	}
	groups, reason := c.byProject(selectable)
	if reason != "" {
		return redFirstCannotJudge(out, reason)
	}

	dir, err := lymtemp.MkdirTemp("red-first-")
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: cannot make the checkout directory: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	defer os.RemoveAll(dir)
	checkout := filepath.Join(dir, "base")
	if err := c.exportBase(checkout); err != nil {
		return output.Err(out, fmt.Sprintf("%s: cannot check out the base %s: %v; way forward: re-run the same command", redFirstVerb, c.base, err))
	}
	kept, err := c.keepHeadProjectFiles(checkout, groups)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: %v; way forward: re-run the same command", redFirstVerb, err))
	}

	stderr := c.request.run.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	labels := dotnetLabels(selectable)
	printLine(&lockedWriter{w: stderr}, fmt.Sprintf("%s: running %s on the code of %s in a checkout of it; %d changed files of the test projects keep their working-tree text",
		redFirstVerb, strings.Join(labels, ", "), c.base, kept))

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	var red, buildErrors, noTestRan []string
	unjudgedCode := 0
	for _, group := range groups {
		var captured bytes.Buffer
		sink := &lockedWriter{w: io.MultiWriter(&captured, stderr)}
		run := c.request.run
		run.tiers = tiers
		run.targets = []string{filepath.Join(checkout, filepath.FromSlash(group.project))}
		run.terms = filterTerms(group.tests)
		run.checkoutRoot = checkout
		run.noCarry = true
		run.stderr = sink
		code := c.runTiers(runCtx, sink, run)
		if caught.Load() != 0 {
			output.Err(out, redFirstVerb+": the call was cancelled; way forward: re-run the same command")
			return exitAfterSignal(caught, 1)
		}
		verdict := judgeDotnetRedFirst(code, captured.String(), group.tests, filepath.Join(checkout, filepath.FromSlash(path.Dir(group.project))))
		red = append(red, dotnetLabels(verdict.red)...)
		if verdict.buildError != "" {
			buildErrors = append(buildErrors, fmt.Sprintf("%s: %s", group.project, verdict.buildError))
			printLine(sink, fmt.Sprintf("%s: the tests of %s do not build on the base, so they count as red: %s", redFirstVerb, group.project, verdict.buildError))
		}
		switch {
		case verdict.noTestRan:
			noTestRan = append(noTestRan, noTestRanReason(group.project, run.terms, code))
		case verdict.outcome == redFirstUnjudged && unjudgedCode == 0:
			unjudgedCode = code
		}
	}
	if len(red) == 0 && unjudgedCode == 0 && len(noTestRan) > 0 {
		return redFirstCannotJudge(out, strings.Join(noTestRan, "; "))
	}
	var extra map[string]any
	if len(buildErrors) > 0 {
		extra = map[string]any{"build_errors": buildErrors}
	}
	return redFirstVerdictExit(out, c.base, labels, red, unjudgedCode, extra)
}

// redFirstCannotJudge prints why the check cannot judge the tests on the base and returns gateslot.RedFirstUnjudgedExit.
func redFirstCannotJudge(out io.Writer, reason string) int {
	output.Err(out, fmt.Sprintf("%s: %s, so the check cannot judge the tests on the base; way forward: none needed, a red-first gate passes this exit", redFirstVerb, reason))
	return gateslot.RedFirstUnjudgedExit
}

// noTestRanReason says why the tier run of project, which exited with exitCode, discovered no test.
// An exit code 0 means the filter terms selected none; any other means a restore or build failure ended the run.
func noTestRanReason(project string, terms []string, exitCode int) string {
	if exitCode == 0 {
		return fmt.Sprintf("the filter of %s (%s) selected no test", project, strings.Join(terms, ", "))
	}
	return fmt.Sprintf("the run of %s ended with exit %d before any test ran", project, exitCode)
}

// changedTests returns the test methods the working tree adds or changes against the base, in the changed `.cs` files.
func (c dotnetRedFirstCall) changedTests() ([]dotnetimpact.ChangedTest, error) {
	baseFiles, headFiles := map[string][]byte{}, map[string][]byte{}
	for _, rel := range c.changed {
		if !strings.HasSuffix(rel, ".cs") {
			continue
		}
		data, found, err := c.baseTree.file(rel)
		if err != nil {
			return nil, fmt.Errorf("read %s at the base: %w", rel, err)
		}
		if found {
			baseFiles[rel] = data
		}
		data, err = os.ReadFile(filepath.Join(c.root, filepath.FromSlash(rel)))
		switch {
		case err == nil:
			headFiles[rel] = data
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
	}
	return dotnetimpact.ChangedTests(baseFiles, headFiles), nil
}

// targetTiers returns the tiers of the call whose command takes {targets}, which alone can run one test project.
func (c dotnetRedFirstCall) targetTiers() []int {
	var numbers []int
	for _, number := range c.tiers {
		if tier, err := c.cfg.Tier(number); err == nil && gateslot.HoldsTargets(tier.Command) {
			numbers = append(numbers, number)
		}
	}
	return numbers
}

// byProject groups tests by the test project that holds each one's file, in the order of each project's first test.
// The reason is non-empty when a test file's project cannot be told.
func (c dotnetRedFirstCall) byProject(tests []dotnetimpact.ChangedTest) ([]projectTests, string) {
	var groups []projectTests
	for _, test := range tests {
		project, reason := nearestProject(c.root, test.File)
		if reason != "" {
			return nil, reason
		}
		index := slices.IndexFunc(groups, func(group projectTests) bool { return group.project == project })
		if index < 0 {
			index = len(groups)
			groups = append(groups, projectTests{project: project})
		}
		groups[index].tests = append(groups[index].tests, test)
	}
	return groups, ""
}

// keepHeadProjectFiles puts the working-tree text of each changed file whose project is one of groups' into checkout.
// It removes from checkout such a file the working tree no longer holds.
// Every other file keeps the base's text there.
// It returns how many files it wrote or removed.
func (c dotnetRedFirstCall) keepHeadProjectFiles(checkout string, groups []projectTests) (int, error) {
	kept := 0
	for _, rel := range c.changed {
		project, reason := nearestProject(c.root, rel)
		if reason != "" || !slices.ContainsFunc(groups, func(group projectTests) bool { return group.project == project }) {
			continue
		}
		target := filepath.Join(checkout, filepath.FromSlash(rel))
		data, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(rel)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return 0, fmt.Errorf("remove %s from the base's checkout: %w", rel, err)
			}
		case err != nil:
			return 0, fmt.Errorf("read %s: %w", rel, err)
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return 0, fmt.Errorf("write %s into the base's checkout: %w", rel, err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return 0, fmt.Errorf("write %s into the base's checkout: %w", rel, err)
			}
		}
		kept++
	}
	return kept, nil
}

// nearestProject returns the worktree-relative slash path of the `.csproj` of rel's project.
// That is the `.csproj` in the nearest directory above rel that holds one.
// The search starts in rel's own directory and reads the working tree under root.
// A directory that does not exist is climbed past.
// The reason is non-empty when no directory holds one or when the nearest holds more than one.
// It is non-empty when a directory cannot be read too.
func nearestProject(root, rel string) (string, string) {
	for dir := path.Dir(rel); ; dir = path.Dir(dir) {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Sprintf("cannot read %s, so the project of %s cannot be told: %v", dir, rel, err)
		}
		var projects []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(path.Ext(entry.Name()), ".csproj") {
				projects = append(projects, path.Join(dir, entry.Name()))
			}
		}
		switch {
		case len(projects) == 1:
			return projects[0], ""
		case len(projects) > 1:
			return "", fmt.Sprintf("%s holds more than one .csproj, so the project of %s cannot be told", dir, rel)
		case dir == ".":
			return "", fmt.Sprintf("no .csproj holds %s", rel)
		}
	}
}

// dotnetVerdict is the judgement of a run of one test project's changed tests on the base.
type dotnetVerdict struct {
	outcome redFirstOutcome
	// red are the changed tests that failed on the base; set for redFirstRed.
	red []dotnetimpact.ChangedTest
	// buildError is the first compile error of the project's files when its tests did not build on the base.
	buildError string
	// noTestRan is set for an unjudged run whose tiers discovered no test.
	// A run whose tiers timed out, crashed their runner or stopped for memory pressure never sets it.
	// An exit code 0 means the filter selected none, any other that a restore or build failure ended the run.
	noTestRan bool
}

// judgeDotnetRedFirst judges a tier run of one test project's tests on the base from its exit code and captured output.
// projectDir is the absolute directory of the project in the base's checkout.
// An exit code 0 is green, unless the tier summary lines report no test discovered: the filter selected none.
// Otherwise a test is red when the failure report names it as failed under a tier run.
// A failed name matches the test when it is the test's method, or ends with `.` and the method.
// The name's argument list is left out of that comparison.
// With no such name, every test is red when the output reports a C# compile error in a file under projectDir:
// a test that reads a member only the change adds does not build on the base.
// A failing run that makes no test red judged none of them.
// It ran no test when the tier summary lines report none discovered or the output holds none.
// The exception is a tier that timed out, crashed its runner or was stopped for memory pressure.
// Such a tier may have started tests before it ended.
func judgeDotnetRedFirst(exitCode int, captured string, tests []dotnetimpact.ChangedTest, projectDir string) dotnetVerdict {
	discovered, summarized := gateslot.TierDiscovered(captured)
	if exitCode == 0 {
		if summarized && discovered == 0 {
			return dotnetVerdict{outcome: redFirstUnjudged, noTestRan: true}
		}
		return dotnetVerdict{outcome: redFirstGreen}
	}
	failed := gateslot.TierFailureNames(captured)
	var red []dotnetimpact.ChangedTest
	for _, test := range tests {
		if slices.ContainsFunc(failed, func(name string) bool { return failedNameMatches(name, test.Method) }) {
			red = append(red, test)
		}
	}
	if len(red) > 0 {
		return dotnetVerdict{outcome: redFirstRed, red: red}
	}
	if buildError := projectCompileError(captured, projectDir); buildError != "" {
		return dotnetVerdict{outcome: redFirstRed, red: tests, buildError: buildError}
	}
	_, timedOut := gateslot.FindTierTimeout(captured)
	_, crashed := gateslot.FindRunnerCrash(captured)
	_, pressured := gateslot.FindMemoryPressureStop(captured)
	return dotnetVerdict{outcome: redFirstUnjudged, noTestRan: (!summarized || discovered == 0) && !timedOut && !crashed && !pressured}
}

// failedNameMatches reports whether name, a failed test's name in a VSTest result, names the test method method.
// A parameterized test's name carries its arguments in parentheses, and a fully qualified name carries its class.
func failedNameMatches(name, method string) bool {
	bare, _, _ := strings.Cut(name, "(")
	return bare == method || strings.HasSuffix(bare, "."+method)
}

// projectCompileError returns the text of the first C# compile error in output whose file lies under projectDir, or "".
func projectCompileError(output, projectDir string) string {
	prefix := filepath.Clean(projectDir) + string(filepath.Separator)
	for line := range strings.Lines(output) {
		match := csharpCompileError.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
		if match != nil && strings.HasPrefix(filepath.Clean(filepath.FromSlash(match[1])), prefix) {
			return fmt.Sprintf("%s: %s", filepath.Base(filepath.FromSlash(match[1])), match[2])
		}
	}
	return ""
}

// filterTerms returns the filter term of each of tests, sorted and without repeats.
func filterTerms(tests []dotnetimpact.ChangedTest) []string {
	terms := make([]string, len(tests))
	for i, test := range tests {
		terms[i] = test.FilterTerm()
	}
	slices.Sort(terms)
	return slices.Compact(terms)
}

// dotnetLabels returns each of tests as `<Class>.<Method>`.
func dotnetLabels(tests []dotnetimpact.ChangedTest) []string {
	labels := make([]string, len(tests))
	for i, test := range tests {
		labels[i] = test.Label()
	}
	return labels
}
