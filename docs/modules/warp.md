# warp — the host↔weft-coordinated git module

> 🚧 **Design — not built.** This is the authoritative spec for the planned `warp`
> module and matches the `warp-module` board task. Until warp lands, the code still has
> the separate `internal/worktree`, `internal/gitclone`, and `internal/git` packages
> that warp consolidates — that is the *current* state; this doc is the *target*. Per
> the [doc lifecycle](../overview.md#documentation-lifecycle) it is deleted when warp
> lands. **Sequenced after the `config-test-cleanup` task.**

## Why

lyx maintains a **mirror-host topology**: every *lyx-managed* host worktree/branch has a
paired weft worktree/branch, linked by directory junctions (see the
[weft overlay model](../overview.md#weft-overlay-model)). Today the logic that maintains
this pairing is incomplete and scattered:

- `lyx worktree add` coordinates host+weft on creation, but **nothing coordinates
  branch-switching.** Switching the active branch in the prime worktree with a raw
  `git checkout` desyncs the paired weft worktree + junctions — a real correctness gap,
  and a planned user workflow.
- `internal/git` is a thin exec wrapper; `internal/gitclone` is a separate package;
  `internal/weft` mixes content-sync with some topology reporting (junction integrity).

No single module owns the host↔weft topology invariant, so each new git operation risks
reimplementing the pairing. `warp` becomes that single owner.

## Content vs topology — the dividing line

`warp` is named for the weaving **warp**: the structural threads under tension, the
counterpart to the existing content-focused [`weft`](../overview.md#weft-overlay-model)
module. The split is **content vs topology**, not weft vs non-weft:

```
<outer orchestration cmd>   ← composes: warp add, then lyx init
   ↑                  ↑
warp               initcli / configsync
(TOPOLOGY)         (CONFIG — unchanged)
clone, dual-worktree add/remove,
branch, checkout/switch,
reconcile, cleanup, junctions
   ↑
gitexec            ← leaf: exec + proc.HideWindow + exitcode (rename of internal/git)
```

`weft` (content: config sync/commit/push/pull) and `warp` (topology) are **siblings on
`gitexec`** — neither nests the other, so running a bare git command pulls in only
`gitexec`, not a whole coordinator module.

**Dependency direction is load-bearing:** `warp` must NOT depend on `initcli` /
`configsync`. `warp add` must not call `lyx init` — that would invert the dependency
(topology reaching up into the config layer). A thin **outer orchestration command**
composes `warp add` + `lyx init`; it depends on both, neither depends on it. This
mirrors the existing `git-clone` pattern: clone produces a *dormant* hub, inert until
`lyx init` activates it. `warp add` likewise produces a *dormant* dual-worktree; `init`
activates it; the outer command is the everyday convenience that does both.

## What warp owns

- **clone** (hub-bootstrap) — absorbs `internal/gitclone`. host+weft setup shares the
  pairing code with `warp add`. **The board is a passenger**: a plain `gitexec` clone,
  NOT mirrored; reconcile/cleanup never touch it.
- **dual-worktree add/remove** — `warp add` creates a host worktree + ensures the weft
  branch (adopt-if-exists / create-if-missing) + weft worktree + junctions — a *paired*
  unit, not a single worktree. The misleadingly-named `lyx worktree add` implied one
  worktree; the `warp` namespace makes the dual nature explicit.
- **branch / checkout / switch** — coordinated: switch host+weft together, re-point
  junctions. **This is the priority correctness gap** that triggered the whole module.
- **reconcile / repair** — repairs the pairing for *already-managed* branches (missing
  weft worktree, broken/dangling junction, host branch whose weft sibling was lost). It
  does **NOT** scan all host branches and adopt them — weft branches are opt-in (see
  below). Absorbs the junction-integrity/drift reporting currently in `weft`.
- **cleanup** — delete weft branches with no host sibling. Destructive →
  **dry-run / report by default**, explicit flag to actually delete (same discipline as
  mill-cleanup).

## Branch scope — what gets a weft branch

Only branches **worked on with lyx** get a weft branch. Weft-branch provisioning is an
explicit **setup step**, never implicit mirroring of every host branch. So `main`,
`extract-*`, `mill-checkpoint-*` etc. stay unmirrored unless explicitly set up. Three
entry paths, all converging on the same adopt-or-create logic in `warp`:

1. Create a host worktree with plain `git worktree` (outside lyx), then run `lyx init`
   in it → init triggers warp's adopt-or-create: adopt the weft branch if it exists,
   else create it.
2. Ask lyx to create the worktree for an existing branch (`warp add <branch>`) → same
   adopt-or-create, skips the manual `lyx init`.
3. lyx creates a brand-new branch → fully automatic.

`reconcile` therefore operates on the managed set, not the whole branch namespace.

## Coordinated operations are all-or-nothing

Coordinated operations must **never leave a half-switched / half-paired state** (the
recurring "clean up broken things by hand" pain). Coordinated checkout: precondition
checks first (e.g. refuse to switch if the weft worktree is dirty), and **rollback** the
host side if the weft side fails — the pair is always consistent or untouched, never
half-done. Same discipline for `warp add` and branch create/delete. This realizes the
overview's [correctness-by-tool-design](../overview.md#principles) principle for the
host↔weft pairing.

## CLI surface

`internal/worktree`'s command surface is **fully absorbed into `warp`** — `worktree` is
too git-narrow a name for an operation that sets up a *dual* worktree. The `warp`
namespace conveys the paired nature: `lyx warp add`, `lyx warp checkout`,
`lyx warp reconcile`, `lyx warp cleanup`, `lyx warp clone`. The **outer orchestration
command** (warp add + lyx init) is the everyday "give me a ready-to-work worktree"
convenience — name TBD (candidates: `lyx new <branch>`, `lyx open`).

## The config module: `worktree` → `warp`

Because `warp` replaces `worktree`, there is no longer a "worktree" config module. The
config module/template is **renamed `worktree` → `warp`**: `configreg` registers a
"warp" module, the template is `warp.yaml`, and the user's config file becomes
`_lyx/config/warp.yaml`. No cycle: like `board`/`weft`, `warp` does not import
`configreg`. **Migration:** existing `_lyx/config/worktree.yaml` renames to `warp.yaml`;
`lyx update` should handle the rename so existing hubs don't orphan the old file.

## What moves in (and what stays)

- `internal/git` → renamed **`gitexec`** (thin leaf, logic unchanged: exec +
  `proc.HideWindow` + exit-code parsing).
- `internal/gitclone` → folded into `warp`.
- `internal/worktree` → **deleted.** Its lifecycle, CLI, `Config`/`LoadConfig`, and the
  config template all move into `warp` (template renamed `warp.yaml`).
- `internal/weft` → keeps content-sync (sync/commit/push/pull, ahead/behind/dirty); its
  junction-integrity / drift reporting moves to `warp.reconcile`.

## Decisions settled

- **Keep shelling out to real `git`** via `gitexec`. Do NOT adopt go-git / git2go:
  worktrees are core to lyx and go-git's worktree support is weak; full git compatibility
  (config, credential-helpers, hooks) matters. Go's `exec` is far cheaper than Python's
  `subprocess`, so the Python "use a library for speed" lesson does not transfer. If
  Windows process-spawn ever shows up in a profile, the fix is fewer / batched git calls,
  not a new engine. Measure first.
- **The board is not a mirrored entity** — passenger in `clone` only.
- **`warp` does not depend on the config layer** — the outer command composes warp + init.

## Dependencies / sequencing

- `gitexec` (renamed leaf), `fslink` (junctions), `paths` (geometry), `config`.
- **Sequenced after `config-test-cleanup`**, which moves `worktree.yaml` into
  `internal/worktree` (uniform with `board`/`weft`). `warp` then supersedes that: deletes
  `internal/worktree` and renames the config module `worktree` → `warp`.
  config-test-cleanup's worktree-template work is thus partly redone by `warp` — accepted,
  to keep config-test-cleanup uniform and not entangle it with the rename.

## Out of scope

- Folding `weft` content-sync into `warp` — keep the content/topology split. A later
  consolidation is possible but not part of this module.
- The local test **sandbox** (real host/weft/board via `lyx git-clone`) — its own task
  (`lyx-sandbox`); it is the proving ground for `warp` once `warp` exists.
