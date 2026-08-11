MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: /home/knatte/Code/loomyard/wts/batcher-standalone-split/_mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:consistency] Doc site 5 leaves the contradicting sentence
**Section:** Decisions → doc-amendments, site 5
**Issue:** Site 1 requires `internal/batcher/doc.go` to stop saying batching is "100% webster's own execution-policy decision", but site 5 scopes `internal/websterengine/doc.go` to the config-key clause at `:25–26` only — `doc.go:27` carries the identical sentence ("Batching is 100% webster's own execution-policy decision"), so after this task the two package docs directly contradict each other.
**Fix:** Extend site 5 to `doc.go:23–29`, stating what the ownership sentence becomes there, in the same words site 1 uses for batcher's own doc.

### [NIT:decision] planparser/doc.go's "webster's batcher" has no disposition
**Section:** Decisions → doc-amendments (the eight-site list)
**Issue:** `internal/planparser/doc.go:4` says every consumer of `_lyx/plan/` is "webster's batcher, master, and fork prompt rendering" — the same ownership possessive this task removes elsewhere, and the list claims completeness for sites that "state something this task falsifies".
**Fix:** State explicitly whether this phrase is in scope (reword to "the batcher") or deliberately out, with a one-line reason.

### [NIT:design] The nil-Batcher "typed error" is unnamed
**Section:** Decisions → runlevel-call-site, Nil contract
**Issue:** The guard is specified as returning "a typed error" with no sentinel name or message text, yet a dedicated test must assert it; webster's existing precedents (`ErrRunBusy`, `ErrFingerprintMismatch`) are named sentinels, so the plan writer must invent both the identifier and the wording.
**Fix:** Name the sentinel and its message (e.g. `ErrMissingBatcher`, "webster: RunDeps.Batcher not populated"), so the test asserts a decided string.

### [NIT:decision] configreg entry's SeedOnly flag not stated
**Section:** Scope → `internal/configreg/configreg.go`
**Issue:** `configreg.Module` carries a `SeedOnly` field and `configreg_test.go:32`'s `TestModules_SeedOnly` asserts exactly `models`/`burler` are seed-only; the discussion's registration literal omits the field (correctly, since batcher's key set is closed) but never says so, and never notes that test needs no edit.
**Fix:** One clause recording `SeedOnly: false` (closed key set, reconcile-managed) and that `TestModules_SeedOnly` passes unchanged.

## Verdict

REQUEST_CHANGES
One enumerated doc site leaves a directly contradicting sentence in place; three nits.
MILL_REVIEW_END
