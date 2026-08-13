MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-13
```

## Findings

No findings.

Verified end-to-end against all 12 batch files and every Shared Decision in the overview:

- `internal/gitkit/gitkit.go` is exactly the finished leaf shape: `CopyRepo`/`RepoFixture`, `MustRun`, `SeedConfig`, `GitStatusPorcelain`, `HermeticGitEnv`, `buildRepoTemplate` (renamed from `buildWarpHub`); `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, the `CopyWarpHub` wrapper, `WarpFixture`/`PairedFixture`/`WeftFixture`, and `buildWeftPrime`/`buildWeftOnly` are all gone, with the unused `weftname` import dropped. `internal/gitkit/callerset_enforcement_test.go`'s AST-based `TestCopyRepoCallerSet_LyxcwdOnly` guard is present and correctly worded to avoid self-tripping the tier-purity/hermetic-env token guards.
- `internal/hubforge/hub.go`/`seed.go` match the plan precisely: `WeftBase` carried verbatim from `CloneResult.WeftBase`, `registerTeardown`'s `SkipDir`-avoidance reasoning is spelled out exactly as required, `buildBareTemplate` carries `backend/`, `nested/`, and `wts/some-task/` anchors (the scope addition from batch 4 card 27), and `SeedConfig`/`SeedFabricConfig` write to the correct bases and commit at the correct roots.
- `internal/fabricengine`'s dissolution is complete: no `fabrictest` package remains on disk, the live-state harness lives in `livestate_*_test.go` files under `package fabricengine_test`, `export_test.go`'s `NewPairedFromPathsForTest` rename and narrowing to `fabric_test.go`'s untagged constructor unit test is correct (no `hubforge` import in that file), and all three of batch 11 card 69's implementation-discovered deviations are present exactly as described: `newPlainWeftRepo` in `export_test.go`, the package-local `fabricFixture` struct in `reconcile_stale_registration_test.go`, and `WeftWriteLockPathForTest`/`weftWriteLockPath` relocated into the integration-tagged `commit_lock_integration_test.go`. The new `gitsha_integration_test.go` (batch 11's fourth deviation) correctly isolates the git-spawning fixture helpers behind the `integration` tag with no naming collision against `livestate_verbs_test.go`'s separately-named `liveStateCurrentSHA`.
- Zero-hit gates all confirmed clean by direct grep: `\blyxtest\b`, `\bfabrictest\b`, `CopyPaired|CopyPairedLocal|CopyWeft|CopyWarpHub`, and `NewPairedForTest` all return no matches under `internal/` and `cmd/`. `gitkit.CopyRepo(` appears in exactly 9 call sites, all under `internal/lyxcwd`. No file importing `internal/hubforge` declares `package fabricengine`/`loomengine`/`treadleengine` etc. (all are the `_test` external form), consistent with the Fabric-Fixture Invariant.
- `internal/lyxcwd/enforcement_test.go`'s `fabricVocabularyOwners` and `weftnameImportOwners` maps both carry `internal/hubforge` with the exact justification comments the plan specifies; the prior round's stale-owner-list note is resolved (both maps now name `gitkit`/`hubforge`, not a stale set).
- `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, and `cmd/lyx/destructiveguard_test.go` all carry the `hubforge.NewHub` token additions and the `fabrictest` exclusion removal exactly as batch 3/2 specify.
- Spot-checked a representative cross-section of consumer migrations across batches 4-10 (`idecli`, `webstercli`, `boardtest`, `perchcli`'s nested/wts-some-task anchor cards, `loomengine`'s `checkResolved` export shim and `checkout -b` deletion, `treadleengine`'s shim, `fabriccli`'s `TestRunCLI_EnvMapToOption` scaffolding removal, `fabricengine`'s `weftgit_exclude_test.go` and `reconcile_empty_anchor_integration_test.go`'s deliberate `"backend"`-anchor construction) — every one matches its card's field-mapping table and stated resolution, including thoughtful, well-documented deviations (e.g. `boardtest/sync_test.go` explicitly re-establishing upstream tracking that the old `CopyWeft` fixture provided for free but `hubforge`'s genuinely-empty weft bare template does not).
- Docs batch (12) fully verified: `manifest/roadmap.md`'s Done entry is past-tense, link-free, and numerically accurate (141/132/9); `manifest/designs/lyxtest-real-hubs.md` is deleted; `docs/overview.md`, `CLAUDE.md`, `docs/shared-libs/lyxcwd.md`, `manifest/designs/fabric-unified-view.md`, `crucible/review-prompt-template.md`, and all four `docs/benchmarks/*.md` files carry the required renames while preserving historical benchmark rows verbatim, exactly as the batch-local decision requires.

No BLOCKING or NIT findings identified.

## Verdict

APPROVE
Implementation matches the plan and CONSTRAINTS.md with no detected deviations across a thorough cross-batch verification.
MILL_REVIEW_END
