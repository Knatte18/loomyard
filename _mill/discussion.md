# Discussion: weft engine: paths geometry, paired worktrees, lyx weft

```yaml
task: 'weft engine: paths geometry, paired worktrees, lyx weft'
slug: weft-engine
status: discussing
parent: main
```

## Problem

lyx's overlay artifacts (`_lyx/` config + task-state, `_codeguide/` docs, the board)
are today assumed to live committed inside the host repo. That pollutes repos we don't
own and was a recurring source of trouble in the predecessor (millpy). The **weft repo**
removes that assumption: a separate companion git repo, living in the same Hub, that
mirrors the host's branch and folder structure so the host stays pristine. The full
design is in the wiki proposal `proposal-weft-repo.md` (the shared design for three
backlog tasks: **weft-engine**, **loom-git-clone**, **weft-producers**).

This task, **weft-engine**, is the Go core — proposal sections **§1–3**:

1. `internal/paths` gains weft geometry (host→weft path math).
2. `internal/worktree` paired spawn + teardown (host worktree ↔ `<slug>-weft` weft worktree).
3. a new `lyx weft` command (`internal/weft`) that owns all git into the weft repo.

**Why now:** it is the foundation task — `loom-git-clone` (the hub-creator skill) and
`weft-producers` (`lyx config`, codeguide hook) both build on the geometry + `lyx weft`
this task lands. The weft overlay model is already the documented target architecture in
[`docs/overview.md`](../docs/overview.md) §"Weft overlay model" (which tags this task as
"task 006"); this task is its first Go realization.

## Scope

**In:**

- **§1 `internal/paths`** — add `Layout` methods for weft geometry: `WeftRepoRoot()`,
  `WeftWorktree()`, `WeftWorktreePath(slug)`, `WeftLyxDir()`, `WeftCodeguideDir()`. Pure
  geometry math from the existing `Hub` / `WorktreeRoot` / `Prime` / `RelPath` fields,
  using the `-weft` sibling-suffix convention. Plus white-box tests. Update the geometry
  method list in `CONSTRAINTS.md` and `docs/overview.md`.
- **§2 `internal/worktree`** — make `lyx worktree add` create a **pair**: the existing
  host worktree *plus* a weft worktree `<slug>-weft` on the mirrored branch, seed the
  `_lyx` junction (host → weft) + the host worktree's `.git/info/exclude` entry. Teardown
  (`lyx worktree remove`) removes **both** worktrees and **both** branches. Existing portal
  + launcher creation is kept (additive). Hard-requires a pre-existing weft repo.
- **§3 `internal/weft` + `lyx weft`** — new module + new `main.go` dispatch case.
  Subcommands `status | commit | push | pull | sync`, all geometry-derived, committing the
  configured pathspec (`_lyx`) via `git -C <hub>/<slug>-weft`. Detached, coalesced push
  modeled on the board pusher. New `_lyx/config/weft.yaml` (`pathspec` knob).
- **ide module touch (small)** — add the `files.watcherExclude` key for `**/_lyx/**` to the
  ide module's `writeVSCodeConfig` default settings block (the #498 junction-lock fix), so
  all `.vscode/settings.json` writes stay owned by the ide module.

**Out:**

- **`_codeguide` junction seeding + pathspec** — deferred to task 008 (`weft-producers`).
  This task adds the `WeftCodeguideDir()` *geometry method only* (§1 lists it); it does **not**
  seed a `_codeguide` junction, does **not** add `_codeguide` to the weft pathspec, and does
  **not** add a `**/_codeguide/**` watcherExclude. Task 008 flips codeguide on by changing
  `weft.yaml`'s `pathspec` and adding the junction + exclude.
- **The hub-creator** (`/loom-git-clone`, §6–7) — creating the weft repo, the weft Prime
  worktree, mirrored branches, board-wiki clone. weft-engine *consumes* a pre-existing weft
  repo; it never creates one.
- **The `_lyx/config/` bulk move + `lyx config` TUI** (§4) — task 008. (Note: the config
  loader *already* reads `_lyx/config/<module>.yaml`, so adding `weft.yaml` there is
  consistent with the current code, not a pull-forward of §4.)
