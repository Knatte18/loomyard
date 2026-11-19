// mutationreport.go declares the report the mutation report step writes into the worktree's gate directory.
// It renders the step's summary line and the lines a review reads, and matches a report against a commit.

package gatecli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// MutationReportFileName is the mutation report's file name in the worktree's gate directory.
const MutationReportFileName = "mutation.json"

// mutationReviewLineCap caps the lines a review reads of a report; a longer list ends with a line naming the report.
const mutationReviewLineCap = 40

// MutationReport is what one mutation report step of `lym gate test` found for the worktree's diff.
// Its FileHashes cover the Go files of the changed packages only,
// so a change to another file their tests read, such as testdata, leaves the report matching HEAD.
type MutationReport struct {
	// Written is when the step wrote the report.
	Written time.Time `json:"written"`
	// GateKey is the gate.yaml key whose call wrote the report.
	GateKey string `json:"gate_key"`
	// Note says why the step judged nothing, such as a red gate or no parent branch; empty when it judged.
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
	// Functions are the functions the diff changes.
	Functions []MutationFunction `json:"functions,omitempty"`
	// FileHashes maps the package of each changed function to the ContentHash of each of its Go files.
	// Packages are keyed by module-relative path, and files by name.
	FileHashes map[string]map[string]string `json:"file_hashes,omitempty"`
	// Mutants counts the mutants run, killed, survived and unjudged, those unbuildable and those the cap left out.
	Mutants MutationCounts `json:"mutants"`
	// Survivors are the mutants every covering test passed.
	Survivors []MutationSurvivor `json:"survivors,omitempty"`
	// Unjudged are the mutants left without a verdict, with why.
	Unjudged []MutationUnjudgedMutant `json:"unjudged_mutants,omitempty"`
	// Uncovered are the changed blocks no test of their package's map covers.
	Uncovered []MutationUncoveredBlock `json:"uncovered_blocks,omitempty"`
	// NotJudged are the changed packages the step judged nothing of, with why.
	NotJudged []MutationNotJudged `json:"not_judged,omitempty"`
}

// MutationFunction is one function a mutation report's diff changes, with its text hash.
type MutationFunction struct {
	// Package is the module-relative slash path of the function's package, and File the worktree-relative path of its file.
	Package string `json:"package"`
	File    string `json:"file"`
	// Func is the function's name, `Type.Method` for a method.
	Func string `json:"func"`
	// Hash is the ContentHash of the function's declaration text in the worktree.
	Hash string `json:"hash"`
	// New is set when the base holds no function it matches.
	New bool `json:"new,omitempty"`
}

// mutationReportPath returns the path of the mutation report in the gate directory of the worktree at location.
func mutationReportPath(location *lymcwd.Location) string {
	return filepath.Join(gateslot.WaitDir(location.AnchorPath()), MutationReportFileName)
}

// ReadMutationReport reads the mutation report of the worktree at location, and reports whether one exists.
// A key the record does not declare is ignored, never refused.
func ReadMutationReport(location *lymcwd.Location) (MutationReport, bool, error) {
	reportPath := mutationReportPath(location)
	return state.ReadJSON[MutationReport](reportPath, lock.TrackedFileLockPath(reportPath, ".lock"))
}

// MutationReportMatches reports whether report still holds for the tree of the commit rev of the worktree at location.
// A report that judged nothing, one marked red_gate or prime, and a worktree with no parent branch never match.
// Otherwise it holds when the functions rev changes since the current merge base are those the report records.
// Their hashes must match too, and so must the hashes of the Go files of their packages at rev.
// A change to a non-Go file those packages' tests read, such as testdata, leaves the report matching.
func MutationReportMatches(location *lymcwd.Location, report MutationReport, rev string) (bool, error) {
	if report.Note != "" || report.RedGate || report.Prime {
		return false, nil
	}
	recorded := make([]string, len(report.Functions))
	for i, fn := range report.Functions {
		recorded[i] = reachFunctionIdentity(fn.Package, fn.File, fn.Func, fn.Hash, fn.New)
	}
	return changedFunctionsMatch(location, report.ModuleDir, recorded, report.FileHashes, rev)
}

// SummaryLine returns the one stderr line the step prints for a report written at path: its note, or its counts.
func (r MutationReport) SummaryLine(path string) string {
	if r.Note != "" {
		return fmt.Sprintf("gate test: mutation report: %s; written to %s", r.Note, path)
	}
	return fmt.Sprintf("gate test: mutation report: %s; written to %s", r.counts(), path)
}

// ReviewLines renders a report written at path as a review reads it, at most mutationReviewLineCap lines.
// The first line holds the counts.
// A line per survivor follows with its file, line, operator, function and possible killers.
// A list over the cap is cut and ends with a line naming path.
func (r MutationReport) ReviewLines(path string) []string {
	lines := []string{"mutation: " + r.counts()}
	for _, survivor := range r.Survivors {
		lines = append(lines, fmt.Sprintf("mutation: survivor %s:%d, %s in %s; %s",
			survivor.File, survivor.Line, survivor.Operator, survivor.Function, survivor.killers()))
	}
	if len(lines) > mutationReviewLineCap {
		lines = append(lines[:mutationReviewLineCap-1], "mutation: list cut; the rest is in "+path)
	}
	return lines
}

// counts renders the mutants run, killed, survived and unjudged.
// It also renders the uncovered changed blocks and the packages not judged.
// The count of error lines follows when the report holds any.
func (r MutationReport) counts() string {
	text := fmt.Sprintf("%d mutants run, %d killed, %d survived, %d unjudged, %d uncovered changed blocks, %d packages not judged",
		r.Mutants.Run, r.Mutants.Killed, r.Mutants.Survived, r.Mutants.Unjudged, len(r.Uncovered), len(r.NotJudged))
	if len(r.Errors) > 0 {
		text += fmt.Sprintf(", %d errors", len(r.Errors))
	}
	return text
}

// killers renders the tests of other packages that may kill the survivor, or says that none reaches its function.
func (s MutationSurvivor) killers() string {
	if len(s.PossibleKillers) == 0 {
		return "no test of another package reaches its function"
	}
	list := "possible killers: " + strings.Join(s.PossibleKillers, ", ")
	if s.MorePossibleKillers > 0 {
		list += fmt.Sprintf(", and %d more", s.MorePossibleKillers)
	}
	return list
}
