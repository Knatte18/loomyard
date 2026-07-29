# Batch: async-push-plumbing

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
batch: async-push-plumbing
number: 2
cards: 3
verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
depends-on: []
```

## Batch Scope

This batch adds the detached, both-sides async-push machinery `Fabric.Commit` (batch 3) fires — independent of the commit logic itself, so it is its own root batch. It moves the detached-spawn logic down into `fabricengine` (an engine may spawn a detached `lyx <module>` child, per the `boardengine.spawnSync` precedent), extends the existing hidden `lyx fabric --weft-path push` bypass with a companion `--warp-path` so one child pushes **both** repos, and reduces `fabriccli.spawnPush` to a thin caller. The external interface batch 3 consumes: `fabricengine.SpawnDetachedPush(warpPath, weftPath string) error` and `fabricengine.PushWarpAt(warpPath string, opts SyncOptions) error`. Batch-local decision (from `_mill/discussion.md`'s `async-push-both-sides-detached` open item): skip-env gating is **helper-internal** (the engine helper checks `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` itself and forks no child when set, exactly as today's `fabriccli.spawnPush` does — deliberately not board's caller-side `if !b.skipGit` gating), and consolidation means moving `spawnPush`'s logic into `fabricengine`, never a cross-package import (the CLI/Cobra Invariant forbids `fabricengine` importing `fabriccli`).

## Cards

### Card 4: Engine-level detached-push spawn helper + PushWarpAt

- **Context:**
  - `internal/fabriccli/spawn.go`
  - `internal/boardengine/spawn.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/gitrepo/push.go`
  - `internal/proc/proc_linux.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/spawn_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a new `internal/fabricengine/spawn.go` add `func SpawnDetachedPush(warpPath, weftPath string) error` mirroring `boardengine.spawnSync` and the current `fabriccli.spawnPush`: return `nil` immediately (no child forked) when `os.Getenv("WEFT_SKIP_GIT") == "1"` or `os.Getenv("WEFT_SKIP_PUSH") == "1"`; otherwise resolve `os.Executable()`, build args starting with `"fabric"`, append `"--warp-path", <abs warpPath>` only when `warpPath != ""` and `"--weft-path", <abs weftPath>` only when `weftPath != ""` (via `filepath.Abs`), append `"push"`, and — when at least one path was supplied — `exec.Command(exe, args...)`, `proc.Detach(cmd)`, and `cmd.Start()` (intentionally not `Wait`ed, stdin/stdout/stderr left nil). Return `nil` without forking when both paths are empty. Also add `func PushWarpAt(warpPath string, opts SyncOptions) error` — the warp analog of `PushWeftAt` in `weftgit.go`: return `nil` when `opts.SkipGit || opts.SkipPush`, else `gitrepo.New(warpPath).PushCoalesced()`. Add an untagged Tier-1 `spawn_test.go` (no git spawn, no `exec.Command` token): assert `SpawnDetachedPush` returns nil with no error when `WEFT_SKIP_GIT=1` (via `t.Setenv`), when `WEFT_SKIP_PUSH=1`, and when both paths are empty; assert `PushWarpAt` returns nil when `opts.SkipGit` and when `opts.SkipPush`. Do not exercise the real fork path (it would re-exec the test binary); the real both-sides push is covered by card 6.
- **Commit:** `feat(fabric): add engine-level detached both-sides push spawn helper`

### Card 5: Wire --warp-path bypass into the fabric CLI and thin out spawnPush

- **Context:**
  - `internal/fabricengine/spawn.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabriccli/weft_verbs.go`'s `addWeftVerbs`, register a second hidden persistent flag `--warp-path` alongside the existing `--weft-path` (same `MarkHidden` treatment, internal help text). Add a `warpPath` closure var beside `weftPath`. In `PersistentPreRunE`, read both injected flags via `InheritedFlags().GetString`; set `bypass = injectedWeft != "" || injectedWarp != ""`, populate `weftPath`/`warpPath` from whichever is set, and keep the existing "only `push` is valid in bypass mode" gate (reject any other verb with the existing "subcommand requires a worktree context" error). In the `push` subcommand's bypass branch, push each supplied side: when `warpPath != ""` call `fabricengine.PushWarpAt(warpPath, fabricengine.SyncOptions{})` and when `weftPath != ""` call the existing `fabricengine.PushWeftAt(weftPath, fabricengine.SyncOptions{})`, surfacing the first error through `output.Err` and otherwise emitting `output.Ok`. Reduce `internal/fabriccli/spawn.go`'s `spawnPush(weftPath string) error` to `return fabricengine.SpawnDetachedPush("", weftPath)` (weft-only, preserving the existing `lyx fabric sync` verb's behavior at its unchanged call site) and drop the now-duplicated env-gating/`os.Executable`/`exec.Command`/`proc.Detach` code that moved into the engine helper.
- **Commit:** `feat(fabric): push both repos via --warp-path bypass and thin spawnPush`

### Card 6: Integration test — bypass pushes both repos

- **Context:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/spawn.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabriccli/pushbypass_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` test file exercising the bypass push path synchronously through the `fabriccli.RunCLI(&out, args)` seam (the same seam `cli_test.go`'s `TestRunCLI_WeftPathPushOnly` uses). Build a weft repo with a bare upstream and an unpushed commit via `lyxtest.CopyWeft` + `lyxtest.MustRun`, and a warp repo with its own bare upstream and an unpushed commit via `lyxtest.MustRun` git commands. Assert that `RunCLI(&out, []string{"--warp-path", <warp>, "--weft-path", <weft>, "push"})` exits 0 and that **both** bare upstreams advanced to include the previously-unpushed commits (both sides pushed via `PushWarpAt`/`PushWeftAt`). Add a case asserting `--warp-path <warp>` with a non-`push` verb (e.g. `status`) still exits 1 with the "requires a worktree context" rejection, mirroring the existing `--weft-path` push-only test. Reuse `cli_test.go`'s `decodeResult` helper for JSON parsing where useful.
- **Commit:** `test(fabric): bypass push advances both warp and weft upstreams`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/` runs the new Tier-1 `spawn_test.go` (helper-internal skip-env gating and `PushWarpAt` gating), the new `pushbypass_integration_test.go` (both-sides real push + push-only-bypass rejection), and every existing test in both packages — regression-guarding the `spawnPush` thinning and the `weft_verbs.go` bypass changes against the existing `cli_test.go` suite (`TestRunCLI_WeftPathPushOnly`, `TestRunCLI_NoArgs`, `TestRunCLI_UnknownSubcommand`). No detached-child end-to-end assertion is attempted (a child launched from the test binary re-execs the test binary); the deterministic proof that a supplied path is pushed lives in the synchronous bypass-handler test here, which is exactly the code the detached child runs.
