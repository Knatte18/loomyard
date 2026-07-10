# Batch: linux-launch-surface

```yaml
task: "Facilitate Linux support (Win11-side prep)"
batch: "linux-launch-surface"
number: 5
cards: 4
verify: GOOS=linux go build ./internal/warpengine/... ./internal/hubgeometry/... ./internal/vscode/... && go test ./internal/warpengine/... ./internal/hubgeometry/... ./internal/vscode/...
depends-on: []
```

## Batch Scope

This batch makes the operator-facing launch surface work on Linux: (a) a `.sh` launcher branch
in `warpengine`'s `writeLaunchers` (which today early-returns to a no-op on non-Windows), built
from a new **pure, GOOS-parameterized content builder** so both the `.cmd` and `.sh` outputs are
fixture-tested on the Windows host; (b) a GOOS-aware **menu** launcher filename in
`internal/hubgeometry` (`MenuLauncherPath` hardcodes `ide-menu.cmd`), per the Hub Geometry
Invariant that geometry tokens live only in `hubgeometry`; and (c) a real
`internal/vscode/launch_linux.go` (today it returns `ErrUnsupported`, leaving `lyx ide spawn`
dead on Linux).

Scoped geometry note: only the **menu** launcher filename is a geometry token (it lives in
`hubgeometry`); the `ide` and `warp-checkout` filenames are built inside `warpengine` and their
`.cmd`/`.sh` extension logic stays there — do not over-migrate them into `hubgeometry`. This
batch shares no file with any other batch (root, parallel).

## Cards

### Card 16: Pure GOOS-parameterized launcher content builder

- **Context:**
  - `internal/warpengine/launchers.go`
- **Edits:** none
- **Creates:**
  - `internal/warpengine/launcher_content.go`
  - `internal/warpengine/launcher_content_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a build-tag-free `launcher_content.go` (package `warpengine`) add a pure
  `launcherScript(goos, climbRel, lyxArgs string) (content []byte, mode os.FileMode)` and a pure
  `launcherExt(goos string) string`. `launcherExt` returns `".cmd"` for `"windows"`, `".sh"`
  otherwise. `launcherScript` normalizes `climbRel` with `filepath.ToSlash` first, then: for
  `"windows"`, converts slashes to backslashes and returns
  `"@cd /d \"%~dp0" + climbBack + "\" && lyx " + lyxArgs + "\r\n"` with mode `0o644` (matching the
  current `ide.cmd`/`warp-checkout.cmd`/`ide-menu.cmd` bodies at `launchers.go:44,55-59,76-77`);
  for non-windows, keeps forward slashes and returns
  `"#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/" + climbFwd + "\" && lyx " + lyxArgs + "\n"` with
  mode `0o755` and LF endings. In `launcher_content_test.go` (untagged, host-runnable) assert both
  OS branches for representative `lyxArgs` (`ide spawn <slug>`, `warp checkout`, `ide menu`) and
  climb paths (empty and nested): exact body string, mode bits (0o644 vs 0o755), shebang presence
  for `.sh`, path separator direction, and line endings (CRLF for `.cmd`, LF for `.sh`), plus the
  `launcherExt` mapping.
- **Commit:** `feat(warpengine): pure GOOS-parameterized launcher content builder`

### Card 17: GOOS-aware menu launcher filename in hubgeometry

- **Context:**
  - `internal/warpengine/launchers.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `hubgeometry.go`, make `MenuLauncherPath()` (`hubgeometry.go:308-310`)
  GOOS-aware: add an unexported helper `menuLauncherName() string` returning `"ide-menu.cmd"` on
  `runtime.GOOS == "windows"` and `"ide-menu.sh"` otherwise, and use it in the
  `filepath.Join(l.Hub, "_launchers", l.RelPath, menuLauncherName())` call (keep the `_launchers`
  literal inline here — it is a geometry token owned by this package). Update the doc comment
  (`hubgeometry.go:301-307`) to say the extension is GOOS-selected. Add the `runtime` import.
  `MenuLauncherRel()` needs no change (it derives from the path's *dir*, not its filename). In
  `hubgeometry_test.go`, update the `MenuLauncherPath` subtests (`hubgeometry_test.go:352-386`)
  so each `want` computes the expected filename from `runtime.GOOS` (e.g. via a local
  `"ide-menu.cmd"`/`"ide-menu.sh"` switch) rather than hardcoding `ide-menu.cmd`, so the test is
  green on the Windows host now and on Linux in the follow-up.
- **Commit:** `feat(hubgeometry): GOOS-aware menu launcher filename`

### Card 18: Wire the .sh branch into writeLaunchers

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/warpengine/launcher_content.go`
- **Edits:**
  - `internal/warpengine/launchers.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `launchers.go`, replace the non-Windows early return
  (`launchers.go:32-34`) with a real cross-platform body that routes both the Windows and
  non-Windows cases through `launcherScript`/`launcherExt` (from card 16). Compute the extension
  once via `ext := launcherExt(runtime.GOOS)`; write `ide<ext>` and `warp-checkout<ext>` into
  `l.LauncherDir(slug)` using the climb `l.LauncherSpawnRel(slug)` and args `"ide spawn "+slug`
  / `"warp checkout"`; write the menu launcher at `l.MenuLauncherPath()` (already GOOS-aware
  after card 17) using climb `l.MenuLauncherRel()` and args `"ide menu"`. Use the `mode` returned
  by `launcherScript` in each `os.WriteFile` (so `.sh` files are `0o755`). Preserve the existing
  never-clobber menu guard (`os.Stat` → return early if the menu launcher already exists,
  `launchers.go:63-68`) and the `os.MkdirAll` directory creation. `removeLaunchers`
  (`launchers.go:94-102`) is unchanged (`os.RemoveAll` on the launcher dir is
  extension-agnostic). The existing `//go:build integration`, Windows-gated `launchers_test.go`
  keeps asserting the `.cmd` output on Windows; the `.sh` output is covered by card 16's
  untagged builder test. Also update the file's package/leading doc comment (`launchers.go:1-2`,
  currently stating launchers are Windows-only / a no-op elsewhere) to describe the new
  cross-platform behavior — `.cmd` on Windows, executable `.sh` on other platforms.
