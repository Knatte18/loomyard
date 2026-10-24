// promote.go holds the two entry points "lyx beam promote" takes over a cancel's phases: ending a task's run without touching its pair, and clearing the run's records and claim afterwards.

package battencli

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ErrPromoteEndOnly matches, through errors.Is, an EndRunForPromote that found origin unreachable and so only ended the run's processes.
var ErrPromoteEndOnly = errors.New("battencli: origin was unreachable, so the promote only ended the run's processes")

// promoteRerun returns the quoted command every refusal of a promote's phases names as the step to run again.
func promoteRerun(slug string) string {
	return fmt.Sprintf("%q", "lyx beam promote "+slug)
}

// EndRunForPromote ends the batten run of slug for good and keeps its pair: the cancel's own refusals, marker and teardown of the run's processes and strands, stopping before the records and the claim.
// The cancel marker is removed when the phase ends, so no marker stands before ClearRunForPromote.
// With origin unreachable the phase is end-only and the error matches ErrPromoteEndOnly.
// Every refusal names a re-run of `lyx beam promote`.
func EndRunForPromote(ctx context.Context, prime *lyxcwd.Location, slug string) error {
	deps, err := hubCancelDeps(prime, slug)
	if err != nil {
		return err
	}
	return endRunForPromote(ctx, slug, deps)
}

// endRunForPromote is EndRunForPromote over deps.
func endRunForPromote(ctx context.Context, slug string, deps cancelDeps) error {
	rerun := promoteRerun(slug)
	report, err := endRunPhase(ctx, slug, false, rerun, deps)
	if err != nil {
		return err
	}
	defer report.hold.end(slug)
	if report.EndOnly {
		return fmt.Errorf("%w; way forward: run %s again once origin answers", ErrPromoteEndOnly, rerun)
	}
	if err := report.hold.finishMarker(true); err != nil {
		return fmt.Errorf("battencli: could not remove the cancel marker of %s: %w; way forward: run %s again", slug, err, rerun)
	}
	return nil
}

// ClearRunForPromote removes the records of slug's run and clears its board claim: the rest of a cancel without --remove, after EndRunForPromote.
// Every step is a no-op once done, so it is run again to finish an interrupted clear.
func ClearRunForPromote(ctx context.Context, prime *lyxcwd.Location, slug string) error {
	deps, err := hubCancelDeps(prime, slug)
	if err != nil {
		return err
	}
	return clearRunForPromote(slug, deps)
}

// clearRunForPromote is ClearRunForPromote over deps.
func clearRunForPromote(slug string, deps cancelDeps) error {
	report := cancelReport{Slug: slug}
	return clearRunPhase(slug, false, promoteRerun(slug), &report, deps)
}
