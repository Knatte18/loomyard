// claims_test.go holds the one table of raw-wording claims about the embedded stencils.
// Each row names a stencil file, the embedded text, and the claims its prose must keep making.
// A claim is a short, distinctive phrase rather than a paragraph, so ordinary prose edits do not break it.
// A claim's why carries its provenance (crucible round, finding, sabotage proof, live observation); a claim that cites one is protected and is never dropped.
// Marker sets, Fill round-trips, Go-contract co-versioning and the rubric marker allowlist stay in their own tests.

package stencils

import (
	"strings"
	"testing"
)

// claim is one wording assertion about a stencil's raw text.
// must is a phrase the text has to contain; mustNot is a phrase it must not contain.
// A non-empty section limits the check to that heading's section, from the heading line up to the next "## " heading.
type claim struct {
	must    string
	mustNot string
	section string
	why     string
}

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
	}, attackSurfaceClaims...)},
	{"loom-rubric-plan-review.md", LoomRubricPlanReview, append([]claim{
		{must: "independently reviewable/testable unit", why: "granularity is one card per independently reviewable/testable unit"},
		{must: "blast-radius conclusion", why: "ImpactSummary carries a real blast-radius conclusion"},
		{must: "_lyx/discussion/decision-record.md", why: "fidelity is judged against the decision record at its anchor-relative path"},
		{must: "writer/reviewer symmetry note", why: "the rubric names the writer/reviewer symmetry"},
		{must: "does not run is a finding against the plan", why: "verify must cover every targeted package"},
		{must: "commit-subject-mismatch", why: "anything this round's own gate already checks is not flagged, through commit-subject-mismatch"},
		{must: "Dependency edges are derived, never authored", why: "dependency edges are derived, never authored"},
		{must: "no graded blast radius to summarise", why: "Rename carries no ImpactSummary because there is no graded blast radius"},
		{must: "support-log.md", why: "support-log.md is outside this review entirely"},
		{must: "findings.md", why: "the live generation's findings join the answer key"},
		{must: "record.json` carries a `class`", why: "the live generation's round needs a class"},
		{must: "`first_card`", why: "the live generation's round matches first_card"},
		{must: "the decision record alone is the answer key", why: "generation 0 falls back to the decision record alone"},
		{must: "prior-generation/", why: "the prior-generation archive is never a subject"},
		{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"},
	}, attackSurfaceClaims...)},
	{"loom-rubric-webster-review.md", LoomRubricWebsterReview, []claim{
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
	{"loom-template-plan.md", LoomTemplatePlan, append([]claim{
		{must: "ends with a `Prior plan` section", why: "the template tells the agent to act on a trailing Prior plan section before writing"},
		{must: "must cover every package any card targets", why: "the verify section covers every targeted package"},
		{must: "hermetic build-tagged tests", why: "the verify section includes hermetic build-tagged tests"},
		{must: "compiled rather than run", why: "live-substrate tags are compiled rather than run"},
		{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"},
	}, attackSurfaceClaims...)},
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
	}},
	{"bouncer-template-seed.md", BouncerTemplateSeed, focusEntryClaims()},
	{"bouncer-template-judge.md", BouncerTemplateJudge, append([]claim{
		{must: "ratify or reject each departure explicitly", why: "the judge ratifies or rejects each focus departure"},
		{must: "restates every site, commit and claim it depends on", why: "a focus entry is self-contained"},
		{must: "never refers the reviewer to a prior round's review, fixer report, or finding ID", why: "a focus entry never points at a prior round"},
	}, focusEntryClaims()...)},
	{"webster-body-implementer.md", WebsterBodyImplementer, []claim{
		{must: "{{.specs_dir}}", why: "a normative citation names the deployed specs through the marker, so a bare path cannot creep back"},
	}},
	{"friction-directive-implementer.md", FrictionDirectiveImplementer, frictionOptionalClaims},
	{"friction-directive-review-fix.md", FrictionDirectiveReviewFix, frictionOptionalClaims},
	{"friction-directive-orchestrator.md", FrictionDirectiveOrchestrator, frictionOptionalClaims},
	{"friction-directive-interview.md", FrictionDirectiveInterview, frictionOptionalClaims},
}

// focusEntryClaims is what both Bouncer stencils say about a focus entry.
func focusEntryClaims() []claim {
	return []claim{
		{must: "never caps severity", why: "a focus entry never caps severity"},
		{must: "never pre-states a verdict", why: "a focus entry never pre-states a verdict"},
		{must: "against the rubric's `Do not flag` list", why: "a focus entry is weighed against the rubric's Do not flag list"},
	}
}

// sectionOf returns the text from the line equal to heading up to the next "## " heading, and whether the heading exists.
func sectionOf(text, heading string) (string, bool) {
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimRight(line, "\r\n") == heading {
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

func TestStencilClaims(t *testing.T) {
	for _, row := range wordingClaims {
		t.Run(row.file, func(t *testing.T) {
			text := string(row.text)
			for _, c := range row.claims {
				scope, where := text, "the file"
				if c.section != "" {
					section, ok := sectionOf(text, c.section)
					if !ok {
						t.Errorf("%s has no %q section; claim %q (%s) cannot be checked", row.file, c.section, c.must+c.mustNot, c.why)
						continue
					}
					scope, where = section, "the "+c.section+" section"
				}
				if c.must != "" && !strings.Contains(scope, c.must) {
					t.Errorf("%s: %s does not contain %q; want: %s", row.file, where, c.must, c.why)
				}
				if c.mustNot != "" && strings.Contains(scope, c.mustNot) {
					t.Errorf("%s: %s contains %q; want it absent: %s", row.file, where, c.mustNot, c.why)
				}
			}
		})
	}
}

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
}
