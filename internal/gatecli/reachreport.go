// reachreport.go declares the report the reach report step writes into the worktree's gate directory.
// It renders the note the step prints from that report.

package gatecli

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/modfile"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// ReachReportFileName is the reach report's file name in the worktree's gate directory.
const ReachReportFileName = "reach.json"

// maxReachTestsPerFunction caps the tests a changed function's line keeps; the rest are counted in MoreTests.
const maxReachTestsPerFunction = 5

// reachNoteLineCap caps the lines of the note the step prints; a longer note ends with a line naming the report.
const reachNoteLineCap = 40

// ReachReport is what one reach report step of `lym gate test` found for the worktree's diff.
type ReachReport struct {
	// Written is when the step wrote the report.
	Written time.Time `json:"written"`
	// GateKey is the gate.yaml key whose call wrote the report.
	GateKey string `json:"gate_key"`
	// Note says why the step judged nothing, such as a red gate or no coverage map; empty when it judged.
	Note string `json:"note,omitempty"`
	// RedGate is set when a tier of the call was red, so the step judged nothing.
	RedGate bool `json:"red_gate,omitempty"`
	// Prime is set for a report written in the hub's prime, whose base is HEAD and which never matches a review's HEAD.
	Prime bool `json:"prime,omitempty"`
	// Base is the commit the step diffed from: the merge base with the parent branch, or HEAD in the prime.
	Base string `json:"base,omitempty"`
	// ModuleDir is the worktree-relative slash path of the Go module whose packages the report names.
	ModuleDir string `json:"module_dir,omitempty"`
	// Errors are the failures the step met, one line each; none changes the gate's result.
	Errors []string `json:"errors,omitempty"`
	// Maps are the coverage maps the answer read, those of the packages it names.
	Maps []ReachMap `json:"maps,omitempty"`
	// Unrefreshed holds one line per package left unrefreshed, with its reason and its way forward.
	Unrefreshed []string `json:"unrefreshed,omitempty"`
	// Functions holds the changed functions, sorted by package, file and name.
	Functions []ReachFunction `json:"functions,omitempty"`
	// FileHashes maps each package the answer names, by module-relative path, to the ContentHash of each Go file, by name.
	// The Go files are those of the package of a changed function and of each package whose tests the report names.
	FileHashes map[string]map[string]string `json:"file_hashes,omitempty"`
}

// ReachMap is one coverage map a reach report read.
type ReachMap struct {
	// Package is the module-relative slash path of the map's package.
	Package string `json:"package"`
	// Commit and BuiltAt are the commit the map was built from and when.
	Commit  string    `json:"commit"`
	BuiltAt time.Time `json:"built_at"`
	// Worktree is set for a map read from the worktree's own maps, and clear for one of the hub's shared maps.
	Worktree bool `json:"worktree,omitempty"`
}

// ReachFunction is one changed function with the tests that reach it.
type ReachFunction struct {
	// Package is the module-relative slash path of the function's package, and File the worktree-relative path of its file.
	Package string `json:"package"`
	File    string `json:"file"`
	// Func is the function's name, `Type.Method` for a method.
	Func string `json:"func"`
	// Hash is the ContentHash of the function's declaration text in the worktree.
	Hash string `json:"hash"`
	// New is set when the merge base holds no function it matches.
	New bool `json:"new,omitempty"`
	// Tests are the tests that reach the function, its own package's first, fewest reached functions first, at most five.
	Tests []ReachTest `json:"tests,omitempty"`
	// MoreTests counts the reaching tests past the five kept.
	MoreTests int `json:"more_tests,omitempty"`
	// Caveats say why a function no test reaches may still be run by a test the maps cannot see.
	Caveats []string `json:"caveats,omitempty"`
}

// ReachTest is one test that reaches a function.
type ReachTest struct {
	// Package is the module-relative slash path of the test's package, and Test the test's name.
	Package string `json:"package"`
	Test    string `json:"test"`
}

