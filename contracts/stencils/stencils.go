// stencils.go is the single Go file at the top-level stencils/ package root: //go:embed reaches
// only files at or below its own directory, so every stencil's shipped-default byte var and its
// //go:embed directive must live here, one per family subfolder below.
// Beside the embedded vars, this file declares the name-to-default registry that
// internal/stencilstore.Registry consumes, and is the only place a stencil's on-disk path and its
// Go identifier are both named -- the one place a new stencil is registered.

package stencils

import (
	_ "embed"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// LandingTemplateConflict is the landing conflict-resolution producer's shipped-default prompt.
//
//go:embed landing/landing-template-conflict.md
var LandingTemplateConflict []byte

// LoomTemplateDiscussion is the loom Discussion producer's shipped-default interview prompt.
//
//go:embed loom/loom-template-discussion.md
var LoomTemplateDiscussion []byte

// LoomTemplatePlan is the loom Plan producer's shipped-default autonomous prompt.
//
//go:embed loom/loom-template-plan.md
var LoomTemplatePlan []byte

// LoomRubricDiscussionReview is the Discussion-Review rubric, read by both rows of the
// Discussion-Review perch.
//
//go:embed loom/loom-rubric-discussion-review.md
var LoomRubricDiscussionReview []byte

// LoomRubricPlanReview is the Plan-Review rubric, read by both rows of the Plan-Review perch.
//
//go:embed loom/loom-rubric-plan-review.md
var LoomRubricPlanReview []byte

// LoomRubricWebsterReview is the Webster-Review rubric, read by both rows of the Webster-Review
// perch.
//
//go:embed loom/loom-rubric-webster-review.md
var LoomRubricWebsterReview []byte

// BurlerTemplateRoundOrchestrator is burler's shipped-default per-round orchestrator prompt.
//
//go:embed burler/burler-template-round-orchestrator.md
var BurlerTemplateRoundOrchestrator []byte

// BurlerStep1Explore is burler's shipped-default step-1 (explore) instruction prompt.
//
//go:embed burler/burler-step-1-explore.md
var BurlerStep1Explore []byte

// BurlerStep2Review is burler's shipped-default step-2 (review) instruction prompt.
//
//go:embed burler/burler-step-2-review.md
var BurlerStep2Review []byte

// BurlerStep3Fix is burler's shipped-default step-3 (fix) instruction prompt.
//
//go:embed burler/burler-step-3-fix.md
var BurlerStep3Fix []byte

// BouncerTemplateSeed is the Bouncer's shipped-default seed prompt: the focus-setting pass that
// runs before any round has been reviewed.
//
//go:embed bouncer/bouncer-template-seed.md
var BouncerTemplateSeed []byte

// BouncerTemplateJudge is the Bouncer's shipped-default per-round judge prompt.
//
//go:embed bouncer/bouncer-template-judge.md
var BouncerTemplateJudge []byte

// TreadleTemplateJudgeCircling is treadle's shipped-default per-round circling-check judge prompt.
//
//go:embed treadle/treadle-template-judge-circling.md
var TreadleTemplateJudgeCircling []byte

// TreadleTemplateJudgeMilestone is treadle's shipped-default milestone continuation-gate judge
// prompt.
//
//go:embed treadle/treadle-template-judge-milestone.md
var TreadleTemplateJudgeMilestone []byte

// TreadleTemplateTriage is treadle's shipped-default asking-triage prompt.
//
//go:embed treadle/treadle-template-triage.md
var TreadleTemplateTriage []byte

// TreadleTemplateTargeting is treadle's shipped-default pre-round targeting judge prompt.
//
//go:embed treadle/treadle-template-targeting.md
var TreadleTemplateTargeting []byte

// WebsterTemplateMaster is webster's shipped-default Master-session prompt template.
//
//go:embed webster/webster-template-master.md
var WebsterTemplateMaster []byte

// WebsterTemplateIntegration is webster's shipped-default integration-suite fork prompt template.
//
//go:embed webster/webster-template-integration.md
var WebsterTemplateIntegration []byte

// WebsterPrefixFork is webster's shipped-default in-session fork prompt prefix, joined ahead of
// WebsterBodyImplementer to compose the fork prompt.
//
//go:embed webster/webster-prefix-fork.md
var WebsterPrefixFork []byte

// WebsterPrefixRecovery is webster's shipped-default cold-start recovery prompt prefix, joined
// ahead of WebsterBodyImplementer to compose the recovery prompt.
//
//go:embed webster/webster-prefix-recovery.md
var WebsterPrefixRecovery []byte

// WebsterBodyImplementer is webster's shipped-default shared implementer-job body, composed with
// both WebsterPrefixFork and WebsterPrefixRecovery.
//
//go:embed webster/webster-body-implementer.md
var WebsterBodyImplementer []byte

// PatternDirectiveImplementer is the shipped-default PATTERN directive for RoleImplementer.
//
//go:embed pattern/pattern-directive-implementer.md
var PatternDirectiveImplementer []byte

// PatternDirectiveReviewFix is the shipped-default PATTERN directive for RoleReviewFix.
//
//go:embed pattern/pattern-directive-review-fix.md
var PatternDirectiveReviewFix []byte

// PatternDirectiveOrchestrator is the shipped-default PATTERN directive for RoleOrchestrator.
//
//go:embed pattern/pattern-directive-orchestrator.md
var PatternDirectiveOrchestrator []byte

// registryEntry pairs one stencil's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry: the order stencils are listed here is the order
// `lyx stencil list` prints them in.
var entries = []registryEntry{
	{"landing-template-conflict", &LandingTemplateConflict},
	{"loom-template-discussion", &LoomTemplateDiscussion},
	{"loom-template-plan", &LoomTemplatePlan},
	{"loom-rubric-discussion-review", &LoomRubricDiscussionReview},
	{"loom-rubric-plan-review", &LoomRubricPlanReview},
	{"loom-rubric-webster-review", &LoomRubricWebsterReview},
	{"burler-template-round-orchestrator", &BurlerTemplateRoundOrchestrator},
	{"burler-step-1-explore", &BurlerStep1Explore},
	{"burler-step-2-review", &BurlerStep2Review},
	{"burler-step-3-fix", &BurlerStep3Fix},
	{"bouncer-template-seed", &BouncerTemplateSeed},
	{"bouncer-template-judge", &BouncerTemplateJudge},
	{"treadle-template-judge-circling", &TreadleTemplateJudgeCircling},
	{"treadle-template-judge-milestone", &TreadleTemplateJudgeMilestone},
	{"treadle-template-triage", &TreadleTemplateTriage},
	{"treadle-template-targeting", &TreadleTemplateTargeting},
	{"webster-template-master", &WebsterTemplateMaster},
	{"webster-template-integration", &WebsterTemplateIntegration},
	{"webster-prefix-fork", &WebsterPrefixFork},
	{"webster-prefix-recovery", &WebsterPrefixRecovery},
	{"webster-body-implementer", &WebsterBodyImplementer},
	{"pattern-directive-implementer", &PatternDirectiveImplementer},
	{"pattern-directive-review-fix", &PatternDirectiveReviewFix},
	{"pattern-directive-orchestrator", &PatternDirectiveOrchestrator},
}

// registry implements stencilstore.Registry over entries.
type registry struct{}

// Names returns every registered stencil's name, in entries' declared order.
func (registry) Names() []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// Default returns name's shipped default content, and whether name is a known stencil.
func (registry) Default(name string) ([]byte, bool) {
	for _, e := range entries {
		if e.name == name {
			return *e.def, true
		}
	}
	return nil, false
}

// Registry returns the stencilstore.Registry backed by this package's embedded defaults.
// `cmd/lyx`'s root pre-run and internal/stencilcli are its consumers; no engine imports this
// package -- an engine reads a stencil at call time via stencilstore.Read, which needs no registry.
func Registry() stencilstore.Registry {
	return registry{}
}
