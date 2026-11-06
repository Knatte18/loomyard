// check.go declares the board context the checks read, Finding, and Check, which judges a proposal against the Sketch's decision record and the board.

package raddleparser

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
)

// The names of the checks Check and RecheckBoard report.
const (
	CheckDecisionUnbuilt        = "decision-unbuilt"
	CheckDecisionUnknown        = "decision-unknown"
	CheckCriterionUncarried     = "criterion-uncarried"
	CheckCriterionUnknown       = "criterion-unknown"
	CheckRiskUncarried          = "risk-uncarried"
	CheckRiskUnknown            = "risk-unknown"
	CheckPatternUncarried       = "pattern-uncarried"
	CheckPatternShared          = "pattern-shared"
	CheckPatternUnknown         = "pattern-unknown"
	CheckPatternClusterInvalid  = "pattern-cluster-invalid"
	CheckPatternClusterUnknown  = "pattern-cluster-unknown"
	CheckPatternClusterShared   = "pattern-cluster-shared"
	CheckCriteriaMissDecision   = "criteria-miss-decision"
	CheckCarriedDecisionUnbuilt = "carried-decision-unbuilt"
	CheckBoardUnchecked         = "board-unchecked"
	CheckDependsCycle           = "depends-cycle"
	CheckDependsUnknown         = "depends-unknown"
	CheckPackagesShared         = "packages-shared"
	CheckRecipeInvalid          = "recipe-invalid"
	CheckProfileUnknown         = "profile-unknown"
	CheckTypeInvalid            = "type-invalid"
	CheckSlugTaken              = "slug-taken"
	CheckSlugSketch             = "slug-sketch"
	CheckSlugDuplicate          = "slug-duplicate"
	CheckShapeInvalid           = "shape-invalid"
	CheckCostMissing            = "cost-missing"
	CheckRevisedMoved           = "revised-moved"
)

// The board entry kinds the checks count as work another proposal could collide with.
const (
	KindTask    = "task"
	KindProject = "project"
)

// sectionOpenRisks and sectionCriteria are the decision record sections whose bullets a part carries.
const (
	sectionOpenRisks = "Open risks"
	sectionCriteria  = "Acceptance criteria"
)

// Finding is one broken rule: the check, the part, slug or entry it concerns when it concerns one, and what to fix.
type Finding struct {
	Check  string `json:"check"`
	Part   string `json:"part,omitempty"`
	Detail string `json:"detail"`
}

// BoardEntry is one board entry as the checks read it.
// Kind is KindTask, KindProject or the board's other kind, Open is false for a done entry, and Revised is its `revised` stamp in RFC 3339.
type BoardEntry struct {
	Slug    string
	Kind    string
	Labels  []string
	Open    bool
	Revised string
}

// BoardContext is what the checks read of the board, built by the caller.
// Sketch is the Sketch's slug and Entry the slug of the run's own entry.
// ModuleLabels are the Sketch's labels that are not type labels.
// Store holds the slugs the raddle store keeps, as Sketch slug or filed slug, and TypeLabels the board's type labels.
// Profiles maps each part recipe to the profiles of `profiles.yaml` that hold a block for it.
// Filing is true at Raddle-File's run, where a dependency on an entry gone from the board is dropped.
// LivePattern reports whether the run's PATTERN.md holds the entry id.
type BoardContext struct {
	Sketch       string
	Entry        string
	ModuleLabels []string
	Entries      []BoardEntry
	Store        []string
	TypeLabels   []string
	Profiles     map[string][]string
	Filing       bool
	LivePattern  func(id string) bool
}

// decisionIDPattern matches a decision id as a whole word; a longer id such as D10 is matched whole.
var decisionIDPattern = regexp.MustCompile(`\bD[1-9][0-9]*\b`)

// Check judges p against record, the Sketch's decision record, and board, and returns one finding per broken rule, in the order of the rules.
// It reads the record only through internal/discussionparser.
// A proposal that breaks none returns no finding.
func Check(p Proposal, record []byte, board BoardContext) []Finding {
	c := checker{proposal: p, record: record, board: board, recordIDs: discussionparser.DecisionIDs(record)}
	var findings []Finding
	findings = append(findings, c.coverage()...)
	findings = append(findings, c.criteriaNameDecisions()...)
	for _, slug := range uncheckedEntries(p, board) {
		findings = append(findings, Finding{Check: CheckBoardUnchecked, Detail: fmt.Sprintf("the open entry %q carries a module label of the Sketch and is not in board_checked", slug)})
	}
	findings = append(findings, c.dependencies()...)
	findings = append(findings, c.sharedPackages()...)
	findings = append(findings, c.vocabulary()...)
	findings = append(findings, c.slugs()...)
	findings = append(findings, c.shape()...)
	findings = append(findings, c.costLines()...)
	return findings
}