- **Removing / deprecating portals** (§8) — portals stay "on hold, not deprecated"; paired
  spawn keeps creating them unchanged.
- **Board placement options** (§6) — config/hub-creator concern, not this task.
- **`internal/state`** — stays deferred; by-name pairing means weft does not need it.

## Decisions

### spawn-hard-requires-weft-repo

- Decision: `lyx worktree add` **errors** if the paired weft repo is absent (i.e.
  `WeftRepoRoot()` = `<hub>/<prime>-weft` is not a git repo). No host-only degrade, no
  auto-init. The check runs **early** (with the other prechecks, before any worktree is
  created), so no partial state is left behind.
- Rationale: weft is the decided target architecture; a degrade path would silently produce
  non-weft worktrees and hide the missing-hub-creator setup. Hard-require makes the missing
  precondition loud and detectable (principle 6).
- Consequence: `lyx worktree add` will **error in every current hub** until the hub-creator
  task lands and builds a weft repo. This is accepted. The existing `add_test.go` fixtures
  must be extended to build a paired weft repo (see Testing) or they will fail.
- Rejected: graceful host-only degrade (hides setup gaps); weft-engine auto-initializing a
  local weft repo (overlaps/diverges from the hub-creator's remote + mirrored-branch setup).

### detached-coalesced-push

- Decision: `lyx weft` uses the **board pusher model** — a detached, push-lock-coalesced
  background pusher. `lyx weft sync` commits the pathspec locally, then spawns a detached
  worker that pushes and returns immediately. Port board's `sync.go` (push-lock + commit-
  dirty + push-unpushed loop) and `spawn_windows.go` / `spawn_other.go` into `internal/weft`.
- Rationale: matches the existing board prototype of the git-ownership contract; bursts of
  weft writes (multiple skill lifecycle points) coalesce into few pushes; the caller never
  blocks on the network.
- Rejected: synchronous inline push (simpler, but the user chose to mirror board's proven
  model rather than introduce a second push style).

### lyx-weft-surface

