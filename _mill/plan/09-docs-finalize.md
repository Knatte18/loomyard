# Batch: docs-finalize

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: docs-finalize
number: 9
cards: 3
verify: go build ./...
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
- **Requirements:** In `docs/overview.md`: mark **warp** as ✅ Implemented in the Modules list (host↔weft topology owner) and remove/supersede the separate **worktree** and **git-clone** module entries (their surface is now `lyx warp …`); update the Module dispatch `switch` snippet to show the `warp` case and drop `worktree`/`git-clone`; update the directory-tree block (`internal/git` → `internal/gitexec`, add `internal/warp`, remove `internal/worktree` and `internal/gitclone`); fix any prose that says `lyx worktree add`/`lyx git-clone` to the `lyx warp …` forms. Leave the weft overlay-model sections intact (weft keeps content-sync).
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
- **Requirements:** Update stale references in these durable docs: `internal/git` → `internal/gitexec`; `internal/worktree` / `internal/gitclone` → `internal/warp`; `lyx worktree …` / `lyx git-clone …` command forms → `lyx warp …`. In `docs/modules/loom.md`, where Setup is described as validating the weft pairing via the `worktree`/`weft` modules, name `warp` as the topology owner (do not add loom implementation — loom remains unbuilt). Keep each doc's surrounding meaning intact; this is a reference-accuracy sweep only.
- **Commit:** `docs: sweep gitexec/warp package and command references`

## Batch Tests

`verify: go build ./...`. This batch is docs plus one Go edit (the package header comment in `warp.go`); `go build ./...` confirms the comment is well-formed and nothing else regressed. There is no runnable test surface for the markdown changes — they are reference-accuracy edits validated by review.