// checker holds what the rules of Check share.
type checker struct {
	proposal  Proposal
	record    []byte
	board     BoardContext
	recordIDs []string
}

// partBuilds reports whether any of parts builds the decision id.
func partBuilds(parts []Part, id string) bool {
	return slices.ContainsFunc(parts, func(part Part) bool { return slices.Contains(part.Decisions, id) })
}

// coverage reports the decisions, criteria, risks and PATTERN changes the parts leave out, carry twice or invent.
func (c *checker) coverage() []Finding {
	var findings []Finding
	parts := c.proposal.Parts
	for _, id := range c.recordIDs {
		if !partBuilds(parts, id) {
			findings = append(findings, Finding{Check: CheckDecisionUnbuilt, Detail: fmt.Sprintf("decision %s is built by no part", id)})
		}
	}
	for _, part := range parts {
		for _, id := range part.Decisions {
			if !slices.Contains(c.recordIDs, id) {
				findings = append(findings, Finding{Check: CheckDecisionUnknown, Part: part.Slug, Detail: fmt.Sprintf("builds %s, which the decision record does not hold", id)})
			}
		}
	}

	carried := []struct {
		section, uncarried, unknown string
		texts                       func(Part) []string
	}{
		{sectionCriteria, CheckCriterionUncarried, CheckCriterionUnknown, func(part Part) []string { return part.Criteria }},
		{sectionOpenRisks, CheckRiskUncarried, CheckRiskUnknown, func(part Part) []string { return part.OpenRisks }},
	}
	for _, kind := range carried {
		bullets := discussionparser.SectionBullets(c.record, kind.section)
		for _, bullet := range bullets {
			if !slices.ContainsFunc(parts, func(part Part) bool { return slices.Contains(kind.texts(part), bullet) }) {
				findings = append(findings, Finding{Check: kind.uncarried, Detail: fmt.Sprintf("no part carries the %s bullet %q", kind.section, bullet)})
			}
		}
		for _, part := range parts {
			for _, text := range kind.texts(part) {
				if !slices.Contains(bullets, text) {
					findings = append(findings, Finding{Check: kind.unknown, Part: part.Slug, Detail: fmt.Sprintf("carries %q, which is no %s bullet of the decision record", text, kind.section)})
				}
			}
		}
	}

	changes, _ := discussionparser.ParsePatternChanges(c.record)
	for _, change := range changes {
		var carriers []string
		for _, part := range parts {
			if slices.Contains(part.PatternChanges, change.ID) {
				carriers = append(carriers, part.Slug)
			}
		}
		switch {
		case len(carriers) == 0:
			findings = append(findings, Finding{Check: CheckPatternUncarried, Detail: fmt.Sprintf("no part carries %s", change.ID)})
		case len(carriers) > 1:
			findings = append(findings, Finding{Check: CheckPatternShared, Detail: fmt.Sprintf("%s is carried by %s; exactly one part carries it", change.ID, strings.Join(carriers, ", "))})
		}
	}
	for _, part := range parts {
		for _, id := range part.PatternChanges {
			if !slices.ContainsFunc(changes, func(change discussionparser.PatternChange) bool { return change.ID == id }) {
				findings = append(findings, Finding{Check: CheckPatternUnknown, Part: part.Slug, Detail: fmt.Sprintf("carries %s, which the decision record's PATTERN changes do not hold", id)})
			}
		}
	}
	return append(findings, c.patternClusters(changes)...)
}

