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

// LandingTemplateDescribe is the landing Describe producer's shipped-default prompt.
//
//go:embed landing/landing-template-describe.md
var LandingTemplateDescribe []byte

// LoomTemplateDiscussion is the loom Discussion producer's shipped-default interview prompt.
//
//go:embed loom/loom-template-discussion.md
var LoomTemplateDiscussion []byte

// LoomTemplateDiscussionChair is the chair's shipped-default opening prompt when the Discussion-Write row runs as a seat table.
//
//go:embed loom/loom-template-discussion-chair.md
var LoomTemplateDiscussionChair []byte

// LoomTemplateDiscussionAdvisor is an advisor's shipped-default opening prompt when the Discussion-Write row runs as a seat table.
//
//go:embed loom/loom-template-discussion-advisor.md
var LoomTemplateDiscussionAdvisor []byte

// LoomTemplatePlan is the loom Plan producer's shipped-default autonomous prompt.
//
//go:embed loom/loom-template-plan.md
var LoomTemplatePlan []byte

// LoomTemplateRework is the loom PR-Rework producer's shipped-default autonomous prompt.
//
//go:embed loom/loom-template-rework.md
var LoomTemplateRework []byte

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

// LoomTemplatePriorPlan is the prior-plan block appended to the end of the Plan prompt on a respawn.
//
//go:embed loom/loom-template-prior-plan.md
var LoomTemplatePriorPlan []byte

// LoomTemplateParentReviewDelivery is the one-line prompt that tells the live Discussion-Write session to send its parent a review request.
//
//go:embed loom/loom-template-parent-review-delivery.md
var LoomTemplateParentReviewDelivery []byte

// LoomTemplateParentReviewBrief is the reviewer brief the parent's one-shot fork reads.
//
//go:embed loom/loom-template-parent-review-brief.md
var LoomTemplateParentReviewBrief []byte

// BurlerTemplateReviewOrchestrator is burler's shipped-default reviewer orchestrator prompt.
//
//go:embed burler/burler-template-review-orchestrator.md
var BurlerTemplateReviewOrchestrator []byte

// BurlerTemplateFixOrchestrator is burler's shipped-default fixer orchestrator prompt.
//
//go:embed burler/burler-template-fix-orchestrator.md
var BurlerTemplateFixOrchestrator []byte

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

// BurlerFocusDirective is burler's shipped-default focus-directive block: the text a round's focus file reaches the explore step through, filled with the file's path.
//
//go:embed burler/burler-focus-directive.md
var BurlerFocusDirective []byte

// BouncerTemplateSeed is the Bouncer's shipped-default seed prompt: the focus-setting pass that
// runs before any round has been reviewed.
//
//go:embed bouncer/bouncer-template-seed.md
var BouncerTemplateSeed []byte

// BouncerTemplateJudge is the Bouncer's shipped-default per-round judge prompt.
//
//go:embed bouncer/bouncer-template-judge.md
var BouncerTemplateJudge []byte

// BouncerTemplateEscalation is the Bouncer's shipped-default escalation brief, read by a one-shot
// fork of the run's parent session.
//
//go:embed bouncer/bouncer-template-escalation.md
var BouncerTemplateEscalation []byte

// BouncerTemplateParentNotice is the Bouncer's shipped-default one-line parent notice, which
// points the parent at the escalation brief.
//
//go:embed bouncer/bouncer-template-parent-notice.md
var BouncerTemplateParentNotice []byte

// TreadleTemplateJudgeCircling is treadle's shipped-default per-round circling-check judge prompt.
//
//go:embed treadle/treadle-template-judge-circling.md
var TreadleTemplateJudgeCircling []byte

// TreadleTemplateJudgeMilestone is treadle's shipped-default milestone continuation-gate judge
// prompt.
//
//go:embed treadle/treadle-template-judge-milestone.md
var TreadleTemplateJudgeMilestone []byte

