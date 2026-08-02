# Batch: clean-healthy-renames

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
batch: clean-healthy-renames
number: 4
cards: 2
verify: go test -tags integration ./internal/fabricengine/ ./internal/loomengine/
depends-on: [3]
```

## Batch Scope

Two warp/weft-hiding renames on fabric's preflight surface: `HostClean`→`Clean` (extended to check both warp and weft cleanliness) and `PairInSync`→`Healthy` (kept cheap). Each card is a complete rename — definition, its cross-package caller in `loomengine/preflight.go`, in-package test call sites, and stale doc-comment mentions — so every commit compiles. No new heavyweight work is added under either rename. This batch touches no `weftgit.go`/`commit.go` code paths structurally (only a `PairInSync` comment mention in `commit.go`), so it is independent of the CLI-surface work but sequenced after batch 3 to keep the shared-file chain linear.

## Cards

### Card 16: Rename HostClean to package-level Clean, both sides

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/cleanup.go`
- **Edits:**
  - `internal/fabricengine/hostclean.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the package-level `HostClean(l *hubgeometry.Layout) (clean bool, reason string, err error)` (`hostclean.go:37`) to `Clean` with the same signature. Extend it to check cleanliness of BOTH `l.WorktreeRoot` (warp/host) and `l.WeftWorktree()` (weft), applying the same strict `git status --porcelain` (untracked-strict) check to each; when either side is dirty, return `clean=false` with a combined `reason`. CRITICAL: guard the weft-side check behind a weft-worktree existence stat — if `l.WeftWorktree()` does not exist (`os.IsNotExist`), skip the weft check entirely (do NOT run `git status` against a missing path, which would return an infra error). This preserves preflight's existing contract where a missing weft worktree is a distinct prior `CheckWeftPairing` condition (`preflight.go:113`), not a `Clean` error. Update the sole caller `preflight.go:100` from `fabricengine.HostClean(l)` to `fabricengine.Clean(l)` (still assigned to `clean, reason, err`, still feeding `CheckWorktreeClean`). Verify no existing `Clean` identifier collides in package `fabricengine` (e.g. in `cleanup.go`). In `preflight_integration_test.go`, add positive regression coverage for the genuinely-new weft-dirty failure mode: a dirty-weft-only case (clean warp, dirty weft → `CheckWorktreeClean` fails) and a both-dirty case, alongside the existing dirty-warp-only and missing-weft-worktree cases (the missing-weft case must still surface `CheckWeftPairing`, NOT a `Clean` error — assert `Clean` skipped the weft side gracefully). Trim `hostclean.go`'s long how-it-works doc comment to the `golang-comments` shape and update its file-header comment and any `HostClean` mention (e.g. `drift.go`'s header cross-reference) to the new name. Confirm the added weft check does not duplicate preflight's separate `Healthy`/pairing check (which compares branch names, not worktree dirtiness).
- **Commit:** `refactor(fabric): rename HostClean to Clean, check warp and weft`

### Card 17: Rename PairInSync to Healthy

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/junction.go`
- **Edits:**
  - `internal/fabricengine/drift.go`
  - `internal/loomengine/preflight.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the package-level `PairInSync(l *hubgeometry.Layout) (ok bool, reason string, err error)` (`drift.go:44`) to `Healthy` with the same signature and identical body — do NOT add any new work; it must stay as cheap as it is today (its existing cost — two `rev-parse` git spawns + a `fabric.yaml` load + per-junction stats — is the ceiling, not a floor to build on). Update the functional caller `preflight.go:120` (`fabricengine.PairInSync(l)` → `fabricengine.Healthy(l)`) and the four in-package test call sites: `config_driven_junctions_integration_test.go:123`, `junction_pattern_integration_test.go:418`, `reconcile_stale_removal_test.go:348`, `reconcile_stale_registration_test.go:501`. Update stale doc-comment mentions of `PairInSync` in the same commit: `drift.go`'s file header, `doc.go:68`, and any mention in `commit.go`/`status.go`/`reconcile.go`/`junctionnames.go` (grep for `PairInSync` and update every hit to `Healthy`; if a listed file has no functional or comment hit, drop it from Edits). Preserve the junction-reason string format preflight's classifier keys on (the substring `"junction"` must remain in those reasons). Trim `drift.go`'s long doc comment to the `golang-comments` shape.
- **Commit:** `refactor(fabric): rename PairInSync to Healthy`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/loomengine/` runs the fabric junction/reconcile integration tests (which exercise `Healthy`) and loomengine's preflight tests (which exercise both `Clean` and `Healthy`, including the missing-weft-worktree path that must not turn into a `Clean` error). Card 16 adds the both-sides `Clean` coverage in `preflight_integration_test.go`: dirty warp only → not clean; dirty weft only → not clean; both dirty → not clean; missing weft worktree → `Clean` skips weft (no error) and the existing `CheckWeftPairing` still fires. Scope is the two edited packages.
