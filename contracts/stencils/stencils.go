// stencils.go declares every stencil's shipped-default byte var.
// It also holds the name-to-default registry that internal/stencilstore.Registry consumes.
// It is the one place a new stencil is registered, naming both its on-disk path and its Go identifier.
// Its //go:embed directives live at this package root, which sits above every family subfolder they reach.

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

// LandingTemplateFixer is the landing merge fixer's shipped-default prompt.
//
//go:embed landing/landing-template-fixer.md
var LandingTemplateFixer []byte

// LandingTemplateFixReview is the landing fix review's shipped-default prompt.
// It is the solo session that judges an over-cap merge fixer change before Finalize lands it.
//
//go:embed landing/landing-template-fix-review.md
var LandingTemplateFixReview []byte

// LandingTemplatePRReview is the PR-Review session's shipped-default prompt.
// It is the interactive session the operator discusses a run's pull request with.
//
//go:embed landing/landing-template-pr-review.md
var LandingTemplatePRReview []byte

// DarnTemplateWrite is the darn writer's shipped-default prompt.
// It is the whole instruction set of the one session that makes a board entry's change.
//
//go:embed darn/darn-template-write.md
var DarnTemplateWrite []byte

// DarnFragmentMakeChange is the darn writer prompt's Step 4 list when the verify gate has no cold fixer.
// The writer and its subagents run the gate in it.
//
//go:embed darn/darn-fragment-make-change.md
var DarnFragmentMakeChange []byte

// DarnFragmentMakeChangeGated is the darn writer prompt's Step 4 list when the verify gate has a cold fixer.
// The writer and its subagents run no gate in it.
//
//go:embed darn/darn-fragment-make-change-gated.md
var DarnFragmentMakeChangeGated []byte

// SamplerTemplateAgent is the sampler agent's shipped-default prompt.
// It is the whole instruction set of the interactive session the operator works a change out with.
//
//go:embed sampler/sampler-template-agent.md
var SamplerTemplateAgent []byte

// DriftTemplateExpect is the drift expect agent's shipped-default prompt.
// It is the whole instruction set of the session that records which captured values a task is expected to change.
// It also records the reason for each.
//
//go:embed drift/drift-template-expect.md
var DriftTemplateExpect []byte

// DriftTemplateJudge is the drift judge's shipped-default prompt.
// It is the whole instruction set of the session that rules on each changed captured value of a task.
// It judges each change against the expectation that covers it.
//
//go:embed drift/drift-template-judge.md
var DriftTemplateJudge []byte

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

// LoomDirectiveProjectRecord is the shipped-default project record directive a member run's Discussion-Write lead and darn writer render.
//
//go:embed loom/loom-directive-project-record.md
var LoomDirectiveProjectRecord []byte

// LoomTemplatePlan is the loom Plan producer's shipped-default autonomous prompt.
//
//go:embed loom/loom-template-plan.md
var LoomTemplatePlan []byte

// LoomBlockPlanGlyphs is the plan-format block holding the glyph lookup rule.
// Every plan writer's stencil includes it.
//
//go:embed loom/loom-block-plan-glyphs.md
var LoomBlockPlanGlyphs []byte

// LoomBlockPlanFormat is the plan-format block holding what a card is and the on-disk layout.
// Every plan writer's stencil includes it.
//
//go:embed loom/loom-block-plan-format.md
var LoomBlockPlanFormat []byte

// LoomBlockPlanOverview is the plan-format block holding the overview file format.
// It also holds the glyph and handle target rules, and every plan writer's stencil includes it.
//
//go:embed loom/loom-block-plan-overview.md
var LoomBlockPlanOverview []byte

// LoomBlockPlanCards is the plan-format block holding the card file format, the rename mechanic and the skeleton.
// Every plan writer's stencil includes it.
//
//go:embed loom/loom-block-plan-cards.md
var LoomBlockPlanCards []byte

// LoomTemplateParts is the loom Parts-Write producer's shipped-default autonomous prompt.
//
//go:embed loom/loom-template-parts.md
var LoomTemplateParts []byte

// LoomFragmentPartsLight is the light reading depth of the Parts-Write agent.
// It is filled into the Parts-Write prompt when parts_depth is light.
//
//go:embed loom/loom-fragment-parts-light.md
var LoomFragmentPartsLight []byte

// LoomFragmentPartsMedium is the medium reading depth of the Parts-Write agent.
// It is filled into the Parts-Write prompt when parts_depth is medium.
//
//go:embed loom/loom-fragment-parts-medium.md
var LoomFragmentPartsMedium []byte

// LoomFragmentSelfCheckPlan is the self-check step body of the Plan-Write prompt.
// It has the writer run the plan validator, and it is filled in when the plan gate has no cold fixer.
//
//go:embed loom/loom-fragment-self-check-plan.md
var LoomFragmentSelfCheckPlan []byte