// TreadleTemplateTargeting is treadle's shipped-default pre-round targeting judge prompt.
//
//go:embed treadle/treadle-template-targeting.md
var TreadleTemplateTargeting []byte

// WebsterTemplateMaster is webster's shipped-default Master-session prompt template.
//
//go:embed webster/webster-template-master.md
var WebsterTemplateMaster []byte

// WebsterBodyVerifyFix is webster's shipped-default verify-gate fixer fork prompt.
//
//go:embed webster/webster-body-verify-fix.md
var WebsterBodyVerifyFix []byte

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

// OrchTemplateStart is orch's shipped-default fresh-launch prompt.
//
//go:embed orch/orch-template-start.md
var OrchTemplateStart []byte

// OrchTemplateHandoff is orch's shipped-default one-line handoff instruction.
//
//go:embed orch/orch-template-handoff.md
var OrchTemplateHandoff []byte

// OrchTemplateResume is orch's shipped-default one-line resume prompt.
//
//go:embed orch/orch-template-resume.md
var OrchTemplateResume []byte

// OrchTemplateAdopt is orch's shipped-default one-line launch prompt for an adopted session.
//
//go:embed orch/orch-template-adopt.md
var OrchTemplateAdopt []byte

// OrchTemplateHandoffSoft is orch's shipped-default one-line soft-trigger handoff request.
//
//go:embed orch/orch-template-handoff-soft.md
var OrchTemplateHandoffSoft []byte

// OrchTemplateCompact is orch's shipped-default one-line focus text typed after `/compact`.
//
//go:embed orch/orch-template-compact.md
var OrchTemplateCompact []byte

// OrchTemplateRole is orch's shipped-default role stencil: the whole procedure, rendered to a file the session reads.
//
//go:embed orch/orch-template-role.md
var OrchTemplateRole []byte

// OrchTemplateNote is orch's shipped-default orch note template, rendered to a file the session fills in.
//
//go:embed orch/orch-template-note.md
var OrchTemplateNote []byte

// OrchTemplateReload is orch's shipped-default one-line reload pointer typed after an auto-compaction.
//
//go:embed orch/orch-template-reload.md
var OrchTemplateReload []byte

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

// PatternDirectiveDesigner is the shipped-default PATTERN directive for RoleDesigner.
//
//go:embed pattern/pattern-directive-designer.md
var PatternDirectiveDesigner []byte

// PatternDirectiveJudge is the shipped-default PATTERN directive for RoleJudge.
//
//go:embed pattern/pattern-directive-judge.md
var PatternDirectiveJudge []byte

// FrictionDirectiveImplementer is the shipped-default friction directive for RoleImplementer.
//
//go:embed friction/friction-directive-implementer.md
var FrictionDirectiveImplementer []byte

// FrictionDirectiveReviewFix is the shipped-default friction directive for RoleReviewFix.
//
//go:embed friction/friction-directive-review-fix.md
var FrictionDirectiveReviewFix []byte

// FrictionDirectiveOrchestrator is the shipped-default friction directive for RoleOrchestrator.
//
//go:embed friction/friction-directive-orchestrator.md
var FrictionDirectiveOrchestrator []byte

// FrictionDirectiveInterview is the shipped-default friction directive for RoleInterview.
//
//go:embed friction/friction-directive-interview.md
var FrictionDirectiveInterview []byte

// FrictionTemplateReflection is the reflection agent's shipped-default prompt template.
//
//go:embed friction/friction-template-reflection.md
var FrictionTemplateReflection []byte

// ShedTemplateDriver is the shed driver's shipped-default launch prompt: the whole procedure of the session that drives one seeded run.
//
//go:embed shed/shed-template-driver.md
var ShedTemplateDriver []byte

// ShedTemplateDriverGuide is the driver's shipped-default repair guide: the reference the one-shot fork a driver spawns at an error or interrupted stop reads.
//
//go:embed shed/shed-template-driver-guide.md
var ShedTemplateDriverGuide []byte

