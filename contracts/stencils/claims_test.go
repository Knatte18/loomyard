// claims_test.go holds the one table of raw-wording claims about the embedded stencils.
// Each row names a stencil file, the embedded text, and the claims its prose must keep making.
// A claim is a short, distinctive phrase rather than a paragraph, so ordinary prose edits do not break it.
// A claim's why carries its provenance (crucible round, finding, sabotage proof, live observation); a claim that cites one is protected and is never dropped.
// Marker sets, Fill round-trips, Go-contract co-versioning and the rubric marker allowlist stay in their own tests.

package stencils

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
)

// claim is one wording assertion about a stencil's raw text.
// must is a phrase the text has to contain; mustNot is a phrase it must not contain.
// A non-empty section limits the check to that heading's section, from the heading line up to the next "## " heading.
// A non-empty branch limits it to the bullet that starts with that phrase, from the phrase up to the next blank line.
// anyCase matches the phrase case-insensitively.
type claim struct {
	must    string
	mustNot string
	section string
	branch  string
	anyCase bool
	why     string
}

// wantAll is one must claim per phrase, all sharing a why.
func wantAll(why string, phrases ...string) []claim {
	claims := make([]claim, len(phrases))
	for i, p := range phrases {
		claims[i] = claim{must: p, why: why}
	}
	return claims
}

// wantNone is one mustNot claim per phrase, all sharing a why.
func wantNone(why string, phrases ...string) []claim {
	claims := make([]claim, len(phrases))
	for i, p := range phrases {
		claims[i] = claim{mustNot: p, why: why}
	}
	return claims
}

// inSection scopes every claim to one heading's section.
func inSection(heading string, claims []claim) []claim {
	for i := range claims {
		claims[i].section = heading
	}
	return claims
}

// droppedConceptClaims bars the batch-era concepts from a stencil, matching the two concept words in any case.
func droppedConceptClaims() []claim {
	return []claim{
		{mustNot: "oversized", anyCase: true, why: droppedConceptsWhy},
		{mustNot: "chain", anyCase: true, why: droppedConceptsWhy},
		{mustNot: "## Scope", why: droppedConceptsWhy},
	}
}

