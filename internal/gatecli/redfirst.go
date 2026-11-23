// redfirst.go implements `lym gate red-first`, which proves a bug fix's test was red before the fix.
// It runs the tests the worktree's diff adds or changes once on the code of the diff's base.
// It lays the base's production files, and the non-Go files they embed, over the working tree with a Go build overlay.
// A site reads the verb's exit code.
// The code is 0 when a changed test failed on the base, and a gateslot red-first code otherwise.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// redFirstVerb labels the refusals of `lym gate red-first`.
const redFirstVerb = "gate red-first"

// noSeamWayForward is the way forward of a refusal that only the run's parent can lift.
func noSeamWayForward() string {
	return fmt.Sprintf("the run's parent records the bug as having no seam a test can reach, "+
		"with `lym loom decision add` under a title opening %q", gateslot.NoTestSeamTitle)
}

// redFirstRequest is one `lym gate red-first` invocation after flag parsing.
type redFirstRequest struct {
	// gate is the --gate key whose tiers run the tests.
	gate string
	// base is the --base revision, empty for the merge base with the parent branch.
	base string
	// run carries the go command, the units, the delays and the stderr of the tier run; the verb fills the rest.
	run testRequest
}

// redFirstCmd builds `lym gate red-first`.
// The parameters are those newCommand gives the tier run the verb starts.
func redFirstCmd(goBinary string, samplePeriod, preTierCeiling, timeoutUnit, rerunDelay, pipeWaitDelay time.Duration) *cobra.Command {
	var gate, base string
	cmd := &cobra.Command{
		Use:         "red-first",
		Short:       "run the tests the worktree's diff adds or changes on the base's code and report whether any failed there",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `red-first proves that a bug fix's test was red before the fix.
It takes the tests the worktree's diff adds or changes, from --base, else the merge base with the parent branch.
It runs them once per package, through the tiers of the gate key --gate names, on the code of that base:
a Go build overlay puts the base's production files and the non-Go files they embed back,
and the tests and every other non-Go file keep their working-tree text.
It runs the key's tiers only, never its lints, its scope or its report steps.
A test that does not build on the base counts as failing there.
It exits 0 when a changed test failed on the base, and prints the red tests.
It exits ` + fmt.Sprint(gateslot.RedFirstGreenExit) + ` when every changed test passed on the base, since such a test does not show the bug.
It exits ` + fmt.Sprint(gateslot.RedFirstNoTestExit) + ` when the diff adds or changes no test.
It exits ` + fmt.Sprint(gateslot.RedFirstUnjudgedExit) + ` when the run cannot judge the tests on the base, on a hub whose scoper is not go.
It exits 1 for a refusal, or when the run judged none of the tests.

Examples:

  lym gate red-first --gate darn-write
  lym gate red-first --gate darn-write --base 3f2a9c1`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runRedFirst(cmd.Context(), cmd.OutOrStdout(), redFirstRequest{
				gate: gate, base: base,
				run: testRequest{
					goBinary: goBinary, samplePeriod: samplePeriod, preTierCeiling: preTierCeiling, timeoutUnit: timeoutUnit,
					secondUnit: time.Second, rerunDelay: rerunDelay, pipeWaitDelay: pipeWaitDelay, stderr: cmd.ErrOrStderr(),
				},
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&gate, "gate", "", "gate.yaml key whose tiers run the tests; required")
	cmd.Flags().StringVar(&base, "base", "", "commit whose code the tests run on; default the merge base with the parent branch")
	return cmd
}

// runRedFirst resolves the worktree's diff and runs its changed tests on the base's code.
// It returns the verb's exit code.
// The code is 0 when a changed test failed on the base.
// It is gateslot.RedFirstGreenExit when all passed.
// It is gateslot.RedFirstNoTestExit when the diff holds no changed test.
// It is gateslot.RedFirstUnjudgedExit when the hub's scoper is not go.
// It is 1 for a refusal or a run that judged none of them.
// After a terminating signal the code is 128 plus the signal number.
func runRedFirst(ctx context.Context, out io.Writer, request redFirstRequest) int {
	if request.gate == "" {
		return output.Err(out, redFirstVerb+": --gate names no gate key; way forward: pass --gate with a key of gate.yaml's gates, then re-run the same command")
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: cannot read the working directory: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	if !inHub {
		return output.Err(out, fmt.Sprintf("%s: %s is in no hub, and the gate keys live in a hub; way forward: run lym gate red-first from a worktree of the hub", redFirstVerb, cwd))
	}
	cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", redFirstVerb, err))
	}
	cfg = cfg.WithDefaultDir(location.AnchorRel)
	if !cfg.HasGoCoverageMaps() {
		output.Err(out, fmt.Sprintf("%s: "+noteScoper+"; way forward: %s", redFirstVerb, cfg.Scoper, noSeamWayForward()))
		return gateslot.RedFirstUnjudgedExit
	}
	tiers, err := cfg.TierList(request.gate)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: --gate %q: %v", redFirstVerb, request.gate, err))
	}
	if len(tiers) == 0 {
		return output.Err(out, fmt.Sprintf("%s: --gate %q runs no tier, since the hub declares none; way forward: declare tier0 with lym config gate from the prime, then re-run the same command", redFirstVerb, request.gate))
	}
	root := location.WorktreePath()
	module, err := redundancyModule(cfg, root)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: %v; way forward: declare tier0 or tier1 with its dir at the Go module's root with lym config gate from the prime, then re-run the same command", redFirstVerb, err))
	}
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	base, refusal := verbDiffBase(location, fabric, request.base, redFirstVerb)
	if refusal != "" {
		return output.Err(out, refusal)
	}
	changed, err := impactset.ChangedPaths(ctx, root, base)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	baseTree := commitTree{fabric: fabric, rev: base}
	tests, err := changedTestsAt(baseTree, workingTree{root: root}, module, changed)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: find the tests the diff changes: %v; way forward: fix the file it names, then re-run the same command", redFirstVerb, err))
	}
	if len(tests) == 0 {
		output.Err(out, fmt.Sprintf("%s: the diff from %s adds or changes no test, so nothing shows the bug failing before the fix; "+
			"way forward: add a test that fails on the merge base and passes with the fix and commit it; "+
			"when the bug has no seam a test can reach, %s", redFirstVerb, base, noSeamWayForward()))
		return gateslot.RedFirstNoTestExit
	}
	return redFirstCall{
		request: request, tiers: tiers, root: root, module: module, base: base, changed: changed, tests: tests, baseTree: baseTree,
		runTiers: runTest,
	}.run(ctx, out)
}