- **Commit:** `feat(warpengine): emit .sh launchers on Linux`

### Card 19: Implement vscode Linux launch

- **Context:**
  - `internal/vscode/launch_windows.go`
  - `internal/vscode/color.go`
- **Edits:**
  - `internal/vscode/launch_linux.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `ErrUnsupported` stub in `launch_linux.go` with a working
  `Launch(worktreeDir string) error` that runs `exec.Command("code", worktreeDir)` and
  `cmd.Start()`, wrapping a start error as `fmt.Errorf("launch code: %w", err)` — mirroring
  `launch_windows.go` but **without** the Windows `cmd /c` PATH shim and without a
  `proc.HideWindow` call (it is a no-op on Linux; do not import `internal/proc`). Add the
  `os/exec` and `fmt` imports. Keep the file relying on the `_linux` filename suffix for its
  build constraint (no `//go:build` line), matching the current `launch_linux.go`. `ErrUnsupported`
  stays defined in `color.go` for any genuinely-unsupported future platform.
- **Commit:** `feat(vscode): implement code launch on Linux`

## Batch Tests

`verify` cross-compiles the three touched packages for Linux (`GOOS=linux go build ...`, which
compiles the Linux-tagged `vscode/launch_linux.go` the host `go test` never sees) then runs
`go test ./internal/warpengine/... ./internal/hubgeometry/... ./internal/vscode/...`. Host
coverage: the pure launcher content builder for both `.cmd` and `.sh`
(`launcher_content_test.go`), the GOOS-aware menu filename (`hubgeometry_test.go`, updated to
derive the expected extension from `runtime.GOOS`), and the existing vscode config/color tests.
The `writeLaunchers` wiring and the real Linux `code` launch execute only on a real Linux box
(deferred follow-up); here they are compile-checked by the cross-compile step. The Windows-gated,
integration-tagged `launchers_test.go` continues to assert the `.cmd` path on Windows.
