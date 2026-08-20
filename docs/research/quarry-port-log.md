# quarry port log

This is a transient port record for the "Extract scout into its own standalone repo" task.
It is deleted by this task's final batch (`06-lyx-removal`), once the extraction lands.
Each quarry-side batch appends its own `## Batch N` section here, so every batch that writes in the quarry worktree also produces a commit in this worktree — see the `two-repo-worktree-authorization` decision in `_mill/discussion.md`.

## Batch 1 — quarry-scaffold

Landed in `/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch `main`):

- Repo scaffolding: `go.mod` (module `github.com/Knatte18/quarry`, go 1.26, direct requires `cobra` v1.10.2, `yaml.v3` v3.0.1, `gofrs/flock` v0.8.1), `.gitignore`, `LICENSE` (Apache-2.0, copied byte-for-byte from Loomyard).
- `README.md` carrying the mandated platform-set, windows-native-strategy-only, and toolchain-cache-re-key statements, plus the config/state precedence chains and the two test tiers.
- The three leaf shared packages copied verbatim: `internal/lock`, `internal/proc`, `internal/output`. Their external test files' self-import (`lock_test.go`, `output_test.go`) was updated from `github.com/Knatte18/loomyard/...` to `github.com/Knatte18/quarry/...`, the one import-path edit required by the module rename itself — no other lines touched.
- The four research/benchmark docs (`scout-spike.md`, `scout-multilang.md`, `scout-agent-usage-findings.md`, `scout-vs-grep.md`) moved into `docs/`, with relative links repointed per the plan's rule.
- `docs/servers.yaml.example`, ported from `internal/scoutengine/template.yaml` with its operator-visible prose reworded for quarry.

Quarry commit SHAs (from `git -C /home/knatte/Code/quarry/wts/quarry log --oneline`):

```
80718f8 docs(quarry): move scout research docs and add servers.yaml example
4206148 feat(quarry): copy lock, proc, and output leaf packages verbatim
db298a9 docs(quarry): README with platform set, windows caveat, and cache re-key note
197d892 chore(quarry): initial import scaffolding from loomyard 1fda8a01c13ec3ec7bb4ef056e5ec9d8aaaac5be
```

`go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...` passes: `internal/lock`, `internal/output`, `internal/proc` all green.

## Batch 2 — quarry-cli-infra

Landed in `/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch `main`), three new files added to `internal/cli/`:

- `internal/cli/cwdcontext.go` / `cwdcontext_test.go` — the context-carried cwd-injection seam ported from `internal/lyxcwd/cwdcontext.go`: `WithCwd`/`CwdFrom`, both exported. Loomyard's version routes its `os.Getwd` call through `lyxcwd`'s package-local `Getwd` wrapper, since `lyxcwd` is that repo's sole `os.Getwd` caller (the Cwd Resolution Invariant); quarry has no such invariant, so `CwdFrom` calls `os.Getwd` directly and the wrapper is not ported. `lyxcwd.Resolve` and every other hub-anchoring symbol in that package was **not** ported — quarry has no hub to resolve. This is the written antecedent for batch 4 deleting `lookupContext`'s in-hub branch: that branch exists in `internal/scoutcli/cli.go` only because `lyxcwd.Resolve` exists to call, and once the port carries no `Resolve` symbol at all, the branch it drove has nothing left to call.
- `internal/cli/exec.go` / `exec_test.go` — the `clihelp` exit-state and cobra execution seam ported from `internal/clihelp/exec.go`. Symbols carried across, verbatim in semantics: `exitState` (unexported, `code`/`abort` fields, `Code() int`), `NewExitContext`, `exitStateFromCtx`, `SetExit`, `Abort`, `ShouldAbort`, `WrapRun`, `WrapRunCtx`, `RunRootCtx`, `RunRoot`, `Execute`, `ExecuteIn`, `GroupRunE`. `ExecuteIn` seeds cwd via this package's own `WithCwd` (card 6) rather than `lyxcwd`'s. Left behind: `jsonhelp.go`'s helpers — none of scout's used surface calls them. The `init()` disabling `cobra.MousetrapHelpText` was kept, with its comment rewritten from "lyx is orchestration-only" to "quarry is a CLI never launched by double-click." `exec_test.go`'s `TestExecuteIn_HandlerObservesInjectedCwd` additionally asserts the process cwd is unchanged before and after the call, confirming no code path calls `os.Chdir`.
- `internal/cli/paths.go` / `paths_test.go` — new (not ported): `resolveConfigPath` and `resolveStateDir`, replacing `configengine.ConfigFile` and the deleted `lyxcwd.Resolve` branch per the plan's config/state/toolchain-cache path-axis decision. Each goes through a package-level function-variable seam (`userConfigDir`, `userCacheDir`), mirroring `toolchain.go`'s existing `userCacheDir` pattern in the engine. `workspaceKey` derives a short, collision-resistant state-directory name from a target directory's basename plus the first 12 hex characters of the SHA-256 of its cleaned absolute form — `paths_test.go` asserts the resulting `<state-dir>/go/daemon.sock` stays under the Linux `sockaddr_un` 108-byte limit for a realistically deep target path, since depth alone must not matter once only the basename and a fixed-length hash reach the directory name.

