MILL_REVIEW_BEGIN
# Review: loom: Discussion-Write producer

```yaml
duration_s: 273.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:design] Archive-then-respawn dirt window unstated
**Section:** Decisions → `commit-produced-artifacts` **Issue:** On a `Discussion-Validate` bounce, `shedadapters.archiveStaleOutputs` renames the two files the previous round already committed, so between the archive and the next `Done` the weft carries two deletions of tracked files plus two untracked siblings — a window the "keep the weft clean" rationale does not name, and one that persists if the round blocks (no `on_stuck`) rather than reaching `Done`. **Fix:** Add one sentence accepting that window explicitly, alongside the existing commit-failure acceptance clause.

### [NIT:scope] Step 2/Step 3 interview framing left implicit
**Section:** Scope → In (stencil rewrite) **Issue:** The stencil rewrite enumerates five edits but does not say whether Step 3's operator-facing framing ("Interview relentlessly", "For every question, give your recommended answer") and Step 2's "before asking the operator anything" stay verbatim under `autonomous-only`; `keep-mode-rules-both-branches` implies they do (mode differences live in `{{.mode_rules}}` at Step 4), but it is not stated. **Fix:** State that the mode-neutral interview prose stays untouched and that all mode-specific wording remains confined to the `{{.mode_rules}}` marker.

## Verdict

APPROVE
Decisions are complete, source-grounded, and verified against the tree; only two documentation nits remain.
MILL_REVIEW_END
