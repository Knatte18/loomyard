# Discussion: Extend worktree module: portals and launchers

```yaml
task: 'Extend worktree module: portals and launchers'
slug: mhgo-portals-launchers
status: discussing
parent: main
```

## Problem

`mhgo` (the Go port of mill) manages git worktrees flat inside a **container**
directory (the parent of the hub). Today `mhgo worktree add/remove` only handle
the git worktree itself. Two machine-local conveniences are missing:

1. **Portals** — when several worktrees are active, there is no single place from
   which the hub's VS Code instance can browse each worktree's live task state.
2. **Launchers** — opening a worktree in an IDE still means navigating to it by
   hand. There is no one-click way to open VS Code on a given worktree, and no
   menu to pick from all active worktrees.

This task extends `mhgo worktree add/remove` to also create/tear down two
machine-local, **non-git** directories in the container — `_portals/` and
`_launchers/` — and adds a new `mhgo ide` module that the launchers delegate to.
The container is **not** a git repository and must never contain an `_mhgo/`
directory; all of this is machine-local scaffolding outside any git tree.

**Why now:** the worktree module shipped (roadmap milestone 4) with only the git
lifecycle. The parallel-work ergonomics (observe many worktrees at once, open any
of them fast) are the immediate next gap, and they unblock the operator's daily
flow. A Go-powered picker also replaces mill's slow Python `millpy-vscode` chooser.

## Scope

**In:**

- **`worktree add`** additionally:
  - Creates the portal junction `<container>/_portals/<slug>` → the worktree's
    `_mhgo/` directory (the dir that holds mhgo config/state, committed in the
    repo). Junction on Windows, symlink on POSIX.
  - Creates the per-worktree launcher `<container>/_launchers/<slug>/ide.cmd`.
  - Ensures the container-root menu launcher `<container>/_launchers/ide-menu.cmd`
    exists (created if missing; static content).
  - Becomes **transactional**: `git push` moves to the **last** step; on any
    failure after the worktree is created, performs a **full rollback** (remove
    portal, launcher dir, `git worktree remove --force`, `git branch -D`) so no
    stale/partial worktree is ever left behind.
- **`worktree remove`** additionally tears down `<container>/_portals/<slug>` and
  `<container>/_launchers/<slug>/` (best-effort). This teardown is sequenced
  **before** (or independently of) the existing target-exists check in
  `remove.go:40-42`, so it still runs when the worktree dir is already gone — the
  early "not found" return must not skip portal/launcher cleanup. The
  container-root `ide-menu.cmd` is left in place.
- **cwd ≠ worktree-root fix (repo-wide, permanent):** stop assuming the invocation
  cwd is the worktree root anywhere in the repo. Resolve the worktree root via
  `git.FindRoot(cwd)`. Full audit result (see Decisions → cwd-not-worktree-root):
  - `internal/worktree/add.go` + `remove.go` — `container := filepath.Dir(sourceDir)`
    assumes cwd == git root. **Fix.**
  - `internal/muxpoc` — `LoadState(cwd)`/`SaveState(cwd)` anchor state at
    `cwd/.mhgo/`, and `socketName(filepath.Base(cwd))` derives the psmux session
    from the cwd basename, so running from a subfolder silently splits
    session/state. **Fix** (anchor both on `git.FindRoot(cwd)`), even though
    muxpoc is a POC — the operator wants this class of bug gone everywhere.
  - `internal/board` cwd usage (`cli.go` config resolution, `init.go` scaffolding
    `_mhgo/` at cwd) is **correct by design** — the cwd-authoritative model, not a
    bug; left unchanged.
- **New `board` health-check** — a fast `board` API (e.g. `Board.HealthCheck()
  error`) that verifies the board dir exists and `tasks.json` is readable. The
  board module is the sole authority on board validity; `ide` calls it and never
  stats the board dir itself.
