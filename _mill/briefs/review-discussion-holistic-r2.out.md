MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Destructive guard DOES reach fabrictest
**Section:** Constraints → Fabric Destruction Chokepoint Invariant ("This does not apply to `fabrictest` (a different package)").
**Issue:** `cmd/lyx/destructiveguard_test.go:129` does `filepath.WalkDir(moduleRoot/internal/fabricengine)` and skips only `_test.go` files — it is directory-scoped, not package-clause-scoped, so `internal/fabricengine/fabrictest/{hub,states,manifest,verbs,refusal}.go` are scanned, and any `RemoveAll(`, `os.Remove(` or `fslink.Remove(` in them (planting `staleWiredJunction`, tearing down hostile shapes, temp cleanup) fails `go test ./cmd/lyx` on a false premise — the same shape as the vocabulary-owner-row trap the discussion did catch.
**Fix:** State the disposition explicitly: either allowlist rows (with reasons) for fabrictest's non-test files, or a rule that fabrictest's non-test files carry none of the eight tokens (with the state builders confined to `_test.go`), and say whether CONSTRAINTS.md's invariant text changes.

### [BLOCKING:design] Refusal cells' permit roots vs pre-refusal mutation
**Section:** manifest-permit-granularity / refused-before-the-gate-vs-by-the-gate.
**Issue:** `remove.go:61-66` removes the portal and launchers *before* the dirty pre-flight at `remove.go:68-76`, so a dirty-`Remove` cell that refuses has already lost `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>`; the discussion's refusal framing ("refuses, and the hub still exists") implies empty permit roots and never says whether refusal cells may carry permit roots at all.
**Fix:** Decide and record whether pre-refusal portal/launcher destruction is a permitted root for refusal cells or is itself instance nine, and state that refusal expectations carry a permit-root field.

### [BLOCKING:design] No expectation kind for a correctly-proceeding non-clean cell
**Section:** clean-state-effect-assertions / dirty-what-per-cell.
**Issue:** Only `clean` cells get an expected-effect assertion, and the only two non-clean kinds are `RefusedByGate`/`RefusedBefore` — but `dirtyWarpUntracked` against a `scopeTracked` verb (`checkout.go:42` uses `worktreeDirty(scopeTracked, …)`) must *succeed*, leaving that cell's expected outcome undefined; the state/scope pairing determines refuse-vs-proceed and is never enumerated.
**Fix:** Add a third expectation kind (proceeds + effect + untracked content survives) and state per verb which dirtiness scope it probes, so each dirtiness cell's expected outcome is derivable rather than guessed.

### [NIT:consistency] Refusal-helper names disagree across sections
**Section:** Scope / refusal-check-assertion vs package-file-layout / Testing / Q&A.
**Issue:** Scope and the refusal decisions name `RefusedByGate(err, check)` and `RefusedBefore(err, substring)`; the file-layout decision, Testing item 4 and the Q&A log name a single `RefusedBy`.
**Fix:** Settle on one spelling and use it in every section.

### [NIT:consistency] "boardtest holds both its helpers and its suites" overstated
**Section:** where-the-suite-lives.
**Issue:** `internal/boardengine/boardtest` contains only `doc.go` plus `*_test.go` files — it has no non-test helper source, so the mirroring claim holds for `doc.go`-untagged shape but not for helpers.
**Fix:** Reword to cite `lyxtest` for the non-test-helper half and `boardtest` for the untagged-`doc.go`/suite half only.

## Verdict

REQUEST_CHANGES
Three design gaps: a false guard-scope premise, undefined refusal-cell permits, and no proceed expectation.
MILL_REVIEW_END
