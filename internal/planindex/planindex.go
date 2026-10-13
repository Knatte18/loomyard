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
// File is the worktree-relative file a finding concerns, empty for a finding about no file.
type Finding struct {
	Check    string
	Card     string
	Detail   string
	Severity Severity
	Ref      string
	File     string
}

// Error implements the error interface, formatted as "check[/card]: detail", exactly as planparser.ValidationError.Error does, plus its own Severity — so a caller rendering a mixed []Finding set can distinguish an informational create-new-unit from a blocking glyph-not-found in the one string that record exists.
func (f Finding) Error() string {
	if f.Card == "" {
		return fmt.Sprintf("%s[%s]: %s", f.Check, f.Severity, f.Detail)
	}
	return fmt.Sprintf("%s/%s[%s]: %s", f.Check, f.Card, f.Severity, f.Detail)
}

// PackDeclaration is one declaration of a resolved member: its file, its line span and its signature.
// The signature is the code index's verbatim, except a var or const keeps its first line only, since its signature carries the whole initializer.
type PackDeclaration struct {
	File      string
	Start     int
	End       int
	Signature string
}

// PackEntry is one member glyph a batch's cards name, with every declaration the code index resolved it to.
type PackEntry struct {
	Glyph        string
	Declarations []PackDeclaration
}

// PackUnresolved is one ref of a batch's cards the code index could not resolve, with the reason in plain words.
type PackUnresolved struct {
	Ref    string
	Reason string
}

// BindReason is the closed set of reasons a plan handle failed to bind.
type BindReason string

const (
	// BindNoDeltaMatch marks a handle whose expected glyph the batch's delta created or renamed nothing to.
	BindNoDeltaMatch BindReason = "no-delta-match"
	// BindSiblingUnmatched marks a handle that matched the delta while another handle of its card did not, so the card bound none of them.
	BindSiblingUnmatched BindReason = "sibling-unmatched"
)

// BindOutcome is what happened to one plan handle a card owns.
// Exactly one of Glyph and Reason is set: Glyph when the handle bound, Reason when it did not.
type BindOutcome struct {
	// Card is the owning card's number.
	Card int
	// Handle is the handle as the card spells it.
	Handle string
	// Glyph is the glyph the handle bound to, empty when it failed.
	Glyph string
	// Reason is why the handle failed to bind, empty when it bound.
	Reason BindReason
}

// Pack is the kick-start pack of one card, or of any card-less caller: the member glyphs it names, resolved, and the refs that did not resolve, each in the order they are named.
type Pack struct {
	Resolved   []PackEntry
	Unresolved []PackUnresolved
}

// CardPack is one card of a batch paired with its own kick-start pack.
type CardPack struct {
	Card planparser.Card
	Pack Pack
}

// Index is the code index a plan gate calls.
// An infrastructure failure of the index is an error wrapping ErrQuarryUnavailable, distinct from any finding.
type Index interface {
	// Validate resolves the whole plan's refs against the tree at worktreeRoot, as ValidateFormat with a nil done does, and also reports a plan not yet approved.
	Validate(plan *planparser.Plan, worktreeRoot string) ([]Finding, error)
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
	// A finding is blocking for a resolved reference and informational for a method matched by name alone, since it runs no type load.
	LaterDeleteReferences(plan *planparser.Plan, deleting, later []planparser.Card, worktreeRoot string) ([]Finding, error)
	// Delta reads the git delta of the range fromSHA..toSHA in worktree.
	// On an ErrQuarryUnavailable error the returned Delta is the empty delta, never nil.
	Delta(worktree, fromSHA, toSHA string) (Delta, error)
	// KickstartPack resolves the member glyphs the cards' targets and Uses name against the tree at worktreeRoot, and returns one CardPack per card, in the order of the cards given.
	// Each pack lists the plan: handles and the refs that did not resolve beside its resolved entries.
	// A plan whose language opts out of glyphs yields nil.
	// On an ErrQuarryUnavailable error every card lists its selected member glyphs unresolved with the error, so a caller degrades instead of refusing.
	KickstartPack(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]CardPack, error)
}

// Delta is the git delta of one batch, read once and consumed by every check that needs it.
type Delta interface {
	// BindHandles binds the plan handles of cards to the symbols the delta created, and reports the outcome of each handle beside the findings.
	BindHandles(plan *planparser.Plan, planDir string, cards []planparser.Card) ([]Finding, []BindOutcome, error)
	// ScopeGuard reports the delta's changes outside the targets the cards declare.
	ScopeGuard(cards []planparser.Card) []Finding
	// DetectDrift reports the plan references the delta invalidates among the cards not in completed.
	DetectDrift(fullPlan *planparser.Plan, completed []planparser.Card, planDir, worktreeRoot, sha, now string) ([]Finding, error)
}