- Decision: ship the full surface `lyx weft <status|commit|push|pull|sync>`:
  - `commit` — `git -C <weft> add -- <pathspec>` then commit locally (no push). No-op /
    idempotent when nothing staged (board's `diff --cached --quiet` pattern).
  - `push` — run the push-coalescing loop **synchronously** (acquire push-lock, commit any
    dirty pathspec, push unpushed with rebase-retry, loop until clean). This same entry is
    what the detached worker invokes; calling it directly forces a synchronous backup.
  - `pull` — `git pull --ff-only` in the weft worktree.
  - `sync` — commit the pathspec locally, then **spawn `lyx weft push` detached** and return
    immediately (the lifecycle call skills invoke at skill-end / after config edits).
  - `status` — drift report (see weft-status-semantics).
- Rationale: §3 lists all five; full surface lets producers and the hub-creator script the
  exact verb they need.
- Rejected: minimal `sync`+`status` only (diverges from §3's listed set).

### weft-branch-mirrors-host

- Decision: the weft worktree's branch **has the same name as the host branch**
  (`cfg.BranchPrefix + slug`; host `feature-x` ↔ weft `feature-x`; host `main` ↔ weft `main`).
  Created from the weft Prime's current branch tip via
  `git -C <WeftRepoRoot> worktree add -b <branch> <WeftWorktreePath(slug)>`.
- Rationale: host and weft are **separate git repos / separate remotes**, so identical
  branch names cannot collide. The `-weft` directory suffix already distinguishes host from
  weft on disk; duplicating it in the branch name is redundant. `lyx weft` never has to
  compute the weft branch at commit time — the weft worktree is already checked out on it.
  Matches the proposal's "mirrors the host's branch structure" model.
- Rejected: distinct `<branch>-weft` branch names (adds a host→weft name transform
  everywhere; redundant with the dir suffix).

### weft-initial-push-at-spawn

- Decision: paired spawn pushes the weft branch `-u origin <branch>` **synchronously at
  creation**, mirroring the existing host step-9 `git push -u origin <branch>`. So the weft
  branch's upstream is set before the detached pusher ever runs.
- Rationale: symmetric with the host push already in `Add`; avoids the detached pusher
  having to special-case a no-upstream first push.
- Rejected: leaving the first push to `lyx weft sync` (pushes more complexity into the
  pusher's no-upstream path).

### codeguide-geometry-only

- Decision: add the `WeftCodeguideDir()` geometry method now; **do not** seed a `_codeguide`
  junction, **do not** add `_codeguide` to the weft pathspec, **do not** add a
  `**/_codeguide/**` watcherExclude. All of that is task 008.
- Rationale: honors the overview's task-006/008 boundary; the geometry method is cheap and
  §1 explicitly lists it, while junction activation is producer-side work.
- Rejected: seeding both junctions now (pulls task-008 scope forward).

### keep-portals-additive

- Decision: paired spawn keeps creating the portal junction and launchers exactly as today;
  weft junctions are **additive**. (The portal target `<hub>/<slug>/<RelPath>/_lyx` now
  resolves *through* the new host `_lyx` junction into the weft worktree — still valid.)
- Rationale: §8 keeps portals "on hold, not deprecated".
- Rejected: replacing portals with weft junctions (contradicts §8).

### exclude-ownership-split

- Decision: split the two excludes by module ownership:
  - **`.git/info/exclude`** (git-side) — seeded by `worktree add` (the worktree module's
    domain). Append `_lyx` to the new host worktree's exclude, located via
    `git -C <newhost> rev-parse --git-path info/exclude`. Idempotent (skip if already
    present).
  - **`files.watcherExclude`** (VS Code settings) — owned entirely by the **ide module**.
    Add `**/_lyx/**` to the `writeVSCodeConfig` default settings block. `worktree add` does
    **not** touch `.vscode/settings.json`.
- Rationale: the user's rule — "writing to anything that is VS Code's settings definitely
  belongs to ide." Timing is correct: `lyx ide` writes settings *then* launches VS Code, so
  the watcherExclude is present before the file watcher can lock the junction (#498).
- Rejected: `worktree add` writing settings.json directly (crosses the ide boundary and
  risks ide's skip-if-absent dropping the color/title block).

### weft-config-pathspec-only

- Decision: `_lyx/config/weft.yaml` holds a single scalar knob `pathspec` (default `"_lyx"`,
  a space-separated list of overlay dirs the weft stages/commits). Task 008 flips codeguide
  on by setting it to `"_lyx _codeguide"` — no code change.
- Rationale: the config loader returns a flat `map[string]string` (scalars only), so a
  space-separated string is the natural shape; `pathspec` is the one knob that genuinely
  varies and cleanly carries the 008 hand-off.
- Rejected: adding `commit_prefix` (fixed message is fine) or a `push` toggle (overlaps the
  `WEFT_SKIP_PUSH` test guard). The `-weft` suffix stays a non-configurable constant.

### weft-status-semantics

- Decision: `lyx weft status` reports, as JSON: the weft worktree path, its checked-out
  branch, working-tree dirtiness of the pathspec (`git status --porcelain -- <pathspec>`),
  ahead/behind vs upstream (`rev-list --count @{u}..HEAD` and reverse), and **junction
  integrity** — whether the host `_lyx` is a junction whose target is the weft worktree's
  `_lyx`. This is the principle-6 "drift detectable" surface.
- Rationale: a future `lyx doctor` builds on this; status is how an operator/skill confirms
  the overlay is wired and synced.
- Rejected: status reporting only dirty/clean (misses junction drift, the failure mode
  unique to this model).

### weft-test-guards

- Decision: add `WEFT_SKIP_GIT` / `WEFT_SKIP_PUSH` env guards, mirroring board's
  `BOARD_SKIP_GIT` / `BOARD_SKIP_PUSH`. `WEFT_SKIP_GIT=1` disables the weft git/sync path
  entirely; `WEFT_SKIP_PUSH=1` commits locally but skips push and the detached spawn.
- Rationale: lets unit tests exercise the file/junction/commit logic offline; integration
  tests wire a local bare remote for real push/pull. Consistent with the board precedent.
- Rejected: no guards / real bare remotes everywhere (slower; can't isolate the file layer).

### teardown-dirty-gate-both

- Decision: `lyx worktree remove` (without `--force`) requires **both** the host **and** the
  weft worktree to be clean; reject otherwise, directing the operator to run `lyx weft sync`
  first. `--force` removes both regardless. Order: remove host junctions (existing
  `removeLinks` already strips the host-root `_lyx` junction) → `git worktree remove` host →
  `git -C <WeftRepoRoot> worktree remove [--force]` weft → `git branch -D <branch>` in both
  → prune both. Junctions come off before any worktree removal (Windows lock hazard).
- Rationale: symmetric with the existing host clean-or-`--force` contract; prevents silent
  loss of uncommitted weft task-state. `lyx weft sync` is the documented lifecycle escape.
- Rejected: always force-removing the weft (risks losing uncommitted `_lyx` state);
  auto-running a final sync inside Remove (couples teardown to the pusher).

## Technical context

mill-plan needs the following codebase facts.

**Geometry (`internal/paths/paths.go`).** `Resolve(cwd)` builds a `Layout{Cwd, WorktreeRoot,
Hub, RelPath, Prime}`. `Hub = filepath.Dir(WorktreeRoot)`; `Prime` is the `Main==true`
entry from `List()`; `PrimeName() = filepath.Base(Prime)`. Existing `WorktreePath(slug) =
Join(Hub, slug)` and `PortalTarget(slug) = Join(Hub, slug, RelPath, "_lyx")` are the
patterns the new weft methods parallel. New methods:
- `WeftRepoRoot() = Join(Hub, PrimeName()+"-weft")` — the weft Prime worktree; the
  `git -C` target for `worktree add/remove` on the weft repo.
- `WeftWorktreePath(slug) = Join(Hub, slug+"-weft")` — parallel to `WorktreePath(slug)`;
  used by spawn/teardown for a named slug.
- `WeftWorktree() = Join(Hub, filepath.Base(WorktreeRoot)+"-weft")` — the weft worktree
  paired with the *current* host worktree; used by `lyx weft`.
- `WeftLyxDir() = Join(WeftWorktree(), RelPath, "_lyx")` — junction target / pathspec base,
  RelPath-mirrored like `PortalTarget` (collapses to `<weft>/_lyx` at RelPath ".").
- `WeftCodeguideDir() = Join(WeftWorktree(), RelPath, "_codeguide")` — geometry only.

**Path invariant (`CONSTRAINTS.md`, `enforcement_test.go`).** Raw `os.Getwd` and
`git rev-parse --show-toplevel` are banned outside `internal/paths` and `cmd/lyx/main.go`,
enforced by a source-tree scan in `internal/paths/enforcement_test.go`. The new geometry
methods belong in `internal/paths`. Note: `git rev-parse --git-path info/exclude` is a
*different* token and is **not** banned — `worktree add` may call it via `git.RunGit`. Add
the new method names to the geometry lists in `CONSTRAINTS.md` and `docs/overview.md`.

**git plumbing (`internal/git/git.go`).** `RunGit(args, cwd) -> (stdout, stderr, exitCode,
err)`. Non-zero exit is *not* a Go error (err==nil, exitCode set). All weft/worktree git
goes through this with an explicit `cwd` — never a process `cd`.

**Worktree module (`internal/worktree/`).** `Add(l, slug)` (add.go) runs prechecks
(clean / branch-exists / target-exists / remote), `git worktree add -b <branch> <target>`,
`createPortal`, `writeLaunchers`, then `git push -u origin <branch>` last, with
`rollbackAdd` undoing everything on any post-create failure. `Remove(l, slug, force)`
(remove.go) does early portal/launcher teardown, dirty-gate, `removeLinks(target)` (strips
symlinks/junctions in the worktree root — this already removes the host `_lyx` junction),
then `git worktree remove`. `createJunction(link, target)` (junction_windows.go via
`mklink /J`; junction_other.go for non-Windows) refuses to clobber an existing link.
`Config.BranchPrefix` (config.go) is the host/weft branch prefix (default ""). The module
is **stateless** (worktree.go) — pairing is by-name from `git worktree list`, no registry.

**Board prototype to port (`internal/board/`).** `sync.go` is the reference detached pusher:
`Sync(path)` acquires `pushLockFile`, loops `commitDirty` (under `writeLockFile`) +
`pushUnpushed` (rebase-retry on non-fast-forward / rejected / fetch-first) until clean;
`hasUnpushed` via `rev-list --count @{u}..HEAD` (returns true when no upstream);
`ensureLockfilesIgnored` appends `*.lock` to a committed `.gitignore` so lock files are
never staged. `board.go writeOp` shows the spawn-detached-on-write pattern; `spawn_windows.go`
/ `spawn_other.go` are the detached-process launchers. `internal/lock` provides
`AcquireWriteLock(path)`. Mirror these in `internal/weft`, parameterized by the weft worktree
path and the `WEFT_SKIP_*` env vars. **Lock files** live under the weft worktree (e.g.
`<weft>/_lyx/*.lock`), gitignored via a weft analog of `ensureLockfilesIgnored`.

**Config (`internal/config/config.go`).** `Load(baseDir, module, defaults) ->
map[string]string` reads `<baseDir>/_lyx/config/<module>.yaml` (already the config-subfolder
path) merged over `defaults`, scalar values only. weft's `LoadConfig` mirrors
`worktree/config.go`: defaults `{"pathspec": "_lyx"}`. Pathspec is split on whitespace into
dirs; each dir is joined with `RelPath` for the geometry-scoped pathspec (never `git add .`).

**ide module (`internal/ide/vscode.go`).** `writeVSCodeConfig(worktreeDir, relpath, slug,
color)` writes `.vscode/settings.json` (only if absent) and registers `.vscode/` in the
managed `.gitignore` via `gitignore.Ensure`. Add a `"files.watcherExclude": {"**/_lyx/**":
true}` entry to its default settings map. `.vscode/settings.json` is gitignored (per-worktree,
host-side), so the exclude never reaches the host repo.

**Dispatch (`cmd/lyx/main.go`).** `run()` switches on the module arg; add
`case "weft": return weft.RunCLI(out, moduleArgs)`. All output is JSON: `{"ok":true,...}` /
`{"ok":false,"error":"..."}` via `internal/output`.

**Hub reality.** No weft repo exists in any real hub yet (the `wts/weft-repo` dir here is
just the docs-task branch, not a weft repo). All weft-engine testing is against synthetic
temp-dir git repos.

## Constraints

From `CONSTRAINTS.md`:
- All worktree/Hub geometry resolves through `internal/paths` (`Getwd`, `Resolve`, `Layout`
  methods). Raw `os.Getwd` / `git rev-parse --show-toplevel` banned outside `internal/paths`
  and `cmd/lyx/main.go`; enforced by `internal/paths/enforcement_test.go` scanning the tree.
  → The weft geometry methods MUST live in `internal/paths`; `internal/weft` and
  `internal/worktree` derive paths only through the `Layout`.
- Documentation lifecycle: durable design lives in `docs/overview.md` + package header
  comments; mechanical per-module docs are deleted when the module lands. → Document the weft
  module's purpose/rationale in the `internal/weft` package header; update `overview.md`'s
  weft section + geometry method list + module list; no new `docs/modules/weft.md`.

From the project principles (`docs/overview.md`):
- One-shot, daemonless, file-coordinated; processes cooperate via files + locks. → weft's
  detached pusher uses file locks exactly like board; no daemon.
- cwd-authoritative, cwd ≠ git-repo-path. → `lyx weft` resolves from cwd; the pathspec is
  RelPath-scoped so a sync from a subpath commits that subpath's `_lyx`.
- Correctness by tool-design: make the right path easiest and drift detectable. → `lyx weft`
  owns weft git so raw `git -C` is never needed; `lyx weft status` surfaces drift.

## Testing

White-box (`package x`) unit tests next to source; cross-cutting/integration in a black-box
test package where it already exists (board's `boardtest` precedent). Use the existing
`newTestRepo` / `addRemote` temp-dir fixture pattern.

- **`internal/paths` (§1)** — TDD candidate. Table-driven tests for each new method at
  RelPath "." and at a subpath, asserting the `-weft` sibling path and RelPath mirroring.
  Pure functions, no git needed. Confirm `enforcement_test.go` still passes (no banned
  tokens introduced).
- **`internal/worktree` paired spawn (§2)** — extend `testhelpers_test.go`: a new fixture
  that, in the temp Hub, also creates the weft Prime worktree `<hub>/<prime>-weft` (an init'd
  git repo with a committed `_lyx/` tree and a bare weft remote). Scenarios: paired add
  creates both worktrees on the mirrored branch; the `_lyx` junction exists host→weft;
  `.git/info/exclude` contains `_lyx` (idempotent on re-seed); portal + launchers still
  created; **hard-require** — add errors (no partial state) when `<prime>-weft` is absent;
  rollback removes both worktrees + both branches + junction on a forced post-create failure.
  Teardown: remove strips the host junction first, removes both worktrees + branches; the
  dirty-gate rejects when either side is dirty and `--force` overrides. **The existing
  `add_test.go` cases must be updated to build a weft repo** (they currently assume host-only
  add and will otherwise hit the hard-require error). Use `WEFT_SKIP_PUSH` to avoid network.
- **`internal/weft` (§3)** — TDD candidate. Offline (`WEFT_SKIP_PUSH`) tests for `commit`
  (stages only the pathspec; idempotent no-op when clean), `status` (dirty/clean,
  ahead/behind, junction integrity true/false when junction is broken), config `pathspec`
  resolution + RelPath scoping, and the `git add .` guard (a stray file outside the pathspec
  is never staged). Integration tests with a local bare remote: `push` rebase-retry on a
  non-fast-forward, `pull --ff-only`, and `sync` → detached worker → commit lands on the
  remote (assert by polling the bare repo, as board's git tests do). Lock-file gitignore:
  the weft lock files are never staged.
- **`cmd/lyx` dispatch** — `run(["weft", ...])` routes to `weft.RunCLI`; unknown weft
  subcommand returns the JSON error + exit 1 (mirror `main_test.go`).

## Q&A log

- **Q:** What does `lyx worktree add` do when no paired weft repo exists? **A:** Hard-require
  — error early (no host-only degrade, no auto-init). Accept that `worktree add` errors in
  current hubs until the hub-creator lands, and that `add_test.go` fixtures must build a weft repo.
- **Q:** How does `lyx weft` push to the remote? **A:** Detached coalesced pusher — port
  board's `sync.go` push-lock loop + detached spawn into `internal/weft`.
- **Q:** Include `_codeguide` now? **A:** Geometry method `WeftCodeguideDir()` only; defer the
  `_codeguide` junction + pathspec + watcherExclude to task 008.
- **Q:** Keep portals / how to seed the excludes? **A:** Keep portals + launchers (additive);
  seed `.git/info/exclude` in `worktree add`; seed `files.watcherExclude` via the ide module.
- **Q:** `lyx weft` subcommand surface? **A:** Full set — `status | commit | push | pull | sync`.
- **Q:** Does the weft module need config? **A:** Yes — `_lyx/config/weft.yaml` with a single
  scalar `pathspec` knob (default `"_lyx"`); task 008 flips codeguide on via this value.
- **Q:** Offline test guards? **A:** `WEFT_SKIP_GIT` / `WEFT_SKIP_PUSH`, mirroring board.
- **Q:** Weft branch naming — same as host or distinct? **A:** Mirror (same name). Separate
  repos → no collision; the `-weft` dir suffix already distinguishes; trivial derivation.
- **Q:** Where does `files.watcherExclude` seeding live, given ide owns settings.json with
  skip-if-absent semantics? **A:** In the ide module (anything that is VS Code's settings
  belongs to ide); `worktree add` never writes `.vscode/`. Timing is correct because `lyx ide`
  writes settings before launching VS Code.
