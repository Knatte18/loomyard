MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
duration_s: 148.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus (Anthropic), reviewing via the mill discussion-review harness
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [NIT:consistency] begin-batch/record-batch are not producer rows
**Demoted-from:** BLOCKING
**Section:** `drift-boundaries` (and `planglyph-root-resolution`, first bullet)
**Issue:** Both decisions call `begin-batch` and `record-batch` "producer rows" and invoke the Gate Self-Check Parity Invariant's row+verb pairing, but the shed registry (`internal/shedrecipe/registry.go:21-35`) has no such entries; they are Master's bracket CLI verbs (`internal/webstercli/beginbatch.go:31` — "Master's bracket call immediately before forking one batch's implementer") over `websterengine` functions, so only `Plan-Revalidate` is an actual `ShedProducer` row.
**Fix:** State the real seam for the two webster boundaries (in-`websterengine` bracket-verb hooks, root from `deps.Geom.WorktreeRoot`) and say explicitly whether parity applies to them at all, since they already have verbs and no row.

### [NIT:consistency] amendments.md is a third planparser write path
**Demoted-from:** BLOCKING
**Section:** `rewrite-write-path` + `amendment-log` + Constraints
**Issue:** Constraints says "`SetApproved` is joined by exactly one new write path, `RewriteRefs`; no third", while `amendment-log` puts an append-only `_lyx/plan/amendments.md` under planparser's sole ownership — an append is a write neither `SetApproved` nor `RewriteRefs` performs.
**Fix:** Name the amendment append as a second new planparser write path (or fold it into `RewriteRefs`'s contract), and add the CONSTRAINTS.md Planparser bullet edit ("`SetApproved` is the one write path", line 213) to the same-commit doc list, which currently names CONSTRAINTS.md only for the new chokepoint invariant.

### [NIT:scope] New `lyx quarry` module: package name and sandbox coverage unstated
**Demoted-from:** BLOCKING
**Section:** `cli-verb-surface`, Constraints, Testing
**Issue:** A new root command registers a module, which trips the Sandbox Suite Coverage invariant (`cmd/lyx/sandbox_coverage_test.go`: covered by a `**Covers:**` scenario or allowlisted with a reason) — unmentioned anywhere; the CLI package name is also never given, and a `quarrycli` → `planglyph` pairing deviates from the CLI/Cobra Invariant's `<module>cli` → `<module>engine` rule, whose only recorded deviation today is `stencilcli`.
**Fix:** State the package name, state whether the naming deviation is recorded in CONSTRAINTS.md, and give the sandbox disposition (scenario or allowlist entry with reason).

### [BLOCKING:design] quarry's git spawn is invisible to two mechanical guards
**Section:** Constraints (Test Tier Purity) + Testing
**Issue:** `quarry.DeltaGit` spawns git via quarry's `internal/gitsrc`, but both enforcing guards are raw-substring scans of loomyard source — `cmd/lyx/tierpurity_test.go`'s `bannedTokens` (`gitexec.Run`, `exec.Command`, …) and `cmd/lyx/spawnobservability_test.go` — so neither sees a `DeltaGit` call; the discussion asserts the tagging discipline but never says how it is enforced, and never addresses Live-Substrate Spawn Observability at all for a spawn reachable from `lyx webster record-batch`.
**Fix:** Decide and state whether `bannedTokens` gains a quarry-call token, and whether the `DeltaGit` site logs its spawn via `internal/logger` or carries a written exemption in the observability guard's allowlist.

## Verdict

REQUEST_CHANGES
Four unresolved items: two seam mischaracterizations, one undecided module identity, one unenforced constraint.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
