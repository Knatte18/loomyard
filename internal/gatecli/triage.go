// triage.go builds the triage lines a failing call's failure report holds under each failed test.
// A triage line says whether the test reaches a function the call's change set touches, read from the coverage maps.

package gatecli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// The caps and phrases of the triage lines.
const (
	// triageMaxFunctions caps the changed functions one triage line names; the count of the rest follows.
	triageMaxFunctions = 5
	// triageNotInMaps is the line of a failed test no coverage map holds.
	triageNotInMaps = "not in the coverage maps"
	// triageReachesNone says a mapped test reaches none of the call's changes.
	triageReachesNone = "reaches none of this run's changes"
	// triageReasonNoParent and the two reasons after it say why a mapped test's line has no change set to judge against.
	triageReasonNoParent        = "no parent branch"
	triageReasonMergeBaseFailed = "the merge base read failed"
	triageReasonChangeSetFailed = "the changed functions could not be read"
	// triageLastGreenPlaceholder stands in the line's `lym gate reach` command for the commit the reader fills in.
	triageLastGreenPlaceholder = "<last green commit>"
)

// triageLookup answers, for a failed test, the triage lines of the failure report.
// It reads one coverage map reader and the change set of one call.
type triageLookup struct {
	reader *testreach.Reader
	// changed holds the keys of the functions the call changes; it is empty when noChangeReason is set.
	changed []testcover.FuncKey
	// noChangeReason says why the call has no change set; it is empty when the call has one.
	noChangeReason string
	// relative maps an import path to the module-relative path the maps are keyed by.
	// newTriageLookup sets the identity, for a caller whose paths are module-relative already.
	relative func(importPath string) string
}

// newTriageLookup returns the triage lookup of one call, and false when open fails, so the report carries no triage line.
// open opens the coverage maps.
// mergeBase returns the merge base with the parent branch and whether the call has a parent branch.
// changed returns the functions the call changes against that base.
// A call with no parent branch, a failed merge base read or a failed read of the changed functions has no change set:
// its lines then say so and why.
func newTriageLookup(
	open func() (*testreach.Reader, error),
	mergeBase func() (string, bool, error),
	changed func(base string) ([]testreach.ChangedFunc, error),
) (triageLookup, bool) {
	reader, err := open()
	if err != nil {
		logger.Warn("gate test: could not read the coverage maps; the failure report carries no triage line", "cause", err)
		return triageLookup{}, false
	}
	lookup := triageLookup{reader: reader, relative: func(importPath string) string { return importPath }}
	base, found, err := mergeBase()
	switch {
	case err != nil:
		logger.Warn("gate test: could not read the merge base for the triage lines", "cause", err)
		lookup.noChangeReason = triageReasonMergeBaseFailed
	case !found:
		lookup.noChangeReason = triageReasonNoParent
	default:
		funcs, err := changed(base)
		if err != nil {
			logger.Warn("gate test: could not find the changed functions for the triage lines", "base", base, "cause", err)
			lookup.noChangeReason = triageReasonChangeSetFailed
			break
		}
		for _, fn := range funcs {
			lookup.changed = append(lookup.changed, fn.Key)
		}
	}
	return lookup, true
}

// lines returns the triage lines of the failed test test of the package pkg, each opening with gateslot.TriageLinePrefix.
// A subtest path is cut at its first slash, since a map holds top-level tests.
// A test no map holds gets one line saying so.
// A mapped test gets one line naming the changed functions it reaches.
// The line names at most triageMaxFunctions of them, then the count of the rest.
// A mapped test reaching none gets a line saying so, then a line holding the command that finds the landing to blame.
// With no change set, a mapped test gets one line saying why none exists.
// Each line about a mapped test names the commit of the map that holds it.
func (l triageLookup) lines(pkg, test string) []string {
	pkg = l.relative(pkg)
	top, _, _ := strings.Cut(test, "/")
	reached, source, held := l.reader.FuncsReachedBy(pkg, top, l.changed)
	if !held {
		return []string{gateslot.TriageLinePrefix + triageNotInMaps}
	}
	switch {
	case l.noChangeReason != "":
		return []string{fmt.Sprintf("%sno change set exists, %s; map commit %s", gateslot.TriageLinePrefix, l.noChangeReason, source.Commit)}
	case len(reached) == 0:
		return []string{
			fmt.Sprintf("%s%s; map commit %s", gateslot.TriageLinePrefix, triageReachesNone, source.Commit),
			fmt.Sprintf("%slym gate reach --test %s.%s --since %s", gateslot.TriageLinePrefix, pkg, top, triageLastGreenPlaceholder),
		}
	}
	names := make([]string, 0, min(len(reached), triageMaxFunctions))
	for _, key := range reached[:min(len(reached), triageMaxFunctions)] {
		names = append(names, l.relative(key.Package)+"."+key.Func)
	}
	named := strings.Join(names, ", ")
	if rest := len(reached) - len(names); rest > 0 {
		named += fmt.Sprintf(" and %d more", rest)
	}
	return []string{fmt.Sprintf("%sreaches changed functions: %s; map commit %s", gateslot.TriageLinePrefix, named, source.Commit)}
}

// addTriage fills the triage lines of the failed tests of the call's failed runs.
// It does so only in a hub whose scoper is go, and only when a run failed a test.
// A map or change set it cannot read leaves the report without triage lines.
func (e *execution) addTriage(inHub bool) {
	if !inHub || e.cfg.Scoper != "go" || !slices.ContainsFunc(e.failed, func(run failedRun) bool {
		return slices.ContainsFunc(run.failures, runFailure.isTest)
	}) {
		return
	}
	module, err := redundancyModule(e.cfg, e.request.root)
	if err != nil {
		logger.Warn("gate test: no Go module holds the coverage maps; the failure report carries no triage line", "cause", err)
		return
	}
	prime, err := hubgeom.IsPrime(e.location)
	if err != nil {
		logger.Warn("gate test: could not tell whether the worktree is the prime; the failure report carries no triage line", "cause", err)
		return
	}
	step := reachStep{location: e.location, root: e.request.root}
	lookup, ok := newTriageLookup(
		func() (*testreach.Reader, error) {
			return testreach.Open(reachModuleOf(module), testcover.Dir(fabricengine.BoltDir(e.location.HubPath)), step.worktreeMapsDir(prime))
		},
		func() (string, bool, error) {
			if prime {
				return "", false, nil
			}
			return parentMergeBase(e.location)
		},
		func(base string) ([]testreach.ChangedFunc, error) {
			fabric, err := fabricengine.Open(e.location)
			if err != nil {
				return nil, err
			}
			paths, err := impactset.ChangedPaths(e.ctx, e.request.root, base)
			if err != nil {
				return nil, err
			}
			return changedFuncsAt(workingTree{root: e.request.root}, fabric, base, module, paths)
		},
	)
	if !ok {
		return
	}
	lookup.relative = module.relativePackage
	for i := range e.failed {
		for j := range e.failed[i].failures {
			if failure := &e.failed[i].failures[j]; failure.isTest() {
				failure.triage = lookup.lines(failure.pkg, failure.test)
			}
		}
	}
}
