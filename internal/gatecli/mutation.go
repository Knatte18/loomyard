// mutation.go runs the mutation report step a `--gate` key's reports list names, after the other report steps.
// It runs the mutation check of `lym gate mutate` over the worktree's diff.
// It writes every finding into its report and prints one summary line; nothing it meets changes the call's result.

package gatecli

import (
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

// mutationSite labels the slot holder record of the mutation report step.
const mutationSite = siteLabelPrefix + " mutation"

// mutationStep is one call's mutation report step.
// It reads the call through the reach step's view of it, whose map refresh it runs too, and the hub env module's config.
type mutationStep struct {
	reach  reachStep
	hubEnv envconfig.Config
}

// mutationStep returns the mutation report step of the call e runs.
func (e *execution) mutationStep() mutationStep {
	return mutationStep{reach: e.reachStep(), hubEnv: e.request.hubEnv}
}

// run writes the step's report into the worktree's gate directory and prints its summary line to stderr.
// red tells a call one of whose runs failed, and inHub a call inside a hub; either way the report then judges nothing.
// A report the step cannot write is logged at Warn, and the call's result stands.
func (s mutationStep) run(red, inHub bool) {
	path := mutationReportPath(s.reach.location)
	var report MutationReport
	switch {
	case red:
		report = MutationReport{RedGate: true, Note: noteRedGate}
	case !inHub:
		report = MutationReport{Note: noteNoHub}
	case !s.reach.cfg.HasGoCoverageMaps():
		report = MutationReport{Note: fmt.Sprintf(noteScoper, s.reach.cfg.Scoper)}
	default:
		report = s.answer()
	}
	report.GateKey = s.reach.gateKey
	report.Written = time.Now()
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), report); err != nil {
		logger.Warn("gate test: could not write the mutation report; the gate's result stands", "path", path, "cause", err)
		return
	}
	printLine(s.reach.stderr, report.SummaryLine(path))
}

// answer returns the report for the worktree's diff.
// The diff is from the merge base with the parent branch, or from HEAD in the prime.
// A task worktree with no parent branch gets a report that judges nothing.
func (s mutationStep) answer() MutationReport {
	r := s.reach
	var report MutationReport
	module, err := redundancyModule(r.cfg, r.root)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.ModuleDir = module.rel
	fabric, err := fabricengine.Open(r.location)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	prime, err := hubgeom.IsPrime(r.location)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.Prime = prime
	base, err := r.diffBase(fabric, prime)
	switch {
	case err != nil:
		report.Errors = append(report.Errors, err.Error())
		return report
	case base == "":
		report.Note = noteNoParent
		return report
	}
	report.Base = base

	found, err := mutationCall{
		ctx: r.ctx, cfg: r.cfg, location: r.location, root: r.root, module: module, fabric: fabric, prime: prime, base: base,
		request: mutateRequest{
			goBinary: r.goBinary, environ: r.environ, secondUnit: r.secondUnit, minuteUnit: r.minuteUnit, grace: r.grace,
			cappedParallel: memoryCappedParallel, stderr: r.stderr,
		},
		hubEnv: s.hubEnv, guardKilled: r.guardKilled, site: mutationSite,
	}.run()
	s.recordFindings(&report, module, found, err)
	return report
}

// recordFindings adds what a mutation call found to report, and the error the call ended with, if any.
// A call that ended in error records the error and its partial findings but no changed function or file hash,
// so the report never matches a commit and no review reads its counts as a judgment of that tree.
func (s mutationStep) recordFindings(report *MutationReport, module goModule, found mutationFindings, err error) {
	report.Errors = append(report.Errors, found.refreshErrors...)
	report.Mutants = found.outcome.counts
	report.Survivors, report.Unjudged, report.Uncovered = found.outcome.survivors, found.outcome.unjudged, found.outcome.uncovered
	report.NotJudged = append(found.notJudged, found.outcome.notJudged...)
	if err != nil {
		if s.reach.ctx.Err() != nil {
			err = errors.New(reasonCancelled)
		}
		report.Errors = append(report.Errors, err.Error())
		return
	}
	for _, fn := range found.funcs {
		pkg := module.relativePackage(fn.Key.Package)
		report.Functions = append(report.Functions, MutationFunction{
			Package: pkg, File: path.Join(packageDirOf(module.rel, pkg), fn.Key.File), Func: fn.Key.Func, Hash: fn.Hash, New: fn.New,
		})
	}
	for _, pkg := range changedPackages(module, found.funcs) {
		hashes, err := goFileHashes(workingTree{root: s.reach.root}, packageDirOf(module.rel, pkg))
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("hash the Go files of %s: %v", pkg, err))
			continue
		}
		if report.FileHashes == nil {
			report.FileHashes = map[string]map[string]string{}
		}
		report.FileHashes[pkg] = hashes
	}
}
