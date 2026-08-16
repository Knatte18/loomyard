MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [NIT:decision] Webster error-path test site left unassigned
**Section:** §Testing → `internal/websterengine` **Issue:** "worth having at one of the two sites at least" names neither `RenderRecoveryPrompt` nor `RenderMasterPrompt` as the required site, so a plan writer picks arbitrarily or writes none. **Fix:** Name the site (or say "both") explicitly.

### [NIT:consistency] `stencilstore`'s allowlist rationale unstated
**Section:** §Constraints → Pattern Leaf Invariant **Issue:** The `CONSTRAINTS.md` amendment is told to carry the cycle-freedom reason "the way the existing text already does for `internal/lyxdirs`", but that argument is given only for `internal/stencil`; `internal/stencilstore` is not a zero-import leaf (it imports `internal/stencil` and `internal/logger`, verified), so the wording for the second entry is undefined. **Fix:** State which rationale the `stencilstore` entry carries (shared infrastructure + verified acyclic closure, not leafness).

### [NIT:design] Strip-only vs CRLF claim slightly overstated
**Section:** §Decisions → "`Directive` strips the leading banner" **Issue:** "CRLF is already handled upstream by the `*.md text eol=lf` `.gitattributes` `Reconcile` seeds" is true for git-materialised files, but an operator editing the board copy with a CRLF-writing editor yields CRLF prose that `Classify` still reports untouched (`BodyHash` normalises, `Read` does not). **Fix:** Note this as accepted parity with `Fill` rather than as CRLF being eliminated.

## Verdict

APPROVE
Claims verified against source; decisions complete, scope closed, no blocking gaps.
MILL_REVIEW_END
