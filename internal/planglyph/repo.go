// repo.go is planglyph's one call site for quarry.Open and (*quarry.Repo).Resolve — the package's
// two entry points into quarry.Repo — and declares Finding, this package's own finding type.

package planglyph

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// ErrQuarryUnavailable marks a non-nil error from quarry.Open or (*quarry.Repo).Resolve: a
// category distinct from any per-target verdict, so a caller distinguishes it with errors.Is
// rather than by string matching. quarry's own contract draws exactly this line — the failure
// envelope's own presence marks that quarry could not answer at all, never that the answer is
// negative — and conflating the two would let a transport failure read as a clean not_found,
// which is, under this package's Create inversion (create.go), a pass: a quarry outage would
// silently mark every Create card done.
//
// Rejected, and worth stating so it is not reintroduced: degrading to format-only validation with
// a warning is the exact failure mode where a plan looks validated and was not; and making the
// error informational everywhere makes the outage invisible at precisely the boundaries whose
// whole job is to be mechanical.
var ErrQuarryUnavailable = errors.New("planglyph: quarry could not answer")

// Severity is the closed vocabulary a Finding's own Severity is drawn from.
type Severity string

const (
	// SeverityBlocking marks a finding that fails the gate it is reported against.
	SeverityBlocking Severity = "blocking"
	// SeverityInformational marks a finding surfaced for visibility that never fails a gate.
	SeverityInformational Severity = "informational"
)

// Finding is this package's own finding type: the same Check, Card and Detail fields
// planparser.ValidationError carries, plus a Severity every converted planparser finding is
// stamped with by fromValidationError.
type Finding struct {
	Check    string
	Card     string
	Detail   string
	Severity Severity
}

// Error implements the error interface, formatted as "check[/card]: detail", exactly as
// planparser.ValidationError.Error does, plus its own Severity — so a caller rendering a mixed
// []planglyph.Finding set (via renderFindings in internal/loomcli/validate.go) can distinguish an
// informational create-new-unit from a blocking glyph-not-found in the one string that record
// exists.
func (f Finding) Error() string {
	if f.Card == "" {
		return fmt.Sprintf("%s[%s]: %s", f.Check, f.Severity, f.Detail)
	}
	return fmt.Sprintf("%s/%s[%s]: %s", f.Check, f.Card, f.Severity, f.Detail)
}

// fromValidationError converts v into a Finding stamped SeverityBlocking — the severity every
// planparser check reports today, per the plan's blocking-policy Shared Decision.
func fromValidationError(v planparser.ValidationError) Finding {
	return Finding{Check: v.Check, Card: v.Card, Detail: v.Detail, Severity: SeverityBlocking}
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
// with ErrQuarryUnavailable for the same reason openRepo does. It is this package's one call to
// (*quarry.Repo).Resolve.
func resolveTargets(repo *quarry.Repo, targets []string) ([]quarry.ResolveResult, error) {
	results, err := repo.Resolve(targets)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve: %v", ErrQuarryUnavailable, err)
	}
	return results, nil
}