// LoomFragmentSelfCheckPlanGated is the self-check step body of the Plan-Write prompt.
// It has the writer run no validator, and it is filled in when the plan gate has a cold fixer.
//
//go:embed loom/loom-fragment-self-check-plan-gated.md
var LoomFragmentSelfCheckPlanGated []byte

// LoomFragmentSelfCheckDiscussion is the self-check step body of the Discussion-Write prompt.
// It has the writer run the discussion validator, and it is filled in when the discussion gate has no cold fixer.
//
//go:embed loom/loom-fragment-self-check-discussion.md
var LoomFragmentSelfCheckDiscussion []byte

// LoomFragmentSelfCheckDiscussionGated is the self-check step body of the Discussion-Write prompt.
// It has the writer run no validator, and it is filled in when the discussion gate has a cold fixer.
//
//go:embed loom/loom-fragment-self-check-discussion-gated.md
var LoomFragmentSelfCheckDiscussionGated []byte

// LoomTemplatePartWriter is a part writer's shipped-default opening prompt.
// It is the seat that turns one part of the parts file into plan cards.
//
//go:embed loom/loom-template-part-writer.md
var LoomTemplatePartWriter []byte

// LoomTemplatePartsChair is the Parts-Plan-Write chair's shipped-default opening prompt.
// It is the lead that checks the parts, joins them into one plan and writes the shared spots.
//
//go:embed loom/loom-template-parts-chair.md
var LoomTemplatePartsChair []byte

// LoomTemplateArbiter is the loom arbiter's shipped-default opening prompt.
// It is the cold session that settles one stop its run's driver could not.
//
//go:embed loom/loom-template-arbiter.md
var LoomTemplateArbiter []byte

// LoomRubricDiscussionReview is the Discussion-Review rubric, read by every row of the Discussion-Review perch.
//
//go:embed loom/loom-rubric-discussion-review.md
var LoomRubricDiscussionReview []byte

// LoomRubricPlanReview is the Plan-Review rubric, read by every row of the Plan-Review perch.
//
//go:embed loom/loom-rubric-plan-review.md
var LoomRubricPlanReview []byte

// LoomRubricRaddleReview is the Raddle-Review rubric, read by every row of the split perch over a raddle proposal.
//
//go:embed loom/loom-rubric-raddle-review.md
var LoomRubricRaddleReview []byte

// LoomRubricCodeReview is the Code-Review rubric.
// Every row of the Code-Review perch reads it, in whichever recipes hold one.
//
//go:embed loom/loom-rubric-code-review.md
var LoomRubricCodeReview []byte

// LoomTemplatePriorPlan is the prior-plan block appended to the end of the Plan prompt on a respawn.
//
//go:embed loom/loom-template-prior-plan.md
var LoomTemplatePriorPlan []byte

// RaddleTemplateAgent is the raddle agent's shipped-default prompt.
// It is the whole instruction set of the session that proposes how a decision record splits into entries.
// The operator approves those entries.
//
//go:embed raddle/raddle-template-agent.md
var RaddleTemplateAgent []byte

// BurlerStep1Explore is burler's shipped-default step-1 (explore) instruction prompt.
//
//go:embed burler/burler-step-1-explore.md
var BurlerStep1Explore []byte

// BurlerTemplateReviewer is burler's shipped-default reviewer seat stencil, the one seat of the review half's table.
//
//go:embed burler/burler-template-reviewer.md
var BurlerTemplateReviewer []byte

// BurlerTemplateFixer is burler's shipped-default fixer seat stencil, the one seat of the fix half's table.
//
//go:embed burler/burler-template-fixer.md
var BurlerTemplateFixer []byte

// BurlerStepReview is burler's shipped-default review instruction prompt of the review half.
//
//go:embed burler/burler-step-review.md
var BurlerStepReview []byte

// BurlerReviewFormat is burler's shipped-default review-file format, the one format both halves point at.
//
//go:embed burler/burler-review-format.md
var BurlerReviewFormat []byte

// BurlerStepFix is burler's shipped-default fix instruction prompt of the fix half.
//
//go:embed burler/burler-step-fix.md
var BurlerStepFix []byte

// BurlerFocusDirective is burler's shipped-default focus-directive block.
// It is the text a round's focus file reaches the explore step through, filled with the file's path.
//
//go:embed burler/burler-focus-directive.md
var BurlerFocusDirective []byte

// BurlerForkReading is burler's shipped-default fork-reading part.
// It is the pre-read a fanned round's handler does in the explore step, so every fork inherits the reading.
//
//go:embed burler/burler-fork-reading.md
var BurlerForkReading []byte

// BurlerClusterSolo is burler's shipped-default cluster-rules part of a round that names no fan:
// the single-reviewer statement.
//
//go:embed burler/burler-cluster-solo.md
var BurlerClusterSolo []byte

