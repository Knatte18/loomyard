// check.go holds the rules that judge a proposal against the Sketch's decision record and the board;
// it also holds the board context the rules read and the Finding type they return.

package raddleparser

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
)

// The names of the rules a proposal can break; each finding carries one of them.
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
	CheckDependsNote            = "depends-note"
	CheckPackagesShared         = "packages-shared"
	CheckRecipeInvalid          = "recipe-invalid"
	CheckProfileUnknown         = "profile-unknown"
	CheckTypeInvalid            = "type-invalid"
	CheckSlugTaken              = "slug-taken"
	CheckSlugSketch             = "slug-sketch"
	CheckSlugDuplicate          = "slug-duplicate"
	CheckShapeInvalid           = "shape-invalid"
	CheckPerchOutcome           = "perch-outcome"
	CheckCostMissing            = "cost-missing"
	CheckRevisedMoved           = "revised-moved"
)

// The board entry kinds the checks count as work another proposal could collide with.
const (
	KindTask    = "task"
	KindProject = "project"
)

// KindNote is the board entry kind no entry can depend on, which the board refuses at filing.
const KindNote = "note"

// sectionOpenRisks and sectionCriteria are the decision record sections whose bullets a part carries.
const (
	sectionOpenRisks = "Open risks"
	sectionCriteria  = "Acceptance criteria"
)

// Finding reports one broken rule: the check it breaks and what to fix.
// Part is the slug of the part or entry it concerns, when it concerns one.
type Finding struct {
	Check  string `json:"check"`
	Part   string `json:"part,omitempty"`
	Detail string `json:"detail"`
}

// BoardEntry is one board entry as the checks read it.
// Kind is the entry's kind, such as task or project.
// Open is false for a done entry.
// Revised is the entry's `revised` stamp in RFC 3339.
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
// Store holds the slugs the raddle store keeps, each as a Sketch slug or a filed slug.
// TypeLabels are the board's type labels.
// Profiles maps each part recipe to the profiles of `profiles.yaml` that hold a block for it.
// Filing is true at Raddle-File's run, where a dependency on an entry gone from the board is dropped.
// Perch is true in a perch-mode run, whose split perch approves the proposal in place of the operator:
// the checks then allow only one task, or one project whose slug is the run's own entry.
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
	Perch        bool
	LivePattern  func(id string) bool
}

// decisionIDPattern matches a decision id as a whole word;
// a longer id such as D10 is matched whole.
var decisionIDPattern = regexp.MustCompile(`\bD[1-9][0-9]*\b`)

// Check judges p against record, the Sketch's decision record, and board.
// It returns one finding for each instance of a broken rule, in the order the checks run.
// It reads the record only through internal/discussionparser.
// A proposal that breaks no rule returns no finding.
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
	findings = append(findings, c.perchOutcome()...)
	findings = append(findings, c.costLines()...)
	return findings
}

// checker holds the inputs that the rules share, including the decision ids parsed from the record.
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

// coverage reports the decisions, criteria, risks and PATTERN changes the parts leave out or invent.
// It also reports a PATTERN change that more than one part carries.
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

// patternClusters reports the pattern_clusters mappings the filing could not write.
// A mapping fails when its part does not carry its change, or when its change is mapped twice.
// It also fails when its text is empty, spans more than one line, or is over the cap of a PATTERN.md entry.
// Its entry must be one PATTERN.md holds, and no other declared change or mapping may already claim it.
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

// criteriaNameDecisions reports a part whose carried criteria leave a built decision unnamed.
// It also reports a carried criterion or risk that names a decision neither it nor a part on its depends_on path builds.
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

// reachable returns the slugs that slug depends on, directly or through a chain of depends_on entries.
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

// moduleLabelShared reports whether the entry shares a module label with the Sketch.
func moduleLabelShared(entry BoardEntry, board BoardContext) bool {
	return slices.ContainsFunc(entry.Labels, func(label string) bool { return slices.Contains(board.ModuleLabels, label) })
}

