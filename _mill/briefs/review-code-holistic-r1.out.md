MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-26
```

## Findings

No findings.

Verified end-to-end across all four batches against the plan and CONSTRAINTS.md:

- `internal/loomengine/config.go`: `LoomStatusFile` re-rooted onto `lyxdirs.DotLyxDirName`, `LoomStatusRel` fully deleted (confirmed absent repo-wide outside plan/mill artifacts), doc comments rewritten per card 7's requirements (loomDirName, LoomStatusLock, LoomDriverLog, LoomBootstrapLock, LoomScratchDir all updated; LoomRunLock correctly left untouched).
- `cmd/lyx/notransients_test.go` and `constructoranchoring_test.go`: `LoomStatusFile` moved into the `.lyx`/transient group and the `dotLyxConstructors` regression map, matching card 1 exactly, including the "in full" enumeration fix.
- `internal/loomengine/loomstatus_test.go` and `config_test.go`: assertions moved to `DotLyxDirName`; the two `LoomStatusRel`-only tests deleted cleanly (cards 2-3).
- `internal/loomcli/run.go`: seed-commit pathspec reduced to `OriginRecordRel()` alone; comment, `Long` help text, and header doc rewritten consistently (card 4).
- `internal/landingshed/{deps,publish,finalize}.go` and `internal/loomcli/{landingdeps,landingdeps_test}.go`: `CommitStatus` seam fully removed, `commitstatus_test.go` deleted, field count in the drift-guard test comment updated to fourteen (card 5).
- `internal/loomcli/smoke_test.go`: `seedAndCommitStatus` renamed to `seedStatus` with the commit call dropped, `poisonStatusFile` no longer commits, `TestSmokeBootstrap_CleanlinessAfterSeed` and the origin-record self-heal test both rewritten to match the new no-commit reality; no dangling `slices` import or `weftHeadChangedFiles` helper (card 6).
- Stencils/recipe (cards 8-10): rubric and discussion-template fences retargeted to `.lyx/loom/status.json` / `.lyx/loom/`, with matching test literal updates; no `warp`/`weft` tokens introduced.
- Doc comments across `status.go`, `report.go`, `loomshed/seed.go`, `shedengine/{shed,doc}.go` updated to the new path and the durability claim removed without being replaced by a misleading "ephemeral" claim (card 11).
- Docs batch (cards 12-16): `loom-status-spec.md`, `loom.md` (all six locations plus the new migration note), `shed.md`, `self-report.md`, `fabric-unified-view.md` (group membership moved correctly), `docs/overview.md`, and the sandbox suite's S8 fixture note all consistently re-point at `.lyx/loom/status.json` / `.lyx/`, with the pinned `#crash-recovery...` anchor left untouched.
- Regression test (card 17): `internal/loomcli/landing_integration_test.go` builds a parent + two task pairs, seeds and diverges each status file, lands both sequentially via `landingshed.Finalize` with a strict-fail shuttle fake, and asserts both no-conflict and no `loom/status.json` tracked on the parent's weft sibling (via `h.PairWeftSibling`) — matches the plan's two required assertions. Helper names are locally renamed and do not collide with existing identifiers in package `loomcli`.

No out-of-plan files, no duplicated helpers across batches, no constraint violations found (Cwd Resolution, Lyxdirs Single-Declarer, Durable-vs-Ephemeral State, Fabric Vocabulary, Test Tier Purity, Hermetic Git Test Environment, Markdown Link Integrity all satisfied by the reviewed diff).

## Verdict

APPROVE
All four batches are complete, mutually consistent, and correctly implement the plan's shared decisions.
MILL_REVIEW_END
