# Batch: docs-finalize

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: docs-finalize
number: 9
cards: 3
verify: go vet -tags integration ./... && go test -tags integration ./...
depends-on: [8]
```

## Batch Scope

Land the documentation per the doc lifecycle now that warp is built: delete the mechanical design doc, move its durable rationale into the package header comment, and update the authoritative `docs/overview.md` plus the remaining docs that still name the old `internal/git` / `internal/worktree` / `internal/gitclone` packages and the `lyx git-clone` / `lyx worktree` commands.

Batch-local decision: `docs/modules/warp.md` is deleted (mechanical per-module design docs are removed when the module lands — the implementation + package doc become the source of truth). `docs/overview.md` is durable and is updated, not deleted.

## Cards

### Card 31: Delete design doc; add warp package doc comment

- **Context:**
  - `internal/warp/clone.go`
  - `internal/warp/add.go`
  - `internal/warp/checkout.go`
  - `internal/warp/reconcile.go`
  - `docs/modules/warp.md`
- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Deletes:**
  - `docs/modules/warp.md`
- **Requirements:** Add a package header doc comment to `internal/warp/warp.go` capturing warp's purpose and key rationale distilled from `docs/modules/warp.md`: warp owns the host↔weft *topology* (clone, dual-worktree add/remove, coordinated checkout, reconcile, cleanup, prune, the junction mechanism, drift detection) as the sibling-on-`gitexec` counterpart to content-focused `weft`; the dormant-pairing / `lyx init`-activation model; the all-or-nothing coordinated-op discipline. Then delete `docs/modules/warp.md` (doc lifecycle: removed when the module lands).
- **Commit:** `docs(warp): package doc; remove landed design doc`

### Card 32: Update overview.md for the warp module

- **Context:**
  - `internal/warp/warp.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`: mark **warp** as ✅ Implemented in the Modules list (host↔weft topology owner) and remove/supersede the separate **worktree** and **git-clone** module entries (their surface is now `lyx warp …`); update the Module dispatch `switch` snippet to show the `warp` case and drop `worktree`/`git-clone`; update the directory-tree block (`internal/git` → `internal/gitexec`, add `internal/warp`, remove `internal/worktree` and `internal/gitclone`); fix any prose that says `lyx worktree add`/`lyx git-clone` to the `lyx warp …` forms; and **remove or retarget the two dangling links to `modules/warp.md`** (around lines 227 and 308 — the `See [modules/warp.md]` module-list link and the doc-lifecycle `[modules/warp.md] … (design)` reference), since card 31 deletes that file in this same batch. Leave the weft overlay-model sections intact (weft keeps content-sync).
- **Commit:** `docs(overview): warp lands; gitexec rename; drop worktree/git-clone`

### Card 33: Sweep remaining doc references

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `docs/modules/README.md`
  - `docs/modules/loom.md`
  - `docs/shared-libs/paths.md`
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update stale references in these durable docs: `internal/git` → `internal/gitexec`; `internal/worktree` / `internal/gitclone` → `internal/warp`; `lyx worktree …` / `lyx git-clone …` command forms → `lyx warp …`. In `docs/modules/loom.md`, where Setup is described as validating the weft pairing via the `worktree`/`weft` modules, name `warp` as the topology owner (do not add loom implementation — loom remains unbuilt). Keep each doc's surrounding meaning intact; this is a reference-accuracy sweep only. **Intentionally excluded (frozen, do not sweep):** `docs/roadmap.md` and `docs/benchmarks/test-suite-timing.md` — the roadmap's landed entries and the benchmark log are point-in-time historical records; rewriting their `internal/git`/`internal/worktree`/`internal/gitclone` package names would falsify the record. They are deliberately left as-is.
- **Commit:** `docs: sweep gitexec/warp package and command references`

## Batch Tests

`verify: go vet -tags integration ./... && go test -tags integration ./...`. As the final batch this is also the **module-wide integration gate**: `go vet -tags integration ./...` compiles every package (including integration-tagged test files) and confirms no import cycle was introduced by the rename/move/fold and the `initcli → warp` / `configreg → warp` edges; `go test -tags integration ./...` runs the entire suite — unit and integration — across the whole module to catch any cross-package regression. This batch itself is docs plus one Go edit (the package header comment in `warp.go`); the markdown changes have no runnable surface and are validated by review, but the module-wide gate guards the cumulative result of batches 1–8.
