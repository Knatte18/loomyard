// redundancyreport.go declares the report the redundancy report step writes into the worktree's gate directory.
// It reads and writes the report through internal/state and matches a report against a tree.

package gatecli

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// redundancyReportFileName is the redundancy report's file name in the worktree's gate directory.
const redundancyReportFileName = "redundancy.json"

// Verdicts the redundancy report gives a test.
const (
	// RedundancyContained marks a test the tests left after the greedy pass cover block for block.
	RedundancyContained = "contained"
	// RedundancyUnique marks a judged test that covers a block no test left after the greedy pass covers.
	RedundancyUnique = "unique"
	// RedundancyNotJudged marks a test the step did not judge, with the reason.
	RedundancyNotJudged = "not judged"
)

// RedundancyReport is what one redundancy report step of `lym gate test` found for the worktree's diff.
// The step writes it, and a reader matches it against a later tree before trusting its verdicts.
type RedundancyReport struct {
	// Written is when the step wrote the report.
	Written time.Time `json:"written"`
	// GateKey is the gate.yaml key whose call wrote the report.
	GateKey string `json:"gate_key"`
	// Note says why the step judged nothing, such as a red gate or no parent branch; empty when it judged.
	Note string `json:"note,omitempty"`
	// RedGate is set when a tier of the call was red, so the step judged nothing.
	RedGate bool `json:"red_gate,omitempty"`
	// Base is the merge base with the parent branch the step diffed from.
	Base string `json:"base,omitempty"`
	// ModuleDir is the worktree-relative slash path of the Go module whose packages the report names.
	ModuleDir string `json:"module_dir,omitempty"`
	// Errors are the failures the step met, one line each; none changes the gate's result.
	Errors []string `json:"errors,omitempty"`
	// TestHashes maps each package, by module-relative path, to the function hashes of its tests.
	// It holds each test the step took, judged or not, by name.
	TestHashes map[string]map[string]string `json:"test_hashes,omitempty"`
	// FileHashes maps each judged package to the ContentHash of each of its Go files, by name.
	FileHashes map[string]map[string]string `json:"file_hashes,omitempty"`
	// Packages holds one line per package with a test the step went on to judge.
	Packages []RedundancyPackage `json:"packages,omitempty"`
	// Tests holds one line per test the step took, sorted by file and then name.
	Tests []RedundancyTest `json:"tests,omitempty"`
}

// RedundancyPackage is one package's line of a redundancy report: whether its coverage map let its tests be judged.
type RedundancyPackage struct {
	// Package is the module-relative slash path of the package.
	Package string `json:"package"`
	// Judged is set when the package's map let its tests be judged; Reason says why not otherwise.
	Judged bool   `json:"judged"`
	Reason string `json:"reason,omitempty"`
	// MapCommit and MapBuiltAt are the commit and build time of the map a judged package was judged against.
	// MapAge is the map's age when the report was written.
	MapCommit  string    `json:"map_commit,omitempty"`
	MapBuiltAt time.Time `json:"map_built_at"`
	MapAge     string    `json:"map_age,omitempty"`
	// Drift names the Go files whose text at the merge base hashes otherwise than the map records.
	// A file on one side alone counts.
	// It shows how far the map lags the run's base, never the run's own edits.
	Drift []string `json:"drift,omitempty"`
}

// RedundancyTest is one test's line of a redundancy report.
type RedundancyTest struct {
	// Package is the module-relative slash path of the test's package, and File the worktree-relative path of its file.
	Package string `json:"package"`
	Name    string `json:"name"`
	File    string `json:"file"`
	// Verdict is RedundancyContained, RedundancyUnique or RedundancyNotJudged.
	Verdict string `json:"verdict"`
	// Covering names, for a contained test, the tests that together cover its blocks, largest overlap first.
	Covering []string `json:"covering,omitempty"`
	// Reason says why a test is not judged.
	Reason string `json:"reason,omitempty"`
}

// redundancyReportPath returns the path of the redundancy report in the gate directory of the worktree at location.
func redundancyReportPath(location *lymcwd.Location) string {
	return filepath.Join(gateslot.WaitDir(location.AnchorPath()), redundancyReportFileName)
}

// ReadRedundancyReport reads the redundancy report of the worktree at location, and reports whether one exists.
// A key the record does not declare is ignored, never refused.
func ReadRedundancyReport(location *lymcwd.Location) (RedundancyReport, bool, error) {
	return readRedundancyReport(redundancyReportPath(location))
}

func readRedundancyReport(path string) (RedundancyReport, bool, error) {
	return state.ReadJSON[RedundancyReport](path, lock.TrackedFileLockPath(path, ".lock"))
}

// writeRedundancyReport replaces the report at path with report, atomically.
// It keeps each key of the replaced record that report does not declare.
func writeRedundancyReport(path string, report RedundancyReport) error {
	return state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), report)
}

// RedundancyReportMatches reports whether report still holds for the tree of the commit rev of the worktree at location.
// It holds when the step's selection over rev returns the tests report records with the same function hashes.
// The selection is taken against the current merge base with the parent branch.
// It also holds only when the Go files of each package report judged hash at rev as report records.
// A report that judged nothing, and a worktree with no parent branch, never match.
func RedundancyReportMatches(location *lymcwd.Location, report RedundancyReport, rev string) (bool, error) {
	if report.Note != "" || report.RedGate {
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
	tests, err := changedTests(tree, fabric, base, report.ModuleDir, changed)
	if err != nil {
		return false, err
	}
	return report.matches(tree, tests)
}

// matches reports whether tests, the step's selection over tree, are the tests the report records with the same hashes,
// and the Go files of each package the report judged hash in tree as the report records.
func (r RedundancyReport) matches(tree redundancyTree, tests []redundancyCandidate) (bool, error) {
	equal := func(a, b map[string]string) bool { return maps.Equal(a, b) }
	if !maps.EqualFunc(testHashesOf(tests), r.TestHashes, equal) {
		return false, nil
	}
	for pkg, recorded := range r.FileHashes {
		current, err := goFileHashes(tree, packageDirOf(r.ModuleDir, pkg))
		if err != nil {
			return false, err
		}
		if !maps.Equal(current, recorded) {
			return false, nil
		}
	}
	return true, nil
}

// retries reports whether a later call on the same tree runs the step again rather than keep the report.
// It does for a report that judged nothing, holds an error line or holds a package with no usable map.
// It does also for a report that holds a test the spent budget or a cancelled call left unrun.
func (r RedundancyReport) retries() bool {
	return r.Note != "" || r.RedGate || len(r.Errors) > 0 ||
		slices.ContainsFunc(r.Packages, func(p RedundancyPackage) bool { return !p.Judged }) ||
		slices.ContainsFunc(r.Tests, func(t RedundancyTest) bool {
			return t.Reason == reasonBudgetSpent || t.Reason == reasonCancelled
		})
}

// summaryLine returns the stderr line that names the report's counts, or its note, and its path.
func (r RedundancyReport) summaryLine(path string) string {
	if r.Note != "" {
		return fmt.Sprintf("gate test: redundancy report: %s; written to %s", r.Note, path)
	}
	counts := map[string]int{}
	for _, test := range r.Tests {
		counts[test.Verdict]++
	}
	return fmt.Sprintf("gate test: redundancy report: %d contained, %d unique, %d not judged, %d errors; written to %s",
		counts[RedundancyContained], counts[RedundancyUnique], counts[RedundancyNotJudged], len(r.Errors), path)
}