// BurlerClusterFan is burler's shipped-default cluster-rules part of a fanned round:
// the spawn mechanics, the fork rules and the consolidation rules, filled with the round's lens list.
//
//go:embed burler/burler-cluster-fan.md
var BurlerClusterFan []byte

// BurlerFixSelfCheck is burler's shipped-default fix self-check part:
// the procedure by which a twill fixer has read-only forks check its own change set.
// It is filled with the deployed rubric's path and the fixer fan's lens list.
//
//go:embed burler/burler-fix-self-check.md
var BurlerFixSelfCheck []byte

// BurlerFixCheckRubric is burler's shipped-default fix-check rubric:
// the severity vocabulary and finding rules a twill fixer's self-check forks work under.
//
//go:embed burler/burler-fix-check-rubric.md
var BurlerFixCheckRubric []byte

// BouncerTemplateSeed is the Bouncer's shipped-default seed prompt:
// the focus-setting pass that runs before any round has been reviewed.
//
//go:embed bouncer/bouncer-template-seed.md
var BouncerTemplateSeed []byte

// BouncerTemplateJudge is the Bouncer's shipped-default per-round judge prompt.
//
//go:embed bouncer/bouncer-template-judge.md
var BouncerTemplateJudge []byte

// BouncerTemplateEscalation is the Bouncer's shipped-default escalation brief.
// A one-shot fork of the run's parent session reads it.
//
//go:embed bouncer/bouncer-template-escalation.md
var BouncerTemplateEscalation []byte

// BouncerFragmentDecideOpen is the Bouncer's shipped-default decide step of the escalation brief:
// it applies to a round below its perch's hard cap and fills the brief's decide_options marker.
//
//go:embed bouncer/bouncer-fragment-decide-open.md
var BouncerFragmentDecideOpen []byte

// BouncerFragmentDecideCapped is the Bouncer's shipped-default decide step of the escalation brief:
// it applies to a round at or past its perch's hard cap and fills the brief's decide_options marker.
//
//go:embed bouncer/bouncer-fragment-decide-capped.md
var BouncerFragmentDecideCapped []byte

// BouncerFragmentDecision is the Bouncer's shipped-default decision detail of the escalation brief:
// it fills the brief's cause_detail marker for cause decision.
//
//go:embed bouncer/bouncer-fragment-decision.md
var BouncerFragmentDecision []byte

// BouncerTemplateParentNotice is the Bouncer's shipped-default one-line parent notice:
// it points the parent at the escalation brief.
//
//go:embed bouncer/bouncer-template-parent-notice.md
var BouncerTemplateParentNotice []byte

// TreadleTemplateJudgeCircling is treadle's shipped-default per-round circling-check judge prompt.
//
//go:embed treadle/treadle-template-judge-circling.md
var TreadleTemplateJudgeCircling []byte

// TreadleTemplateJudgeMilestone is treadle's shipped-default milestone continuation-gate judge prompt.
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

// WebsterBodyVerifyFix is webster's shipped-default shared verify-gate fixer body.
// The fixer fork's prompt prefix is joined ahead of it to compose the fixer fork prompt,
// and the shared gate-fixer cold frame is joined ahead of it for the cold fixer strand.
//
//go:embed webster/webster-body-verify-fix.md
var WebsterBodyVerifyFix []byte

// WebsterPrefixVerifyFixFork is webster's shipped-default verify-gate fixer fork prompt prefix:
// it is joined ahead of the shared verify-gate fixer body to compose the fixer fork prompt.
//
//go:embed webster/webster-prefix-verify-fix-fork.md
var WebsterPrefixVerifyFixFork []byte

// WebsterTemplatePlanFix is webster's shipped-default cold plan fixer prompt:
// it matches a plan-conflict's cards to the tree.
//
//go:embed webster/webster-template-plan-fix.md
var WebsterTemplatePlanFix []byte

// WebsterPrefixFork is webster's shipped-default in-session fork prompt prefix:
// it is joined ahead of the shared implementer job body to compose the fork prompt.
//
//go:embed webster/webster-prefix-fork.md
var WebsterPrefixFork []byte

// WebsterPrefixRecovery is webster's shipped-default cold-start recovery prompt prefix:
// it is joined ahead of the shared implementer job body to compose the recovery prompt.
//
//go:embed webster/webster-prefix-recovery.md
var WebsterPrefixRecovery []byte

// WebsterFragmentCardFixer is the paragraph that fills the card_gate_fixer marker of the shared implementer job body:
// it applies when a failed card gate call goes through a cold fixer first.
//
//go:embed webster/webster-fragment-card-fixer.md
var WebsterFragmentCardFixer []byte

// WebsterBodyImplementer is webster's shipped-default shared implementer-job body:
// it is composed with both the in-session fork prefix and the cold-start recovery prefix.
//
//go:embed webster/webster-body-implementer.md
var WebsterBodyImplementer []byte