// redFirstCall is one run of a diff's changed tests on its base, once the diff is resolved.
type redFirstCall struct {
	request redFirstRequest
	// tiers are the tier numbers of the gate key, which run the tests.
	tiers []int
	// root is the worktree root and module the Go module the tests belong to.
	root   string
	module goModule
	// base is the commit whose production code the tests run on.
	base string
	// changed are the worktree-relative slash paths that differ between base and the working tree.
	changed []string
	// tests are the tests the diff adds or changes, at least one.
	tests []testreach.ChangedTest
	// baseTree reads the files of the base commit.
	baseTree redundancyTree
	// runTiers runs one package's tests through the gate key's tiers and returns the exit code.
	// A test replaces it to stand in for the real tier runs.
	runTiers func(ctx context.Context, out io.Writer, request testRequest) int
}

// run lays the base's production files and the non-Go files they embed over the working tree.
// It runs the changed tests and prints the verdict.
// It runs the tiers once per package of the changed tests, so a test of one package never runs by another's name.
// The tier runs' output and failure reports go to the verb's stderr as they come.
// Each is kept for its package's verdict.
func (c redFirstCall) run(ctx context.Context, out io.Writer) int {
	dir, err := lymtemp.MkdirTemp("red-first-")
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: cannot make the overlay directory: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	defer os.RemoveAll(dir)
	replace, err := baseOverlay(dir, c.root, c.module, c.changed, c.baseTree)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: %v; way forward: re-run the same command", redFirstVerb, err))
	}
	overlayPath, err := writeOverlay(dir, replace)
	if err != nil {
		return output.Err(out, fmt.Sprintf("%s: cannot write the overlay: %v; way forward: re-run the same command", redFirstVerb, err))
	}

	stderr := c.request.run.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	labels := testLabels(c.tests)
	printLine(&lockedWriter{w: stderr}, fmt.Sprintf("%s: running %s on the code of %s; the overlay replaces %d files", redFirstVerb, strings.Join(labels, ", "), c.base, len(replace)))

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	var red []testreach.ChangedTest
	unjudgedCode := 0
	for _, group := range testsByPackage(c.tests) {
		var captured bytes.Buffer
		sink := &lockedWriter{w: io.MultiWriter(&captured, stderr)}
		run := c.request.run
		run.tiers = c.tiers
		run.dir = c.module.dir
		run.targets = testTargetsOf(c.module, group)
		run.args = []string{"-overlay=" + overlayPath, "-run=^(" + strings.Join(testNamesOf(group), "|") + ")$"}
		run.stderr = sink
		code := c.runTiers(runCtx, sink, run)
		if caught.Load() != 0 {
			output.Err(out, redFirstVerb+": the call was cancelled; way forward: re-run the same command")
			return exitAfterSignal(caught, 1)
		}
		verdict := judgeRedFirst(code, captured.String(), group)
		red = append(red, verdict.red...)
		if verdict.outcome == redFirstUnjudged && unjudgedCode == 0 {
			unjudgedCode = code
		}
	}

	switch {
	case len(red) > 0:
		return output.Ok(out, map[string]any{"base": c.base, "red_tests": testLabels(red), "tests": labels})
	case unjudgedCode == 0:
		output.Err(out, fmt.Sprintf("%s: every test the diff adds or changes passed on the base %s: %s; "+
			"a test that passes before the fix does not show the bug; "+
			"way forward: when the change fixes a bug, make a test reach the bug through code the base already has, so it fails there, "+
			"and commit it, then re-run the same command; "+
			"when it fixes no bug, as a change that only adds tests pinning existing behaviour, "+
			"the run's parent records that with `lym loom decision add` under a title opening %q",
			redFirstVerb, c.base, strings.Join(labels, ", "), gateslot.NoBugFixedTitle))
		return gateslot.RedFirstGreenExit
	}
	return output.Err(out, fmt.Sprintf("%s: the tiers exited %d on the base %s and judged none of %s; "+
		"way forward: read the failure report above and re-run the same command", redFirstVerb, unjudgedCode, c.base, strings.Join(labels, ", ")))
}

