# Review: Worktree spawn/teardown as Shed producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Adding three registry engines breaks loom's own coverage guard, undecided how
**Section:** Technical context ("The registry") and Testing ("The registry `Names()` test and `internal/loomrecipe/coverage_guard_test.go` both move from fourteen to seventeen keys")
**Issue:** `internal/loomrecipe/coverage_guard_test.go`'s `TestCoverageGuard_EveryLoomRowHasAnEngine` asserts that every entry in `shedrecipe.Names()` is either reached by one of *loom's own* rows (via `loomRowEngines`) or explicitly listed in `coverageGuardAllowedUnreachableEngines` (currently `SingleLLM`, `Stub`). `WorktreeCreate`/`LoomRun`/`WorktreeTeardown` are reached only by the new `lifecyclerecipe`, never by any loom row, so this existing test fails the moment those three keys are added — unless they're added to the allowlist, which contradicts that allowlist's own stated purpose (closing the gap on registry entries no row reaches) and turns a real "second legitimate consumer" case into an unexplained tolerated-orphan entry. "Moves from fourteen to seventeen keys" describes a symmetric key-count bump; the actual mechanical requirement is a decision the discussion never makes: extend `coverageGuardAllowedUnreachableEngines` (weakens loom's own guard), or give `lifecyclerecipe` its own equivalent coverage-guard test and restructure the shared registry's closed-coverage claim to span two consumers.
**Suggested fix:** Add a decision resolving how the shared registry's per-consumer coverage guards compose once a second recipe consumer exists, and correct the Testing section's description of what actually changes in `coverage_guard_test.go`.

### [NIT:decision] Sandbox Suite Coverage disposition for `lifecyclecli` left unstated
**Section:** Constraints ("Sandbox Suite Coverage")
**Issue:** The bullet restates the invariant ("either exercised by the sandbox suite or explicitly excluded with a written reason") without saying which applies to `lifecyclecli`, unlike every other constraint bullet in the doc which states the concrete disposition.
**Suggested fix:** State whether `lifecyclecli` is added to the sandbox suite or excluded, and if excluded, give the one-line reason now rather than leaving it to plan time.

## Verdict

REQUEST_CHANGES
One BLOCKING finding: the registry-coverage-guard conflict from a second `shedrecipe` consumer is a real design gap, not a mechanical follow-on.
