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
  branch (adopt-if-exists / create-if-missing) + weft worktree — a *paired* unit, not a
  single worktree. It does **not** wire junctions (that is `lyx init`'s job, keyed to the
  working subfolder — see [activation](#junction-activation)). The misleadingly-named `lyx
  worktree add` implied one worktree; the `warp` namespace makes the dual nature explicit.
- **branch / checkout / switch** — coordinated: switch host+weft together, re-point
  junctions. **This is the priority correctness gap** that triggered the whole module.
  Owns the **fork-point**: a new weft branch forks from the *parent's* weft branch
  (merge-base-preserving mirror-host, [overview](../overview.md#branch-model)), not orphan —
  so `_codeguide` can squash-merge-back later. That fork math is warp's, not a guess at
  branch-create time.
- **reconcile / repair** — repairs the pairing for *already-managed* worktrees (missing
  weft worktree, broken/dangling junction, host branch whose weft sibling was lost). It
  walks **worktrees**, not all host branches (weft branches are opt-in — see
  [pairing scope](#pairing-scope--what-gets-a-weft-worktreebranch)). Absorbs the
  junction-integrity/drift reporting currently in `weft`.
- **list / status** — the **paired view**: every host-WT ↔ weft-WT, its branch, in-sync?,
  junction health. Supersedes `lyx worktree list` + the pairing-health part of `weft status`.
- **prune** — remove orphaned/stale pairs (today `internal/worktree/prune.go`).
- **cleanup** — delete weft branches with no host sibling. Destructive →
  **dry-run / report by default**, explicit flag to actually delete (same discipline as
  mill-cleanup). **Gated on `_codeguide` merge-back:** never delete a task weft branch whose
  `_codeguide` has not been folded back to its parent — else cleanup *is* the data loss
  (the merge itself is `weft`'s job; warp only gates on it — see
  [responsibility boundary](#responsibility-boundary--warp-vs-weft-vs-host)).

## Responsibility boundary — warp vs weft vs host

The content-vs-topology split, made concrete across all three:

- **warp = topology, both sides.** Branch/worktree existence, pairing, fork-point,
  checkout, the junction *mechanism*, reconcile, cleanup. **Never commits weft content.**
- **weft = the single git-writer for the weft repo (content).** "lyx weft owns the overlay's
  git" — every weft-resident content module **authors** and lets `weft` commit: `config`
  (`configcli`/`update`) and `codeguide` both write through the junction and commit via
  `weft` (`commit`/`push`/`pull`/`sync`). The `_codeguide` squash-merge-back is `weft`'s git
  operation, triggered by the codeguide module — **not** warp, **not** raw git.
- **host content = the developer's raw git.** lyx deliberately does not wrap host
  commits/push/pull (host stays pristine, developer-maintained). warp touches host only for
  *topology* (branch/worktree/checkout).

So a weft-resident module never does raw git and never routes commits through warp — it goes
through `weft`. warp owns *when* a weft branch may safely live or die; `weft` owns *what* is
committed into it. **Out of scope:** splitting `_codeguide` into its own repo/topology (its
merge-back is *why* weft is merge-base-preserving) — cleaner in theory but too many moving
parts; one weft, one topology.

## Pairing scope — what gets a weft worktree/branch

The invariant is **per host-worktree, not per-branch**: every host worktree in the hub has
a paired weft worktree (on a weft branch). A branch gets a weft branch **precisely when it
has a host worktree** — so the prime (a worktree on `main`) has the weft prime, while
branches with no worktree (`main`'s other siblings, `extract-*`, `mill-checkpoint-*`) get
no weft. The weft side is created at worktree-creation time, three ways, all converging on
the same adopt-or-create logic in `warp`:

1. The **prime** — cloned by `warp clone` (host prime + weft prime both cloned, on their
   mains → in sync).
2. **`warp add <branch>`** (paired spawn) — creates host-WT + weft-WT + weft branch. It
   does **not** wire junctions (it cannot know the working subfolder yet — see
   [activation](#junction-activation)).
3. A host worktree created **outside lyx** (raw `git worktree`) → warp adopts-or-creates
   the weft side (the pairing); junctions are still wired later by `lyx init`.

`reconcile` therefore walks **worktrees**, not the whole branch namespace: it ensures each
host worktree has an in-sync weft worktree, and never adopts arbitrary branches.

## Junction activation

Creating the weft worktree (the pairing) is distinct from **wiring the junctions** that
route `<cwd>/_lyx` (and `_codeguide`) into the weft. The pairing is geometry; the junction
target is a **working-subfolder** decision warp cannot know at creation time. So:

- **Everything warp creates is dormant.** `warp clone` and `warp add` produce a paired but
  *unwired* worktree — no junctions. They cannot know which subfolder you will work in, and
  you may activate **several** subfolders (a monorepo can `_lyx` in more than one), so
  presuming the worktree root would be wrong.
- **`lyx init` is the activator**, run in the cwd you want active. It is a **forwarder**
  that runs sub-steps in order: (1) wire the junction(s) for the cwd via warp's junction
  primitive, **then** (2) reconcile config (`configsync`). Junctions **first**, because
  config must land in the weft *through* the junction — and the cloned weft may already hold
  committed config that reconcile then merges (preserves values) rather than shadows. Keyed
  to **cwd/subdir**, not the worktree root.
- **Keying is therefore cwd-driven**, a deliberate change from today's `lyx worktree add`,
  which wires a single root junction ([weft.go](../../internal/worktree/weft.go)).
- The **junction mechanism is warp's** (topology, `fslink`); `lyx init` (config layer)
  *calls* that primitive — never the reverse (`init → warp`, never `warp → init`).

**Combined command (deferred):** an outer `warp add` + `lyx init` convenience is wanted but
comes later. **Hard requirement now:** the method that **spawns a task into a new worktree**
(loom's spawn / Setup) MUST run **both** — `warp add` then `lyx init` (at the worktree root,
the default cwd) — so a spawned task worktree is fully usable (pairing + junctions + config),
not a dormant pairing.

## Host-pollution guard

A weft-resident file (under `_lyx` / `_codeguide`) accidentally committed into the **host**
repo breaks the pristine-host invariant. The protection and its limits:

- **Normal case is safe.** The junction path is in the host worktree's `.git/info/exclude`,
  so a plain `git add _lyx/x` is **refused** (ignored), `git add .` / `-A` **skip** it, and
  `git commit -a` won't include it. An agent doing a plain add gets an error and moves on.
- **Two real risks:** (1) `git add -f _lyx/x` force-bypasses the exclude and stages weft
  content into the host index; (2) a worktree where the **exclude entry is missing**
  (junction wired without exclude — an activation-atomicity failure) — then `git add .`
  silently stages the whole `_lyx` tree into host.
- **Guard (principle 6 — detectable + easy-undo, not a hard block):**
  - warp's junction primitive writes the junction **and** the `.git/info/exclude` entry
    **atomically** — never one without the other (closes risk 2).
  - `warp status` / `reconcile` / `doctor` detect any `_lyx` / `_codeguide` path **tracked in
    the host index** (force-added, or exclude was missing) and flag it + offer
    `git rm --cached` + restore junction/exclude (closes risk 1).
  - No hard block: a `pre-commit` hook is brittle and bypassable (out of scope); the bar is
    "caught and trivially reverted", not "impossible".

## Coordinated operations are all-or-nothing

Coordinated operations must **never leave a half-switched / half-paired state** (the
recurring "clean up broken things by hand" pain). Coordinated checkout: precondition
checks first (e.g. refuse to switch if the weft worktree is dirty), and **rollback** the
host side if the weft side fails — the pair is always consistent or untouched, never
half-done. Same discipline for `warp add` and branch create/delete. This realizes the
overview's [correctness-by-tool-design](../overview.md#principles) principle for the
host↔weft pairing.

## Drift detection — when

Because lyx is **daemonless**, it cannot autonomously notice a raw `git checkout` the
instant it happens. The bar is *detectable*, not *impossible* (see
[principle 6](../overview.md#principles)). Drift — host worktree on branch X while its
weft sibling is still on the old branch, junctions pointing at stale `_lyx` — is caught at
three points:

1. **Precondition check on every warp/weft/loom operation (the guarantee).** Each
   operation first verifies the pairing — host worktree's current branch == its weft
   sibling's branch, junctions resolve — and refuses/warns on divergence before acting, so
   no real lyx work proceeds on a desynced pair. In particular **`lyx loom run`'s Setup
   phase** (which already validates "weft pairing present") is strengthened to "present
   **and in sync**". The detection primitive is **stateless**: the weft sibling is
   deterministic (`<prime>-weft`), so it is two `git rev-parse --abbrev-ref HEAD` calls + a
   junction stat — no registry (the check already lives in `weft/status.go`).
2. **On demand — `lyx warp status` / `lyx doctor`.** Ask any time.
3. **Optional `post-checkout` git hook (proactive, non-blocking).** Fires at the moment of
   a raw `git checkout`/`switch` and warns ("host/weft out of sync — run `lyx warp
   reconcile`") or offers repair. Belt-and-suspenders, never a hard block (principle 6).

**Make the correct path easiest.** Expose **`lyx warp checkout`** as a one-click shortcut
in the per-worktree launcher menu (the left-hand menu), so switching a prime's branch the
*coordinated* way is lower-friction than raw `git checkout` — the principle-6 friction
asymmetry, reducing how often drift is created at all.

**Repair:** `lyx warp reconcile` switches the weft sibling to the mirrored branch +
re-points junctions. **Policy knob:** if the host is on an *unmanaged* branch (no weft
sibling — see [branch scope](#branch-scope--what-gets-a-weft-branch)), reconcile either
**auto-adopts** (creates the weft branch) or **reports** ("run `warp add`/`init`") — decide
which; reporting is the safer default.

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
