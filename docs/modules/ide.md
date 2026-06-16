# Module: ide

> **Status:** implemented (roadmap milestone 4). One-shot VS Code launcher with
> interactive worktree picker. Driven by `lyx ide <subcommand>`.

The ide module (`internal/ide`) launches VS Code on git worktrees with automatic
configuration (color-coded title bars, Claude auto-start tasks, syntax cleanup).
It owns two entry points: `spawn` (open a specific worktree) and `menu` (interactive
picker over active worktrees). Consumes [`paths`](../shared-libs/paths.md),
[`board`](board.md), and [`git`](../shared-libs/git.md).

## What problem this solves

Opening a worktree in VS Code still requires navigating to it by hand. The operator
needs a one-click way to open any active worktree, visually distinct by color so
parallel tasks are obvious, with Claude already running and the interface cleaned
of clutter. A fast Go picker replaces mill's slow Python `millpy-vscode` chooser.

## Subcommands

| Command | Does |
|---|---|
| `lyx ide spawn <slug>` | Open VS Code on worktree `<slug>`: generate `.vscode/` config (only if absent), assign a title-bar color, and launch VS Code. |
| `lyx ide menu` | Interactive numbered picker over active worktrees (slug + title from board); opens the chosen one via `spawn`. Hard-errors if the board is absent. |

## Color assignment

**Color palette** (reused from mill, in order):
- `#2d7d46` green (reserved for the main/hub worktree)
- `#7d2d6b` purple
- `#2d4f7d` blue
- `#7d5c2d` yellow
- `#6b2d2d` red
- `#2d6b6b` cyan
- `#4a2d7d` indigo
- `#7d462d` orange

**Assignment logic:** each worktree gets the first **unused non-green** color,
discovered by scanning sibling `.vscode/settings.json` files for the current
`titleBar.activeBackground` setting. Wrapping occurs to the first non-green if all
colors are in use.

## Configuration generation

`lyx ide spawn <slug>` generates two files inside `.vscode/` — **only if they do
not already exist** (never clobbers operator edits):

### tasks.json

A single `Start Claude` shell task with `runOptions.runOn: "folderOpen"`:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Start Claude",
      "type": "shell",
      "command": "claude",
      "runOptions": {
        "runOn": "folderOpen"
      },
      "presentation": {
        "echo": true,
        "reveal": "always",
        "panel": "new"
      }
    }
  ]
}
```

This causes Claude to auto-start in a dedicated integrated terminal when VS Code
opens (subject to VS Code's one-time "Allow Automatic Tasks" trust prompt).

### settings.json

Three display customizations:

- **`workbench.colorCustomizations`** — title bar colors (the assigned non-green color
  for active/inactive bars, white text for active, semi-transparent white for inactive)
- **`window.title`** — `"<slug>"` (or `"<short>: <slug>"` if a short name is available,
  for human readability when multiple windows are open)
- **`workbench.startupEditor: "none"`** — kills the Welcome tab
- **`workbench.secondarySideBar.defaultVisibility: "hidden"`** — hides the right-side
  AI/chat panel, so the editor is maximized by default

The `.vscode/` directory is added to the managed `.gitignore` block via
[`internal/gitignore`](../shared-libs/gitignore.md), so it is not committed
(machine-local configuration, no merge conflicts).

## Interactive menu

`mhgo ide menu` is the one **interactive exception** to the JSON-in/JSON-out convention:

1. **Discovers active worktrees** via `git worktree list --porcelain`, excluding the
   main worktree, and keeps only those with `_mhgo/` at the captured `relpath`
   (mhgo-instantiated worktrees).
2. **Hard-errors on absent board:** calls `Board.HealthCheck()` first; a non-nil
   result is a hard failure. The board module is the sole authority on board
   validity; `ide` never stats the board dir itself.
3. **Looks up titles via board:** for each discovered slug, fetches the task title
   via `board.GetTask(slug)`. The board is the sole reader of `tasks.json`.
4. **Prints a numbered picker:** one line per worktree: `1) <slug> — <title>`.
   Reads a number from stdin, opens the choice via `ide spawn`, or accepts `q` to quit.
   Zero active worktrees prints a message and exits 0; invalid input re-prompts
   (once) or errors.

The operator explicitly controls which worktree to open; auto-spawning tasks is
deferred.

## Dependencies and authority

- **Board dependency:** `ide` imports `internal/board` and reads task titles through
  the public facade (`board.LoadConfig` → `board.New` → `board.ListTasksBrief` /
  `board.GetTask`). The board's `HealthCheck()` method verifies the board dir exists
  and `tasks.json` is readable (cheap stat-level check, no JSON unmarshal). `ide`
  never stats the board dir itself — all board-validity checks flow through the board
  module.
- **Sole tasks reader:** only `board` reads or validates `tasks.json`. All other
  modules (including `ide`) respect this invariant.
- **Geometry:** `ide` resolves worktree geometry via [`internal/paths`](../shared-libs/paths.md);
  raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/paths`
  and `cmd/mhgo/main.go`.

## Platforms

**Windows-only.** Launchers, menu discovery, and VS Code launch are Windows-specific:

- `code` command (Windows `code.cmd` on PATH via PATH resolution) opens VS Code.
- `.cmd` launchers and `ide-menu.cmd` (created by `worktree add`, discovered by menu)
  are Windows-only.
- On POSIX, `ide spawn` and `ide menu` are no-ops with a clear "unsupported on this
  platform" message.

Uses the established `_windows.go` / `_other.go` build-tag split (mirroring
`git_windows.go` / `git_other.go`, `spawn_windows.go` / `spawn_other.go`).

## Parked: mhgo shell

A future follow-up (deferred from this task) is `mhgo shell`: a fast way to start a
pwsh+Claude terminal inside an already-running VS Code window. Investigation found no
clean external CLI to inject a terminal into a live VS Code instance on demand; the
supported mechanisms are the `runOn: folderOpen` task (used here) or an in-window
trigger (default build task / keybinding). This is recorded and may inspire a future
design.