// testsByPackage splits tests into one slice per package, in the order of each package's first test.
func testsByPackage(tests []testreach.ChangedTest) [][]testreach.ChangedTest {
	var groups [][]testreach.ChangedTest
	index := map[string]int{}
	for _, test := range tests {
		i, seen := index[test.Package]
		if !seen {
			i = len(groups)
			index[test.Package] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], test)
	}
	return groups
}

// redFirstOutcome is what a run of the changed tests on the base showed.
type redFirstOutcome int

// The outcomes of a run of the changed tests on the base.
const (
	// redFirstUnjudged is a run that named none of the tests as failed and did not pass.
	redFirstUnjudged redFirstOutcome = iota
	// redFirstRed is a run in which a changed test failed.
	redFirstRed
	// redFirstGreen is a run in which every changed test passed.
	redFirstGreen
)

// redFirstVerdict is the judgement of a run of the changed tests on the base.
type redFirstVerdict struct {
	outcome redFirstOutcome
	// red are the changed tests that failed on the base; set for redFirstRed.
	red []testreach.ChangedTest
}

// judgeRedFirst judges a tier run of tests on the base from its exit code and its captured output.
// An exit code 0 is green.
// Otherwise a test is red in one of two cases.
// The output's failure report names its package and name as failed under a tier or guard run.
// What else the report holds does not matter.
// Or it names its package as failed outside any test, which is how a test that does not build on the base fails.
// A failing run that makes no test red judged none of them.
func judgeRedFirst(exitCode int, captured string, tests []testreach.ChangedTest) redFirstVerdict {
	if exitCode == 0 {
		return redFirstVerdict{outcome: redFirstGreen}
	}
	failed := gateslot.NamedFailedTests(captured)
	outside := gateslot.PackagesFailedOutsideTests(captured)
	var red []testreach.ChangedTest
	for _, test := range tests {
		named := slices.ContainsFunc(failed, func(f gateslot.FailedTest) bool {
			return f.Tier >= 0 && f.Package == test.Package && f.Test == test.Name
		})
		if named || slices.Contains(outside, test.Package) {
			red = append(red, test)
		}
	}
	if len(red) == 0 {
		return redFirstVerdict{outcome: redFirstUnjudged}
	}
	return redFirstVerdict{outcome: redFirstRed, red: red}
}