// patternClusters reports a pattern_clusters mapping the filing could not write: one of a change its part does not carry, a change mapped twice or a text that is no one-line entry text within the cap, one onto an entry PATTERN.md does not hold, and one onto an entry another declared change or mapping already claims.
func (c *checker) patternClusters(changes []discussionparser.PatternChange) []Finding {
	claimedBy := make(map[string]string, len(changes))
	for _, change := range changes {
		claimedBy[change.ID] = change.ID
	}
	mapped := map[string]bool{}
	var findings []Finding
	for _, part := range c.proposal.Parts {
		for _, cluster := range part.PatternClusters {
			invalid := func(problem string) {
				findings = append(findings, Finding{Check: CheckPatternClusterInvalid, Part: part.Slug, Detail: fmt.Sprintf("the mapping of %s %s", cluster.SketchID, problem)})
			}
			text := strings.TrimSpace(cluster.Text)
			switch {
			case !slices.Contains(part.PatternChanges, cluster.SketchID):
				invalid("names a change this part does not carry; way forward: list the id in the part's pattern_changes, or drop the mapping")
			case mapped[cluster.SketchID]:
				invalid("is written twice; way forward: keep one mapping per id")
			case text == "" || strings.Contains(text, "\n"):
				invalid("needs its entry's whole new text on one line; way forward: write it in text")
			case discussionparser.PatternTextOverCap(text):
				invalid("carries a rule text over the cap of a PATTERN.md entry; way forward: shorten the text and move its detail to the entry's background file")
			}
			mapped[cluster.SketchID] = true

			if !c.board.LivePattern(cluster.Entry) {
				findings = append(findings, Finding{Check: CheckPatternClusterUnknown, Part: part.Slug, Detail: fmt.Sprintf("maps %s onto %s, which PATTERN.md does not hold; way forward: name a live entry, or keep the change under its own id", cluster.SketchID, cluster.Entry)})
			}
			if claimant, claimed := claimedBy[cluster.Entry]; claimed && claimant != cluster.SketchID {
				findings = append(findings, Finding{Check: CheckPatternClusterShared, Part: part.Slug, Detail: fmt.Sprintf("maps %s onto %s, which %s already claims; way forward: fold both changes into one mapping's text, since a record declares an entry once", cluster.SketchID, cluster.Entry, claimant)})
			}
			claimedBy[cluster.Entry] = cluster.SketchID
		}
	}
	return findings
}

// namesDecision reports whether any of texts names the decision id as a whole word.
func namesDecision(texts []string, id string) bool {
	return slices.ContainsFunc(texts, func(text string) bool { return slices.Contains(decisionIDPattern.FindAllString(text, -1), id) })
}

// criteriaNameDecisions reports a part whose carried criteria leave a built decision unnamed, and a carried criterion or risk that names a decision no part on the carrier's path builds.
func (c *checker) criteriaNameDecisions() []Finding {
	var findings []Finding
	for _, part := range c.proposal.Parts {
		for _, id := range part.Decisions {
			if slices.Contains(c.recordIDs, id) && !namesDecision(part.Criteria, id) {
				findings = append(findings, Finding{Check: CheckCriteriaMissDecision, Part: part.Slug, Detail: fmt.Sprintf("builds %s and carries no criterion that names it", id)})
			}
		}

		reachable := c.reachable(part.Slug)
		builtOnPath := func(id string) bool {
			if slices.Contains(part.Decisions, id) {
				return true
			}
			return slices.ContainsFunc(c.proposal.Parts, func(other Part) bool { return reachable[other.Slug] && slices.Contains(other.Decisions, id) })
		}
		for _, text := range slices.Concat(part.Criteria, part.OpenRisks) {
			for _, id := range slices.Compact(decisionIDPattern.FindAllString(text, -1)) {
				if slices.Contains(c.recordIDs, id) && !builtOnPath(id) {
					findings = append(findings, Finding{Check: CheckCarriedDecisionUnbuilt, Part: part.Slug, Detail: fmt.Sprintf("carries %q, which names %s, built by neither this part nor a part on its depends_on path", text, id)})
				}
			}
		}
	}
	return findings
}

// reachable returns the slugs of the parts on slug's depends_on path: the parts it depends on, directly or through other parts.
func (c *checker) reachable(slug string) map[string]bool {
	seen := map[string]bool{}
	var visit func(string)
	visit = func(from string) {
		for _, part := range c.proposal.Parts {
			if part.Slug != from {
				continue
			}
			for _, dep := range part.DependsOn {
				if !seen[dep] {
					seen[dep] = true
					visit(dep)
				}
			}
		}
	}
	visit(slug)
	return seen
}

// moduleLabelShared reports whether the entry carries a label of the Sketch.
func moduleLabelShared(entry BoardEntry, board BoardContext) bool {
	return slices.ContainsFunc(entry.Labels, func(label string) bool { return slices.Contains(board.ModuleLabels, label) })
}