// OrchTemplateStart is orch's shipped-default fresh-launch prompt.
//
//go:embed orch/orch-template-start.md
var OrchTemplateStart []byte

// OrchTemplateHandoff is orch's shipped-default handoff instruction.
//
//go:embed orch/orch-template-handoff.md
var OrchTemplateHandoff []byte

// OrchTemplateResume is orch's shipped-default one-line resume prompt.
//
//go:embed orch/orch-template-resume.md
var OrchTemplateResume []byte

// OrchTemplateHandoffSoft is orch's shipped-default soft-trigger handoff request.
//
//go:embed orch/orch-template-handoff-soft.md
var OrchTemplateHandoffSoft []byte

// OrchTemplateCompact is orch's shipped-default one-line focus text typed after `/compact`.
//
//go:embed orch/orch-template-compact.md
var OrchTemplateCompact []byte

// OrchTemplateNote is orch's shipped-default orch note template, rendered to a file the session fills in.
//
//go:embed orch/orch-template-note.md
var OrchTemplateNote []byte

// OrchTemplateReload is orch's shipped-default one-line reload pointer typed after an auto-compaction.
//
//go:embed orch/orch-template-reload.md
var OrchTemplateReload []byte

// OrchTemplateUsageLimitWake is orch's shipped-default one-line wake line:
// it is typed once a usage limit that stopped the orch should have reset.
//
//go:embed orch/orch-template-usage-limit-wake.md
var OrchTemplateUsageLimitWake []byte

// PatternDirectiveImplementer is the shipped-default PATTERN directive for the implementer role.
//
//go:embed pattern/pattern-directive-implementer.md
var PatternDirectiveImplementer []byte

// PatternDirectiveReviewFix is the shipped-default PATTERN directive for the review-fix role.
//
//go:embed pattern/pattern-directive-review-fix.md
var PatternDirectiveReviewFix []byte

// PatternDirectiveOrchestrator is the shipped-default PATTERN directive for the orchestrator role.
//
//go:embed pattern/pattern-directive-orchestrator.md
var PatternDirectiveOrchestrator []byte

// PatternDirectiveDesigner is the shipped-default PATTERN directive for the designer role.
//
//go:embed pattern/pattern-directive-designer.md
var PatternDirectiveDesigner []byte

// PatternDirectiveJudge is the shipped-default PATTERN directive for the judge role.
//
//go:embed pattern/pattern-directive-judge.md
var PatternDirectiveJudge []byte

// PatternFragmentChanges is the shipped-default fragment a PATTERN directive appends:
// it lists the task's declared PATTERN changes.
//
//go:embed pattern/pattern-fragment-changes.md
var PatternFragmentChanges []byte

// FrictionDirectiveImplementer is the shipped-default friction directive for the implementer role.
//
//go:embed friction/friction-directive-implementer.md
var FrictionDirectiveImplementer []byte

// FrictionDirectiveReviewFix is the shipped-default friction directive for the review-fix role.
//
//go:embed friction/friction-directive-review-fix.md
var FrictionDirectiveReviewFix []byte

// FrictionDirectiveOrchestrator is the shipped-default friction directive for the orchestrator role.
//
//go:embed friction/friction-directive-orchestrator.md
var FrictionDirectiveOrchestrator []byte

// FrictionDirectiveInterview is the shipped-default friction directive for the interview role.
//
//go:embed friction/friction-directive-interview.md
var FrictionDirectiveInterview []byte

// FrictionTemplateReflection is the reflection agent's shipped-default prompt template.
//
//go:embed friction/friction-template-reflection.md
var FrictionTemplateReflection []byte

// ShedTemplateDriver is the shed driver's shipped-default launch prompt:
// the whole procedure of the session that drives one seeded run.
//
//go:embed shed/shed-template-driver.md
var ShedTemplateDriver []byte

// ShedTemplateDriverGuide is the driver's shipped-default repair guide:
// the reference the one-shot fork a driver spawns at an error or interrupted stop reads.
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

// ShedTemplateTackler is the shed tackler's shipped-default opening prompt:
// the cold session that settles one stop the shed driver's rules could not.
// It escalates a stop it cannot settle to a stronger agent or the run's parent.
//
//go:embed shed/shed-template-tackler.md
var ShedTemplateTackler []byte

// ParentDirectiveParent is the shared parent directive's shipped-default variant for a run with a recorded parent.
//
//go:embed parent/parent-directive-parent.md
var ParentDirectiveParent []byte

// ParentDirectiveOperatorBan is the shipped-default operator-ban line:
// the parent variant carries it for a non-interactive role.
//
//go:embed parent/parent-directive-operator-ban.md
var ParentDirectiveOperatorBan []byte

// ParentDirectiveNone is the shared parent directive's shipped-default no-parent variant.
//
//go:embed parent/parent-directive-none.md
var ParentDirectiveNone []byte