- **New module `mhgo ide`** (`internal/ide`):
  - `mhgo ide spawn <slug>` — generate the worktree's `.vscode/` config (only if
    missing), assign a title-bar color, and launch VS Code on the worktree.
  - `mhgo ide menu` — interactive picker over all active worktrees (slug + title),
    opening the chosen one via the same spawn path; hard-errors via the board
    health-check if the board is absent/unreadable.
- **New shared lib `internal/gitignore`** — a single managed `.gitignore` block
  ("portal into `.gitignore`") that multiple modules contribute entries to.
  Refactor `board/init.go`'s block logic into it; `init` registers `.mhgo/`,
  `ide` registers `.vscode/`.
- **Docs:** update `docs/modules/worktree.md` (container layout diagram + Portals
  + Launchers sections + "container is not a git repo" note); add
  `docs/modules/ide.md`; add a `docs/shared-libs/` entry for `internal/gitignore`;
  update `docs/overview.md` and `docs/roadmap.md` module lists.

**Out:**

- **`mhgo shell` module — PARKED / DEFERRED.** No `shell.cmd` launcher is
  generated. Documented as deferred (see Decisions → shell-parked). The desire
  it served (a fast terminal in another worktree) is superseded by VS Code
  auto-starting Claude in the worktree it already lives in.
- **Auto-open the IDE on `worktree add`** — deferred (per the folded brief).
  `worktree add` writes launchers but does not launch anything.
- **"Reopen all worktrees" command** — deferred.
- **Filtering out worktrees that already have a VS Code window open** (mill's
  `--filter-open`) — not in this task.
- **Claude Code title color** — only the VS Code title-bar color is set now; the
  matching Claude color is future work.
- **A per-worktree mhgo status file.** The portal points at `_mhgo/` regardless;
  the "browse live task status" payoff grows when/if mhgo gains a per-worktree
  status artifact. Not built here.
- **POSIX launchers/menu/spawn.** Windows-first (see Decisions → cross-platform).

## Decisions

### module-structure

- Decision: `ide` is a new top-level module (`internal/ide`, `mhgo ide ...`),
  mirroring `board`/`worktree`/`muxpoc`. `main.go` gains one `case "ide"`.
- Rationale: matches the brief's `mhgo ide spawn` / `mhgo ide menu` naming and the
  one-module-per-namespace convention in `docs/overview.md`.
- Rejected: a `mhgo launch ide` subcommand (diverges from naming); folding into
  `worktree` (bloats it, breaks the `mhgo ide` namespace).

### portal-target-is-_mhgo

- Decision: the portal junction `<container>/_portals/<slug>` targets the
  worktree's `_mhgo/` directory at the captured `relpath`
  (`<container>/<slug>/<relpath>/_mhgo`). Never `_mill`.
- Rationale: `_mill/` is millhouse's; mhgo uses `_mhgo/`. `_mhgo/` starts with `_`
  (not `.`), so it is **committed** into the repo — therefore a fresh worktree
  checkout has it at the same `relpath`, and the junction target exists.
- Rejected: hardcoding `_mill` (wrong tool); a config flag for the dir name
  (YAGNI — the name is fixed to `_mhgo`).

### cwd-not-worktree-root

- Decision: fix the cwd-≠-worktree-root assumption permanently and **repo-wide**,
  including the throwaway `muxpoc` POC. Resolve the worktree root with
  `internal/git.FindRoot(cwd)` and use it as the anchor wherever code currently
  treats cwd as the worktree root.
  - **worktree:** in `add.go`/`remove.go`, compute
    `gitroot = git.FindRoot(cwd)`, `container = filepath.Dir(gitroot)`,
    `relpath = rel(gitroot, cwd)`. **Config resolution stays cwd-based** — the call
    site (`cli.go`) keeps passing `cwd` to `LoadConfig`, and `FindBaseDir` still
    strictly requires `_mhgo/` at cwd (no parent walk). Only `container`/`relpath`
    move to git-root derivation; the cwd-authoritative config invariant is
    untouched. (Reconciles review GAP: the fix is at the call-site/derivation
    level, not a change to config resolution.)
  - **muxpoc:** anchor both the session identity and the state directory on the
    worktree root, not the cwd. `socketName` derives from
    `filepath.Base(git.FindRoot(cwd))` (stable per worktree, shared across
    subfolders); `LoadState`/`SaveState` and their callers (`up`, `down`,
    `status`, `attach`, `review`, `daemon`, `cmd.socketArg`) resolve the worktree
    root first and anchor `.mhgo/` on it. Update `state_test.go`'s `socketName`
    expectations.
  - **board:** unchanged — `cli.go` config-from-cwd and `init.go` scaffolding
    `_mhgo/` at cwd are the intended cwd-authoritative behavior, not bugs.