// changedTestsAt returns the tests the working tree head adds or changes against the tree base.
// changed are the worktree-relative slash paths that differ between the two.
// It reads the `_test.go` files of each package of module that holds a changed one, in both trees.
func changedTestsAt(base, head redundancyTree, module goModule, changed []string) ([]testreach.ChangedTest, error) {
	dirs := map[string]bool{}
	for _, rel := range changed {
		if dir := path.Dir(rel); strings.HasSuffix(rel, "_test.go") && isModulePackage(module.rel, dir) {
			dirs[dir] = true
		}
	}
	baseFiles, headFiles := testreach.FileSet{}, testreach.FileSet{}
	for _, dir := range slices.Sorted(maps.Keys(dirs)) {
		for _, side := range []struct {
			tree  redundancyTree
			files testreach.FileSet
		}{{base, baseFiles}, {head, headFiles}} {
			names, err := side.tree.goFiles(dir)
			if err != nil {
				return nil, fmt.Errorf("list the Go files of %s: %w", dir, err)
			}
			for _, name := range names {
				if !strings.HasSuffix(name, "_test.go") {
					continue
				}
				data, found, err := side.tree.file(path.Join(dir, name))
				if err != nil {
					return nil, fmt.Errorf("read %s: %w", path.Join(dir, name), err)
				}
				if found {
					side.files[path.Join(dir, name)] = data
				}
			}
		}
	}
	return testreach.ChangedTests(reachModuleOf(module), baseFiles, headFiles)
}

// baseOverlay writes into dir the base's text of each changed file of module that the base's build reads.
// Those are the production Go files and the non-Go files a production Go file at the base embeds.
// The go command reads an embedded file through the overlay too.
// It returns the overlay's replacements: the absolute working-tree path of the file, mapped to the written copy.
// A file the base lacks maps to "", which the Go overlay reads as deleted.
// changed are the worktree-relative slash paths that differ between base and the working tree under root.
// These files are left as the working tree holds them:
// a test file, a Go file outside the module's packages or under testdata, a file outside the module,
// and a non-Go file no production file embeds.
// They are the tests' inputs, or nothing the build reads.
func baseOverlay(dir, root string, module goModule, changed []string, base redundancyTree) (map[string]string, error) {
	replace := map[string]string{}
	for i, rel := range changed {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if strings.HasSuffix(rel, ".go") {
			if !isModulePackage(module.rel, path.Dir(rel)) {
				continue
			}
		} else {
			if !inModule(module.rel, path.Dir(rel)) {
				continue
			}
			embedded, err := embeddedAtBase(base, module.rel, rel)
			if err != nil {
				return nil, err
			}
			if !embedded {
				continue
			}
		}
		data, found, err := base.file(rel)
		if err != nil {
			return nil, fmt.Errorf("read %s at the base: %w", rel, err)
		}
		copyPath := ""
		if found {
			copyPath = filepath.Join(dir, fmt.Sprintf("%d-%s", i, path.Base(rel)))
			if err := os.WriteFile(copyPath, data, 0o600); err != nil {
				return nil, fmt.Errorf("write the base copy of %s: %w", rel, err)
			}
		}
		replace[filepath.Join(root, filepath.FromSlash(rel))] = copyPath
	}
	return replace, nil
}

// goEmbedDirective opens the comment line that names the files a Go package embeds.
// A space or a tab separates it from its patterns.
const goEmbedDirective = "//go:embed"

