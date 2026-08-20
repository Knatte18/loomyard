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