- Path-form note (review NOTE): `git.FindRoot` returns `rev-parse --show-toplevel`
  output (forward slashes, possibly symlink-resolved on Windows) while
  `os.Getwd()` returns backslashes. Normalize the git-root via `filepath.Clean`
  (and `filepath.FromSlash`) before computing `relpath` or building junction /
  launcher (`mklink`, `%~dp0`) paths, so the forms match.
- Rationale: the operator has repeatedly hit "cwd assumed to be repo root" bugs
  and wants the entire class eliminated. `docs/overview.md` principle 4 mandates
  cwd-authoritative, cwd ≠ repo. `internal/git.FindRoot` already exists — reuse it.
- Rejected: patching only `worktree` (leaves the same bug live in muxpoc);
  documenting muxpoc as a known limitation without fixing (operator explicitly
  wants it fixed everywhere); changing config resolution to walk up to the git
  root (would break the cwd-authoritative `_mhgo/`-at-cwd invariant).

### transactional-add-rollback

- Decision: `worktree add` becomes all-or-nothing. Reorder so `git push -u origin`
  is the **last** step; create the worktree, then portal, then launchers, then
  push. On any failure at or after worktree creation, roll back fully: remove the
  portal junction, remove `_launchers/<slug>/`, `git worktree remove --force
  <target>`, `git branch -D <branch>`, and `git worktree prune`. Leave zero
  residue.
- Rollback self-failure: rollback is **best-effort and continues through all
  steps** even if one fails (e.g. `git worktree remove --force` errors → still
  attempt `os.RemoveAll`, `branch -D`, `worktree prune`, portal/launcher cleanup).
  The **original add error is what surfaces** to the caller; rollback-step errors
  are logged/annotated but never mask the root cause.
- Rationale: the operator cannot tolerate stale worktrees ("et ork å rydde opp i").
  Putting `push` last means rollback never has to delete a remote branch — every
  rolled-back artifact is local.
- Rejected: best-effort add that reports per-item portal/launcher errors but keeps
  the worktree (leaves partial state); fail-hard after push (messy remote rollback).

### portal-and-launchers-owned-by-worktree

- Decision: the container-level artifacts (`_portals/<slug>`, `_launchers/<slug>/`,
  `_launchers/ide-menu.cmd`) are created/removed by the **worktree** module
  (lifecycle artifacts). The `.vscode/` config is owned by the **ide** module
  (written only when VS Code is actually used).
- Rationale: clean separation — worktree owns worktree lifecycle; ide owns IDE
  specifics. The operator explicitly did not want `worktree add` writing `.vscode/`.
- Rejected: `worktree add` generating `.vscode/` (couples worktree to IDE policy,
  writes IDE config even when VS Code is never opened).

### launcher-content-relative-paths

