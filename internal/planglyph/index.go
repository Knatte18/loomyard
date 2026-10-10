// index.go implements planindex.Index and planindex.Delta over this package's gates and its quarry delta.

package planglyph

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

var (
	_ planindex.Index = index{}
	_ planindex.Delta = batchDelta{}
)

// NewIndex returns the real code index, each method forwarding to the gate of the same name in this package.
// matcher is the fabric-reference rule the index's format and rework validations run as planparser's card-fabric-reference check; dispatch validation does not run it.
// Its type load is unslotted.
func NewIndex(matcher planparser.FabricReferenceMatcher) planindex.Index {
	return index{matcher: matcher, loader: defaultTypesLoader}
}

// NewSlottedIndex is NewIndex whose caller-uncovered type load waits for a slot of slots first, noting the wait in waitDir.
// A nil slots makes it NewIndex.
func NewSlottedIndex(matcher planparser.FabricReferenceMatcher, slots *gateslot.Pool, waitDir string) planindex.Index {
	if slots == nil {
		return NewIndex(matcher)
	}
	return index{matcher: matcher, loader: goListLoader{timeout: typesLoadTimeout, slots: slots, waitDir: waitDir}}
}

// index forwards every planindex.Index method to this package's function, and holds the matcher the plan-gate check needs and the loader its type load runs through.
type index struct {
	matcher planparser.FabricReferenceMatcher
	loader  typesLoader
}

// Validate is the package's Validate, its type load run through the index's loader.
func (i index) Validate(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	return validate(plan, worktreeRoot, i.loader)
}

// ValidateFormat is the package's ValidateFormatAfter plus the card-fabric-reference findings, which sit outside planparser's entry points.
func (i index) ValidateFormat(plan *planparser.Plan, worktreeRoot string, done []planparser.Card) ([]Finding, error) {
	findings, err := validateFormatAfter(plan, worktreeRoot, done, i.loader)
	return append(findings, convertAll(planparser.CheckCardFabricReference(plan, i.matcher))...), err
}

// ValidateRework is the package's ValidateRework plus the card-fabric-reference findings.
func (i index) ValidateRework(plan *planparser.Plan, worktreeRoot string, told int) ([]Finding, error) {
	findings, err := validateRework(plan, worktreeRoot, told, i.loader)
	return append(findings, convertAll(planparser.CheckCardFabricReference(plan, i.matcher))...), err
}

func (index) ValidateDispatch(plan *planparser.Plan, worktreeRoot string, completed, forthcoming []planparser.Card) ([]Finding, error) {
	return ValidateDispatch(plan, worktreeRoot, completed, forthcoming)
}

func (index) DoneChecks(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error) {
	return DoneChecks(plan, cards, worktreeRoot)
}

func (index) LaterDeleteReferences(plan *planparser.Plan, deleting, later []planparser.Card, worktreeRoot string) ([]Finding, error) {
	return LaterDeleteReferences(plan, deleting, later, worktreeRoot)
}

// Delta reads the range's git delta; on ErrQuarryUnavailable the returned delta is the empty one, which a caller may still bind handles against.
func (index) Delta(worktree, fromSHA, toSHA string) (planindex.Delta, error) {
	answer, err := Delta(worktree, fromSHA, toSHA)
	if err != nil && !errors.Is(err, ErrQuarryUnavailable) {
		return nil, err
	}
	return batchDelta{answer: answer}, err
}

// batchDelta is one batch's quarry delta behind planindex.Delta.
type batchDelta struct {
	answer quarry.GitDeltaAnswer
}

func (d batchDelta) BindHandles(plan *planparser.Plan, planDir string, cards []planparser.Card) ([]Finding, error) {
	return BindHandles(plan, planDir, d.answer, cards)
}

func (d batchDelta) ScopeGuard(cards []planparser.Card) []Finding {
	return ScopeGuard(cards, d.answer)
}

// DetectDrift checks the plan pending after completed, the same view PendingPlan builds.
func (d batchDelta) DetectDrift(fullPlan *planparser.Plan, completed []planparser.Card, planDir, worktreeRoot, sha, now string) ([]Finding, error) {
	return DetectDrift(fullPlan, PendingPlan(fullPlan, completed), planDir, worktreeRoot, d.answer, sha, now)
}