// reachReportPath returns the path of the reach report in the gate directory of the worktree at location.
func reachReportPath(location *lymcwd.Location) string {
	return filepath.Join(gateslot.WaitDir(location.AnchorPath()), ReachReportFileName)
}

// ReadReachReport reads the reach report of the worktree at location, and reports whether one exists.
// A key the record does not declare is ignored, never refused.
func ReadReachReport(location *lymcwd.Location) (ReachReport, bool, error) {
	reportPath := reachReportPath(location)
	return state.ReadJSON[ReachReport](reportPath, lock.TrackedFileLockPath(reportPath, ".lock"))
}

// ReachReportMatches reports whether report still holds for the tree of the commit rev of the worktree at location.
// A report that judged nothing, one marked red_gate or prime, and a worktree with no parent branch never match.
// Otherwise it holds when the functions rev changes since the current merge base are those the report records.
// Their hashes must match too.
// The Go files of each package the report names must also hash at rev as the report records.
func ReachReportMatches(location *lymcwd.Location, report ReachReport, rev string) (bool, error) {
	if report.Note != "" || report.RedGate || report.Prime {
		return false, nil
	}
	base, found, err := parentMergeBase(location)
	if err != nil || !found {
		return false, err
	}
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return false, err
	}
	changed, err := fabric.ChangedPathsBothSides(base, rev)
	if err != nil {
		return false, err
	}
	tree := commitTree{fabric: fabric, rev: rev}
	goMod, found, err := tree.file(path.Join(report.ModuleDir, "go.mod"))
	if err != nil || !found {
		return false, err
	}
	module := goModule{rel: report.ModuleDir, path: modfile.ModulePath(goMod)}
	if module.path == "" {
		return false, nil
	}
	funcs, err := changedFuncsAt(tree, fabric, base, module, changed)
	if err != nil {
		return false, err
	}
	current := make([]string, len(funcs))
	for i, fn := range funcs {
		pkg := module.relativePackage(fn.Key.Package)
		current[i] = reachFunctionIdentity(pkg, path.Join(packageDirOf(module.rel, pkg), fn.Key.File), fn.Key.Func, fn.Hash, fn.New)
	}
	recorded := make([]string, len(report.Functions))
	for i, fn := range report.Functions {
		recorded[i] = reachFunctionIdentity(fn.Package, fn.File, fn.Func, fn.Hash, fn.New)
	}
	slices.Sort(current)
	slices.Sort(recorded)
	if !slices.Equal(current, recorded) {
		return false, nil
	}
	for pkg, hashes := range report.FileHashes {
		atRev, err := goFileHashes(tree, packageDirOf(report.ModuleDir, pkg))
		if err != nil {
			return false, err
		}
		if !maps.Equal(atRev, hashes) {
			return false, nil
		}
	}
	return true, nil
}

// ReachPendingOf returns the source of the pending reach report of the worktree at location, asked for a gate key.
// The report answers its note and its unreached functions, each by file and name with its hash.
// It does so when the call of the asked key wrote it and it still matches the worktree's HEAD.
// A report of another key answers false.
// So does one that judged nothing, one with no unreached function and one that no longer matches HEAD.
// A report that cannot be read or matched answers false with a Warn naming the cause and no error.
// The note is advisory and never stops a gate.
func ReachPendingOf(location *lymcwd.Location) func(gateKey string) (verifytree.ReachPending, bool, error) {
	return func(gateKey string) (verifytree.ReachPending, bool, error) {
		skip := func(step string, cause error) (verifytree.ReachPending, bool, error) {
			logger.Warn("gatecli: the reach turn is skipped", "step", step, "path", reachReportPath(location), "cause", cause)
			return verifytree.ReachPending{}, false, nil
		}
		report, found, err := ReadReachReport(location)
		if err != nil {
			return skip("read the reach report", err)
		}
		if !found || report.GateKey != gateKey || report.Note != "" || report.RedGate || report.Prime {
			return verifytree.ReachPending{}, false, nil
		}
		fabric, err := fabricengine.Open(location)
		if err != nil {
			return skip("open fabric", err)
		}
		head, err := fabric.HeadSHA()
		if err != nil {
			return skip("read HEAD", err)
		}
		matches, err := ReachReportMatches(location, report, head)
		if err != nil {
			return skip("match the reach report against HEAD", err)
		}
		if !matches {
			return verifytree.ReachPending{}, false, nil
		}
		unreached := map[string]string{}
		for _, fn := range report.Functions {
			if fn.unreached() {
				unreached[fn.File+"#"+fn.Func] = fn.Hash
			}
		}
		if len(unreached) == 0 {
			return verifytree.ReachPending{}, false, nil
		}
		note := strings.Join(report.NoteLines(reachReportPath(location)), "\n")
		return verifytree.ReachPending{Note: note, Functions: unreached}, true, nil
	}
}

