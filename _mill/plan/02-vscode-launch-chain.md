# Batch: vscode-launch-chain

```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
batch: "vscode-launch-chain"
number: 2
cards: 2
verify: go test ./internal/vscode/... ./internal/ideengine/...
depends-on: []
```

## Batch Scope

This batch turns the generated `.vscode/tasks.json` into the reed launch chain: four sequenced VS Code tasks that run `lyx reed up`, `lyx reed add --if-absent …`, `lyx reed attach` on `folderOpen`, with both binary paths stamped absolute by the caller.
It is one batch because `WriteConfig`'s signature change, the literal it builds, and its only call site are a single compile unit — the call site must move in the same commit or the build breaks.
It carries no code dependency on batch 1: the flag reaches the generated file as a string, so the two batches can run in either order, and the `depends-on: []` here says so.
The external interface batch 3 consumes is the convention itself — the task names and the fact that a worktree opens straight into the orchestrator strand.

## Cards

### Card 6: `WriteConfig` writes the sequenced reed launch chain

- **Context:**
  - `internal/gitignore/gitignore.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junction.go`
- **Edits:**
  - `internal/vscode/config.go`
  - `internal/vscode/config_test.go`
  - `internal/ideengine/spawn.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `WriteConfig` in `internal/vscode/config.go` to `WriteConfig(worktreeDir, relpath, slug, color, lyxPath, claudePath string) error`.
  `WriteConfig` owns the fallback rule and is the single place it lives: an empty `lyxPath` becomes the bare name `lyx`, an empty `claudePath` becomes the bare name `claude`, applied once at the top of the function before the tasks literal is built.
  Replace the single `Start Claude` shell task with four entries.
  Three are `"type": "shell"` step tasks, each carrying `command` set to the resolved lyx path and `args` as a JSON array — `["reed", "up"]`, `["reed", "add", "--if-absent", "--cmd", <resolved claude path>, "--name", "claude", "--focus"]`, and `["reed", "attach"]` — with labels naming their step.
  Arguments stay in the `args` array rather than being concatenated into the command string, so VS Code applies its own per-platform quoting and a stamped path containing spaces survives on both platforms.
  The `up` and `add` steps carry `presentation` with `reveal: "silent"`, `panel: "shared"`, `echo: true` and an explicit `focus: false`;
  the `attach` step carries `reveal: "always"`, `panel: "new"`, `echo: true` and `focus: true`, because it is the terminal the operator types into and VS Code's `presentation.focus` defaults to `false`.
  The fourth is the entry task, labelled `Start Claude`, carrying exactly four keys — `label`, `dependsOn` listing the three step labels in order, `dependsOrder: "sequence"`, and `runOptions.runOn: "folderOpen"` — with no `type`, no `command`, and no `presentation` of its own.
  Write a comment at the tasks literal recording that `dependsOrder: "sequence"` is relied on for ordering alone: VS Code's runner has historically run the next dependent task regardless of the previous one's exit code, and the chain is safe either way because `AddStrand` pre-flights `requireSessionLocked` and `attach` pre-flights `Status`, so a failed `up` ends with no strand, no pane, and no bare `claude`.
  Add no compensating guard of the runner's behaviour.
  In `internal/ideengine/spawn.go`, resolve both paths before the `WriteConfig` call and pass them through: `os.Executable()` for lyx and `exec.LookPath("claude")` for claude, each degrading to the empty string on error so `WriteConfig`'s own rule applies.
  `Spawn` substitutes no bare name itself.
  In `internal/vscode/config_test.go`, extend `TestWriteVSCodeConfigCreatesFilesWhenAbsent` (and add cases beside it) to assert the generated shape: the three step tasks exist and are sequenced by the entry task's `dependsOn` plus `dependsOrder: "sequence"`;
  the add step's args carry `--if-absent`, `--name claude` and `--focus`;
  each step's `command` is the stamped lyx path the caller passed rather than the bare name;
  the add step's `--cmd` argument is the stamped claude path;
  each task's `presentation` matches the split above, `focus` included;
  the entry task carries exactly its four keys and holds no `type`, `command` or `presentation`;
  and `folderOpen` still triggers the entry task.
  Add one case per fallback, driven against `WriteConfig` because it owns the rule: an empty `lyxPath` yields the bare name `lyx` in every step's `command`, and an empty `claudePath` yields the bare name `claude` in the add step's `--cmd`, each still producing a valid JSON file.
  `TestWriteVSCodeConfigDoesNotClobber` keeps passing with its assertions unchanged beyond the new signature — it is the regression guard for the never-clobber contract that makes existing worktrees keep their current task.
  `internal/vscode/config_test.go` holds a third positional call site, in `TestWriteVSCodeConfigRegistersInGitignore`;
  update that call to the new signature as well, leaving its own gitignore assertions unchanged, since the two new parameters have no bearing on them.
  All three call sites must move together or the package stops compiling and this batch's own `verify:` fails.
  In `docs/overview.md`, rewrite the **ide** bullet to say that the generated `folderOpen` task is now the reed launch chain rather than a bare `claude`, that both binary paths are stamped absolute at generation time, and that an existing worktree keeps its current `tasks.json` because `WriteConfig` never clobbers — the manual upgrade being to delete `.vscode/tasks.json` and re-run `lyx ide spawn`.
- **Commit:** `feat(vscode): generate the reed launch chain as sequenced tasks`

### Card 7: `Spawn` stamps both resolved paths into the generated file

- **Context:**
  - `internal/vscode/config.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/ideengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `TestSpawn` in `internal/ideengine/spawn_test.go`, keeping its existing table and stubbed `CodeLauncher` shape, with assertions on the `tasks.json` the spawn generated.
  Parse the written file and assert the add step's argument list carries `--if-absent`, `--name`, `claude` and `--focus`, and that the entry task still triggers on `folderOpen` — the same convention `internal/vscode/config.go` writes, asserted here through the real `Spawn` path rather than a direct `WriteConfig` call.
  Assert that each step task's `command` is an absolute path, which is what proves `Spawn` resolved and passed the lyx path rather than leaving `WriteConfig` to fall back to the bare name.
  Under `go test` the resolved executable is the test binary, and that is the expected value here;
  the assertion is on the shape of the stamp, so pin absoluteness rather than any particular filename.
  Reading `os.Executable()` for its value is deliberate and is a different act from re-execing it, which CONSTRAINTS.md bars under `go test`.
- **Commit:** `test(ideengine): assert Spawn stamps the reed chain's binary paths`

## Batch Tests

`verify: go test ./internal/vscode/... ./internal/ideengine/...` runs both packages' untagged suites — `internal/vscode/config_test.go`'s extended generated-shape and fallback cases from card 6, its unchanged non-clobber and gitignore guards, and `internal/ideengine/spawn_test.go`'s end-to-end stamp assertions from card 7.
Both packages are untagged and hermetic: they write into `t.TempDir()` and stub the VS Code launcher, so nothing here needs a fixture hub or a tag.
The scope is the two packages this batch edits;
`docs/overview.md` carries no runnable surface of its own and is covered by the repo-wide markdown-link gate batch 3 runs.