// joinClaims concatenates claim groups into one row.
func joinClaims(groups ...[]claim) []claim {
	var all []claim
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

const (
	websterDoneCheckSection = "## A done-check failure arrives as `batch_failed`"
	websterPlanDriftSection = "## A plan-drift refusal ends your run as stuck"
	websterGateSection      = "## A gate failure: spawn one fixer fork"
	websterOutcomeKeysLine  = "`{{.outcome_path}}` itself carries exactly these three keys, quoted here, exactly:"
	orchNoScribeHandoffWhy  = "the orch stencils carry their own handoff procedure, so none names the scribe handoff skill"
	droppedConceptsWhy      = "the batch-era concept (oversized batches, deferred-verify chains, the per-batch Scope section) is dropped from the prompt"
)

// stencilClaims is one row per stencil file.
type stencilClaims struct {
	file   string
	text   []byte
	claims []claim
}

// frictionOptionalClaims is the property every friction directive shares: writing the note is optional, and an absent note is normal.
var frictionOptionalClaims = []claim{
	{must: "**optional**", why: "the note is optional"},
	{must: "normal outcome and never an error", why: "an absent note is normal"},
}

// attackSurfaceClaims are the two phrases every writer and reviewer of a new edge or weakened guard carries.
var attackSurfaceClaims = []claim{
	{must: "skip or let through", why: "attack-surface question asks what the guard can skip or let through"},
	{must: "what bounds it", why: "attack-surface question asks what bounds it"},
}

const discussionFenceSection = "## What you may write"

var wordingClaims = []stencilClaims{
	{"loom-rubric-discussion-review.md", LoomRubricDiscussionReview, append([]claim{
		{must: "Notes for the plan writer", why: "a missing Notes section is not a deficiency"},
		{must: "Rejected alternatives", why: "missing rejected alternatives is by design"},
		{must: "Plan-Write`'s own quarry lookups", why: "incomplete cross-reference enumeration belongs to the compiler and Plan-Write"},
		{must: "Relocation and exclusion findings", why: "relocation and exclusion findings are legitimate"},
		{must: "completeness-before-leanness test", why: "completeness comes before leanness"},
		{must: "writer/reviewer symmetry note", why: "the rubric names the writer/reviewer symmetry"},
		{must: "## Finding class", why: "the rubric defines the finding classes for its segment"},
		{must: "`design` means", why: "the rubric defines the gating class for its segment"},
	}, attackSurfaceClaims...)},
	{"loom-rubric-plan-review.md", LoomRubricPlanReview, append([]claim{
		{must: "independently reviewable unit", why: "granularity is one card per independently reviewable unit"},
		{must: "part of a module's public surface", why: "granularity gives a public-surface symbol its own card"},
		{mustNot: "reviewable/testable", why: "granularity no longer turns on testability"},
		{must: "`PATTERN-test-economy`", why: "a redundant planned test is a finding, grounded by the rule"},
		{must: "blast-radius conclusion", why: "ImpactSummary carries a real blast-radius conclusion"},
		{must: "_lyx/discussion/decision-record.md", why: "fidelity is judged against the decision record at its anchor-relative path"},
		{must: "writer/reviewer symmetry note", why: "the rubric names the writer/reviewer symmetry"},
		{must: "does not run is a finding against the plan", why: "verify must cover every targeted package"},
		{must: "`llm` tag that the section compiles rather than runs", why: "the rubric does not flag a section that compiles the llm tag"},
		{must: "`tmux` tag is token-free, so a section that runs it", why: "the rubric does not flag a section that runs the tmux tag"},
		{must: "`delete-target-gone` fires only at Webster's dispatch", why: "anything this round's own gate already checks is not flagged, and the one check the gate never runs is named"},
		{must: "Dependency edges are derived, never authored", why: "dependency edges are derived, never authored"},
		{must: "no graded blast radius to summarise", why: "Rename carries no ImpactSummary because there is no graded blast radius"},
		{must: "support-log.md", why: "support-log.md is outside this review entirely"},
		{must: "findings.md", why: "the live generation's findings join the answer key"},
		{must: "record.json` carries a `class`", why: "the live generation's round needs a class"},
		{must: "`first_card`", why: "the live generation's round matches first_card"},
		{must: "the decision record alone is the answer key", why: "generation 0 falls back to the decision record alone"},
		{must: "prior-generation/", why: "the prior-generation archive is never a subject"},
		{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"},
		{must: "## Finding class", why: "the rubric defines the finding classes for its segment"},
		{must: "`design` means", why: "the rubric defines the gating class for its segment"},
	}, attackSurfaceClaims...)},
	{"loom-rubric-webster-review.md", LoomRubricWebsterReview, []claim{
		{must: "## Finding class", why: "the rubric defines the finding classes for its segment"},
		{must: "`design` means", why: "the rubric defines the gating class for its segment"},
		{must: "`PATTERN-test-economy`", why: "a redundant or uncovered test is a finding, grounded by the rule"},
		{must: "git merge-base", why: "the review range is derived via git merge-base"},
		{must: "could not be determined", why: "an undeterminable review range raises a BLOCKING finding"},
		{must: "carries a `class`", why: "the rework branch finds the live round by class"},
		{must: "`first_card`", why: "the rework branch finds the live round by first_card"},
		{must: "`head_sha`", why: "the rework range starts at the rejected head"},
		{must: "git log --first-parent --no-merges", why: "the rework range excludes mid-run merges"},
		{must: "Plan-Write`'s and `Plan-Burler`'s own gates", why: "anything the plan's own gates already check is not flagged"},
		{must: "measuring stick and never the subject", why: "the plan is the measuring stick and never the subject"},
		{must: "_lyx/reviews/webster/", why: "this segment's own round artifacts are never the subject"},
		{must: "target repository's own conventions", why: "comment-convention compliance checks the target repository's own conventions"},
		{must: "outranks a convention inferred from the surrounding code", why: "a written rule outranks a convention inferred from surrounding code"},
		{must: "A line width is never inferred from the surrounding code", why: "a line width is never inferred from surrounding code"},
		{must: "assert-no-callers", why: "the per-card mechanical check names assert-no-callers for a Delete card"},
		{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"},
	}},
	{"loom-template-plan.md", LoomTemplatePlan, joinClaims(wantAll("the card-granularity contract reaches the agent", "What a card is", "independently committable", "Carries the coverage for the behavior it introduces", "`PATTERN-test-economy`", "no substitute for test coverage"),
		wantNone("the old bundle-a-test wording cannot return", "Bundles its own test", "not a substitute for a bundled test"),
		wantAll("a reworded message's asserting tests join the card's targets", "greps the repository for the old text", "every test asserting it joins that card's targets"),
		wantAll("root: resolution rules reach the agent", "`root:` is optional", "`<root>/<path>`", "worktree-root-relative"),
		wantAll("Uses: and overlap semantics reach the agent", "names what the card reads but does not change", "is a contradiction: is it being changed, or only read?"),
		wantAll("a card's verify is runnable shell, never prose, and exceptional next to the plan-level verify", "runnable shell commands", "never prose", "exceptional rather than routine", "the single integration check for the whole plan"),
		wantAll("the type-label grammar reaches the agent", "one or more bold type labels from `**Create:**`, `**Edit:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**`", "sub-bullets are the card's targets for that label"),
		wantAll("the ImpactSummary requirement reaches the agent", "`**ImpactSummary:**` on `Edit`/`Delete` cards only", "inline on the label line"),
		wantNone("skills load from the spec, so the stencil names none", "scribe:prose", "scribe:testing"),
		wantAll("the parent directive renders ahead of the pattern directive", "{{.parent_directive}}"),
		wantNone("the Plan producer is autonomous, with no operator for chat-reply discipline to serve", "scribe:conversation"),
		wantAll("every glyph lookup goes through the lyx quarry verb group, and a line copied from a quarry answer is copied verbatim",
			"lyx quarry glyphs <dir>", "lyx quarry glyphs --text <dir>", "lyx quarry resolve <glyph>...", "lyx quarry toc <path>", "lyx quarry expand <glyph>",
			"you copy a line verbatim out of a quarry answer"),
		wantNone("delta and name are pipeline-internal and are never named to the planner", "lyx quarry delta", "lyx quarry name"),
		wantAll("the closing step runs validate-plan until it exits 0", "lyx loom validate-plan", "re-run it until it exits 0"),
		wantAll("a scope addition after the Discussion is recorded with decision add, never written as an operator addition", "lyx loom decision add", "`--by`", "Never write such an addition into the plan as an operator addition"),
		[]claim{
			{must: "ends with a `Prior plan` section", why: "the template tells the agent to act on a trailing Prior plan section before writing"},
			{must: "must cover every package any card targets", why: "the verify section covers every targeted package"},
			{must: "hermetic build-tagged tests", why: "the verify section includes hermetic build-tagged tests"},
			{must: "compiled rather than run", why: "live-substrate tags are compiled rather than run"},
			{must: "`llm` tag, which gates tests that spawn a real LLM, is compiled rather than run", why: "the plan template names the llm tag as compiled, never run"},
			{must: "`tmux` tag is a token-free tier that a plan's `## verify:` may run", why: "the plan template names the tmux tag as runnable by a plan verify"},
			{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"},
		},
		attackSurfaceClaims,
	)},
	{"loom-template-prior-plan.md", LoomTemplatePriorPlan, []claim{
		{must: "## Prior plan", why: "names the section"},
		{must: "Read the archived plan before writing anything", why: "read before writing"},
		{must: "by copying it into the plan directory rather than re-deriving it", why: "carry forward by copying"},
		{must: "Re-verify every glyph you carry forward with `lyx quarry`", why: "re-verify carried glyphs with quarry"},
		{must: "Never modify the archive directory", why: "the archive is never modified"},
		{must: "Never copy `00-overview.md` from the archive", why: "the archived overview is never copied"},
		{must: "`approved: false`", why: "the rewritten overview is unapproved"},
	}},
	{"loom-template-discussion.md", LoomTemplateDiscussion, append([]claim{
		{must: "read-only to you", why: "observed live: the Discussion writer rewrote _lyx/config/loom.yaml mid-run and flipped discussion_interactive, so the default is read-only"},
		{must: "`_lyx/config/`", why: "observed live: the driver's own config is fenced"},
		{must: "lyx config reconcile --apply", why: "observed live: reconcile --apply counts as an edit of the fenced config"},
		{must: "`_lyx/shed/<slug>/`", why: "the phase machine's status file is fenced"},
		{must: "`_lyx/plan/`", why: "a later phase's artifact is fenced"},
		{must: "no `git add`", why: "mutating git is fenced"},
		{must: "do not repair it", why: "a broken environment is reported, never repaired"},
		{must: "not a test to write", why: "a must-cover scenario is a behavior, which an existing test or table row may meet"},
		{must: "lyx board list", why: "Step 1 reads the wider board"},
		{must: "{{.decision_record_path}}", section: discussionFenceSection, why: "the fence names the output files by marker, so the permitted set cannot drift from the paths Step 5 writes"},
		{must: "{{.support_log_path}}", section: discussionFenceSection, why: "the fence names the output files by marker, so the permitted set cannot drift from the paths Step 5 writes"},
		{must: "lyx loom review delivered", section: discussionFenceSection, why: "the fence carves out the parent-review delivered verb"},
		{must: "parent-review entry to `## Review rounds`", section: discussionFenceSection, why: "the fence carves out the parent-review round entry"},
	}, attackSurfaceClaims...)},
	{"burler-focus-directive.md", BurlerFocusDirective, []claim{
		{must: "The rubric binds over the focus directive", why: "precedence: rubric over directive"},
		{must: "steers attention and order, not verdicts", why: "a focus directive steers, never judges"},
		{must: "a valid verdict", why: "a departure from the focus is a valid verdict"},
		{must: "Severity comes from the rubric's mapping", why: "severity stays with the rubric"},
		{must: "advisory and yields to evidence", why: "the focus is advisory"},
		{must: "`exclude_lenses` keeps its mechanical meaning", why: "exclude_lenses is not softened by the focus"},
		{must: "never licenses reading a prior round's files before your review is saved", why: "a focus entry never licenses reading prior rounds first"},
		{must: "followed only as far as its own text goes", why: "a focus entry is followed only as far as its text goes"},
		{must: "`## Focus departures`", why: "departures are reported under their heading"},
	}},
	{"burler-step-2-review.md", BurlerStep2Review, []claim{
		{must: "## Focus departures", why: "the review format has a Focus departures section"},
		{must: "Cluster rules", why: "instruction 2 carries the Cluster rules section the composed cluster block fills"},
		{must: "{{.cluster_rules}}", why: "the Cluster rules section is filled through its marker, so renaming the marker cannot go unnoticed"},
		{must: "`class`", why: "every finding carries the required class key"},
		{must: "`design`", why: "the review format names the design class"},
		{must: "`scope`", why: "the review format names the scope class"},
		{must: "`decision`", why: "the review format names the decision class"},
		{must: "`consistency`", why: "the review format names the consistency class"},
	}},
	{"burler-template-round-orchestrator.md", BurlerTemplateRoundOrchestrator, []claim{
		{must: "Sequencing rule", why: "the orchestrator states the sequencing rule between its two jobs"},
		{must: "fully written to", why: "the review file is fully written before the fix job starts"},
		{must: "before you touch", why: "the review is saved before the orchestrator touches a target file"},
	}},
	{"burler-step-3-fix.md", BurlerStep3Fix, []claim{
		{must: "not whether it gets fixed", why: "every finding is fixed, severity decides only the order"},
		{must: "never push", why: "the fixer commits to code source and never pushes"},
		{must: "nothing fixed", why: "the fixer report states when nothing was fixed"},
		{must: "adds a test only when the finding is a coverage gap", why: "a fix grows the suite only for a coverage gap, per PATTERN-test-economy"},
	}},
	{"bouncer-template-seed.md", BouncerTemplateSeed, focusEntryClaims()},
	{"bouncer-template-judge.md", BouncerTemplateJudge, append([]claim{
		{must: "ratify or reject each departure explicitly", why: "the judge ratifies or rejects each focus departure"},
		{must: "restates every site, commit and claim it depends on", why: "a focus entry is self-contained"},
		{must: "never refers the reviewer to a prior round's review, fixer report, or finding ID", why: "a focus entry never points at a prior round"},
		{must: "Read the round facts at `{{.facts_path}}`", why: "the judge reads the facts file first"},
		{must: "Read the latest review at `{{.report_path}}`", why: "the judge reads the latest review"},
		{must: "Read the previous ledger at `{{.previous_ledger}}`", why: "the judge reads the previous ledger"},
		{must: "{{.decision_rule}}", why: "the decision rule is Go-held so CIRCLING is offered only from the checkpoint round on"},
		{must: "legal only where the decision rule", why: "a CIRCLING verdict is legal only where the decision rule offers it"},
		{must: "counts as a gating finding when the latest review's finding", why: "a recurring key takes its class from the latest review"},
		{must: "Never relabel a BLOCKING finding downward", why: "the departure bound protects BLOCKING findings"},
		{must: "Name the ID of every departed finding", why: "every departure is named in the rationale"},
		{must: "Fixed findings never count as convergence", why: "a fix is new text no fresh reviewer has seen"},
		{must: "is no convergence signal", why: "the review's own top-level verdict does not converge the loop"},
		{mustNot: "{{.artifacts}}", why: "the judge no longer reads the artifacts"},
	}, focusEntryClaims()...)},
	{"bouncer-template-escalation.md", BouncerTemplateEscalation, joinClaims(
		wantAll("the brief tells the fork to record design calls and the decision and to resume the run",
			"lyx loom decision add", "lyx loom circling accept", "lyx loom circling continue", "lyx loom start"),
		wantAll("the brief names both causes", "`circling`", "`budget`"),
		wantAll("the brief says accept passes the segment unconverged", "recorded as unconverged"),
		wantAll("a fork that cannot settle the question records nothing and reports back", "record nothing", "The run stays `awaiting`"),
	)},
	{"bouncer-template-parent-notice.md", BouncerTemplateParentNotice, joinClaims(
		wantAll("the notice points the parent at the brief", "{{.brief_path}}"),
		wantNone("the notice interpolates no judge, review or ledger text", "{{.review_path}}", "{{.ledger_path}}", "{{.verdict_path}}"),
	)},
	{"webster-body-implementer.md", WebsterBodyImplementer, joinClaims(
		[]claim{{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"}},
		wantAll("the report carries the minimal fork-return contract's keys", "status:", "head_sha:", "deviations:"),
		wantAll("the fork is told the fresh-read rule and to commit per card",
			"## The FRESH-READ rule", "Commit the card to the repo", "One commit per card is the norm"),
		wantAll("the per-card loop reads the card file, falls back to the Card Index intent when the card's own is empty, and takes a pinned commit subject from the card file",
			"Read the card file",
			"fall back to that card's one-line intent from the Card Index",
			"unless the card FILE carries a `**Commit:**` line"),
		wantAll("the card's gate command is Go-rendered through the card_gates marker, so the fork derives none itself",
			"{{.card_gates}}", "Run the card's gate command from the list below"),
		wantNone("the hand-derived build+unit gate wording is gone", "this card's package's unit tests", "build+unit"),
		wantNone("the superseded report grammar is gone, the report is deliberately minimal", "out_of_scope:", "tests: green"),
		droppedConceptClaims(),
	)},
	{"webster-prefix-fork.md", WebsterPrefixFork, joinClaims(
		wantAll("the fork inherits Master's loop instructions and must be told forcefully not to drive the loop itself or wait for a report only it writes",
			"NEVER run any `lyx webster` command", "YOU are the one who WRITES that report"),
		wantAll("a fork writing Merriam's outcome or summary file forges the run's terminal judgment and halts the run at its exit audit, so the fork prefix names both as never written",
			"NEVER write, create or delete either of them", "`{{.outcome_path}}`", "`{{.summary_path}}`"),
		wantNone("the superseded report grammar is gone, the report is deliberately minimal", "out_of_scope:", "tests: green"),
		droppedConceptClaims(),
	)},
	{"webster-prefix-recovery.md", WebsterPrefixRecovery, droppedConceptClaims()},
	{"webster-body-verify-fix.md", WebsterBodyVerifyFix, joinClaims(
		wantAll("the fixer reads the gate report and fixes the cause in source",
			"{{.report_path}}", "Fix the cause in source"),
		wantAll("the fixer never deletes, skips or weakens a test",
			"Never delete, skip or weaken a test"),
		wantAll("the fixer never touches the plan directory or _lyx",
			"Never touch the plan directory `{{.plan_dir}}` or anything under `_lyx`"),
		wantAll("the fixer inherits Merriam's final-action instruction to write the contract files, so its rules name both as never written",
			"Never write, create or delete Merriam's contract files `{{.outcome_path}}` and `{{.summary_path}}`"),
		wantAll("the fixer commits each fix as fix: <summary> and runs the failing packages' tests with the integration tag",
			"`fix: <summary>`", "`-tags integration`"),
		wantAll("the fixer does not run the plan-level verify and never drives the webster loop",
			"never run the plan-level verify", "NEVER run any `lyx webster` command"),
	)},
	{"webster-template-master.md", WebsterTemplateMaster, joinClaims(
		inSection(websterDoneCheckSection, joinClaims(
			wantNone("postBatchChecks records a deleted-symbol reference as a later-card warning, never a batch_failed done-check",
				"a symbol this batch deleted that the remaining plan still references"),
			wantAll("the done-check section teaches the later-card warning and begin-batch's own way forward, and names the batch_failed subject",
				"A finding about a later card (drift, or a symbol this batch deleted that a later card still references) comes back on the envelope's `warnings` and the batch still records;",
				"that later card's own `begin-batch` refuses it, naming \"edit the plan so the named cards match the tree, run \"lyx webster rebaseline --card NN\" naming each card you edited, then begin-batch NN again\".",
				"A done-check failure comes back as `{\"batch_failed\": true}`"),
		)),
		wantAll("the outcome file's schema keys and values are spelled out, and the summary file's first-line rule is named",
			websterOutcomeKeysLine,
			"outcome: done | stuck | paused",
			`stuck_reason: null | "<one line>"`,
			"batches_done: <int>",
			"{{.summary_path}}",
			"first line `# <title>`"),
		wantAll("the never-touch-`_lyx`, never-self-edit, never-/model and never-named-subagent statements stay in prose, so an edit that waters one down fails here",
			"NEVER run any git command against `_lyx`",
			"NEVER edit, create, or delete any file other than",
			"NEVER use a `/model` switch",
			"NEVER spawn a non-fork or named subagent"),
		wantAll("Master needs no script to read or write a file: it reads with Read and Grep and writes its two contract files with Write",
			"You read files with Read and Grep and write your two contract files with Write; NEVER run a script, an interpreter or a heredoc to read or write a file."),
		wantAll("the plan and state files are read and written as ordinary files at the told plan directory, and an audit finding is never worked around",
			"{{.plan_dir}}` holds the plan",
			"Read and write them all as ordinary files",
			"You never run git against `_lyx` or the plan directory; they are committed for you.",
			"## Audit findings: policy warns, correctness fails the batch",
			"Still never work around an audit",
			"never write outside your two contract files"),
		wantNone("the policy-violation and card-not-done rungs were removed, an audit finding replaces them",
			"## A policy violation ends your run as stuck",
			"once a violation exists",
			"## A card-not-done refusal ends your run as stuck",
			`"card_not_done"`),
		wantAll("the failed progress rung and the batch_failed and report_archived ladder rungs name the verb each one takes, so a run recovers in-session instead of ending stuck",
			"- `failed` → webster rejected that batch's report",
			`"batch_failed": true`,
			`"report_archived": true`,
			"run `lyx webster recover-batch <NN>` backgrounded, then follow the recover-batch rungs",
			"call `lyx webster begin-batch <NN>` and re-fork that batch",
			"`lyx webster rebaseline --card NN`"),
		wantAll("recover-batch's own batch_failed is a failed recovery, a terminal rung, never another recover-batch",
			"- `recover-batch <NN>` refuses with `{\"batch_failed\": true}` → the recovery strand said done but webster's checks rejected its work",
			"Do NOT call `recover-batch` for that batch again"),
		wantAll("a finding recovery cannot check is refused toward run --fresh, so it is terminal for the run too",
			"- `recover-batch <NN>` refuses with `{\"needs_fresh\": true}` →",
			"comes back from `recover-batch` as `{\"needs_fresh\": true}`"),
		wantAll("begin-batch's plan_drifted refusal ends the run as stuck, exactly as a fabric-sync failure does, and the verb is not retried",
			websterPlanDriftSection, `"plan_drifted": true`, "do not retry the verb"),
		inSection(websterPlanDriftSection, wantAll("record-batch and recover-batch refuse with plan_drifted too, and the operator's way forward is restore-plan",
			"`record-batch` and `recover-batch` also refuse with `{\"plan_drifted\": true}`",
			"`lyx webster restore-plan`")),
		wantNone("Merriam spawns no integration fork and reads no integration report",
			"integration fork", "integration_prompt_path", "integration_report_path", "integration report"),
		inSection(websterGateSection, wantAll("on a gate failure Merriam reads the findings, spawns one fixer fork with the Go-rendered prompt, ends its turn, and rewrites its contract files with a Verify gate fixes section on the notification",
			"`Gate findings recorded at …`",
			"Do NOT localize or fix the failure yourself",
			"Spawn exactly ONE fixer fork in the background",
			"{{.verify_fix_prompt_path}}",
			"not an arrival at the gate",
			"## Verify gate fixes",
			"re-arrives at the gate")),
		wantAll("round fable-r1, crucible: a freshly spawned Master classified the injected orchestration prompt as suspicious content, reasoned that no lyx tool was in its toolset and ended its turn asking, which killed ~40% of real spawns; the prompt states it is real, that lyx is a CLI driven through Bash, and that the session gets its bearings through the status verb",
			"get your bearings against the real state on disk",
			"started by `lyx webster run`",
			"it is an ordinary CLI binary",
			"RUNNING it with your",
			"run `lyx webster status`",
			"confirm the harness, the run state, and the plan are all present",
			"No operator answers a question here"),
		wantNone("Master is not told it was started non-interactively, and recovery is never a foreground or re-call loop",
			"non-interactively",
			"re-call `recover-batch`",
			"re-call `lyx webster recover-batch",
			"poll_wait_s",
			"running` snapshot"),
		wantAll("every recover-batch call blocks to a terminal state, so Merriam runs it backgrounded, ends its turn and acts on the completion notification",
			"Run `lyx webster recover-batch <NN>` as a **backgrounded** Bash command, end your turn, and act on the command's completion notification",
			"Never run it in the foreground, never poll it and never `sleep`.",
			"- `recover-batch <NN>` completes with a terminal `status: done`"),
		wantAll("the bracket sequence, the verbatim prompt forwarding, the backgrounded-fork wait discipline and the recovery ladder are stated in prose",
			"`begin-batch` before every fork",
			`subagent_type: "fork"`,
			"with no name",
			"forwarded verbatim",
			"you are an **IMPLEMENTER",
			"STOP reading this Master prompt",
			"never evidence that you are the Master",
			"this instruction is authoritative",
			"BACKGROUNDED agent",
			"End your turn right after spawning it",
			"your turn ended while the fork runs, never a polling loop",
			"`record-batch` on the fork's completion notification",
			"every `recover-batch` run backgrounded, with your turn ended until its completion notification",
			"Drive it STRICTLY in order",
			"re-fork the same batch once",
			"SAME prompt file and no new `begin-batch`",
			`"paused": true`,
			"OR `status: dead`",
			"## A fabric-sync error ends your run as stuck",
			"already has a report",
			"consume that report",
			"`done` → skip",
			"`stuck` → its fork reported stuck",
			"`dead` → its recovery already failed",
			"On the fork's completion notification, rewrite `{{.outcome_path}}` and `{{.summary_path}}` once more"),
		wantNone("Master never polls for a report: await-batch and the sleep poll are not its verbs", "lyx webster await-batch", "sleep 20", "sleep 30"),
		wantAll("the card-list order is the listed one rather than ascending batch number, and no batch is skipped or reordered",
			"the order listed above, top to bottom",
			"NOT necessarily ascending batch number",
			`no batch is ever skipped or reordered because it "looks independent."`),
		wantNone("the retired ordering clauses stay gone", "there is no DAG here to reorder around", "batch N assumes every batch before it is already committed"),
		wantAll("the master template's spawn directive carries the anti-poll clause, so the fixer fork does not continue Master's poll loop",
			"you do NOT poll or wait for any report file", "Your FIRST action is to Read this file"),
		droppedConceptClaims(),
	)},
	{"friction-directive-implementer.md", FrictionDirectiveImplementer, frictionOptionalClaims},
	{"friction-directive-review-fix.md", FrictionDirectiveReviewFix, frictionOptionalClaims},
	{"friction-directive-orchestrator.md", FrictionDirectiveOrchestrator, frictionOptionalClaims},
	{"friction-directive-interview.md", FrictionDirectiveInterview, frictionOptionalClaims},
	{"orch-template-role.md", OrchTemplateRole, joinClaims(
		wantAll("the role file carries every theme of the orch procedure",
			"## The run loop", "## Parent-review", "## Escalations from a child", "## PR-Gate", "## After landing",
			"## The board", "## Where a finding goes", "## Acting without asking", "## Status reports", "## Messaging", "## Which role can do what"),
		wantNone(orchNoScribeHandoffWhy, "scribe:handoff"),
		[]claim{
			{must: "creates the task pair, drives the loom run inside it and tears the pair down", why: "the batten run owns the pair's creation and teardown"},
			{must: "Batten reads the decision and resumes the child itself", why: "batten resumes the child after a PR-Gate decision"},
			{must: "`lyx loom reject <review-file>`", why: "a reject names its review file"},
			{must: "marks the board task done, pushes main and closes the PR", why: "Finalize marks the task done, pushes and closes the PR"},
			{must: "the driver's drive reports from the prime's own copies", why: "after landing the notes and reports are read from the prime"},
			{must: "run `lyx loom start` in the task worktree to resume the run", why: "a circling decision resumes nothing by itself"},
			{must: "A goto leaves the run paused, so run `lyx loom start`", why: "a goto leaves the run paused"},
			{must: "resume it with `lyx batten run <slug>`", why: "a batten pause or goto is resumed by batten run"},
			{must: "`lyx loom decision add`", why: "a design call is recorded with decision add"},
			{mustNot: "`lyx fabric add", why: "batten run creates the pair"},
			{mustNot: "`lyx fabric remove", why: "batten's teardown removes the pair"},
			{mustNot: "squash-merges", why: "Finalize squashes onto main and closes the PR"},
			{mustNot: "start the run again", why: "batten resumes the child itself"},
		}),
	},
	{"orch-template-note.md", OrchTemplateNote, joinClaims(
		wantAll("the note template carries the sections a handoff fills in", "## Doing now", "## Next step with the operator", "## Waiting on the operator"),
		wantNone(orchNoScribeHandoffWhy, "scribe:handoff"),
	)},
	{"orch-template-start.md", OrchTemplateStart, wantNone(orchNoScribeHandoffWhy, "scribe:handoff")},
	{"orch-template-handoff.md", OrchTemplateHandoff, wantNone(orchNoScribeHandoffWhy, "scribe:handoff")},
	{"orch-template-resume.md", OrchTemplateResume, wantNone(orchNoScribeHandoffWhy, "scribe:handoff")},
	{"orch-template-adopt.md", OrchTemplateAdopt, wantNone(orchNoScribeHandoffWhy, "scribe:handoff")},
	{"orch-template-handoff-soft.md", OrchTemplateHandoffSoft, wantNone(orchNoScribeHandoffWhy, "scribe:handoff")},
	{"orch-template-compact.md", OrchTemplateCompact, wantNone(orchNoScribeHandoffWhy, "scribe:handoff")},
	{"friction-template-reflection.md", FrictionTemplateReflection, []claim{
		{must: "Merge notes that describe the same problem into one issue", why: "duplicates are merged, not filed per note"},
		{must: "Drop only a note that is about the task's own code, or that describes nothing wrong", why: "filtering is grouping; intake filters later"},
		{must: "`loom-crash-resume*`", why: "the crash-resume note is one of the Go-written kinds the reflection reads"},
		{must: "file an issue only when the notes, the reason or the trace show a lyx problem behind the event", why: "a halt or crash-resume event is filed only when a lyx problem shows behind it"},
		{must: "states what happened, why it is abnormal and what to do", why: "every issue body carries what happened, why it is abnormal and what to do"},
		{must: "ends with a provenance line naming the task slug", why: "each issue carries the task slug and its source notes"},
		{mustNot: "Tier 1", why: "no Go code files an issue, so the reflection has no Tier 1 filer to defer to"},
		{must: "never quotes the task repository's source, diffs or plan text", why: "the issue repository is public, so the content rule is a hard rule"},
		{must: "File only through `lyx selfreport create`, never through `gh`", why: "filing goes through lyx selfreport create alone"},
		{must: "Write it only after every issue you decided on was created", why: "Go reads the report's existence as proof that filing succeeded"},
		{must: "end your turn without writing the report", why: "a failed filing leaves no report, so the notes stay for the next reflection"},
	}},
	{"shed-template-driver.md", ShedTemplateDriver, []claim{
		{must: "`" + shedrun.ParkMarkerFileName + "`", why: "the stencil names the Go-declared park marker filename, so renaming either side without the other fails"},
		{mustNot: "lyx selfreport create", why: "the driver files no issue itself: the run's reflection is the one filer"},
		{must: "friction: failed", why: "the parent is notified on a done stop whose envelope reports a failed reflection"},
		{must: "`parent_notice`", why: "an awaiting stop's parent_notice is relayed to the parent"},
	}},
}

// focusEntryClaims is what both Bouncer stencils say about a focus entry.
func focusEntryClaims() []claim {
	return []claim{
		{must: "never caps severity", why: "a focus entry never caps severity"},
		{must: "never pre-states a verdict", why: "a focus entry never pre-states a verdict"},
		{must: "against the rubric's `Do not flag` list", why: "a focus entry is weighed against the rubric's Do not flag list"},
	}
}

// sectionOf returns the text from the line equal to heading, or starting with heading and a space, up to the next "## " heading, and whether the heading exists.
// A heading may carry a trailing clause after the part a claim names.
func sectionOf(text, heading string) (string, bool) {
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		if line == heading || strings.HasPrefix(line, heading+" ") {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], ""), true
}

// branchOf returns the text from the first occurrence of start up to the next blank line, and whether start exists.
func branchOf(text, start string) (string, bool) {
	_, rest, ok := strings.Cut(text, start)
	if !ok {
		return "", false
	}
	branch, _, _ := strings.Cut(rest, "\n\n")
	return start + branch, true
}

//lyx:guard
func TestStencilClaims(t *testing.T) {
	for _, row := range wordingClaims {
		t.Run(row.file, func(t *testing.T) {
			text := string(row.text)
			for _, c := range row.claims {
				scope, where := text, "the file"
				if c.branch != "" {
					branch, ok := branchOf(text, c.branch)
					if !ok {
						t.Errorf("%s has no %q branch; claim %q (%s) cannot be checked", row.file, c.branch, c.must+c.mustNot, c.why)
						continue
					}
					scope, where = branch, "the "+c.branch+" branch"
				} else if c.section != "" {
					section, ok := sectionOf(text, c.section)
					if !ok {
						t.Errorf("%s has no %q section; claim %q (%s) cannot be checked", row.file, c.section, c.must+c.mustNot, c.why)
						continue
					}
					scope, where = section, "the "+c.section+" section"
				}
				contains := strings.Contains
				if c.anyCase {
					contains = func(s, substr string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(substr)) }
				}
				if c.must != "" && !contains(scope, c.must) {
					t.Errorf("%s: %s does not contain %q; want: %s", row.file, where, c.must, c.why)
				}
				if c.mustNot != "" && contains(scope, c.mustNot) {
					t.Errorf("%s: %s contains %q; want it absent: %s", row.file, where, c.mustNot, c.why)
				}
			}
		})
	}
}

//testtiming:keep pins the exact text branchOf scopes a claim to, which TestStencilClaims cannot see because a mis-scoped claim there still passes
func TestBranchOf(t *testing.T) {
	text := "intro\n- `a` → one\n  more\n\n- `b` → two\n"
	got, ok := branchOf(text, "- `a` →")
	if !ok || got != "- `a` → one\n  more" {
		t.Errorf("branchOf(a) = %q, %v; want the a bullet only", got, ok)
	}
	if got, ok := branchOf(text, "- `b` →"); !ok || got != "- `b` → two\n" {
		t.Errorf("branchOf(b) = %q, %v; want the b bullet to the end", got, ok)
	}
	if _, ok := branchOf(text, "- `c` →"); ok {
		t.Error("branchOf(c) found a bullet the text lacks")
	}
}

//testtiming:keep pins the exact text sectionOf scopes a claim to, including never matching a longer heading, which TestStencilClaims cannot see because a mis-scoped claim there still passes
func TestSectionOf(t *testing.T) {
	text := "intro\n## One\nalpha\n## Two\nbeta\n"
	got, ok := sectionOf(text, "## One")
	if !ok || got != "## One\nalpha\n" {
		t.Errorf("sectionOf(One) = %q, %v; want the One section only", got, ok)
	}
	if got, ok := sectionOf(text, "## Two"); !ok || got != "## Two\nbeta\n" {
		t.Errorf("sectionOf(Two) = %q, %v; want the Two section to the end", got, ok)
	}
	if _, ok := sectionOf(text, "## Three"); ok {
		t.Error("sectionOf(Three) found a heading the text lacks")
	}

	clause := "## Onerous\ndelta\n## One rule, with a clause\ngamma\n"
	if got, ok := sectionOf(clause, "## One rule,"); !ok || got != "## One rule, with a clause\ngamma\n" {
		t.Errorf("sectionOf(One rule,) = %q, %v; want the heading with its trailing clause", got, ok)
	}
	if got, ok := sectionOf(clause, "## One"); !ok || got != "## One rule, with a clause\ngamma\n" {
		t.Errorf("sectionOf(One) = %q, %v; want the One heading, never Onerous", got, ok)
	}
}
