This confirms `tierpurity_test.go`'s `allowedSpawners` map correctly mirrors this established `excludedModules` style, as Card 9 required. I now have complete confidence in the review. Here is my final output.

MILL_REVIEW_BEGIN
# Review: Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-12
```

## Findings

No findings. I read all four plan files (overview + batches 1–4), `_mill/discussion.md`, and every source file in the manifest, and cross-checked each card's requirements against the actual diffs.

Batch 1: `init_test.go`'s `TestInit_FirstRun` correctly derives `want := len(configreg.Modules())` with the required comment and import; `menu.go` sets `cfg.Path = hubgeometry.BoardDir(l.Hub)` at the exact spot specified, mirroring `boardcli/cli.go:103`'s pattern and `boardengine.Config.Path`'s `yaml:"-"` contract.

Batch 2 (cards 3–8): verified every file's build tag, package name, moved-test list, and trimmed import block against each card's exact specification for `boardcli` (`cli_test.go`/`cli_unit_test.go`, with `runCLI` correctly placed in the untagged file per the helper-placement decision), `perchcli` (`cli_test.go`/`cli_integration_test.go`, `run_test.go`/`run_integration_test.go`), `muxcli`, `configcli` (all four files, including the `TestDispatchSet_PreservedKeyDetectedByReconcile` move and the new `reconcile_integration_test.go`), and `cmd/lyx` (`main_test.go`/`main_integration_test.go`, `crosscompile_test.go`'s new tag). No test function was deleted; each moved function/helper appears verbatim in exactly one file (confirmed via grep for `gitLsFiles`/`gitLogOneline`/`runCLI`/`seedCwd`/`TestReconcile_NotAGitRepo`/`TestDispatchSet_PreservedKeyDetectedByReconcile` — no duplicates, no orphans).

Batch 3: `tierpurity_test.go` implements the walk/tag/token-match/allowlist mechanics exactly as specified (GOMOD-relative root, `filepath.ToSlash` normalization before matching, raw-substring banned tokens, `allowedSpawners` mirroring `sandbox_coverage_test.go`'s `excludedModules` style, vacuous-scan floor of 20). `CONSTRAINTS.md`'s new Test Tier Purity Invariant section matches the card's required wording (explicitly not "spawn no processes").

Batch 4: `test-suite-timing.md`'s new "Current best times" block and moved 2026-07-12 regression block, and `running-tests.md`'s tier bullets/Commands/harness/Reducing-wall-clock sections, all use the "no expensive spawns" wording rather than "zero processes," consistent with the guard's actual semantics.

Cross-batch contracts hold: `hubgeometry.BoardDir`/`PerchRunsDir`/`ConfigDir`/`ConfigFile` all exist with the signatures the new/edited tests assume; `configreg.Modules()` returns 7 entries consumed consistently by `initengine`, `configsync`, and the config CLI tests. No out-of-plan files, no constraint violations, no duplicated helpers across packages.

## Verdict

APPROVE
Both reds are fixed correctly, all re-tiering matches its cards verbatim, the guard and docs are consistent and correct.
MILL_REVIEW_END