- Decision: launchers are thin `.cmd` wrappers that `cd` into an initialized
  worktree dir (one that contains `_mhgo/`) and then call `mhgo`, using
  **`%~dp0`-relative** paths (relative to the `.cmd`'s own location):
  - `_launchers/<slug>/ide.cmd`:
    `@cd /d "%~dp0..\..\<slug>\<relpath>" && mhgo ide spawn <slug>`
  - `_launchers/ide-menu.cmd`:
    `@cd /d "%~dp0..\<hubname>\<relpath>" && mhgo ide menu`
    (`hubname` = the main worktree's directory name; the main worktree is stable.)
- Rationale: the container has no `_mhgo/` and mhgo is cwd-authoritative, so a bare
  `@mhgo ide spawn <slug>` run from the container would fail with "mhgo not
  initialized in this folder". `cd`-into-worktree keeps mhgo running from an
  initialized dir and avoids any container-aware (non-cwd-authoritative) code path.
  Relative `%~dp0` paths tolerate **moving** the whole container; they break only
  on **renaming** the worktree/hub dir, which the operator accepts.
- Notes: `relpath` is captured at `add` time (`rel(gitroot, cwd)`); usually empty
  (init at repo root) → `%~dp0..\..\<slug>`. Assumes the `mhgo` binary is on PATH.
  `<hubname>` (for `ide-menu.cmd`) = `filepath.Base` of the `Main` worktree entry
  returned by `git worktree list --porcelain` (the parser already marks the first
  block `Main=true` with its `.Path` — `internal/worktree/list.go:65,76`). All
  baked path segments use normalized (backslash, `filepath.Clean`) forms — see
  cwd-not-worktree-root path-form note.
- Rejected: absolute baked paths (break on move); teaching `mhgo ide` to resolve
  config from the container with no `_mhgo/` (violates cwd-authoritative principle).

### ide-spawn-vscode-config

- Decision: `mhgo ide spawn <slug>` resolves the worktree as
  `<container>/<slug>`, then:
  1. Generates `.vscode/tasks.json` and `.vscode/settings.json` **only if absent**
     (never clobbers operator edits), at the worktree's `relpath` location next to
     `_mhgo/` — match where VS Code opens.
  2. `tasks.json`: a `Start Claude` shell task with
     `runOptions.runOn: "folderOpen"` that runs `claude` in a dedicated integrated
     terminal, so Claude auto-starts when the folder opens (subject to VS Code's
     one-time "Allow Automatic Tasks" trust prompt).
  3. `settings.json`: `workbench.colorCustomizations.titleBar.*` (the picked
     color), `window.title` (`"<short>: <slug>"` form), `workbench.startupEditor:
     "none"` (kills the Welcome tab), and a key to hide the right-side AI/chat
     panel (e.g. `workbench.secondarySideBar.defaultVisibility: "hidden"` — verify
     the exact key against the installed VS Code during implementation).
  4. Ensures `.vscode/` is in the managed `.gitignore` block via
     `internal/gitignore`.
  5. Launches VS Code on the worktree (`cmd /c code <worktree>` on Windows so
     `code.cmd` resolves via the full PATH, mirroring mill's `_build_code_argv`).
- Rationale: directly delivers "just get going" — open the worktree, Claude is
  already running, no Welcome/AI clutter, each window visually distinct by color.
- Rejected: writing `.vscode/` from `worktree add`; clobbering existing `.vscode/`.

### color-palette

- Decision: reuse mill's exact palette and rotation. Green
  (`#2d7d46`) is reserved for the main worktree (hub); child worktrees get the
  first **unused non-green** color, discovered by scanning sibling worktrees'
  `.vscode/settings.json` `titleBar.activeBackground`; wrap to the first non-green
  if all are in use. Palette order:
  `green #2d7d46, purple #7d2d6b, blue #2d4f7d, yellow #7d5c2d, red #6b2d2d,
  cyan #2d6b6b, indigo #4a2d7d, orange #7d462d`.
- Rationale: the operator is used to these colors and the green=main convention
  (mill `_spawn_core.pick_worktree_color` / `WORKTREE_COLOR_NAME_TO_HEX`).
- Rejected: hash-slug-to-hue (unfamiliar colors, green not reserved).

### ide-menu

- Decision: `mhgo ide menu` is the one **interactive** command (the JSON-in/out
  convention's deliberate exception). It:
  - Discovers active worktrees via `git worktree list --porcelain` (exclude the
    main worktree), keeping those that are mhgo-instantiated (have `_mhgo/` at
    `relpath`); slug = worktree directory name.
  - Calls `Board.HealthCheck()` first; a non-nil result is a **hard error**
    (board must be present).
  - Looks up each slug's title **only via the board module** (see
    board-is-sole-tasks-reader).
  - Prints a numbered picker (`1) <slug> — <title>`), reads a number from stdin,
    and opens the choice through the `ide spawn` path. `q` quits; zero active
    worktrees prints a message and exits 0; invalid input re-prompts or errors.
  - Does **not** auto-spawn a new task (spawn integration is deferred).
- Rationale: a fast Go picker replacing the slow Python `millpy-vscode` chooser.
- Rejected: a JSON-only menu (defeats the purpose); auto-spawn (out of scope).

### board-is-sole-tasks-reader

- Decision: only the `board` module ever reads or validates `tasks.json`/the board
  dir. `internal/ide` imports `internal/board` and reads titles through the public
  facade: `board.LoadConfig(cwd, "board")` → `board.New(cfg)` → `b.ListTasksBrief()`
  / `b.GetTask(slug)`. `ide` never stats the board dir itself.
- Board-validity is the board module's job, via a **new fast** `board` API
  (e.g. `Board.HealthCheck() error`) whose contract is pinned to: **stat the board
  dir + open and read `tasks.json` (no JSON unmarshal)**. Cheap and stat-level; it
  does not validate task contents. Reconciles review GAP: the facade read
  methods short-circuit to `(nil, nil)` when the board dir is missing
  (`board.go:176-178, 191-193`), so an absent board would otherwise yield *blank
  titles, not an error*. The menu calls `HealthCheck()` first and treats a
  non-nil result as a **hard error**.
- Adding `HealthCheck` is the one sanctioned new board method (operator-approved);
  all other reads use the existing facade — no other new method.
- Rationale: the operator's invariant — all `tasks.json` access *and* board-validity
  checks go through board. The board facade already exposes lock-free reads from
  disk; a stat-only health check is cheap.
- Rejected: `ide` stat-ing the board dir itself (violates the invariant); relying
  on `(nil, nil)` to mean "absent" (silently produces blank titles, contradicting
  "board must be present"); shelling out to `mhgo board` (extra process + reparse).

### gitignore-shared-lib

- Decision: add `internal/gitignore` that manages the single mhgo-managed block
  (`# === mhgo-managed === … # === end mhgo-managed ===`) as a **set** of entries.
  Public API along the lines of `gitignore.Ensure(repoRoot string, entries
  ...string) (changed bool, err error)` that merges entries idempotently.
  Refactor `board/init.go`'s `updateGitignoreBlock` to call it (`init` registers
  `.mhgo/`); `ide` registers `.vscode/` when it first writes `.vscode/`.
- Rationale: the operator foresaw many modules contributing ignore entries and
  asked for a common "portal into `.gitignore`". A set-based merge lets multiple
  modules coexist in one block without clobbering each other. Because `.gitignore`
  is committed at the repo's cwd/`relpath`, every worktree checkout inherits the
  ignores.
- Rejected: each module owning a separate managed block (multiple blocks to
  maintain); leaving the block logic buried in `board/init.go` (not reusable).

### vscode-gitignored

- Decision: generated `.vscode/` is **gitignored** (machine-local), not committed.
- Rationale: the operator wants highly custom, per-worktree VS Code setups
  (distinct colors per window) without merge conflicts when a task branch merges
  into its parent. Committed `.vscode/` would fight on every merge.
- Rejected: committing `.vscode/` on the task branch.

### shell-parked

- Decision: do not build `mhgo shell` now. Document it as parked/deferred in
  `docs/modules/`. No `shell.cmd` is generated.
- Rationale: the original `shell` use (a terminal pointed into *another* worktree)
  is superseded by the assumption that the IDE always runs in the worktree's own
  cwd (cwd need not be the git root), plus VS Code auto-starting Claude there.
- Note for docs: the open follow-up is "a fast way to start a pwsh+Claude terminal
  inside an already-running VS Code window." Finding from exploration: there is no
  clean external CLI to inject a terminal into a live VS Code window on demand; the
  supported mechanisms are the `runOn: folderOpen` task (used by `ide spawn`) or an
  in-window trigger (default build task / keybinding). Record this in the parked
  section.

### cross-platform

- Decision: Windows-first. Portal = junction (Windows, `mklink /J`) / symlink
  (POSIX). Launchers (`.cmd`), `ide-menu`, and `ide spawn`'s `code` launch are
  Windows-only; on POSIX they no-op with a clear "unsupported on this platform"
  message. Use the existing `_windows.go` / `_other.go` build-tag split.
- Rationale: Windows is the only dev platform; matches the established
  `git_windows.go`/`git_other.go`, `spawn_windows.go`/`spawn_other.go` pattern.
  Keeping the portal cross-platform is cheap (symlink) and keeps POSIX CI green.
- Rejected: full POSIX parity now (YAGNI); Windows-only portal (needless POSIX
  breakage).

## Technical context

Project: `github.com/Knatte18/mhgo` — Go, one-shot CLI modules, JSON in/out,
daemonless, cwd-authoritative. `cmd/mhgo/main.go` is a thin dispatcher routing
`<module>` to `<module>.RunCLI`.

Key existing code to reuse / extend:

- **`internal/worktree/`** — `worktree.go` (facade `New(cfg)`), `cli.go`
  (`RunCLI` router), `add.go` (`Add(sourceDir, slug)`), `remove.go`
  (`Remove(sourceDir, slug, force)`), `list.go` (`List` + porcelain parser),
  `links.go` (`removeLinks` — removes symlinks/junctions *inside* a dir),
  `config.go` (`Config{BranchPrefix}`, `LoadConfig`). Portals/launchers code adds
  new files here (e.g. `portals.go`, `launchers.go`) plus a junction **creation**
  helper with a `_windows.go`/`_other.go` split (only `removeLinks` exists today;
  there is no create-junction yet — Windows needs `cmd /c mklink /J`, POSIX
  `os.Symlink`, mirroring mill's `_junction.create`).
- **`internal/git/git.go`** — `RunGit(args, cwd) (stdout, stderr, exit, err)` and
  `FindRoot(cwd)` (`rev-parse --show-toplevel`). Use `FindRoot` for the
  cwd-≠-gitroot fix.
- **`internal/board/`** — facade `board.go` exposes read methods (`GetTask`,
  `ListTasksBrief`, `ListTasksFull`) that load straight from disk and
  short-circuit to `(nil,nil)` when the board dir is absent (`board.go:176-178,
  191-193`) — hence the need for the new `HealthCheck`. `config.go`
  `LoadConfig(cwd, "board")` resolves the board dir. `init.go`
  `updateGitignoreBlock` is the logic to extract into `internal/gitignore`;
  `init.go` also already scaffolds `_mhgo/` + `board.yaml` + `worktree.yaml`.
- **`internal/muxpoc/`** — cwd-≠-worktree-root sites to fix: `state.go`
  (`LoadState`/`SaveState` anchor `.mhgo/` on cwd; `socketName` from
  `filepath.Base(cwd)`), and callers `up.go`, `down.go`, `status.go`, `attach.go`,
  `review.go`, `daemon.go`, `cmd.go` (`socketArg`). `state_test.go` covers
  `socketName` and must be updated.
- **`internal/config/config.go`** — `FindBaseDir(cwd)` checks `<cwd>/_mhgo`
  (source of the "not initialized" error to reuse/match); `Load(baseDir, module,
  defaults)` is the two-layer loader.
- **`internal/output/output.go`** — `Ok(out, map)` / `Err(out, msg)` JSON helpers.
- **`internal/muxpoc/spawn_windows.go`** — reference for windowless/detached
  launching and `wt.exe` usage on Windows (CREATE_NO_WINDOW etc.).

Mill references (Python, the behavior being ported — read for fidelity, do not
import):

- `plugins/mill/scripts/_spawn_core.py` — `WORKTREE_COLOR_NAME_TO_HEX`,
  `WORKTREE_COLOR_PALETTE`, `pick_worktree_color(worktrees_dir)` (the exact
  palette + rotation to replicate).
- `plugins/mill/scripts/_vscode.py` — settings.json rendering (color + title).
- `plugins/mill/scripts/millpy-vscode.py` — the chooser being replaced; note
  `_build_code_argv` (`cmd /c code <path>` on Windows for PATH resolution).
- `plugins/mill/scripts/_junction.py` — junction create/remove semantics
  (`mklink /J` on Windows, `os.symlink` on POSIX), including the refuse-to-clobber
  guardrail.

Gotchas:

- The container is **not** a git repo and has **no** `_mhgo/`. Anything run from
  the container fails cwd-authoritative config resolution — hence launchers `cd`
  into a worktree first.
- `mklink /J` needs backslash paths and is a `cmd.exe` builtin (`cmd /c mklink /J
  <link> <target>`).
- Junction teardown order: `_portals/<slug>` is a junction *into* the worktree's
  `_mhgo/`; remove the junction with `os.Remove`/`rmdir` (never recurse into the
  target). The worktree's *internal* junctions are still handled by `removeLinks`
  during `git worktree remove`.
- `worktree add` currently pushes in step 7; moving push last changes existing
  behavior and tests — update `add_test.go` accordingly.

## Constraints

- Follow `docs/overview.md` principles: one-shot, daemonless, JSON in/out,
  cwd-authoritative (cwd ≠ git root). New module prints
  `{"ok":true,...}` / `{"ok":false,"error":...}`; the `ide menu` interactive
  picker is the documented exception.
- Self-contained modules with per-file unit tests next to the source
  (`foo.go` ↔ `foo_test.go`); cross-cutting suites in a black-box test package
  only if needed.
- Windows is the primary platform; keep POSIX building (build-tag split).
- No module other than `board` may read `tasks.json`.
- `_portals/` and `_launchers/` are machine-local, outside any git tree; never
  commit them and never place `_mhgo/` in the container.

## Testing

Per-file unit tests, table-driven where natural (Go + project `testing` skill).

- **`internal/gitignore`** (TDD candidate — pure logic, no I/O ambiguity): new
  file creation; adding to an existing managed block; idempotent re-add (no
  change); merging a *set* (two modules' entries coexist); content outside the
  block preserved; block delimiters correct. Then port `board/init.go`'s existing
  gitignore tests to the new lib and assert `init` still produces the same result.
- **`internal/worktree` cwd-≠-worktree-root fix**: `add`/`remove` invoked from a
  **subdirectory** of the worktree still resolve `container = parent(gitroot)`,
  the correct target, and `relpath` (non-empty when run from a subdir); existing
  `add_test.go`/`remove_test.go` updated for the reordered push and for
  portal/launcher side effects. Cover the path-form normalization (forward-slash
  `FindRoot` output vs backslash cwd).
- **`internal/muxpoc` cwd-≠-worktree-root fix**: `socketName` is stable when
  invoked from a worktree subdirectory (derives from the worktree root, not the
  cwd basename); state read/written from a subdir resolves to the same
  worktree-anchored `.mhgo/`. Update `state_test.go` expectations.
- **`internal/board` HealthCheck**: returns nil for a present, readable board;
  non-nil when the board dir is absent or `tasks.json` cannot be opened/read. Per
  the pinned contract it stats the dir and opens/reads `tasks.json` but does **not**
  JSON-unmarshal — so a syntactically corrupt-but-readable file passes health-check
  (parse errors are handled by the readers' existing fallback).
- **`internal/worktree` portals**: junction created pointing at
  `<worktree>/<relpath>/_mhgo`; removed on `remove`; create refuses to clobber a
  non-junction; POSIX symlink path. (Windows-gated assertions where needed.)
- **`internal/worktree` launchers**: `ide.cmd` content exact (`%~dp0..\..` form,
  correct `<slug>`/`<relpath>`); `_launchers/<slug>/` created and torn down;
  `ide-menu.cmd` created-if-missing with the `%~dp0..\<hubname>` form and not
  clobbered if present.
- **`internal/worktree` transactional rollback**: inject a failure after worktree
  creation (e.g. portal/launcher step) and assert **zero residue** — no worktree
  dir, no branch, no portal, no launcher, and the remote was never pushed.
- **`internal/ide` config generation**: `.vscode/tasks.json` and `settings.json`
  generated only when absent; never clobbered; color picked per the palette with
  green reserved and first-unused-non-green chosen given sibling colors; `.vscode/`
  registered in `.gitignore` via `internal/gitignore`.
- **`internal/ide` menu**: active-worktree discovery (exclude main, require
  `_mhgo/`); titles resolved via the board facade; **hard error when
  `Board.HealthCheck()` fails** (board absent/unreadable); numbered-picker
  selection maps to the right worktree; zero-worktree message. Launching `code` is
  the side effect to stub/seam behind the `_windows`/`_other` split so tests don't
  actually open VS Code.
- Keep `BOARD_SKIP_GIT`-style seams in mind so tests never hit the network; the
  `code`/`mklink` calls must be injectable/stubbable.

## Q&A log

- **Q:** Should `mhgo shell` be built now? **A:** No — park it. The old "terminal
  into another worktree" need is replaced by VS Code running in the worktree's own
  cwd + auto-starting Claude. Keep it in docs as deferred.
- **Q:** Is there a clean way to start a pwsh+Claude terminal inside an
  already-running VS Code window? **A:** Not via external CLI; only `runOn:
  folderOpen` tasks or an in-window trigger. We use the folderOpen task.
- **Q:** Portal target dir name? **A:** `_mhgo/` (committed, `_`-prefixed), never
  `_mill`. The container itself gets no `_mhgo/`.
- **Q:** Do we need a config flag for "which subfolder is cwd" (mill's
  `hub_relative_path`)? **A:** No — since `_mhgo/` is committed, capture
  `relpath = rel(gitroot, cwd)` at `add` time; the new worktree has `_mhgo/` at the
  same `relpath`.
- **Q:** Who reads `tasks.json` for titles? **A:** Only `board`; `ide` imports the
  board facade. Board absent → hard error. No new board method (facade already has
  reads).
- **Q:** Commit or gitignore `.vscode/`? **A:** Gitignore — per-worktree custom
  colors, avoid merge mess. Add it via a shared `.gitignore` portal lib.
- **Q:** How to manage `.gitignore` given many future contributors? **A:** A shared
  `internal/gitignore` managing one mhgo-managed block as a set of entries.
- **Q:** add failure handling? **A:** Robust full rollback — no stale worktrees.
  Push moves to last so rollback stays local.
- **Q:** VS Code color scheme? **A:** Mill's palette; green = main always, then
  rotate first-unused-non-green. (Claude color later.)
- **Q:** Absolute or relative launcher paths? **A:** Relative (`%~dp0`-based) —
  tolerate moving the container, accept breakage on rename.
- **Q:** Fix cwd-≠-gitroot? **A:** Yes, permanently and repo-wide; sweep for any
  other site that assumes cwd == git root and fix it too.
- **Q:** (review r1 GAP) "board absent → hard error" contradicts the facade, which
  returns `(nil,nil)`. **A:** `ide` must not stat the board dir; add a fast
  `Board.HealthCheck()` to the board module (board owns validity), and the menu
  hard-errors on a non-nil result.
- **Q:** (review r1 GAP) cwd fix must cover the config/call site. **A:** Config
  resolution stays cwd-based (`_mhgo/` required at cwd); only `container`/`relpath`
  derive from `git.FindRoot(cwd)`. The call site keeps passing cwd for config.
- **Q:** (review r1 NOTE) `FindRoot` path-form mismatch? **A:** Normalize via
  `filepath.Clean`/`FromSlash` before computing `relpath` / building paths.
- **Q:** (review r1 NOTE) where does `<hubname>` come from? **A:** `filepath.Base`
  of the `Main` worktree entry from `git worktree list --porcelain`.
- **Q:** Audit found muxpoc also assumes cwd == worktree root (socket/state) —
  fix it though it's a POC? **A:** Fix ALL cwd-≠-worktree-root issues, muxpoc
  included.
