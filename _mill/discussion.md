# Discussion: Introduce warp: the host↔weft-coordinated git module

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
slug: warp-module
status: discussing
parent: main
```

## Problem

lyx maintains a **mirror-host topology**: every lyx-managed host worktree/branch has a
paired weft worktree/branch, linked by directory junctions (`<host>/_lyx` →
`<hub>/<slug>-weft/_lyx`). The host repo stays pristine (developer-owned); all lyx overlay
artifacts (config, codeguide, board, task state) live in the separate weft repo.

Today the logic that maintains this pairing is **incomplete and scattered** across three
packages with no single owner of the invariant:

- `internal/worktree` coordinates host+weft on *creation* (`lyx worktree add`), but
  **nothing coordinates branch-switching.** A raw `git checkout` in the host worktree
  desyncs the paired weft worktree + junctions — a real correctness gap and a planned
  user workflow. This is the **priority gap** that triggered the module.
- `internal/git` is a thin exec wrapper; `internal/gitclone` is a separate hub-bootstrap
  package; `internal/weft` mixes content-sync with topology reporting (junction integrity).

Because no module owns the host↔weft topology invariant, each new git operation risks
re-implementing the pairing. **`warp` becomes that single owner** — named for the weaving
warp (the structural threads under tension), the topology counterpart to the existing
content-focused `weft` module. The split is **content vs topology**: `weft` owns *what* is
committed into the weft repo; `warp` owns the *shape* — branch/worktree existence, pairing,
fork-point, coordinated checkout, junction mechanism, reconcile, cleanup.

**Why now:** the design is fully specified in `docs/modules/warp.md` and its sequencing
dependency `config-test-cleanup` is **done** (commit `9fff46f`), so warp can proceed.

## Scope

**In:**

- **`internal/git` → renamed `internal/gitexec`** (thin leaf: `RunGit` + `proc.HideWindow` +
  exit-code parsing — logic unchanged). **All** importers swept, not just `weft`/`warp`:
  surviving **production** importers `internal/paths` (`paths.go`, `worktreelist.go`) and
  `internal/board` (`git.go`, `sync.go`) must be updated too, plus the new `warp` and the
  test-only importers (`internal/update`, `internal/initcli`, `cmd/lyx` tests). Omitting
  `paths`/`board` leaves the build broken.
- **`internal/gitclone` → folded into `warp`** as the `clone` verb (hub-bootstrap: host +
  weft + board, no junctions — dormant hub; the board is a plain passenger, never mirrored).
- **`internal/worktree` → deleted**, its surface absorbed into `warp`: `add`/`remove`/`list`,
  launchers, portals, weft-side junction wiring, `Config`/`LoadConfig`, config template.
- **Coordinated `warp checkout`** (the priority gap): switch host + weft together, re-point
  junctions, own the fork-point. All-or-nothing (precondition checks + rollback).
- **`warp reconcile`** — repairs the pairing for already-managed worktrees and adopts
  raw (non-lyx) host worktrees; absorbs the junction-integrity/drift reporting currently in
  `weft/status.go`. Walks **worktrees**, never the whole branch namespace.
- **`warp list` / `warp status`** — the paired view (host-WT ↔ weft-WT, branch, in-sync?,
  junction health). Supersedes `lyx worktree list` + the pairing-health part of `weft status`.
- **`warp prune`** — remove orphaned/stale pairs. **New functionality** (see Technical
  context: there is no `lyx worktree prune` today; `prune.go` is an internal ancestor-sweeper).
- **`warp cleanup`** — delete weft branches with no host sibling; dry-run/report by default,
  destructive only on `--apply`/`--force`, with a `_codeguide` merge-back gate wired in.
- **Junction-wiring relocation:** `warp add`/`clone` produce a **dormant** pairing (no
  junctions); **`lyx init` becomes the activator** that wires the cwd-keyed junction(s) +
  `.git/info/exclude` *atomically*, then reconciles config. Touches `internal/initcli`.
- **Host-pollution guard (detection side):** `warp status`/`reconcile` flag `_lyx`/`_codeguide`
  paths tracked in the host index. For `_lyx` (the junction warp wires) it offers `git rm
  --cached` + restore junction/exclude. For `_codeguide` it is **report-only this task** (warp
  wires no `_codeguide` junction yet — nothing to restore). The junction primitive writes
  junction + exclude atomically (closes the missing-exclude risk).
- **Drift detection:** host-branch vs weft-branch comparison as a precondition on warp/weft
  ops + on demand via `warp status`; plus an optional **`post-checkout` git hook** that warns
  on raw checkout drift.
- **`lyx warp checkout` launcher shortcut** — warp's absorbed launcher generation
  (`launchers.go`, moved from `worktree`) emits an additional per-worktree `warp-checkout.cmd`
  (alongside the existing `ide.cmd`) that invokes `lyx warp checkout`. **No `internal/ide`
  change** — the `ide menu` is a worktree *picker* (pick a worktree → spawn IDE), not a
  per-worktree action menu, so the shortcut lives entirely in warp's launcher code.
- **Config module rename `worktree` → `warp`:** `configreg` entry swap + template file
  rename to `warp.yaml`; user config file becomes `_lyx/config/warp.yaml`.
- **`cmd/lyx/main.go` dispatch:** replace the `worktree` and `git-clone` cases with a single
  `warp` case (`weft` stays).

**Out:**

- **Outer orchestration command** (`lyx new` / `lyx open` = `warp add` + `lyx init`) —
  deferred by design; its only hard-required caller (`loom`) is not built. `warp add` and
  `lyx init` remain composable standalone.
- **Strengthening `loom`'s Setup phase** to validate "pairing present **and in sync**" — the
  `loom` module does not exist yet (no `internal/loom`, not wired in `main.go`). warp only
  *provides* the stateless precondition primitive; wiring it into loom is loom's task.
- **`lyx doctor`** — does not exist; drift is exposed via `lyx warp status` for now.
- **Config migration** (`worktree.yaml` → `warp.yaml` value-preserving rename in `lyx update`)
  — there are **no existing hubs** to migrate, so no migration code. The rename is registry +
  template only.
- **Folding `weft` content-sync into `warp`** — keep the content/topology split. `weft` keeps
  `commit`/`push`/`pull`/`sync` and ahead/behind/dirty; only its junction-integrity/drift
  reporting moves to `warp.reconcile`.
- **Codeguide merge-back implementation** — `_codeguide` junction activation / squash-merge-back
  is deferred elsewhere; `warp cleanup` only *gates on* it (conservatively), never performs it.
- **Splitting `_codeguide` into its own repo/topology** — out of scope; one weft, one topology.
- **go-git / git2go adoption** — settled: keep shelling out to real `git` via `gitexec`.

## Decisions

### scope-full-minus-unbuilt-deps

- Decision: This task delivers the **full warp module as far as is buildable now** — every
  verb, the gitexec rename, the worktree+gitclone fold-in, the config rename, junction
  relocation, host-pollution guard, drift detection (incl. the git hook), and the launcher
  shortcut. The only exclusions are items blocked by **unbuilt dependencies** (loom Setup
  wiring, the outer command, codeguide merge-back) — not an MVP slice.
- Rationale: The design (`docs/modules/warp.md`) is the agreed scope; principle 5 ("land one
  milestone at a time, refactors behaviour-preserving") plus principle "design the full
  scope" mean we build everything the current tree supports.
- Rejected: A structural-consolidation-only slice deferring checkout/reconcile/cleanup —
  rejected; the coordinated checkout *is* the priority gap and must ship.

### gitexec-rename

- Decision: Rename `internal/git` → `internal/gitexec` in this task. `RunGit(args []string,
  cwd string) (stdout, stderr string, exitCode int, err error)` is unchanged (non-zero git
  exit is returned in `exitCode` with `err == nil`; only spawn failures set `err` + `exitCode
  == -1`). Update **all** importers: the surviving production importers `internal/paths`
  (`paths.go`, `worktreelist.go`) and `internal/board` (`git.go`, `sync.go`) must be swept
  (note `internal/paths` sits *below* `warp` and imports the leaf directly — `paths → gitexec`
  is fine), alongside `weft` and the new `warp`; test-only importers are `internal/update`,
  `internal/initcli`, and `cmd/lyx` tests. Omitting `paths`/`board` breaks the build.
- Rationale: Settled in the design; `gitexec` is the honest leaf name once `warp`/`weft` sit
  as siblings on it. Mechanical, behaviour-preserving.
- Rejected: Keep the `internal/git` name — leaves a misleadingly generic package below two
  coordinator modules.

### single-warp-package

- Decision: One `internal/warp` package, files split by verb: `clone.go`, `add.go`,
  `remove.go`, `checkout.go`, `reconcile.go`, `cleanup.go`, `list.go`, `status.go`,
  `prune.go`, `junction.go`, `launchers.go`, `portals.go`, `config.go`, `template.yaml`,
  `warp.go` (facade). Exposes `RunCLI(out io.Writer, args []string) int` and routes
  subcommands with an internal `switch` (the hand-rolled dispatch convention; **no cobra**).
- Rationale: Matches the module-per-package convention (`board`/`weft`); `worktree` +
  `gitclone` collapse cleanly into it.
- Rejected: Sub-packages per verb — over-structured for a single module's domain logic
  (principle 2: a module's logic + tests live in one package).

### junction-relocation-dormant-then-init

- Decision: `warp add`/`clone` create a **dormant** pairing (host WT + weft WT + weft branch,
  **no junctions**). **`lyx init` becomes the activator**: run in the cwd you want active, it
  (1) wires the junction(s) for that cwd via warp's junction primitive, **then** (2) reconciles
  config (`configsync`). Junctions first, because config must land in the weft *through* the
  junction. Keyed to **cwd/subdir**, not the worktree root (a monorepo may activate several
  subfolders). The junction mechanism is warp's (topology, `fslink`); `init` (config layer)
  *calls* it — never the reverse (`init → warp`, never `warp → init`).
- Rationale: warp cannot know the working subfolder at creation time; presuming the worktree
  root would be wrong. This realizes the content-vs-topology dependency direction (warp must
  not depend on `initcli`/`configsync`) and the dormant-hub pattern that `gitclone` already
  follows.
- Rejected: Keep junction-wiring inside `warp add` (single root junction, as
  `worktree.Add`/`seedLyxJunction` does today) — abandons the dormant/activation model and
  the cwd-keyed multi-subfolder support.

### coordinated-checkout-all-or-nothing

- Decision: `lyx warp checkout <branch>` switches host + weft together and re-points
  junctions, as an all-or-nothing operation: **precondition-check first** (refuse if the weft
  worktree is dirty; surface git's own refusal if the host has conflicting uncommitted
  changes), perform the host switch, then the weft switch + junction re-point; **roll back the
  host side if the weft side fails**. The pair is always consistent or untouched, never
  half-switched. Same rollback discipline as `warp add` (already present via `rollbackAdd`).
- Rationale: Realizes the overview's correctness-by-tool-design principle for the pairing;
  eliminates the "clean up broken state by hand" pain.
- Rejected: Best-effort switch without rollback — leaves half-switched pairs, the exact
  failure mode the module exists to prevent.

### checkout-onto-unmanaged-branch-forks-weft

- Decision: When `warp checkout <branch>` targets a host branch with **no weft sibling**
  (unmanaged), warp **creates the weft branch using the same fork-point logic as `warp add`**
  — fork from the parent's weft branch (merge-base-preserving mirror-host), then switch both
  sides. The result is a fully managed pair, exactly as if the branch had just been created
  via `add`.
- Rationale: warp owns the fork-point; the coordinated path should "just work" and be strictly
  easier than raw `git checkout` (the principle-6 friction asymmetry). Reuses the
  adopt-or-create/fork-point code path shared with `add`.
- Rejected: Refuse + report ("run `warp add`") — would make coordinated checkout unable to
  switch onto a fresh branch, undercutting the whole point of the shortcut.

### add-adopts-existing-weft-branch

- Decision: `warp add <slug>` **adopts** an existing weft branch (builds the weft worktree
  from it) instead of aborting. Today's `add.go` aborts on a weft-branch-exists precheck;
  this changes to adopt-or-create (create weft branch if missing, adopt if present).
- Rationale: Matches the design's adopt-or-create; supports re-pairing after a weft worktree
  was lost while its branch survived. Same logic reused by `checkout` and `reconcile`.
- Rejected: Keep abort-if-exists — blocks the legitimate re-pair workflow.

### reconcile-reports-on-unmanaged-branch

- Decision: When `warp reconcile` finds a host worktree on an **unmanaged** branch (no weft
  sibling), it **reports** ("run `warp add` / `init`") and touches nothing. It does adopt
  worktrees that are missing only the weft *worktree* (branch present) and repairs
  broken/dangling junctions. `reconcile` walks **worktrees**, not the whole branch namespace.
- Rationale: Reporting is the safer default — auto-creating weft branches during a sweep risks
  surprising branch creation. (Note the contrast with `checkout`, which *does* fork, because
  there the user explicitly asked to move onto that branch.)
- Rejected: Auto-adopt during reconcile — too implicit for a repair sweep.

### cleanup-codeguide-gate-conservative

- Decision: `warp cleanup` is **dry-run/report by default**; on `--apply` it deletes weft
  branches with **no host sibling**, but routes task weft branches through a
  `codeguideFoldedBack(branch) bool` gate. Until codeguide merge-back exists, that gate
  **conservatively protects** task branches (deletable only with an explicit `--force`). The
  gate is the wired-in extension point — when codeguide merge-back lands it implements the real
  "has `_codeguide` been folded back to the parent?" check.
- Rationale: Same destructive discipline as `mill-cleanup`. "Build it with support for it"
  (Q2): the gate exists and is honored now, so enabling the real check later is a one-function
  change, with no data-loss window in the interim.
- Rejected: Defer cleanup entirely (no gate to build against later); or delete with no gate
  (the deletion *is* the data loss the gate exists to prevent).

### host-pollution-guard

- Decision: Detection-and-easy-undo, not a hard block (principle 6). (1) warp's junction
  primitive writes the junction **and** the `.git/info/exclude` entry **atomically** — never
  one without the other (closes the missing-exclude risk). (2) `warp status`/`reconcile`/the
  status path detect any `_lyx`/`_codeguide` path **tracked in the host index** (force-added,
  or exclude was missing) and flag it. For `_lyx` it offers `git rm --cached` + restore
  junction/exclude (closes the force-add risk). For `_codeguide` the action is **report-only
  this task** — warp creates no `_codeguide` junction yet (`paths.HostJunctions(slug)` returns
  only the `_lyx` entry; `_codeguide` activation is deferred), so there is no junction/exclude
  to restore; the guard just flags the polluting `_codeguide` path. No `pre-commit` hook
  (brittle, bypassable — out of scope).
- Rationale: The bar is "caught and trivially reverted", not "impossible".
- Rejected: A hard `pre-commit` block — brittle and bypassable.

### drift-detection-three-points-incl-hook

- Decision: Drift = host worktree on branch X while its weft sibling is on the old branch /
  junctions stale. Caught at: (1) **precondition check** on warp/weft operations — host
  branch == weft sibling branch + junctions resolve, refuse/warn before acting; the primitive
  is **stateless** (weft sibling is deterministically `<x>-weft`, so two `git rev-parse
  --abbrev-ref HEAD` calls + a junction stat — no registry); (2) **on demand** via `warp
  status`; (3) an **optional `post-checkout` git hook** that fires after a raw
  `git checkout`/`switch` and **warns** ("host/weft out of sync — run `lyx warp reconcile`")
  — never a hard block.
- Rationale: lyx is daemonless and cannot notice a raw checkout the instant it happens; the
  hook is the earliest proactive detection point (Q3: include it).
- Hook script source: an **embedded POSIX `sh` script** (`go:embed` asset in `warp`) installed
  into the repo's common `.git/hooks/post-checkout`. On git-for-Windows, hooks run under git's
  bundled bash, so a POSIX `sh` body is the portable choice. Install is idempotent and
  **non-clobbering** — if a user `post-checkout` already exists, chain it (invoke the existing
  hook, then warp's check) rather than overwrite. Exact body + chaining mechanics are deferred
  to the plan.
- Rejected: Omit the hook — loses the at-creation-time warning. (See Technical context for the
  shared-`.git/hooks` validation item that the "if it works" caveat refers to.)
- Repair: `warp reconcile` switches the weft sibling to the mirrored branch + re-points
  junctions.

### config-rename-no-migration

- Decision: Rename the config module `worktree` → `warp`: swap the `configreg.Modules()`
  entry (`{"worktree", worktree.ConfigTemplate}` → `{"warp", warp.ConfigTemplate}`) and rename
  the embedded template file to `warp.yaml`; the user file becomes `_lyx/config/warp.yaml`.
  **No migration code** — there are no existing hubs (Q7).
- Rationale: `configreg` is a neutral registry with no per-module self-registration, so the
  rename is a one-line entry swap + a template-file rename. With no deployed hubs, the
  value-preserving `lyx update` rename the design once contemplated is unnecessary.
- Rejected: Build `worktree.yaml` → `warp.yaml` migration in `lyx update` — no hubs to migrate.

### board-not-mirrored

- Decision: The board is a **passenger** in `clone` only — a plain `gitexec` clone, not
  mirrored. `reconcile`/`cleanup`/`prune` never touch it.
- Rationale: The board is shared task state, not per-worktree topology.
- Rejected: Treating the board as a mirrored entity — it has no host↔weft pairing.

## Technical context

The CLI has **no cobra**: `cmd/lyx/main.go` is a hand-rolled string-`switch` dispatcher;
every module exposes `RunCLI(out io.Writer, args []string) int` and routes its own
subcommands. Output is JSON via `internal/output.Ok(w, map[string]any) int` /
`output.Err(w, msg) int`; exit 1 on error. Mirror this exactly in `warp`.

**`internal/git/git.go` (→ `gitexec`):** sole export
`RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`. This
4-tuple convention (non-zero git exit ≠ Go error) is used by every caller — preserve it.
Production importers that survive this task and **must** be updated by the rename:
`internal/paths` (`paths.go`, `worktreelist.go`) and `internal/board` (`git.go`, `sync.go`),
plus `weft` (`sync.go`, `status.go`) and the new `warp`. Test-only importers: `internal/update`,
`internal/initcli`, `cmd/lyx` tests. (`worktree` + `gitclone` importers disappear with those
packages.)

**`internal/gitclone/` (→ `warp clone`):** `RunCLI` handles
`lyx git-clone <host-url> <weft-url> [board-url]`. Internal: `cloneHub(cwd, hostURL, weftURL,
boardURL)` derives Hub = `<cwd>/<name>-HUB`, clones host → `<Hub>/<name>`, weft →
`<Hub>/<name>-weft`, board → `<Hub>/_board`; **strict-abort** with `teardownHub` (full
`os.RemoveAll`) on any clone failure. `deriveHostName`, `deriveBoardURL` (`<weft>` → strip
`.git`, append `.wiki.git`). `var removeAll = os.RemoveAll` is a test seam. No junctions —
produces a dormant hub.

**`internal/worktree/` (→ deleted, absorbed):**
- `RunCLI`: `lyx worktree add <slug>` / `list` / `remove [--force] <slug>`. **There is no
  `prune` subcommand** — `prune.go`'s `pruneEmptyAncestors(start, stop)` is an internal
  empty-dir sweeper used by launcher/portal teardown. So `warp prune` (remove orphaned/stale
  pairs) is **new functionality**, not a move.
- `add.go` `Add(l *paths.Layout, slug string, opts AddOptions) (AddResult, error)` is the
  paired transactional create: host clean check → branch/target/remote prechecks → weft
  prechecks → resolve parent host branch via `rev-parse --abbrev-ref HEAD` → `git worktree
  add -b <branch> <target>` (host) → `createWeftWorktree` forking from the **parent's weft
  branch** (the fork-point math to reuse) → `seedLyxJunction` + `seedGitExclude` → portal →
  launchers → push host + weft. `rollbackAdd` tears down in reverse (junction **first** for
  the Windows junction-lock hazard). **Reuse:** the fork-point logic for `checkout`-onto-
  unmanaged and the adopt-or-create change; **move:** `seedLyxJunction`/`seedGitExclude` out
  of `add` and into `lyx init` (junction relocation).
- `weft.go` (unexported helpers to absorb): `weftRepoExists`, `weftBranchExists`,
  `createWeftWorktree(l, slug, branch, startPoint)`, `seedLyxJunction` (iterates
  `l.HostJunctions(slug)`, create-or-verify via `fslink`), `seedGitExclude` (appends junction
  `Name` to `.git/info/exclude` via `rev-parse --git-path info/exclude`, idempotent),
  `removeHostJunction`, `removeWeftWorktree`.
- `config.go`: `type Config struct { BranchPrefix string }`, `LoadConfig(baseDir, module
  string)`; `template.yaml` is a single key `branch_prefix: ${env:LYX_BRANCH_PREFIX:-}`
  (empty default → branch == slug). Becomes `warp.yaml`.
- `list.go`: `List(sourceDir)` delegates to `paths.List`. `launchers.go`/`portals.go`:
  per-worktree launcher scripts (Windows `.cmd`) + portal junctions — move verbatim.

**`internal/weft/` (keeps content-sync; status drift moves to warp):**
- `status.go` `Status(weftWorktree, hostLink, weftLyxDir string, pathspec []string)
  (map[string]any, error)` reports `branch`, `dirty`, `ahead`/`behind`, `junction_ok`,
  `junction_reason`. `checkJunction(hostLink, weftLyxDir)` (host link exists / is a link /
  resolves to the weft `_lyx`) is the junction-integrity logic to move to `warp.reconcile`/
  `warp status`. **Important:** `Status` does **not** currently compare host-branch vs
  weft-branch — that branch-equality comparison (two `rev-parse --abbrev-ref HEAD`, one in the
  host worktree, one in the deterministic `<x>-weft` sibling) is **new code** in warp, not a
  move. `sync.go` (`Commit`/`Push`/`Pull` + locks), `spawn.go` (detached push), `cli.go`,
  `config.go` (`pathspec`) **stay in `weft`**.

**`internal/fslink/` (unchanged dependency):** `CreateDirLink(link, target string) error`
(Windows junction / non-Windows symlink, directory-only), `IsLink`, `PointsTo`, `Remove`
(idempotent), `RemoveLinksIn(dir)`. `CreateFileLink` is reserved/unimplemented — do not rely
on file links. Use these for the junction primitive.

**`internal/paths/` (unchanged; the geometry to use):** `Resolve(cwd) → *Layout`
(`ErrNotAGitRepo` on non-repo). Layout methods: `WorktreePath(slug)` (`<Hub>/<slug>`),
`WeftRepoRoot()` (`<Hub>/<PrimeName()>-weft`, the `git -C` target for weft worktree add/remove),
`WeftWorktreePath(slug)` (`<Hub>/<slug>-weft`), `WeftWorktree()` (weft paired with current
host WT), `WeftLyxDir()` / `WeftLyxDirFor(slug)` (junction targets), `HostLyxLink(slug)` /
`HostLyxLinkHere()` (host-side junction endpoints), `HostJunctions(slug) []HostJunction`
(currently one entry `{Name:"_lyx", Link, Target}` — the set the junction primitive iterates),
portal/launcher geometry, `PrimeName()`. The deterministic `<x>-weft` naming is exactly what
the **stateless drift check** relies on (no registry). `paths.List(sourceDir)` parses
`git worktree list --porcelain` → `[]WorktreeEntry{Path, Head, Branch, Main}`. Honor
CONSTRAINTS.md: never compute `_lyx`/config paths or cwd/root geometry from string literals —
always via `paths` helpers (`paths.LyxDirName`, `paths.ConfigDir`, `paths.ConfigFile`,
`paths.Getwd`, `paths.Resolve`).

**`internal/configreg/`:** neutral registry; modules expose `ConfigTemplate() string` and are
listed centrally in `Modules()`. Rename = swap the `worktree` entry for `warp` + rename the
template file. `warp` must **not** import `configreg` (like `board`/`weft`/`worktree` — no cycle).

**`cmd/lyx/main.go`:** replace `case "worktree": worktree.RunCLI(...)` and
`case "git-clone": gitclone.RunCLI(...)` with a single `case "warp": warp.RunCLI(...)`.
`weft` stays. Subcommands: `lyx warp clone | add | remove | checkout | reconcile | cleanup |
list | status | prune`.

**`internal/initcli/` (`lyx init`):** gains junction activation — wire the cwd-keyed
junction(s) + `.git/info/exclude` **atomically** via warp's junction primitive (junctions
first), **then** reconcile config. `init` calls warp's primitive; warp never calls `init`.

**`loom` does not exist** (no `internal/loom`, not in `main.go`). The "strengthen loom Setup"
item is therefore out of scope — warp only provides the stateless precondition primitive.

**post-checkout hook validation item (the "if it works" caveat):** git worktrees share the
**common `.git/hooks`** directory (hooks are not per-worktree). So the `post-checkout` hook is
installed once into the common hooks dir and, when it fires, must determine *which* worktree
it ran in (it executes with cwd = the checked-out worktree) and check that worktree's
deterministic `<x>-weft` sibling. Plan must validate this behaves correctly across multiple
worktrees of one repo, and decide install/update mechanics (install at `clone`/`add`, idempotent,
non-clobbering of any existing user hook — chain if present).

## Constraints

From `CONSTRAINTS.md` (build-enforced where noted):

- **Path invariant.** All cwd/worktree-root geometry resolves through `internal/paths`
  (`paths.Getwd()`, `paths.Resolve()`). Raw `os.Getwd` and `git rev-parse --show-toplevel`
  are **banned** outside `internal/paths` + `cmd/lyx/main.go` — enforced by
  `internal/paths/enforcement_test.go` scanning the whole tree. The new `warp` package must
  not use either primitive directly.
- **`_lyx` / config-file paths** must come from `paths.LyxDirName`, `paths.ConfigDir(base)`,
  `paths.ConfigFile(base, module)` — never string literals like `"_lyx"`/`"warp.yaml"` — in
  **production and test code** (the two exceptions are `internal/paths/*_test.go` and link-
  target geometry / string-content assertions). This is a code-review/planning rule, not
  caught by the enforcement test.
- **`internal/lyxtest` leaf invariant.** `lyxtest` may import only stdlib + `internal/paths`;
  must not import `configreg` or any feature package (`board`/`worktree`/`weft`/now `warp`).
  Enforced by `internal/lyxtest/leaf_enforcement_test.go` on every `go test ./...`. Tests that
  need real config seed it via `lyxtest.SeedConfig` using `warp.ConfigTemplate()` from the
  call site (not from inside `lyxtest`).
- **Documentation lifecycle.** `docs/modules/warp.md` is a mechanical per-module design doc —
  **delete it when warp lands** (the implementation + package header comment become the source
  of truth). Move durable rationale into the `warp` package doc comment.

Other:

- **Keep shelling to real `git`** via `gitexec`; do not adopt go-git/git2go (worktrees are
  core; full git compatibility matters; Go `exec` is cheap). If Windows process-spawn shows up
  in a profile, batch git calls — don't swap engines. Measure first.
- **Windows junction-lock hazard:** on teardown/rollback, remove the junction **before** the
  weft worktree (already done in `rollbackAdd`/`removeHostJunction`) — preserve this ordering.

## Testing

Strategy (Q10): **behaviour-preserving move + TDD for the new verbs.** Principle 5 — the
existing suite is the refactor guardrail.

- **Move + keep green:** relocate the existing `internal/worktree` tests (`add_test`,
  `cli_test`, `config_test`, `list_test`, `remove_test`, `launchers_test`, `portals_test`,
  `weft_test`, `prune_test`, `template_test`) and `internal/gitclone` tests
  (`clone_integration_test`, `gitclone_test`) into `internal/warp`, updating package names,
  config-module name (`worktree` → `warp`), and the `gitexec` import. These must stay green
  through the refactor — they prove the absorbed surface is behaviour-preserving.
- **`gitexec` rename:** existing `internal/git/git_test.go` moves to `internal/gitexec`
  unchanged in logic; update importing tests in `weft`.
- **TDD candidates (new code — write tests first):**
  - **Coordinated `warp checkout`** — happy path (host+weft switch + junction re-point);
    precondition refusal on dirty weft; **host rollback on weft-switch failure** (assert the
    pair is untouched after a forced failure); checkout onto an unmanaged branch forks the
    weft via the add fork-point path and leaves a managed pair.
  - **Stateless drift check** — host-branch == weft-branch comparison: in-sync true; desynced
    (raw checkout of the host) detected; junction-stale detected. Two `rev-parse` + junction
    stat, no registry.
  - **`warp reconcile`** — missing weft worktree (branch present) adopted; broken/dangling
    junction repaired; raw (non-lyx) host worktree adopted; unmanaged-branch worktree
    **reported, untouched**; walks worktrees only (never adopts arbitrary branches).
  - **`warp cleanup`** — dry-run/report default (no deletion without `--apply`); deletes weft
    branch with no host sibling on `--apply`; task branch **protected** by the conservative
    `codeguideFoldedBack` gate (deletable only with `--force`); board never touched.
  - **`warp prune`** — orphaned/stale pair removed; live pairs untouched.
  - **Host-pollution guard** — `warp status`/`reconcile` flag a `_lyx`/`_codeguide` path force-
    added (`git add -f`) into the host index and offer the `git rm --cached` remedy; junction
    primitive writes junction + exclude atomically (assert both present or neither).
  - **Junction relocation** — `warp add`/`clone` produce **no** junctions (dormant); `lyx init`
    wires the cwd-keyed junction + exclude (junctions before config reconcile); cwd-keyed
    (subfolder, not forced to worktree root).
  - **`add` adopt-or-create** — adopts an existing weft branch instead of aborting.
  - **post-checkout hook** — integration test: install at `clone`/`add` (idempotent,
    non-clobbering / chains an existing hook); after a raw `git checkout` it detects host/weft
    drift and emits the warning; validate across two worktrees sharing the common `.git/hooks`.
- **Integration-tagged tests** (real `git`, like `clone_integration_test`) cover the
  coordinated checkout/rollback, reconcile-adopt, cleanup, and hook paths. Unit tests cover
  derivation/precondition/gate logic. Honor the lyxtest leaf invariant and the path-helper
  rules in test code.

## Q&A log

- **Q:** Task scope — full module or a slice? **A:** Full warp module, excluding only what's
  blocked by unbuilt deps (loom Setup wiring, outer command, codeguide merge-back). (Q1=1)
- **Q:** `warp cleanup` with codeguide merge-back unbuilt — defer, gate-stub, or no-gate?
  **A:** Build cleanup *with* the `_codeguide` gate wired in (conservative protection until
  codeguide lands); "just build it so it has support for it." (Q2=1)
- **Q:** Reconcile on an unmanaged host branch — auto-adopt or report? **A:** Report (safer
  default). (Q5=1) Contrast: `checkout` onto an unmanaged branch *does* fork (Q8).
- **Q:** Include the optional `post-checkout` git hook? **A:** Yes, include it — earliest
  proactive drift warning. Validate the shared-`.git/hooks` mechanics ("sounds good if it
  works"). (Q3)
- **Q:** Junction-wiring — relocate into `lyx init` or keep in `warp add`? **A:** Relocate:
  `warp add` dormant, `lyx init` activates (cwd-keyed, atomic junction+exclude, then config).
  (Q6=1)
- **Q:** Config migration `worktree.yaml` → `warp.yaml`? **A:** None needed — no existing hubs;
  registry + template rename only. (Q7)
- **Q:** `warp checkout` onto a host branch with no weft sibling? **A:** Create the weft branch
  via the same fork-point path as `add` ("as if the branch was just created via add"), then
  switch both. (Q8=1)
- **Q:** `warp add` when the weft branch already exists? **A:** Adopt it (build the weft
  worktree from it), not abort. (Q9=1)
- **Q:** Test strategy? **A:** Behaviour-preserving move of existing tests + TDD for the new
  verbs; integration-tagged tests for coordinated/rollback/hook paths. (Q10=1)
- **Q:** Launcher-menu `lyx warp checkout` shortcut? **A:** Include it (cheap; realizes the
  principle-6 friction asymmetry). (Q4=1)
- **Q:** `gitexec` rename, single `internal/warp` package, outer command, `lyx doctor`?
  **A:** Rename `internal/git`→`gitexec` now; one `internal/warp` package split by verb;
  outer command + loom Setup wiring + `doctor` out of scope (unbuilt deps). (baked assumptions,
  unobjected)
- **Q:** [review r1 GAP] `gitexec` rename importer sweep — does it cover all importers?
  **A:** Sweep **all** importers; surviving production importers `internal/paths` and
  `internal/board` must be updated too (not just `weft`/`warp`/tests), else the build breaks.
  (round-1 resolution, option 1)
- **Q:** [review r1 NOTE] `lyx warp checkout` launcher shortcut integration point? **A:** warp's
  absorbed launcher generation emits a per-worktree `warp-checkout.cmd` calling `lyx warp
  checkout`; no `internal/ide` change (the `ide menu` is a worktree picker, not an action menu).
- **Q:** [review r1 NOTE] post-checkout hook script source / Windows execution? **A:** Embedded
  POSIX `sh` script (`go:embed`) installed into the common `.git/hooks/post-checkout`; runs
  under git-for-Windows' bundled bash; idempotent, non-clobbering (chain an existing hook).
  Body/chaining details deferred to the plan.
- **Q:** [review r1 NOTE] host-pollution guard on `_codeguide` with no warp-wired junction?
  **A:** `_codeguide` detection is **report-only** this task (no junction to restore); the `git
  rm --cached` + restore remedy applies to `_lyx` only.
```