// EditDirective is the shared edit directive's shipped default:
// the edit rule every spawned role's opening stencil carries.
//
//go:embed edit/edit-directive.md
var EditDirective []byte

// TwillDirective is the twill directive's shipped default:
// the host-neutral procedure that has a writer review its finished record through fork lenses.
//
//go:embed twill/twill-directive.md
var TwillDirective []byte

// TwillWriterFork is the twill writer-fork part's shipped default:
// the host-neutral procedure in which one fork writes the reviewed files and the writer reviews what it wrote.
//
//go:embed twill/twill-writer-fork.md
var TwillWriterFork []byte

// TwillWriterForkGated is the twill writer-fork part's shipped default for a host whose gate checks the reviewed files:
// the writer-fork procedure with no validator run.
// A gate checks the reviewed files once the writer's turn ends.
//
//go:embed twill/twill-writer-fork-gated.md
var TwillWriterForkGated []byte

// TwillRelaunch is the twill relaunch part's shipped default:
// the host-neutral paragraphs that say what a writer relaunched around its lens spawn call does.
//
//go:embed twill/twill-relaunch.md
var TwillRelaunch []byte

// TwillJudgeCritical is the twill critical-judge part's shipped default:
// the host-neutral steps that have the writer judge each finding as a critical fixer and record each disposition.
//
//go:embed twill/twill-judge-critical.md
var TwillJudgeCritical []byte

// TwillJudgePlain is the twill plain-judge part's shipped default:
// the host-neutral steps that have the writer judge each finding and record whether it was fixed or rejected.
//
//go:embed twill/twill-judge-plain.md
var TwillJudgePlain []byte

// BeamTemplateBoardTidy is the board-tidy agent's shipped-default opening prompt:
// the cold session dobby spawns every few landings to bring the board's notes in line with what the landings built.
//
//go:embed beam/beam-template-board-tidy.md
var BeamTemplateBoardTidy []byte

// SeatDirectiveChair is the chair seat block's shipped default:
// the channel rules a multi-seat step's chair works under, pulled into a seat stencil as an include.
//
//go:embed seat/seat-directive-chair.md
var SeatDirectiveChair []byte

// SeatDirectiveAdvisor is the advisor seat block's shipped default:
// the channel rules an advisor works under, pulled into a seat stencil as an include.
//
//go:embed seat/seat-directive-advisor.md
var SeatDirectiveAdvisor []byte

// SeatDirectiveWriter is the writer seat block's shipped default:
// the channel rules a writer works under, pulled into a writer seat's stencil as an include.
//
//go:embed seat/seat-directive-writer.md
var SeatDirectiveWriter []byte

// SeatDirectiveWriterChair is the writer chair seat block's shipped default:
// the channel rules the lead of a table of writer seats works under, pulled into the lead's stencil as an include.
//
//go:embed seat/seat-directive-writer-chair.md
var SeatDirectiveWriterChair []byte

// SeatDirectiveAssistant is the assistant seat block's shipped default:
// the rules an assistant works under, writing its outputs and nothing else.
// It is pulled into an assistant seat's stencil as an include.
//
//go:embed seat/seat-directive-assistant.md
var SeatDirectiveAssistant []byte

// SeatDirectiveAssistantFanned is the fanned assistant seat block's shipped default:
// the rules an assistant that runs subagents works under, writing its outputs and their reports and nothing else.
// It is pulled into that assistant's stencil as an include.
//
//go:embed seat/seat-directive-assistant-fanned.md
var SeatDirectiveAssistantFanned []byte

// SeatDirectiveAssistantChair is the assistant chair seat block's shipped default:
// how the lead of a table of assistant seats waits for each assistant's end marker.
// It is pulled into the lead's stencil as an include.
//
//go:embed seat/seat-directive-assistant-chair.md
var SeatDirectiveAssistantChair []byte

// BurlerTemplateReviewerAssisted is burler's reviewer seat stencil for a round with an assistant beside the chair:
// the reviewer stencil with the assistant chair's block.
//
//go:embed burler/burler-template-reviewer-assisted.md
var BurlerTemplateReviewerAssisted []byte

// BurlerTemplateAssistReviewer is the burler assistant reviewer seat stencil:
// the assistant seat of an assisted review round, on the engine its model-spec names.
//
//go:embed burler/burler-template-assist-reviewer.md
var BurlerTemplateAssistReviewer []byte

// BurlerTemplateAssistReviewerFanned is burler's assistant reviewer seat stencil for a fanned assistant:
// the assistant reviewer stencil with the lens reports on its write surface and the fanned assistant's block.
//
//go:embed burler/burler-template-assist-reviewer-fanned.md
var BurlerTemplateAssistReviewerFanned []byte

// BurlerAssistConsolidation is the review step's part of an assisted round:
// the chair waits for the assistant and judges its findings as one more report.
//
//go:embed burler/burler-assist-consolidation.md
var BurlerAssistConsolidation []byte