// embeddedAtBase reports whether a production Go file of base embeds the worktree-relative slash path rel.
// It reads the embed patterns of the Go files in rel's directory.
// It reads those in each directory above it too, up to the module's root at moduleRel.
// A pattern embeds rel when it matches rel's path below the Go file's directory, or a directory rel lies under.
func embeddedAtBase(base redundancyTree, moduleRel, rel string) (bool, error) {
	dir := path.Dir(rel)
	for {
		names, err := base.goFiles(dir)
		if err != nil {
			return false, fmt.Errorf("list the Go files of %s at the base: %w", dir, err)
		}
		below := strings.TrimPrefix(rel, dir+"/")
		for _, name := range names {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			source, _, err := base.file(path.Join(dir, name))
			if err != nil {
				return false, fmt.Errorf("read %s at the base: %w", path.Join(dir, name), err)
			}
			if slices.ContainsFunc(embedPatterns(source), func(pattern embedPattern) bool { return embedPatternMatches(pattern, below) }) {
				return true, nil
			}
		}
		if dir == moduleRel || dir == "." {
			return false, nil
		}
		dir = path.Dir(dir)
	}
}

// embedPattern is one pattern of an embed directive.
type embedPattern struct {
	// glob is the pattern, unquoted and without its `all:` prefix.
	glob string
	// all is whether the pattern carried the `all:` prefix.
	// Such a pattern embeds the files of a directory whose names open with a dot or an underscore too.
	all bool
}

// embedPatterns returns the patterns of every embed directive in the Go source.
// It reads a directive's arguments as Go does:
// a double-quoted or backtick-quoted argument runs to its closing quote, and any other argument ends at whitespace.
// An argument with a malformed quote ends the directive's patterns.
func embedPatterns(source []byte) []embedPattern {
	var patterns []embedPattern
	for line := range strings.Lines(string(source)) {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), goEmbedDirective)
		if !found || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		for rest = strings.TrimLeft(rest, " \t"); rest != ""; rest = strings.TrimLeft(rest, " \t") {
			var field string
			if rest[0] == '"' || rest[0] == '`' {
				quoted, err := strconv.QuotedPrefix(rest)
				if err == nil {
					field, err = strconv.Unquote(quoted)
				}
				if err != nil {
					break
				}
				rest = rest[len(quoted):]
			} else {
				end := strings.IndexAny(rest, " \t")
				if end < 0 {
					end = len(rest)
				}
				field, rest = rest[:end], rest[end:]
			}
			glob, all := strings.CutPrefix(field, "all:")
			patterns = append(patterns, embedPattern{glob: glob, all: all})
		}
	}
	return patterns
}

// embedPatternMatches reports whether the embed pattern matches the slash path below, or a directory below lies under.
// A directory's subtree leaves out the names that open with a dot or an underscore, unless the pattern carries `all:`.
// A malformed pattern matches nothing, since the build that reads it fails anyway.
func embedPatternMatches(pattern embedPattern, below string) bool {
	for candidate := below; candidate != "."; candidate = path.Dir(candidate) {
		if matched, _ := path.Match(pattern.glob, candidate); !matched {
			continue
		}
		if candidate == below || pattern.all {
			return true
		}
		inside := strings.Split(strings.TrimPrefix(below, candidate+"/"), "/")
		if !slices.ContainsFunc(inside, func(name string) bool { return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") }) {
			return true
		}
	}
	return false
}

// writeOverlay writes the Go build overlay file that holds replace into dir and returns its path.
func writeOverlay(dir string, replace map[string]string) (string, error) {
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err != nil {
		return "", err
	}
	overlayPath := filepath.Join(dir, "overlay.json")
	return overlayPath, os.WriteFile(overlayPath, data, 0o600)
}

// testTargetsOf returns the module-relative package of each of tests as a `go test` target, sorted and without repeats.
func testTargetsOf(module goModule, tests []testreach.ChangedTest) []string {
	seen := map[string]bool{}
	for _, test := range tests {
		if rel := module.relativePackage(test.Package); rel == "." {
			seen["."] = true
		} else {
			seen["./"+rel] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// testNamesOf returns the name of each of tests, sorted and without repeats.
func testNamesOf(tests []testreach.ChangedTest) []string {
	seen := map[string]bool{}
	for _, test := range tests {
		seen[test.Name] = true
	}
	return slices.Sorted(maps.Keys(seen))
}

// testLabels returns each of tests as `<package>.<Name>`.
func testLabels(tests []testreach.ChangedTest) []string {
	labels := make([]string, len(tests))
	for i, test := range tests {
		labels[i] = test.Package + "." + test.Name
	}
	return labels
}
