# Batch: launchers

```yaml
task: "Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise"
batch: "launchers"
number: 2
cards: 2
verify: go test ./internal/fabricengine/
depends-on: [1]
```

## Batch Scope

This batch retargets the per-worktree launcher script's embedded command from `loom run` to `loom start` and makes the launcher-filename-unchanged decision executable as a test.
It is its own batch because `internal/fabricengine` is a separate package with no compile dependency on the renamed `loomcli` identifiers — the launcher carries the verb as a string, not as a Go symbol — so it neither needs nor blocks batch 1's compile unit.
It depends on batch 1 only so the tree never names `loom start` in a launcher before that verb exists.

Batch-local decision: the filename stays `run<ext>` and `removeLaunchers`' explicit name list keeps `"run"+ext`.
Card 9's new test scenario exists precisely to stop a later tidying pass from "finishing" the rename by renaming the file.

## Cards

### Card 8: retarget the launcher's embedded command

- **Context:**
  - `internal/fabricengine/launcher_content.go`
  - `internal/loomcli/start.go`
- **Edits:**
  - `internal/fabricengine/launchers.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the `launcherScript(runtime.GOOS, spawnRel, "loom run")` call that builds `runContent` so its third argument is `"loom start"`.
  Rewrite the file header comment's last clause, which states that the run launcher file (`run<ext>`) invokes `"lyx loom run"`, so it names `"lyx loom start"`.
  Update the comment block above the `runContent` assignment, which explains that the launcher invokes the explicit two-word verb rather than any root alias, so its wording matches the new verb name while keeping that rationale intact.
  Leave the filename construction `filepath.Join(launcherDir, "run"+ext)`, the `fmt.Errorf("write run%s: %w", ext, err)` message, and `removeLaunchers`' explicit name list `{"ide" + ext, "fabric-checkout" + ext, "run" + ext}` exactly as they are.
  Renaming the file would orphan a stale `run<ext>` in every already-created worktree, which would invoke the new foreground driver and would also break `removeLaunchers`' non-recursive directory removal, since that function deletes by that explicit list and the comment above it calls the slice a mandatory edit point for exactly this reason.
  `writeLauncherScriptIfChanged` rewrites an existing file's content in place, so every existing hub picks up the corrected command with no migration step.
- **Commit:** `fix(fabricengine): point the worktree launcher at lyx loom start`

### Card 9: assert the content changed and the filename did not

- **Context:**
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/launcher_content.go`
- **Edits:**
  - `internal/fabricengine/launcher_content_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update every expected launcher-content literal in this file that names `loom run` so it names `loom start`, matching what card 8 now writes.
  Read each assertion before changing it — this file also covers the `ide` and `fabric-checkout` launchers, whose commands are untouched.
  Add one scenario asserting the two halves of the launcher-filename-unchanged decision together: that the written run launcher's filename is still `"run" + launcherExt(runtime.GOOS)` while its content names `loom start`.
  Without that pairing a later tidying pass renames the file to `start<ext>` and silently breaks teardown on every existing hub, because `removeLaunchers` deletes by an explicit name list and then removes the directory non-recursively.
  This scenario must call `writeLaunchers` itself, not `launcherScript`.
  The file's existing cases all drive `launcherScript`/`launcherExt` directly, and neither of those constructs a path — `runPath` is built by `filepath.Join(launcherDir, "run"+ext)` inside `writeLaunchers`, so a scenario in the existing shape would assert a filename the production code never computes and would not catch the rename it exists to prevent.
  No untagged test in this package exercises `writeLaunchers` today;
  every existing caller of it is `//go:build integration`.
  The mechanism: build a `*lyxcwd.Location` by hand whose `HubPath` is a `t.TempDir()`, the same hand-built-Location pattern other tier 1 suites in this repo already use to avoid a real hub, pass a fresh `*Mutations` recorder and a slug, and call `writeLaunchers` directly.
  Then assert on the real filesystem result: a file exists at `LauncherDir(l, slug)/run<ext>`, its bytes contain `loom start`, and no `start<ext>` file exists beside it.
  `writeLaunchers` performs only `os.OpenRoot`, `MkdirAll` and file writes, so it stays inside the Test Tier Purity Invariant — no `exec.Command`, no `gitexec`, no `hubforge.NewHub`.
  Its one tail hazard is the menu-launcher branch, which calls `PrimeName(l)` and propagates that error rather than degrading;
  if a hand-built `Location` cannot satisfy `PrimeName`, pre-seed the file at `menuLauncherPath(l)` so the never-clobber early return fires before `PrimeName` is ever reached.
- **Commit:** `test(fabricengine): pin the launcher command to loom start and its filename to run`

## Batch Tests

`verify:` runs `go test ./internal/fabricengine/`, the package that owns both edited files.
It completes in under two seconds on this tree and covers `launcher_content_test.go` — the only test asserting on launcher content or filenames — so the per-batch scoping default applies with no carve-out needed.

The two cards are deliberately paired inside one batch: card 8 changes what is written and card 9 changes what is asserted, so splitting them would leave the batch boundary red.