// ShedTemplateDriverNotify is the driver's shipped-default parent-notification rule for a run no batten watches.
//
//go:embed shed/shed-template-driver-notify.md
var ShedTemplateDriverNotify []byte

// ShedTemplateDriverNotifyWatched is the driver's shipped-default parent-notification rule for a run a batten watches.
//
//go:embed shed/shed-template-driver-notify-watched.md
var ShedTemplateDriverNotifyWatched []byte

// ParentDirectiveParent is the shared parent directive's shipped-default variant for a run with a recorded parent.
//
//go:embed parent/parent-directive-parent.md
var ParentDirectiveParent []byte

// ParentDirectiveOperatorBan is the shipped-default operator-ban line the parent variant carries for a non-interactive role.
//
//go:embed parent/parent-directive-operator-ban.md
var ParentDirectiveOperatorBan []byte

// ParentDirectiveNone is the shared parent directive's shipped-default no-parent variant.
//
//go:embed parent/parent-directive-none.md
var ParentDirectiveNone []byte

// EditDirective is the shared edit directive's shipped default: the no-script edit rule every spawned role's opening stencil carries.
//
//go:embed edit/edit-directive.md
var EditDirective []byte

// SeatDirectiveChair is the chair seat block's shipped default: the channel rules a multi-seat step's chair works under, pulled into a seat stencil as an include.
//
//go:embed seat/seat-directive-chair.md
var SeatDirectiveChair []byte

// SeatDirectiveAdvisor is the advisor seat block's shipped default: the channel rules an advisor works under, pulled into a seat stencil as an include.
//
//go:embed seat/seat-directive-advisor.md
var SeatDirectiveAdvisor []byte

// registryEntry pairs one stencil's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry: the order stencils are listed here is the order
// `lyx stencil list` prints them in.
var entries = []registryEntry{
	{"landing-template-conflict", &LandingTemplateConflict},
	{"landing-template-describe", &LandingTemplateDescribe},
	{"loom-template-discussion", &LoomTemplateDiscussion},
	{"loom-template-discussion-chair", &LoomTemplateDiscussionChair},
	{"loom-template-discussion-advisor", &LoomTemplateDiscussionAdvisor},
	{"loom-template-plan", &LoomTemplatePlan},
	{"loom-template-rework", &LoomTemplateRework},
	{"loom-rubric-discussion-review", &LoomRubricDiscussionReview},
	{"loom-rubric-plan-review", &LoomRubricPlanReview},
	{"loom-rubric-webster-review", &LoomRubricWebsterReview},
	{"loom-template-prior-plan", &LoomTemplatePriorPlan},
	{"loom-template-parent-review-delivery", &LoomTemplateParentReviewDelivery},
	{"loom-template-parent-review-brief", &LoomTemplateParentReviewBrief},
	{"burler-template-review-orchestrator", &BurlerTemplateReviewOrchestrator},
	{"burler-template-fix-orchestrator", &BurlerTemplateFixOrchestrator},
	{"burler-step-1-explore", &BurlerStep1Explore},
	{"burler-step-2-review", &BurlerStep2Review},
	{"burler-step-3-fix", &BurlerStep3Fix},
	{"burler-focus-directive", &BurlerFocusDirective},
	{"bouncer-template-seed", &BouncerTemplateSeed},
	{"bouncer-template-judge", &BouncerTemplateJudge},
	{"bouncer-template-escalation", &BouncerTemplateEscalation},
	{"bouncer-template-parent-notice", &BouncerTemplateParentNotice},
	{"treadle-template-judge-circling", &TreadleTemplateJudgeCircling},
	{"treadle-template-judge-milestone", &TreadleTemplateJudgeMilestone},
	{"treadle-template-targeting", &TreadleTemplateTargeting},
	{"webster-template-master", &WebsterTemplateMaster},
	{"webster-body-verify-fix", &WebsterBodyVerifyFix},
	{"webster-prefix-fork", &WebsterPrefixFork},
	{"webster-prefix-recovery", &WebsterPrefixRecovery},
	{"webster-body-implementer", &WebsterBodyImplementer},
	{"orch-template-start", &OrchTemplateStart},
	{"orch-template-handoff", &OrchTemplateHandoff},
	{"orch-template-resume", &OrchTemplateResume},
	{"orch-template-adopt", &OrchTemplateAdopt},
	{"orch-template-handoff-soft", &OrchTemplateHandoffSoft},
	{"orch-template-compact", &OrchTemplateCompact},
	{"orch-template-role", &OrchTemplateRole},
	{"orch-template-note", &OrchTemplateNote},
	{"orch-template-reload", &OrchTemplateReload},
	{"pattern-directive-implementer", &PatternDirectiveImplementer},
	{"pattern-directive-review-fix", &PatternDirectiveReviewFix},
	{"pattern-directive-orchestrator", &PatternDirectiveOrchestrator},
	{"pattern-directive-designer", &PatternDirectiveDesigner},
	{"pattern-directive-judge", &PatternDirectiveJudge},
	{"friction-directive-implementer", &FrictionDirectiveImplementer},
	{"friction-directive-review-fix", &FrictionDirectiveReviewFix},
	{"friction-directive-orchestrator", &FrictionDirectiveOrchestrator},
	{"friction-directive-interview", &FrictionDirectiveInterview},
	{"friction-template-reflection", &FrictionTemplateReflection},
	{"shed-template-driver", &ShedTemplateDriver},
	{"shed-template-driver-guide", &ShedTemplateDriverGuide},
	{"shed-template-driver-notify", &ShedTemplateDriverNotify},
	{"shed-template-driver-notify-watched", &ShedTemplateDriverNotifyWatched},
	{"parent-directive-parent", &ParentDirectiveParent},
	{"parent-directive-operator-ban", &ParentDirectiveOperatorBan},
	{"parent-directive-none", &ParentDirectiveNone},
	{"edit-directive", &EditDirective},
	{"seat-directive-chair", &SeatDirectiveChair},
	{"seat-directive-advisor", &SeatDirectiveAdvisor},
}

