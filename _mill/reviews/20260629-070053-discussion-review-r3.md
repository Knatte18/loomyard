MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-29
```

## Findings

### [GAP] Docs scope omits benchmarks / sandbox-hub / roadmap
**Section:** Scope ("Update docs"), Constraints (Documentation Lifecycle)
**Issue:** The docs bullet lists only `overview.md`, `modules/*`, `shared-libs/*`, but stale renamed paths also live in `docs/benchmarks/*` (live command `go test ./internal/board/boardtest`, plus weft/ide/muxpoc rows), `docs/sandbox-hub.md` (a clickable link `[internal/warp/clone.go](../internal/warp/clone.go)` that will 404, and a `deriveHostName` attribution), and `docs/roadmap.md` (`internal/warp`, `internal/ghissues`).
**Fix:** Add the benchmark docs (at least the functional command paths) and `sandbox-hub.md` to the docs sweep; state explicitly whether the roadmap's stale path refs are corrected given the "roadmap = milestones only" rule.

### [NOTE] configcli integration test is an unlisted warp importer
**Section:** Technical context (warp "Importers to retarget")
**Issue:** `internal/configcli/configcli_integration_test.go` uses `warp.New` / `warp.AddOptions` / `w.Add` / `warp.WireJunctions` (all engine symbols → `warpengine`), but warp's importer list names only `configreg`, `initcli`, `cmd/lyx/main.go` — the build-green gate catches it, yet the importer map is presented as exhaustive and is not.
**Fix:** Add `configcli_integration_test.go` to warp's retarget set (`warp.* → warpengine.*`, keeping `weft.RunCLI → weftcli.RunCLI`).

### [NOTE] boardtest relocation target left soft
**Section:** Technical context (board), Testing
**Issue:** boardtest is to be relocated "likely under `boardengine`" — undecided — yet its `doc.go` comment and the benchmark docs hardcode `internal/board/boardtest`, so the final import/command path is unspecified.
**Fix:** Pin boardtest's new home (e.g. `internal/boardengine/boardtest`) explicitly so its `doc.go` comment and the doc command paths can be updated deterministically.

## Verdict
GAPS_FOUND
Documentation scope is incomplete; one importer omission and a soft boardtest target need pinning.
MILL_REVIEW_END
