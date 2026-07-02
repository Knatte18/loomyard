No further checks needed. Final review below.

MILL_REVIEW_BEGIN
# Review: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-02
```

## Findings

No findings. Cross-checked both batches against the plan and discussion.md in full:

- `SANDBOX-SUITE.md`: S6→S5 rename is clean (body text unchanged, ref example updated to `"S5"`, `S0`-`S8` range references updated, contiguous `S0:`-`S8:` session-log list). New S6/S7/S8 scenarios match every required Goal/Watch/durability-note element (subdir-scoped `_lyx`, `config` as the actual subfolder-scoping demonstrator vs. `board`'s hub-level smoke check, `init --undo` cleanup semantics, weft's fixed `"weft sync"` commit message and detached-push lag, warp `reconcile`'s no-dry-run behavior, `checkout`'s wrapped error format). S4 extension correctly sequences `--set` → `--print` → `reconcile`. Operating-model note correctly carves out S6 as the sole exception. `**Covers:**` tags are exactly `{S3:board, S4:config, S6:init, S7:weft, S8:warp}`; S0/S1/S2/S5 correctly carry none.
- `cmd/lyx/sandbox_coverage_test.go`: `registered` derived from `newRoot().Commands()` (skip `help`/`completion`, matching `longlist_test.go`); `covered` parsed via `**Covers:**` regex with correct 3-level `filepath.Dir` walk-up (matches the code, not the stale "two" comment, at `registration_test.go:71` — that stale comment is pre-existing/untouched by this task, correctly out of scope); `excludedModules` = `{muxpoc, ide, selfreport}` with matching reasons; both coverage and drift-guard assertions plus the `discovered_non_empty` sanity sub-test are present and correctly implemented. Registered set (8) = covered (5) + excluded (3), consistent with `main.go`'s `newRoot()`.
- `CONSTRAINTS.md`: new "Sandbox Suite Coverage" section correctly placed between CLI/Cobra Invariant and Documentation Lifecycle, format matches sibling invariants, "Enforced by" points at the right test.
- Cross-batch contract holds: batch 2's test depends on batch 1's `**Covers:**` tags exactly as `depends-on: [1]` specifies; no drift.
- Confirmed via grep that `tools/sandbox/report_test.go`'s `"S6"`/`"S5"` occurrences are arbitrary fixture strings uncoupled to real scenario content, exactly as discussion.md predicted — no code change needed or made there.
- No out-of-plan files; `All Files Touched` (`CONSTRAINTS.md`, `cmd/lyx/sandbox_coverage_test.go`, `tools/sandbox/SANDBOX-SUITE.md`) matches what's on disk.
- No Hub Geometry / lyxtest / CLI-Cobra invariant violations; no utility duplication (new test calls `newRoot()` directly rather than re-deriving via AST like `registration_test.go`, correctly avoiding duplication per the plan's own rationale).

## Verdict

APPROVE
Implementation fully matches the plan, shared decisions, and constraints across both batches with no deviations.
MILL_REVIEW_END