Quarry commit SHAs (from `git -C /home/knatte/Code/quarry/wts/quarry log --oneline`):

```
66c9673 feat(cli): resolve config and state paths with testable machine-global seams
fb35463 feat(cli): port the clihelp exit-state and cobra execution seam
7e5407d feat(cli): port the context-carried cwd-injection seam
```

`go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...` passes: `internal/cli` (new), `internal/lock`, `internal/output`, `internal/proc` all green.

## Batch 3 — port-engine

Landed in `/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch `main`).

- `tools/port/main.go`: a new, single-file, dependency-free Go program that copies a named file list from a source directory to a destination directory and rewrites exactly two closed categories of token — the loomyard import paths this task's packages depend on, and the two package clauses (`scoutengine` -> `quarry`, `scoutcli` -> `cli`) — anchored at the start of a line so a mention inside a comment or string literal is left alone. Refuses to overwrite an existing destination file without an explicit `-overwrite` flag. Deleted from quarry in batch 5 once the port is proven.
- The port program ran once, over 34 files from Loomyard's `internal/scoutengine` into quarry's `quarry/` package: 32 files landed under their source name; `scoutdaemon_test.go` and `supervised_scout_test.go` landed renamed, as `quarrydaemon_test.go` and `supervised_lsp_test.go`, since "scout" is dead vocabulary in this repo. `template.go` and `template.yaml` were deliberately excluded — their content already landed as `docs/servers.yaml.example` in batch 1, and `ConfigTemplate()` has zero call sites anywhere.
- `"scoutengine: "` literal count: **59 before, 59 after** — confirmed unchanged by the port program's own post-copy count and independently re-confirmed by `grep -rc 'scoutengine: ' quarry/` after every hand edit in this batch.
- Final `grep -ric 'lyx' quarry/*.go` count across the ported engine directory: **7**, all seven in `quarrydaemon_test.go`'s own regression-guard test (`TestDaemonStateFile_NoLyxOrScoutSegment`/`TestDaemonLock_NoLyxOrScoutSegment`), which exists specifically to assert neither `DaemonStateFile` nor `DaemonLock` ever introduces a `.lyx` or `scout` path segment. No other file carries a `lyx` occurrence.
- Hand-edited surface (everything the port program is deliberately restricted from touching):
  - Seven `internal/logger` call sites (one `logger.Info`, six `logger.Warn`, across `ensureserver.go` and `lspclient.go`) replaced with `log/slog`, message strings and structured fields kept byte-identical. `ensureserver.go` gained a package-level `defaultLogHandler` (stderr, `slog.LevelWarn` default). `lspclient.go` now imports nothing outside the standard library.
  - Two path-ownership signature changes: `DaemonStateFile`/`DaemonLock` go from `(anchorRoot, lang)` to `(stateDir, lang)`, joining only the language segment and the filename onto a told leaf directory — the `.lyx`/`scout` path segments and the `lyxdirs` import are deleted outright, not relocated. `LoadRegistry` goes from `(baseDir)` to `(path)`, reading the told file directly and dropping the `configengine` import.
  - The `Options.AnchorRoot` field rename to `Options.StateDir`, threaded through `refs.go`'s `acquireConnection` and `ensureserver.go`'s `ensureServer`/`ensureSupervised` parameter rename (`anchorRoot` -> `stateDir`).
  - The toolchain cache path segment rename from `lyx` to `quarry` in `goToolchainCacheDir`/`goToolchainInstallLock` (`toolchain.go`).
  - A full rewrite of `doc.go`, the module's as-built design record, retargeting every Loomyard-specific citation (Cwd Resolution Invariant, `internal/modelspec`, `internal/scoutcli`, `_lyx/config/servers.yaml`, `.lyx/scout/<lang>/`) onto quarry's own three-axis config/state/toolchain-cache model and quarry's own `docs/` research citations.
  - Five `//go:build scout` files recut to `//go:build lsp`, naming their actual precondition (a real language-server binary on `$PATH`) instead of a tool called scout.
  - Five test files rewritten rather than ported verbatim, each for a reason a path rewrite alone cannot fix: `daemonstate_test.go`/`quarrydaemon_test.go` (state-path arithmetic onto the told `stateDir`), `load_test.go` (registry loader onto the told config path, plus one new directory-vs-file case), `seam_enforcement_test.go`/`lspclient_guard_test.go` (banned-import lists retargeted to quarry's own packages, `lspclient_guard_test.go` tightened to stdlib-only per the logger removal).
  - `refs_integration_test.go` additionally retargeted its live-gopls fixture off the absent `internal/lyxcwd/lyxcwd.go` onto quarry's own `quarry/detect.go`'s `DetectLanguage`.
- One plan extension made mid-batch, both committed in this worktree before the corresponding quarry-side fix: `.gitignore`'s `/quarry` rule collided with the new `quarry/` package directory of the same name (fixed with a `!/quarry/` re-include, keeping the binary ignored); and `go.sum` was missing the content-hash line for `gopkg.in/yaml.v3`, which `load.go` has imported directly since the port (fixed with `go get gopkg.in/yaml.v3@v3.0.1`, no `go.mod` change).

Quarry commit SHAs (from `git -C /home/knatte/Code/quarry/wts/quarry log --oneline`, cards 10 through 22):

```
bd348aa test(quarry): retarget the engine seam and lspclient guards
1a247fb test(quarry): retarget the live references test onto Options.StateDir
f767bf1 test(quarry): retarget the registry loader test onto the told config path
f28a9db test(quarry): rewrite daemon state-path tests onto the told stateDir
9002823 test(quarry): collapse the scout build tag onto lsp
8b9688c docs(quarry): sweep lyx vocabulary out of the ported engine
2153098 refactor(quarry): rename the toolchain cache segment from lyx to quarry
09ed797 refactor(quarry): tell LoadRegistry a resolved config path
45a731a refactor(quarry): rename Options.AnchorRoot to StateDir
64bd052 refactor(quarry): tell the engine its state directory instead of deriving it
2d73374 refactor(quarry): replace internal/logger with log/slog
8fd6f90 feat(quarry): port the scout engine package mechanically
da39d84 feat(tools): add the mechanical port program
```

`go -C /home/knatte/Code/quarry/wts/quarry test ./...` passes end to end (`-count=1`): `internal/cli`, `internal/lock`, `internal/output`, `internal/proc`, and the new `quarry` package all green. `go -C /home/knatte/Code/quarry/wts/quarry vet -tags lsp ./quarry/` also type-checks cleanly, ahead of batch 5's own live-tier run.
