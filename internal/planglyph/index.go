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

// ValidateFormat is the package's ValidateFormatAfter plus the card-fabric-reference, fabric-vocabulary-name and edit-test-file findings, which sit outside planparser's entry points.
// edit-test-file skips the done cards.
func (i index) ValidateFormat(plan *planparser.Plan, worktreeRoot string, done []planparser.Card) ([]Finding, error) {
	findings, err := validateFormatAfter(plan, worktreeRoot, done, i.loader)
	findings = append(findings, convertAll(planparser.CheckCardFabricReference(plan, i.matcher))...)
	findings = append(findings, convertAll(planparser.CheckFabricVocabularyName(plan))...)
	return append(findings, convertAll(planparser.CheckEditTestFile(plan, done))...), err
}

// ValidateRework is the package's ValidateRework plus the card-fabric-reference, fabric-vocabulary-name and edit-test-file findings.
// A rework generation is a fresh plan whose prior cards are archived, so edit-test-file skips no card.
func (i index) ValidateRework(plan *planparser.Plan, worktreeRoot string, told int) ([]Finding, error) {
	findings, err := validateRework(plan, worktreeRoot, told, i.loader)
	findings = append(findings, convertAll(planparser.CheckCardFabricReference(plan, i.matcher))...)
	findings = append(findings, convertAll(planparser.CheckFabricVocabularyName(plan))...)
	return append(findings, convertAll(planparser.CheckEditTestFile(plan, nil))...), err
}

// ValidateDispatch is the package's ValidateDispatch plus the fabric-vocabulary-name findings of the forthcoming cards.
// A completed card's declared name has already landed, so a finding against it is no defect the writer can fix.
func (index) ValidateDispatch(plan *planparser.Plan, worktreeRoot string, completed, forthcoming []planparser.Card) ([]Finding, error) {
	findings, err := ValidateDispatch(plan, worktreeRoot, completed, forthcoming)
	done := cardIDSet(completed)
	for _, finding := range convertAll(planparser.CheckFabricVocabularyName(plan)) {
		if !done[finding.Card] {
			findings = append(findings, finding)
		}
	}
	return findings, err
}

func (index) DoneChecks(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error) {
	return DoneChecks(plan, cards, worktreeRoot)
}

func (index) RebindResigns(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error) {
	return RebindResigns(plan, cards, worktreeRoot)
}

func (index) LaterDeleteReferences(plan *planparser.Plan, deleting, later []planparser.Card, worktreeRoot string) ([]Finding, error) {
	return LaterDeleteReferences(plan, deleting, later, worktreeRoot)
}

// Delta reads the range's git delta; on ErrQuarryUnavailable the returned delta is the empty one, which a caller may still bind handles against.
// Any other error, such as the floor error of a git below lym's minimum, is returned bare with no delta.
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

func (d batchDelta) BindHandles(plan *planparser.Plan, planDir string, cards []planparser.Card) ([]Finding, []planindex.BindOutcome, error) {
	return BindHandles(plan, planDir, d.answer, cards)
}

func (d batchDelta) ScopeGuard(plan *planparser.Plan, cards []planparser.Card) []Finding {
	return ScopeGuard(plan, cards, d.answer)
}

// RemovedTargets checks the plan pending after completed, the same view PendingPlan builds.
func (d batchDelta) RemovedTargets(fullPlan *planparser.Plan, completed, own []planparser.Card, reported []planindex.Rename) []Finding {
	return RemovedTargets(PendingPlan(fullPlan, completed), own, d.answer, reported)
}

// DetectDrift checks the plan pending after completed, the same view PendingPlan builds.
func (d batchDelta) DetectDrift(fullPlan *planparser.Plan, completed []planparser.Card, reported []planindex.Rename, planDir, worktreeRoot, sha, now string) ([]Finding, error) {
	return DetectDrift(fullPlan, PendingPlan(fullPlan, completed), planDir, worktreeRoot, d.answer, reported, sha, now)
}

// ResolveGlyphs resolves glyphs through the package's Resolve and answers each by the status policy statusFindings applies, so a glyph resolves here exactly when a card ref naming it would raise no glyph-not-found or glyph-ambiguous finding.
// A quarry failure is returned wrapping ErrQuarryUnavailable, never an answer.
func (index) ResolveGlyphs(worktreeRoot string, glyphs []string) ([]planindex.GlyphAnswer, error) {
	if len(glyphs) == 0 {
		return nil, nil
	}
	results, err := Resolve(worktreeRoot, glyphs)
	if err != nil {
		return nil, err
	}

	answers := make([]planindex.GlyphAnswer, len(glyphs))
	for i, r := range results {
		verdict, _, classified := resolveVerdict(worktreeRoot, r)
		answer := planindex.GlyphAnswer{Glyph: glyphs[i], Verdict: verdict}
		switch verdict {
		case planindex.GlyphFound:
			var files []string
			for _, symbol := range r.Symbols {
				files = append(files, symbol.File)
			}
			for _, candidate := range classified.Candidates {
				files = append(files, candidate.File)
			}
			answer.Files = dedupe(files)
		case planindex.GlyphAmbiguous:
			for _, candidate := range classified.Candidates {
				answer.Candidates = append(answer.Candidates, candidateList([]constrainedCandidate{candidate}))
			}
		}
		answers[i] = answer
	}
	return answers, nil
}

// dedupe returns values without repeats, keeping first-seen order.
func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	var unique []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}