// reachFunctionIdentity returns the text that tells one changed function and its hash from every other.
func reachFunctionIdentity(pkg, file, name, hash string, isNew bool) string {
	return fmt.Sprintf("%s|%s|%s|%s|%t", pkg, file, name, hash, isNew)
}

// NoteLines renders the note the reach step prints, at most reachNoteLineCap lines, from a report written at path.
// A report that judged nothing renders its note alone.
// Otherwise the note lists each changed function with its tests and the commit and age of each map read.
// It lists the changed functions no test reaches with their caveats, the unrefreshed packages and the errors.
// A note over the cap is cut and ends with a line naming path.
func (r ReachReport) NoteLines(path string) []string {
	if r.Note != "" {
		return []string{fmt.Sprintf("gate test: reach report: %s; written to %s", r.Note, path)}
	}
	var lines []string
	var unreached []ReachFunction
	for _, fn := range r.Functions {
		if fn.unreached() {
			unreached = append(unreached, fn)
			continue
		}
		lines = append(lines, "reach: "+fn.label()+": "+fn.testList())
	}
	for _, m := range r.Maps {
		lines = append(lines, fmt.Sprintf("reach: map of %s from commit %s, %s old%s",
			m.Package, m.Commit, r.Written.Sub(m.BuiltAt).Round(time.Minute), worktreeMark(m.Worktree)))
	}
	for _, fn := range unreached {
		lines = append(lines, "reach: no mapped test reaches "+fn.label())
		for _, caveat := range fn.Caveats {
			lines = append(lines, "reach:   "+caveat)
		}
	}
	for _, line := range r.Unrefreshed {
		lines = append(lines, "reach: "+line)
	}
	for _, line := range r.Errors {
		lines = append(lines, "reach: error: "+line)
	}
	if len(lines) == 0 {
		lines = []string{"reach: the diff changes no function of the module"}
	}
	if len(lines) > reachNoteLineCap {
		lines = append(lines[:reachNoteLineCap-1], "reach: note cut; the rest is in "+path)
	}
	for i := range lines {
		lines[i] = "gate test: " + lines[i]
	}
	return lines
}

// worktreeMark names a map read from the worktree's own maps.
func worktreeMark(worktree bool) string {
	if worktree {
		return " (the worktree's own)"
	}
	return ""
}

// unreached reports whether no mapped test reaches the function.
func (f ReachFunction) unreached() bool {
	return len(f.Tests) == 0 && f.MoreTests == 0
}

// label names the function by its package and name, and the new ones as such.
func (f ReachFunction) label() string {
	label := f.Package + "." + f.Func
	if f.New {
		label += " (new)"
	}
	return label
}

// testList renders the function's tests, own package first as the report orders them, and the count of the rest.
func (f ReachFunction) testList() string {
	names := make([]string, len(f.Tests))
	for i, test := range f.Tests {
		names[i] = test.Test
		if test.Package != f.Package {
			names[i] = test.Package + "." + test.Test
		}
	}
	list := strings.Join(names, ", ")
	if f.MoreTests > 0 {
		list += fmt.Sprintf(", and %d more", f.MoreTests)
	}
	return list
}
