// planindex.go declares the finding types, the quarry-unavailable error and the Index and Delta interfaces of the code index seam.

package planindex

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrQuarryUnavailable marks quarry failing to answer at all — a non-nil error from quarry.Open or (*quarry.Repo).Resolve, and a quarry.Name answer whose length does not match the declarations it was given, which is the same class of failure seen through a batched boundary: a category distinct from any per-target verdict, so a caller distinguishes it with errors.Is rather than by string matching.
// quarry's own contract draws exactly this line — the failure envelope's own presence marks that quarry could not answer at all, never that the answer is negative — and conflating the two would let a transport failure read as a clean not_found, which is, under the Create inversion, a pass: a quarry outage would silently mark every Create card done.
//
// Rejected, and worth stating so it is not reintroduced: degrading to format-only validation with a warning is the exact failure mode where a plan looks validated and was not; and making the error informational everywhere makes the outage invisible at precisely the boundaries whose whole job is to be mechanical.
var ErrQuarryUnavailable = errors.New("planglyph: quarry could not answer")

// Severity is the closed vocabulary a Finding's own Severity is drawn from.
type Severity string

const (
	// SeverityBlocking marks a finding that fails the gate it is reported against.
	SeverityBlocking Severity = "blocking"
	// SeverityInformational marks a finding surfaced for visibility that never fails a gate.
	SeverityInformational Severity = "informational"
)

// Finding is the plan gates' own finding type: the same Check, Card and Detail fields planparser.ValidationError carries, plus a Severity every converted planparser finding is stamped with.
// Ref is the raw plan ref a per-ref finding reports, empty for a finding that is not about one ref.
type Finding struct {
	Check    string
	Card     string
	Detail   string
	Severity Severity
	Ref      string
}

// Error implements the error interface, formatted as "check[/card]: detail", exactly as planparser.ValidationError.Error does, plus its own Severity — so a caller rendering a mixed []Finding set can distinguish an informational create-new-unit from a blocking glyph-not-found in the one string that record exists.
func (f Finding) Error() string {
	if f.Card == "" {
		return fmt.Sprintf("%s[%s]: %s", f.Check, f.Severity, f.Detail)
	}
	return fmt.Sprintf("%s/%s[%s]: %s", f.Check, f.Card, f.Severity, f.Detail)
}

// Index is the code index a plan gate calls.
// An infrastructure failure of the index is an error wrapping ErrQuarryUnavailable, distinct from any finding.
type Index interface {
	// ValidateFormat resolves the plan's refs against the tree at worktreeRoot, as a freshly written plan is checked.
	// done names the cards whose batch webster already recorded done: their work is in the tree, so they are history, and the tree-dependent checks skip them.
	// A nil done checks the whole plan.
	ValidateFormat(plan *planparser.Plan, worktreeRoot string, done []planparser.Card) ([]Finding, error)
	// ValidateRework resolves the plan's refs for a rework round that told the given number of completed cards.
	ValidateRework(plan *planparser.Plan, worktreeRoot string, told int) ([]Finding, error)
	// ValidateDispatch resolves the plan's refs for a batch about to run, after the completed cards and before the forthcoming ones.
	ValidateDispatch(plan *planparser.Plan, worktreeRoot string, completed, forthcoming []planparser.Card) ([]Finding, error)
	// DoneChecks reports the declared work of cards that is missing from the tree at worktreeRoot.
	DoneChecks(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error)
	// LaterDeleteReferences reports the later cards that still reference a symbol the deleting cards delete.
	LaterDeleteReferences(plan *planparser.Plan, deleting, later []planparser.Card, worktreeRoot string) ([]Finding, error)
	// Delta reads the git delta of the range fromSHA..toSHA in worktree.
	// On an ErrQuarryUnavailable error the returned Delta is the empty delta, never nil.
	Delta(worktree, fromSHA, toSHA string) (Delta, error)
}

// Delta is the git delta of one batch, read once and consumed by every check that needs it.
type Delta interface {
	// BindHandles binds the plan handles of cards to the symbols the delta created.
	BindHandles(plan *planparser.Plan, planDir string, cards []planparser.Card) ([]Finding, error)
	// ScopeGuard reports the delta's changes outside the targets the cards declare.
	ScopeGuard(cards []planparser.Card) []Finding
	// DetectDrift reports the plan references the delta invalidates among the cards not in completed.
	DetectDrift(fullPlan *planparser.Plan, completed []planparser.Card, planDir, worktreeRoot, sha, now string) ([]Finding, error)
}