// uncheckedEntries returns the slugs of the open tasks and projects that share a module label with the Sketch.
// The run's own entry is never returned, and an entry counts as checked when board_checked lists it.
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
// A board_checked entry gone from the board passes as landed, whether or not the run is filing.
// It also reports a depends_on that names a note on the board, which the board would refuse at filing.
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
			isPart := slices.ContainsFunc(parts, func(other Part) bool { return other.Slug == dep })
			entry := slices.IndexFunc(c.board.Entries, func(entry BoardEntry) bool { return entry.Slug == dep })
			checked := slices.ContainsFunc(c.proposal.BoardChecked, func(checked BoardCheck) bool { return checked.Slug == dep })
			switch {
			case isPart:
			case entry >= 0 && c.board.Entries[entry].Kind == KindNote:
				findings = append(findings, Finding{Check: CheckDependsNote, Part: part.Slug, Detail: fmt.Sprintf("depends_on names the note %q, and the board refuses a dependency on a note; way forward: drop the dependency, or have the operator promote the note with lym board promote '{\"slug\":\"%s\"}', then run lym loom raddle-propose again", dep, dep)})
			case entry < 0 && !checked:
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

// vocabulary reports a part whose recipe is neither tapestry nor darn.
// It also reports a part whose profile holds no block for its recipe.
// It also reports a part or project whose type is not a type label of the board other than the Sketch.
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
// The Sketch's slug and the run's own entry are exempt.
// A filed Sketch slug gets its own finding, not a taken one.
func takenSlug(slug string, board BoardContext) bool {
	if slug == board.Entry || slug == board.Sketch {
		return false
	}
	return slices.ContainsFunc(board.Entries, func(entry BoardEntry) bool { return entry.Slug == slug }) || slices.Contains(board.Store, slug)
}

// perchProject reports whether the proposal is a perch run's project under the run's own entry.
// The slug rules exempt its slug.
func (c *checker) perchProject() bool {
	p := c.proposal
	return c.board.Perch && p.Shape == ShapeProject && p.Project != nil && p.Project.Slug == c.board.Entry
}

// slugs reports a filed slug that is the Sketch's, taken, or used twice.
// A perch run's project slug that is the run's own entry is not reported as the Sketch's;
// the run's own entry is never reported as taken.
func (c *checker) slugs() []Finding {
	var findings []Finding
	var seen []string
	for i, slug := range filedSlugs(c.proposal) {
		// filedSlugs puts the project's slug first.
		exempt := i == 0 && c.perchProject()
		if slug == c.board.Sketch && !exempt {
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

// shape reports a shape the spec does not name.
// It also reports a tapestry that is not one part building every decision, and a project that does not match the shape.
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

// perchOutcome reports a perch run's proposal that is neither one task nor one project under the run's own entry.
// One task is shape tapestry, or shape tasks with one part.
// A beam is refused, since it would start the family's runs and take their release from the operator.
// It reports nothing outside a perch run.
func (c *checker) perchOutcome() []Finding {
	if !c.board.Perch {
		return nil
	}
	p := c.proposal
	var problem string
	switch {
	case p.Shape == ShapeTasks && len(p.Parts) > 1:
		problem = fmt.Sprintf("shape %s with %d parts", ShapeTasks, len(p.Parts))
	case p.Shape == ShapeBeam:
		problem = fmt.Sprintf("shape %s", ShapeBeam)
	case p.Shape == ShapeProject && p.Project != nil && p.Project.Slug != c.board.Entry:
		problem = fmt.Sprintf("shape %s under the slug %q", ShapeProject, p.Project.Slug)
	default:
		return nil
	}
	return []Finding{{Check: CheckPerchOutcome, Detail: fmt.Sprintf("a perch run does not file %s; it files one task, as shape %s or shape %s with one part, or one project, as shape %s whose project slug is the run's entry %q; way forward: reshape the proposal into one of those", problem, ShapeTapestry, ShapeTasks, ShapeProject, c.board.Entry)}}
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
