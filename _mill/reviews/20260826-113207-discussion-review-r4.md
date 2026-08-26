MILL_REVIEW_BEGIN
# Review: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
duration_s: 201.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Re-pointed burler corruption aborts, never bounces
**Section:** Technical context (`revalidate_test.go` disposition) + Q&A round-3 gap
**Issue:** The chosen replacement corruption — "a Card Index entry naming a card file that is not on disk (`index-file-mismatch`)" — is not an `index-file-mismatch` finding at all: `parseCardFile` (`internal/planparser/parse.go:279-297`) hard-errors `planparser: card file not found`, so `ParsePlan` fails, `planValidate.Call` returns an **error** rather than `Stuck` (`internal/loomshed/planvalidate.go:61-67`), and `TestSequence_PlanRevalidateCatchesPostSegmentRegression` fails at its `Run() error = ...` fatal before ever reaching the `Stuck`→`Plan-Write` assertion it exists to make. `index-file-mismatch` fires on the opposite direction (a `.md` on disk no card names, or a numbering gap).
**Fix:** Name a corruption that parses cleanly and still yields a finding — e.g. the burler drops/renumbers the Card Index entry while leaving `01-first-card.md` on disk (unindexed file ⇒ `index-file-mismatch`), or emits a card-body format fault — and state the disposition in terms of "parseable-but-invalid" rather than by check ID alone.

### [NIT:scope] Plan-Review rubric's "sixteen checks upstream" not in doc list
**Section:** Scope → In (docs list)
**Issue:** `contracts/stencils/loom/loom-rubric-plan-review.md:20,:31` asserts a "sixteen-check mechanical validator" upstream and names the sixteen IDs `format-unrecognized`…`commit-subject-mismatch` as "enforced deterministically upstream"; after the split only fifteen are enforced upstream of the judge, `plan-unapproved` moving downstream to `Plan-Revalidate`. The file is absent from the docs-in-same-commit list.
**Fix:** Add it to the docs list with the disposition (reword the count/upstream claim; the judge's don't-re-derive instruction itself stays).

### [NIT:consistency] `validate-plan` Long names `planparser.Validate`
**Section:** Technical context → `internal/loomcli`
**Issue:** The discussion flags only the "it takes no arguments and no flags" sentence, but `validate.go:76-79`'s Long also states the verb "runs planparser.Validate against it", which becomes false in the new default mode.
**Fix:** State that the whole Long paragraph is rewritten to describe both modes, not just the flags sentence.

### [NIT:consistency] `loomshed/planvalidate.go` doc comments not named
**Section:** Scope → In (docs list) / Technical context → `internal/loomshed`
**Issue:** `planvalidate.go:1-3` and `Call`'s doc (`:47-54`) both pin "then `planparser.Validate`" as the thin-wrap contract; the discussion names only the `logger.Warn` line as needing no change and never states these comments move.
**Fix:** Add both doc comments to the same-commit edit list alongside `internal/planparser/validate.go`'s package doc.

## Verdict

REQUEST_CHANGES
Re-pointed regression corruption is unparseable, so the named test aborts instead of bouncing.
MILL_REVIEW_END