// BurlerAssistClusterFan is an assistant reviewer's fanned cluster-rules part:
// its spawn and in-turn wait come from its engine's assist-fan engine block.
//
//go:embed burler/burler-assist-cluster-fan.md
var BurlerAssistClusterFan []byte

// EngineClaudeAssistFan is the Claude engine's assist-fan block:
// how an assistant reviewer on Claude runs its lens subagents inside its one turn.
//
//go:embed engine/claude/assist-fan.md
var EngineClaudeAssistFan []byte

// EngineCodexAssistFan is the Codex engine's assist-fan block:
// how an assistant reviewer on Codex runs its lens subagents inside its one turn.
//
//go:embed engine/codex/assist-fan.md
var EngineCodexAssistFan []byte

// EngineClaudeAssistReading is the Claude engine's assist-reading block:
// the reading directions an assistant reviewer on Claude follows.
//
//go:embed engine/claude/assist-reading.md
var EngineClaudeAssistReading []byte

// EngineCodexAssistReading is the Codex engine's assist-reading block:
// the reading directions an assistant reviewer on Codex follows.
//
//go:embed engine/codex/assist-reading.md
var EngineCodexAssistReading []byte

// EngineClaudeEditDirective is the Claude engine's copy of the edit directive, rendered for an assistant seat on Claude.
//
//go:embed engine/claude/edit-directive.md
var EngineClaudeEditDirective []byte

// EngineCodexEditDirective is the Codex engine's copy of the edit directive:
// it uses Codex's tool wording and is rendered for an assistant seat on Codex.
//
//go:embed engine/codex/edit-directive.md
var EngineCodexEditDirective []byte

// GatefixFrameCold is the shared cold frame every gate fixer's prompt opens with:
// it is joined ahead of one caller body stencil.
//
//go:embed gatefix/gatefix-frame-cold.md
var GatefixFrameCold []byte

// GatefixBodyDarn is the job body of the cold fixer for the Darn-Write row's gate, joined behind the shared cold frame.
//
//go:embed gatefix/gatefix-body-darn.md
var GatefixBodyDarn []byte

// GatefixBodyPlan is the job body of the cold fixer for the Plan-Write row's gate, joined behind the shared cold frame.
//
//go:embed gatefix/gatefix-body-plan.md
var GatefixBodyPlan []byte

// GatefixBodyDiscussion is the job body of the cold fixer for the Discussion-Write row's gate:
// it is joined behind the shared cold frame.
//
//go:embed gatefix/gatefix-body-discussion.md
var GatefixBodyDiscussion []byte

// GatefixBodyCard is the job body of the cold fixer for a Webster card gate's red agent call:
// it is joined behind the shared cold frame.
//
//go:embed gatefix/gatefix-body-card.md
var GatefixBodyCard []byte

// registryEntry pairs one stencil's registered name with the embedded default bytes behind it.
type registryEntry struct {
	name string
	def  *[]byte
}

