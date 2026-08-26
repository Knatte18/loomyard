MILL_REVIEW_BEGIN
# Review: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
duration_s: 266.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class; exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:consistency] plan-unapproved's position in Validate's order
**Demoted-from:** BLOCKING
**Section:** § Decisions → `validate-splits-into-two-named-functions`, and § Testing → "the validate split"
**Issue:** The decision says `plan-unapproved` "moves to the wrapper" and `Validate` = `ValidateFormat`'s fifteen plus the approval check, which appends it sixteenth; the Testing section then requires "`plan-unapproved` in position two" matching `loom-plan-spec.md:202`'s fixed sixteen-row order, and `validate.go:59-73` today emits it second. The two statements cannot both hold.
**Fix:** State explicitly whether `Validate` splices the approval finding at index 1 (preserving the spec's fixed order) or appends it last and `loom-plan-spec.md`'s ordered list is renumbered — and say which in the same sentence that defines `Validate`.

### [NIT:consistency] Rubric reword: fifteen over a contiguous range
**Section:** § Technical context → `loom-rubric-plan-review.md`
**Issue:** The disposition is "reword the count and the upstream claim", but `:31`'s phrasing is a range — "`format-unrecognized` through `commit-subject-mismatch`" — and `plan-unapproved` sits at position 2 inside it, so "fifteen … through commit-subject-mismatch" is self-contradictory unless the ordering question above is settled first.
**Fix:** Name the replacement phrasing (e.g. an explicit "except `plan-unapproved`, enforced downstream at Plan-Revalidate" clause), tied to whichever ordering the BLOCKING resolves to.

### [NIT:decision] gatefindings_test.go disposition left as an unchosen either/or
**Section:** § Technical context → "Existing tests that encode the old behaviour", and § Testing → `internal/loomshed`
**Issue:** Both places offer the same two options — a genuinely format-invalid single-finding fixture, or running the test in `require_approved` mode — with no pick, unlike every other test disposition in the artefact which names a preferred option.
**Fix:** Pick one; the test's stated subject is "exactly one finding reaches the log line", so say which fixture shape delivers that.

### [NIT:consistency] NewPlanValidate call-site count contradicts its own list
**Section:** § Decisions → `planvalidate-row-mode`
**Issue:** "touches four in-repo call sites, all in tests" is followed by a list naming three files plus "the four constructions in `planvalidate_test.go`" (seven), and the enumeration omits `internal/loomcli/parity_test.go:197` and the second `shape_test.go:52` construction — both verified present, and parity_test.go was named by an earlier review round.
**Fix:** Drop the count, or state it as "every `loomshed.NewPlanValidate` construction in the repo" and let the build enumerate; the number as written is quotable and false.

### [NIT:scope] Plan-Burler's recipe instructions still claim upstream approval enforcement
**Section:** § Scope (docs list) and § Technical context
**Issue:** `contracts/recipes/loom-recipe.yaml:158-161` tells the Plan-Burler overlay fixer the format checks are "already enforced upstream by Plan-Validate", which stops being true for `plan-unapproved`; the docs list covers the rubric and the stencil but not this instructions string, and nothing states whether the fixer round may itself write `approved:`.
**Fix:** Add a disposition for that row's `instructions` text, and state whether the "never self-approve" rule binds the Plan-Burler fixer as well as Plan-Write.

## Verdict

APPROVE
Finding order of `plan-unapproved` after the split is stated two contradictory ways.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
