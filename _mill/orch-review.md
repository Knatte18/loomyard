# Orchestrator review — config-template-fallback (T2)

Reviewed against `manifest/designs/producers-standalone.md`'s T2 brief (told-geometry, wave 1).
Spot-checked the core factual claims against current source — all confirmed accurate: `configengine/config.go`'s two refusal branches (`FindBaseDir`'s `os.IsNotExist` at line 30, the config-read `os.IsNotExist` at line 59), all four callers' `strings`-only-for-the-rewrap shape, `perchengine.LoadConfig` wrapping `LoadConfigWithRegistry`, `burlerengine.LoadConfig`'s documented bypass of `configengine.Load`, `modelspec.LoadRegistry`'s `builtins()` fallback, and `docs/shared-libs/configengine.md`'s existing (soon-to-be-false) claims at line 124/127-130.

## Scope verdict: mostly correct, one real expansion worth flagging

The design's T2 Files list is exactly six files (`configengine/{config.go,config_test.go}` + the four engines' `config.go`), and the discussion's core work matches that precisely — same function signature, same shared-body approach, same four repointed callers, same exclusions (`Load` itself, `burlerengine.LoadConfig`, `modelspec.LoadRegistry`, all CLI call sites all correctly left alone, with an explicit warning that no CLI becomes standalone-runnable from this task alone — correct, that's T5-T8's job).

Test-file changes in the four engines' `config_test.go` (not literally in the Files list) are explicitly required by the design's own Verify line ("a new test per loader..."), so that's not an expansion, just the Files list being terse about tests as it is elsewhere in this design doc.

**One genuine scope expansion: the new Config Strictness Invariant + machine-enforced grep guard in `cmd/lyx`.** Nothing in T2's brief, nor anywhere else in `producers-standalone.md`, calls for a new invariant or guard here — T10 ("consolidation") is where the design places its own new invariant (the three-tier geometry/fabric/orchestrator-state rule), and that's a different invariant entirely. This is not in the design's Files list for T2 either. Assessment:

- **Well-precedented mechanically.** `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map already has 13 entries, at least 9 of which are exactly this shape (`ghguard_test.go`, `gitrepoboundary_test.go`, `boardguard_test.go`, `rawgitmutation_test.go`, `destructiveguard_test.go`, `uncontainedwrite_test.go`, `checkedcall_test.go`, `cwdmutation_test.go`, plus `tierpurity_test.go` itself) — a `go env GOMOD` + grep-set-equality guard is a genuinely common, low-novelty pattern in this repo, not invented machinery.
- **Low collision risk.** No other T1–T10 task touches `cmd/lyx/tierpurity_test.go` per the design's own file-contention analysis, so this doesn't create a merge hazard — but it does mean that contention analysis (which was careful to enumerate every shared file across all ten tasks) is now quietly incomplete, since it never anticipated a new task adding a *new* guard file at all.
- **Inconsistent with T1's sibling call.** `planparser-plan-dir`'s discussion explicitly considered and rejected machine-enforcing its own reworded invariant, reasoning "building one is scope the design did not ask for." T2's discussion reaches the opposite conclusion on near-identical grounds (a reworded/new invariant with no explicit design mandate for enforcement). The two invariants aren't identical — T2's classifies a two-way split across five callers where silent misclassification is a real, growing defect risk as more producers are added, versus T1's single-package import-discipline statement — so a stronger case for enforcement here is defensible. But it's worth a conscious confirmation that this asymmetry is intentional rather than two different discussions independently reaching for different defaults.

**Recommendation:** this is a judgment call, not a defect. If the task owner is fine with T2 introducing new cross-cutting test infrastructure beyond its decomposed brief, no change needed. If tighter adherence to the wave decomposition is preferred, the invariant text itself (CLAUDE.md-mandated, cheap, matches T1's own review-obligation pattern) should stay; the grep guard could be deferred to T10 instead, alongside the invariant it's actually themed with.

The `docs/shared-libs/configengine.md` update is a second, smaller expansion beyond the Files list, but low-risk and clearly warranted: the doc's existing claims (line 124: absent `_lyx/` always errors; lines 127-130: typed wrappers always rewrap on `"not initialized"`) become straightforwardly false after this task, so leaving them stale would violate CLAAUDE.md's Documentation Lifecycle rule outright rather than merely under-apply it.

## Minor notes (non-blocking)

- `allowedSpawners`'s map declaration is at line 28, not line 24 (line 24 is the start of its doc comment). Cosmetic.
- No other correctness issues found in the four callers' shapes, the GOOS-varying reed template table, or the perch model-spec resolution claim — all verified against source.

## Bottom line

Core scope (the six-file config-loader change) is correct and tightly matches the design.
The new invariant + guard is the one item that meaningfully exceeds T2's decomposed brief — technically sound and low-collision-risk, but worth an explicit "yes, we want this now rather than at T10" confirmation before implementation, given the inconsistency with T1's own enforcement call in the same wave.