// uncheckedEntries returns the slugs of the open tasks and projects, other than the run's own entry, that carry a module label of the Sketch and are absent from board_checked.
func uncheckedEntries(p Proposal, board BoardContext) []string {
	var unchecked []string
	for _, entry := range board.Entries {
		if !entry.Open || entry.Slug == board.Entry || (entry.Kind != KindTask && entry.Kind != KindProject) || !moduleLabelShared(entry, board) {
			continue
		}
		if !slices.ContainsFunc(p.BoardChecked, func(checked BoardCheck) bool { return checked.Slug == entry.Slug }) {
			unchecked = append(unchecked, entry.Slug)
		}
	}
	return unchecked
}

// dependencies reports a depends_on cycle and a depends_on that names no part, no board entry and no board_checked entry.
// A board_checked entry gone from the board passes as landed whatever board.Filing says.
func (c *checker) dependencies() []Finding {
	var findings []Finding
	parts := c.proposal.Parts

	const (
		unvisited = iota
		onStack
		done
	)
	state := map[string]int{}
	var stack []string
	var visit func(string)
	visit = func(slug string) {
		state[slug] = onStack
		stack = append(stack, slug)
		for _, part := range parts {
			if part.Slug != slug {
				continue
			}
			for _, dep := range part.DependsOn {
				switch state[dep] {
				case onStack:
					loop := append(slices.Clone(stack[slices.Index(stack, dep):]), dep)
					findings = append(findings, Finding{Check: CheckDependsCycle, Part: dep, Detail: "depends_on cycle: " + strings.Join(loop, " -> ")})
				case unvisited:
					if slices.ContainsFunc(parts, func(other Part) bool { return other.Slug == dep }) {
						visit(dep)
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[slug] = done
	}
	for _, part := range parts {
		if state[part.Slug] == unvisited {
			visit(part.Slug)
		}
	}

	for _, part := range parts {
		for _, dep := range part.DependsOn {
			known := slices.ContainsFunc(parts, func(other Part) bool { return other.Slug == dep }) ||
				slices.ContainsFunc(c.board.Entries, func(entry BoardEntry) bool { return entry.Slug == dep }) ||
				slices.ContainsFunc(c.proposal.BoardChecked, func(checked BoardCheck) bool { return checked.Slug == dep })
			if !known {
				findings = append(findings, Finding{Check: CheckDependsUnknown, Part: part.Slug, Detail: fmt.Sprintf("depends_on names %q, which is no part, no board entry and no board_checked entry", dep)})
			}
		}
	}
	return findings
}

// sharedPackages reports two parts with no depends_on path between them that list a common package.
func (c *checker) sharedPackages() []Finding {
	var findings []Finding
	parts := c.proposal.Parts
	for i, first := range parts {
		for _, second := range parts[i+1:] {
			if c.reachable(first.Slug)[second.Slug] || c.reachable(second.Slug)[first.Slug] {
				continue
			}
			for _, pkg := range first.Packages {
				if slices.ContainsFunc(second.Packages, func(other Package) bool { return other.Path == pkg.Path }) {
					findings = append(findings, Finding{Check: CheckPackagesShared, Part: first.Slug, Detail: fmt.Sprintf("parts %s and %s both list the package %s and no depends_on path orders them", first.Slug, second.Slug, pkg.Path)})
				}
			}
		}
	}
	return findings
}

// validType reports whether label is a type label of the board other than the Sketch's.
func validType(label string, board BoardContext) bool {
	return label != TypeSketch && slices.Contains(board.TypeLabels, label)
}

// vocabulary reports a part recipe that is neither tapestry nor darn, a profile with no block for the recipe, and a part or project type outside the board's vocabulary.
func (c *checker) vocabulary() []Finding {
	var findings []Finding
	for _, part := range c.proposal.Parts {
		if part.Recipe != RecipeTapestry && part.Recipe != RecipeDarn {
			findings = append(findings, Finding{Check: CheckRecipeInvalid, Part: part.Slug, Detail: fmt.Sprintf("recipe %q is neither %s nor %s", part.Recipe, RecipeTapestry, RecipeDarn)})
		} else if !slices.Contains(c.board.Profiles[part.Recipe], part.Profile) {
			findings = append(findings, Finding{Check: CheckProfileUnknown, Part: part.Slug, Detail: fmt.Sprintf("profile %q holds no block for the %s recipe", part.Profile, part.Recipe)})
		}
		if !validType(part.Type, c.board) {
			findings = append(findings, Finding{Check: CheckTypeInvalid, Part: part.Slug, Detail: fmt.Sprintf("type %q is not a type label of the board other than %s", part.Type, TypeSketch)})
		}
	}
	if project := c.proposal.Project; project != nil && !validType(project.Type, c.board) {
		findings = append(findings, Finding{Check: CheckTypeInvalid, Part: project.Slug, Detail: fmt.Sprintf("the project's type %q is not a type label of the board other than %s", project.Type, TypeSketch)})
	}
	return findings
}

// filedSlugs returns the slug of the project, when there is one, then of each part.
func filedSlugs(p Proposal) []string {
	var slugs []string
	if p.Project != nil {
		slugs = append(slugs, p.Project.Slug)
	}
	for _, part := range p.Parts {
		slugs = append(slugs, part.Slug)
	}
	return slugs
}

// takenSlug reports whether slug is on the board or in the raddle store.
// The Sketch's slug and the run's own entry are exempt: slug-sketch judges the Sketch's.
func takenSlug(slug string, board BoardContext) bool {
	if slug == board.Entry || slug == board.Sketch {
		return false
	}
	return slices.ContainsFunc(board.Entries, func(entry BoardEntry) bool { return entry.Slug == slug }) || slices.Contains(board.Store, slug)
}

// slugs reports a filed slug that is the Sketch's, taken, or used twice.
func (c *checker) slugs() []Finding {
	var findings []Finding
	var seen []string
	for _, slug := range filedSlugs(c.proposal) {
		if slug == c.board.Sketch {
			findings = append(findings, Finding{Check: CheckSlugSketch, Part: slug, Detail: "a filed slug equals the Sketch's slug"})
		}
		if takenSlug(slug, c.board) {
			findings = append(findings, Finding{Check: CheckSlugTaken, Part: slug, Detail: fmt.Sprintf("the slug %q is already on the board or in the raddle store", slug)})
		}
		if slices.Contains(seen, slug) {
			findings = append(findings, Finding{Check: CheckSlugDuplicate, Part: slug, Detail: fmt.Sprintf("the slug %q is filed twice", slug)})
		}
		seen = append(seen, slug)
	}
	return findings
}

// shape reports a shape the spec does not name, a tapestry that is not one part building every decision, and a project that does not match the shape.
func (c *checker) shape() []Finding {
	p := c.proposal
	invalid := func(detail string) []Finding {
		return []Finding{{Check: CheckShapeInvalid, Detail: detail}}
	}
	switch p.Shape {
	case ShapeTapestry:
		if len(p.Parts) != 1 {
			return invalid(fmt.Sprintf("shape %s has exactly one part, and the proposal has %d", ShapeTapestry, len(p.Parts)))
		}
		for _, id := range c.recordIDs {
			if !slices.Contains(p.Parts[0].Decisions, id) {
				return invalid(fmt.Sprintf("shape %s has one part that builds every decision, and the part does not build %s", ShapeTapestry, id))
			}
		}
		fallthrough
	case ShapeTasks:
		if p.Project != nil {
			return invalid(fmt.Sprintf("shape %s files no project, and the proposal holds one", p.Shape))
		}
	case ShapeProject, ShapeBeam:
		if p.Project == nil {
			return invalid(fmt.Sprintf("shape %s files a project, and the proposal holds none", p.Shape))
		}
	default:
		return invalid(fmt.Sprintf("shape %q is none of %s, %s, %s and %s", p.Shape, ShapeTapestry, ShapeTasks, ShapeProject, ShapeBeam))
	}
	return nil
}

// costLines reports a missing proposal cost line and a missing next-best cost line.
func (c *checker) costLines() []Finding {
	var findings []Finding
	if strings.TrimSpace(c.proposal.Cost.Line) == "" {
		findings = append(findings, Finding{Check: CheckCostMissing, Detail: "the proposal's cost line is missing"})
	}
	if strings.TrimSpace(c.proposal.Cost.NextBest.Line) == "" {
		findings = append(findings, Finding{Check: CheckCostMissing, Detail: "the next-best split's cost line is missing"})
	}
	return findings
}