// entries is the ordered name-to-default registry,
// and `lym stencil list` prints the stencils in this order.
var entries = []registryEntry{
	{"landing-template-conflict", &LandingTemplateConflict},
	{"landing-template-describe", &LandingTemplateDescribe},
	{"landing-template-fixer", &LandingTemplateFixer},
	{"landing-template-fix-review", &LandingTemplateFixReview},
	{"landing-template-pr-review", &LandingTemplatePRReview},
	{"darn-template-write", &DarnTemplateWrite},
	{"darn-fragment-make-change", &DarnFragmentMakeChange},
	{"darn-fragment-make-change-gated", &DarnFragmentMakeChangeGated},
	{"sampler-template-agent", &SamplerTemplateAgent},
	{"raddle-template-agent", &RaddleTemplateAgent},
	{"drift-template-expect", &DriftTemplateExpect},
	{"drift-template-judge", &DriftTemplateJudge},
	{"loom-template-discussion", &LoomTemplateDiscussion},
	{"loom-template-discussion-chair", &LoomTemplateDiscussionChair},
	{"loom-template-discussion-advisor", &LoomTemplateDiscussionAdvisor},
	{"loom-directive-project-record", &LoomDirectiveProjectRecord},
	{"loom-template-plan", &LoomTemplatePlan},
	{"loom-block-plan-glyphs", &LoomBlockPlanGlyphs},
	{"loom-block-plan-format", &LoomBlockPlanFormat},
	{"loom-block-plan-overview", &LoomBlockPlanOverview},
	{"loom-block-plan-cards", &LoomBlockPlanCards},
	{"loom-template-parts", &LoomTemplateParts},
	{"loom-fragment-parts-light", &LoomFragmentPartsLight},
	{"loom-fragment-parts-medium", &LoomFragmentPartsMedium},
	{"loom-fragment-self-check-plan", &LoomFragmentSelfCheckPlan},
	{"loom-fragment-self-check-plan-gated", &LoomFragmentSelfCheckPlanGated},
	{"loom-fragment-self-check-discussion", &LoomFragmentSelfCheckDiscussion},
	{"loom-fragment-self-check-discussion-gated", &LoomFragmentSelfCheckDiscussionGated},
	{"loom-template-part-writer", &LoomTemplatePartWriter},
	{"loom-template-parts-chair", &LoomTemplatePartsChair},
	{"loom-template-arbiter", &LoomTemplateArbiter},
	{"loom-rubric-discussion-review", &LoomRubricDiscussionReview},
	{"loom-rubric-plan-review", &LoomRubricPlanReview},
	{"loom-rubric-raddle-review", &LoomRubricRaddleReview},
	{"loom-rubric-code-review", &LoomRubricCodeReview},
	{"loom-template-prior-plan", &LoomTemplatePriorPlan},
	{"burler-step-1-explore", &BurlerStep1Explore},
	{"burler-template-reviewer", &BurlerTemplateReviewer},
	{"burler-template-reviewer-assisted", &BurlerTemplateReviewerAssisted},
	{"burler-template-assist-reviewer", &BurlerTemplateAssistReviewer},
	{"burler-template-assist-reviewer-fanned", &BurlerTemplateAssistReviewerFanned},
	{"burler-assist-consolidation", &BurlerAssistConsolidation},
	{"burler-assist-cluster-fan", &BurlerAssistClusterFan},
	{"burler-template-fixer", &BurlerTemplateFixer},
	{"burler-step-review", &BurlerStepReview},
	{"burler-review-format", &BurlerReviewFormat},
	{"burler-step-fix", &BurlerStepFix},
	{"burler-focus-directive", &BurlerFocusDirective},
	{"burler-fork-reading", &BurlerForkReading},
	{"burler-cluster-solo", &BurlerClusterSolo},
	{"burler-cluster-fan", &BurlerClusterFan},
	{"burler-fix-self-check", &BurlerFixSelfCheck},
	{"burler-fix-check-rubric", &BurlerFixCheckRubric},
	{"bouncer-template-seed", &BouncerTemplateSeed},
	{"bouncer-template-judge", &BouncerTemplateJudge},
	{"bouncer-template-escalation", &BouncerTemplateEscalation},
	{"bouncer-fragment-decide-open", &BouncerFragmentDecideOpen},
	{"bouncer-fragment-decide-capped", &BouncerFragmentDecideCapped},
	{"bouncer-fragment-decision", &BouncerFragmentDecision},
	{"bouncer-template-parent-notice", &BouncerTemplateParentNotice},
	{"treadle-template-judge-circling", &TreadleTemplateJudgeCircling},
	{"treadle-template-judge-milestone", &TreadleTemplateJudgeMilestone},
	{"treadle-template-targeting", &TreadleTemplateTargeting},
	{"webster-template-master", &WebsterTemplateMaster},
	{"webster-body-verify-fix", &WebsterBodyVerifyFix},
	{"webster-prefix-verify-fix-fork", &WebsterPrefixVerifyFixFork},
	{"webster-template-plan-fix", &WebsterTemplatePlanFix},
	{"webster-prefix-fork", &WebsterPrefixFork},
	{"webster-prefix-recovery", &WebsterPrefixRecovery},
	{"webster-body-implementer", &WebsterBodyImplementer},
	{"webster-fragment-card-fixer", &WebsterFragmentCardFixer},
	{"orch-template-start", &OrchTemplateStart},
	{"orch-template-handoff", &OrchTemplateHandoff},
	{"orch-template-resume", &OrchTemplateResume},
	{"orch-template-handoff-soft", &OrchTemplateHandoffSoft},
	{"orch-template-compact", &OrchTemplateCompact},
	{"orch-template-note", &OrchTemplateNote},
	{"orch-template-reload", &OrchTemplateReload},
	{"orch-template-usage-limit-wake", &OrchTemplateUsageLimitWake},
	{"pattern-directive-implementer", &PatternDirectiveImplementer},
	{"pattern-directive-review-fix", &PatternDirectiveReviewFix},
	{"pattern-directive-orchestrator", &PatternDirectiveOrchestrator},
	{"pattern-directive-designer", &PatternDirectiveDesigner},
	{"pattern-directive-judge", &PatternDirectiveJudge},
	{"pattern-fragment-changes", &PatternFragmentChanges},
	{"friction-directive-implementer", &FrictionDirectiveImplementer},
	{"friction-directive-review-fix", &FrictionDirectiveReviewFix},
	{"friction-directive-orchestrator", &FrictionDirectiveOrchestrator},
	{"friction-directive-interview", &FrictionDirectiveInterview},
	{"friction-template-reflection", &FrictionTemplateReflection},
	{"shed-template-driver", &ShedTemplateDriver},
	{"shed-template-driver-guide", &ShedTemplateDriverGuide},
	{"shed-template-driver-notify", &ShedTemplateDriverNotify},
	{"shed-template-driver-notify-watched", &ShedTemplateDriverNotifyWatched},
	{"shed-template-tackler", &ShedTemplateTackler},
	{"parent-directive-parent", &ParentDirectiveParent},
	{"parent-directive-operator-ban", &ParentDirectiveOperatorBan},
	{"parent-directive-none", &ParentDirectiveNone},
	{"edit-directive", &EditDirective},
	{"twill-directive", &TwillDirective},
	{"twill-writer-fork", &TwillWriterFork},
	{"twill-writer-fork-gated", &TwillWriterForkGated},
	{"twill-relaunch", &TwillRelaunch},
	{"twill-judge-critical", &TwillJudgeCritical},
	{"twill-judge-plain", &TwillJudgePlain},
	{"seat-directive-chair", &SeatDirectiveChair},
	{"seat-directive-advisor", &SeatDirectiveAdvisor},
	{"seat-directive-writer", &SeatDirectiveWriter},
	{"seat-directive-writer-chair", &SeatDirectiveWriterChair},
	{"seat-directive-assistant", &SeatDirectiveAssistant},
	{"seat-directive-assistant-fanned", &SeatDirectiveAssistantFanned},
	{"seat-directive-assistant-chair", &SeatDirectiveAssistantChair},
	{"engine/claude/assist-fan", &EngineClaudeAssistFan},
	{"engine/codex/assist-fan", &EngineCodexAssistFan},
	{"engine/claude/assist-reading", &EngineClaudeAssistReading},
	{"engine/codex/assist-reading", &EngineCodexAssistReading},
	{"engine/claude/edit-directive", &EngineClaudeEditDirective},
	{"engine/codex/edit-directive", &EngineCodexEditDirective},
	{"beam-template-board-tidy", &BeamTemplateBoardTidy},
	{"gatefix-frame-cold", &GatefixFrameCold},
	{"gatefix-body-darn", &GatefixBodyDarn},
	{"gatefix-body-plan", &GatefixBodyPlan},
	{"gatefix-body-discussion", &GatefixBodyDiscussion},
	{"gatefix-body-card", &GatefixBodyCard},
}

