// repo.go is planglyph's one call site for quarry.Open and every quarry.Repo query method (TOC,
// Glyphs, Resolve, Expand) — the package's entry points into quarry.Repo — and aliases the finding types planindex declares.
//
// Beside openRepo and resolveTargets, this file exports four query wrappers — TOC, Glyphs,
// Resolve and Expand — each taking worktreeRoot plus that verb's own argument, opening the
// repository and delegating to the matching quarry.Repo method unchanged. internal/quarrycli calls
// these rather than importing the facade directly, per the package-ownership-seam Shared Decision.

package planglyph

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// ErrQuarryUnavailable is planindex's error of the same name: quarry failing to answer at all, distinct from any per-target verdict, matched with errors.Is under either name.
var ErrQuarryUnavailable = planindex.ErrQuarryUnavailable

// Severity is planindex's closed vocabulary a Finding's own Severity is drawn from.
type Severity = planindex.Severity

const (
	// SeverityBlocking marks a finding that fails the gate it is reported against.
	SeverityBlocking = planindex.SeverityBlocking
	// SeverityInformational marks a finding surfaced for visibility that never fails a gate.
	SeverityInformational = planindex.SeverityInformational
)

// Finding is planindex's finding type, which every gate of this package reports.
type Finding = planindex.Finding

// fromValidationError converts v into a Finding stamped SeverityBlocking — the severity every
// planparser check reports today, per the plan's blocking-policy Shared Decision.
func fromValidationError(v planparser.ValidationError) Finding {
	return Finding{Check: v.Check, Card: v.Card, Detail: v.Detail, Severity: SeverityBlocking, Ref: v.Ref}
}

// openRepo opens a quarry.Repo rooted at worktreeRoot, wrapping any error with ErrQuarryUnavailable
// so a caller distinguishes an infrastructure failure with errors.Is rather than by string
// matching. It is this package's one call to quarry.Open.
func openRepo(worktreeRoot string) (*quarry.Repo, error) {
	repo, err := quarry.Open(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %v", ErrQuarryUnavailable, worktreeRoot, err)
	}
	return repo, nil
}

// resolveTargets resolves every entry of targets against repo, positionally, wrapping any error
// with ErrQuarryUnavailable for the same reason openRepo does.
// It is this package's one call to (*quarry.Repo).Resolve, which is exactly why the coverage guard lives here and nowhere else: every consumer of a batched Resolve answer (statusFindings, createFindings, DetectDrift's post-repair revalidation, DoneChecks) either iterates the RESULTS slice or looks results up by key with a silent skip on a miss, so an answer covering fewer targets than asked would silently exempt the uncovered targets from the whole resolve-backed pass and the plan would read cleaner than it is.
// The package already guards its two other batched quarry boundaries against the same positional-contract breach (quarry.Name's length guard in CanonicalizeHandles, the per-key guard in doneCheckVerdicts, R5-6);
// this closes the last one (crucible round fable-high-r10, F2).
func resolveTargets(repo *quarry.Repo, targets []string) ([]quarry.ResolveResult, error) {
	results, err := repo.Resolve(targets)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve: %v", ErrQuarryUnavailable, err)
	}
	if err := ensureResolveCoverage(targets, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ensureResolveCoverage reports the ErrQuarryUnavailable-wrapped infrastructure error resolveTargets
// returns when a batched Resolve answer does not carry exactly one result per target — quarry's own
// positional contract not holding at a boundary this package cannot see inside. It is split out of
// resolveTargets so the guard is reachable from a unit test: a real quarry.Repo always satisfies
// the contract, so no test through one can produce the breach.
func ensureResolveCoverage(targets []string, results []quarry.ResolveResult) error {
	if len(results) != len(targets) {
		return fmt.Errorf("%w: resolve returned %d result(s) for %d target(s)", ErrQuarryUnavailable, len(results), len(targets))
	}
	return nil
}

// TOC opens a quarry.Repo rooted at worktreeRoot and answers a table-of-contents query for target
// under the zero quarry.TOCOptions — the same default the lyx quarry toc verb reports. It returns
// quarry's own answer and error unchanged, so a caller distinguishes an infrastructure failure
// from a negative query answer with errors.Is(err, ErrQuarryUnavailable) exactly as openRepo's
// other callers do.
func TOC(worktreeRoot, target string) (quarry.DirAnswer, error) {
	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return quarry.DirAnswer{}, err
	}
	return repo.TOC(target, quarry.TOCOptions{})
}

// Glyphs opens a quarry.Repo rooted at worktreeRoot and answers a glyphs query for target under
// quarry.GlyphsOptions, the frozen preset (*quarry.Repo).Glyphs already queries under. It returns
// quarry's own answer and error unchanged.
func Glyphs(worktreeRoot, target string) (quarry.GlyphsAnswer, error) {
	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return quarry.GlyphsAnswer{}, err
	}
	return repo.Glyphs(target)
}

// Resolve opens a quarry.Repo rooted at worktreeRoot and resolves every entry of targets,
// positionally, via resolveTargets. It returns quarry's own result slice and error unchanged.
func Resolve(worktreeRoot string, targets []string) ([]quarry.ResolveResult, error) {
	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil, err
	}
	return resolveTargets(repo, targets)
}

// Expand opens a quarry.Repo rooted at worktreeRoot and answers an expand query for target. It
// returns quarry's own answer and error unchanged.
func Expand(worktreeRoot, target string) (quarry.ExpandAnswer, error) {
	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return quarry.ExpandAnswer{}, err
	}
	return repo.Expand(target)
}
