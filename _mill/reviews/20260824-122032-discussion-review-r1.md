MILL_REVIEW_BEGIN
# Review: Migrate planparser.Card to Edits/Uses fields

```yaml
duration_s: 171.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:consistency] Validator check arithmetic does not add up
**Demoted-from:** BLOCKING
**Section:** `### validator-checks` + Q&A "Which of the 14 validator checks survive?"
**Issue:** The table's own dispositions yield 15 post-migration checks (5 keep + 3 rework + `rename-format` + `rename-mechanic-missing` + `card-type-missing` + `impact-summary-missing` + `impact-summary-multiline` + `card-field-empty` + `prosa-symbol-target`), not 13; the Q&A's "keep 5, rework 3, drop 6, add 6" sums to 14, not 13; and `validate.go` actually emits 15 distinct `Check:` IDs today (`format-unrecognized`, `plan-unapproved`, `index-file-mismatch`, `card-path-malformed`, `move-format`, `move-redundant`, `move-source-missing`, `move-target-collision`, `move-mechanic-missing`, `card-missing-field`, `card-field-overlap`, `card-numbering`, `path-missing`, `commit-subject-mismatch`, `depends-on-order`), so the "14" baseline the migration is measured against is itself already wrong.
**Fix:** Recount from the table, state the true old count (15) and the true new count, and say explicitly which number goes into the rewritten spec and the three "14 checks" comment sites.

### [BLOCKING:design] Nothing checks a missing `Intent:`
**Section:** `### field-presence` + `### validator-checks`
**Issue:** `Intent:` is declared required per card, but `card-missing-field` — which today enforces required-field presence including `What:` (`validate.go`'s `checkCardMissingField`, `{c.HasWhat, "What:"}`) — is replaced by only `card-type-missing` + `impact-summary-missing`, so a card with no `Intent:` produces no finding at all.
**Fix:** Name the check that enforces required-`Intent:` presence (extend `card-missing-field`'s successor or add `intent-missing`) and fold it into the check-set count.

### [NIT:scope] Old-format fixture inventory is incomplete
**Demoted-from:** BLOCKING
**Section:** `## Scope` — "Test fixtures carrying an old-format card body" (four files listed)
**Issue:** The enumeration misses at least three further carriers of a literal old-format card body / `format: 3` plan: `internal/loomcli/validate_test.go:177-191`, `internal/webstercli/cli_test.go`, and `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` — the last being an agent-facing instruction file, not a Go fixture, so it will not be caught by the "tree is green" backstop the Testing section relies on.
**Fix:** State the enumeration method used (and re-run it over `**What:**`/`format: 3` across `internal/`, `tools/`, and `contracts/`), or scope the inventory as "every file matching X", so a plan writer can reproduce it rather than trusting a hand list.

### [NIT:decision] Design-doc open item on `ImpactSummary` for Delete undispositioned
**Section:** `## Scope` — "record the three open items this task closes"
**Issue:** Only two of `plan-card-format.md`'s three open items get an explicit disposition (`Custom` check → none; the 14-check reconciliation → the table); "Whether `ImpactSummary` on Delete needs a structured shape beyond one line of prose" is only implicitly answered by `prose-fields`.
**Fix:** State that disposition explicitly (one line of prose, no structured shape) in `prose-fields` or the scope bullet.

### [NIT:consistency] Classifier drops the design doc's ground-truth clause silently
**Section:** `### shape-classifier`
**Issue:** The design doc's shape rule reads "distinguished by shape … and, where ambiguous, resolvable against ground truth (`go doc` for a symbol, file existence for a path)"; the decision declines the `go doc` half (correctly, per Test Tier Purity) but presents itself as mirroring the doc "verbatim".
**Fix:** Name the declined clause as a deliberate deviation with its reason, so the rewritten spec does not read as contradicting the design doc.

### [NIT:consistency] Producer Pointer-Rule Invariant not addressed by `doc-reach`
**Section:** `### doc-reach` / `## Constraints`
**Issue:** Both stencils are instruction files; the Producer Pointer-Rule Invariant forbids duplicating format-contract content rather than pointing at it, and the constraints list does not mention it (nor the Gate Self-Check Parity Invariant, whose `loomcli` verb fixture is affected).
**Fix:** State how much format grammar each stencil restates versus points at `loom-plan-spec.md`, and add the invariant to the constraints list.

## Verdict

REQUEST_CHANGES
Check-set count is wrong, required-`Intent:` is unchecked, and the fixture inventory is incomplete.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