// roleOpeningStencils maps each spawned role other than the orch to the stencils that open its session.
// Each carries both the parent directive marker and the edit directive marker, which parentdirective_test.go enforces.
var roleOpeningStencils = map[string][]string{
	"driver":             {"shed-template-driver"},
	"discussion":         {"loom-template-discussion"},
	"discussion-chair":   {"loom-template-discussion-chair"},
	"discussion-advisor": {"loom-template-discussion-advisor"},
	"plan":               {"loom-template-plan"},
	"rework":             {"loom-template-rework"},
	"webster-master":     {"webster-template-master"},
	"webster-recovery":   {"webster-prefix-recovery"},
	"burler-review":      {"burler-template-review-orchestrator"},
	"burler-fix":         {"burler-template-fix-orchestrator"},
	"conflict":           {"landing-template-conflict"},
	"bouncer-judge":      {"bouncer-template-judge"},
	"bouncer-seed":       {"bouncer-template-seed"},
	"treadle-targeting":  {"treadle-template-targeting"},
	"treadle-judge":      {"treadle-template-judge-circling", "treadle-template-judge-milestone"},
	"friction":           {"friction-template-reflection"},
	"describe":           {"landing-template-describe"},
}

// askOperatorPhrases is the closed list of phrases that tell an agent to put a question to the operator in its pane.
var askOperatorPhrases = []string{
	"ask the operator",
	"ask the user",
	"ask the human",
	"ask your operator",
	"check with the operator",
	"confirm with the operator",
}

// editRulePhrases is the closed list of phrases distinctive to the no-script edit rule's statement, matched case-insensitively.
var editRulePhrases = []string{
	"`sed`",
	"heredoc",
	"replace_all",
	"edit or write",
}

// askOperatorBounds are the words that make an ask-the-operator sentence a prohibition rather than an instruction.
var askOperatorBounds = []string{
	"do not",
	"don't",
	"never",
	"must not",
	"may not",
	"before",
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
