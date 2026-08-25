MILL_REVIEW_BEGIN
# Review: loom: Discussion-Burler Fabric Git Invariant fix

```yaml
duration_s: 171.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [NIT:scope] Guard's seam half accepts any seam value
**Section:** Decisions § add-a-parse-level-regression-guard-in-loomrecipe **Issue:** The second assertion only requires the segment's `Bouncer` row to carry a *non-empty* `commit_seam`, so a row pairing overlay discussion targets with `commit_seam: plan` (or a `BurlerRound` row whose `segment` has no `Bouncer` row at all) passes the guard while committing the wrong tree or nothing. **Fix:** State in the discussion whether the guard also relates the seam value to the Burler's target tree, and what it does when no partner `Bouncer` row exists (fail loudly, per the same "a silently-skipped row is a guard that does not guard" rule).

### [NIT:consistency] loom.md already asserts a contradicting claim
**Section:** Decisions § docs-land-in-the-same-commit **Issue:** The discussion says "The gate" section "currently says nothing about the commit split", but `manifest/designs/loom.md:214` states "only the review *profile* (rubric + fasit) differs per phase", which the fix-scope/commit-seam split already contradicts today and will contradict more sharply after the flip. **Fix:** Name that sentence as a rewrite site alongside the additive record, so the plan writer corrects it rather than appending a record beside a false claim.

### [NIT:consistency] "Tests that encode the current value" mis-scopes the sweep
**Section:** Technical context § Tests that encode the current value **Issue:** The claim that every other `fix-scope: source`/`FixScopeSource` occurrence is a `burlerengine` or `burlercli` fixture is inaccurate — `internal/shedrecipe/entries_burler_test.go:70,96,166` and `internal/shedadapters/burler_test.go` also carry it (both synthetic configs, genuinely unaffected), and `manifest/designs/hardener.md:124` names `fix-scope: source` for the future Tenter round agent (correct, warp source, stays). **Fix:** Restate the claim as "every other occurrence is a synthetic fixture or a warp-source round, none of them loom's recipe", listing those three homes.

## Verdict

APPROVE
Scope, decisions, and every load-bearing source claim verified; three non-blocking precision gaps.
MILL_REVIEW_END