// roleOpeningStencils maps each spawned role other than the orch to the stencils that open its session.
// A key names a role with any segment prefix left off: a review segment's session carries `<prefix>-<key>`.
// Each stencil carries both the parent directive marker and the edit directive marker.
// `parentdirective_test.go` enforces that.
var roleOpeningStencils = map[string][]string{
	"driver":               {"shed-template-driver"},
	"arbiter":              {"loom-template-arbiter"},
	"tackler":              {"shed-template-tackler"},
	"discussion":           {"loom-template-discussion-chair"},
	"discussion-advisor":   {"loom-template-discussion-advisor"},
	"plan":                 {"loom-template-plan"},
	"webster-master":       {"webster-template-master"},
	"webster-recovery":     {"webster-prefix-recovery"},
	"verify-fixer":         {"gatefix-frame-cold"},
	"plan-fixer":           {"webster-template-plan-fix"},
	"gate-fixer":           {"gatefix-frame-cold"},
	"burler-review":        {"burler-template-reviewer", "burler-template-reviewer-assisted"},
	"burler-review-assist": {"burler-template-assist-reviewer", "burler-template-assist-reviewer-fanned"},
	"burler-fix":           {"burler-template-fixer"},
	"conflict":             {"landing-template-conflict"},
	"fixer":                {"landing-template-fixer"},
	"fix-reviewer":         {"landing-template-fix-review"},
	"pr-reviewer":          {"landing-template-pr-review"},
	"bouncer-judge":        {"bouncer-template-judge"},
	"bouncer-seed":         {"bouncer-template-seed"},
	"treadle-targeting":    {"treadle-template-targeting"},
	"treadle-judge":        {"treadle-template-judge-circling", "treadle-template-judge-milestone"},
	"friction":             {"friction-template-reflection"},
	"describe":             {"landing-template-describe"},
	"darn":                 {"darn-template-write"},
	"sampler":              {"sampler-template-agent"},
	"raddle":               {"raddle-template-agent"},
	"drift-expect":         {"drift-template-expect"},
	"drift-judge":          {"drift-template-judge"},
	"board-tidy":           {"beam-template-board-tidy"},
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

// editRulePhrases is the closed list of phrases distinctive to the edit rule's statement, matched case-insensitively.
var editRulePhrases = []string{
	"never by shell",
	"heredoc",
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

// registry implements stencilstore.Registry over the stencils this package embeds.
type registry struct{}

// Names returns every registered stencil's name, in declaration order.
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
// No engine imports this package outside its tests.
// An engine reads a stencil at call time via stencilstore.Read, which needs no registry.
func Registry() stencilstore.Registry {
	return registry{}
}
