# Batch: quarry-cli-infra

```yaml
task: "Extract scout into its own standalone repo"
batch: "quarry-cli-infra"
number: 2
cards: 4
verify: go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...
depends-on: [1]
```

## Batch Scope

This batch writes the three pieces of `internal/cli/` that replace Loomyard packages with a tail — the `clihelp` exec seam, the `lyxcwd` cwd-injection seam, and the config/state path resolution that replaces `configengine.ConfigFile` and the deleted `lyxcwd.Resolve` branch.
It is one batch because all three land in one package, the exec seam's `ExecuteIn` is the sole writer of the cwd seam (so they must be designed together), and the path resolution is the seam the exec layer will hand to the engine.
None of it depends on any ported scout code, so it can be written and tested before the port runs — which is what makes batch 3's port land into a package that already compiles.

These are TDD candidates 1, 2, 3, and 4 from the discussion's Testing section: write the tests first, then the implementation.

The external interface batch 4 consumes: `cli.Execute`, `cli.ExecuteIn`, `cli.SetExit`, `cli.GroupRunE`, `cli.NewExitContext`, `cli.CwdFrom`, and the two resolvers `resolveConfigPath` and `resolveStateDir`.

Batch-local decision: this is a **replacement**, not a port, so the copied Loomyard sources are read for semantics and not mechanically transformed.
Everything Loomyard's `clihelp` carries that scout does not use — `jsonhelp.go`'s helpers — is left behind.
Everything scout does use is carried across exactly: `SetExit` has 21 production call sites and `NewExitContext`/`Code()` three test call sites, and those semantics are pinned by tests here before batch 4 repoints anything at them.

## Cards

### Card 6: the cwd-injection seam

- **Context:**
  - `internal/lyxcwd/cwdcontext.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cwdcontext.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cwdcontext_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port the context-carried cwd-injection seam into `package cli`: an unexported `cwdKey struct{}` context key, `WithCwd(ctx context.Context, dir string) context.Context`, and `CwdFrom(ctx context.Context) (string, error)`.
  Reproduce the source semantics exactly: `WithCwd` panics when `dir` is empty and panics when `dir` is not absolute (the panic message names the package as `cli` rather than `lyxcwd`), never normalizes via `filepath.Abs`, and never returns an error;
  `CwdFrom` returns the injected value when present and otherwise falls back to `os.Getwd()`, returning that call's error unchanged.
  Loomyard's version calls a package-local `Getwd` wrapper because `lyxcwd` owns the repo's sole `os.Getwd` call;
  quarry has no such invariant, so call `os.Getwd` directly and drop the wrapper.
  Both identifiers are exported even though only `CwdFrom` has call sites in ported code — `WithCwd` is `ExecuteIn`'s writer half in card 7 and the two are one mechanism.
  Write `cwdcontext_test.go` first, covering: an injected absolute cwd is read back verbatim;
  a context with no injection falls back to the process cwd;
  `WithCwd` panics on an empty string;
  `WithCwd` panics on a relative path.
  Do not port `lyxcwd.Resolve` or any other symbol from that package — quarry has no hub to resolve.
- **Commit:** `feat(cli): port the context-carried cwd-injection seam`

### Card 7: the clihelp replacement — exit state and cobra execution

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/clihelp/exec_test.go`
  - `internal/output/output.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cwdcontext.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/exec.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/exec_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port into `package cli` exactly the `clihelp` surface scout uses, plus the machinery those symbols need: the unexported `exitState` struct with its `code` and `abort` fields and its exported `Code() int` method;
  `NewExitContext(parent context.Context) (context.Context, *exitState)`;
  `exitStateFromCtx`;
  `SetExit(ctx context.Context, code int)`;
  `Abort` and `ShouldAbort`;
  `WrapRun` and `WrapRunCtx`;
  `RunRootCtx`;
  `RunRoot`;
  `Execute(cmd *cobra.Command, out io.Writer, args []string) int`;
  `ExecuteIn(cmd *cobra.Command, cwd string, out io.Writer, args []string) int`;
  and `GroupRunE(cmd *cobra.Command, args []string) error`.
  Reproduce the semantics exactly rather than reimplementing them: `SetExit` is a no-op when `code` is zero and when the context carries no exit state;
  `RunRootCtx` sets `SilenceErrors` and `SilenceUsage`, allocates a fresh exit context, and on a cobra error writes `output.Err(out, strings.TrimSpace(err.Error()))` and returns its value;
  `Execute` merges stdout and stderr into `out` before delegating to `RunRoot`;
  `ExecuteIn` does the same and then seeds cwd via this package's own `WithCwd` before delegating to `RunRootCtx`;
  `GroupRunE` returns `fmt.Errorf("unknown subcommand %q for %q", args[0], cmd.CommandPath())` for a non-empty args slice and delegates to `cmd.Help()` otherwise.
  Keep the `func init()` that sets `cobra.MousetrapHelpText = ""`, and rewrite its comment: it currently cites lyx being orchestration-only and a Loomyard timing document, and must instead say quarry is a CLI never launched by double-click.
  Import `github.com/Knatte18/quarry/internal/output` for the error envelope.
  Do not port `jsonhelp.go` — none of its symbols appear in scout's used surface.
  Write `exec_test.go` first, adapting Loomyard's own `exec_test.go` cases: `SetExit` records a non-zero code readable through `Code()`;
  `SetExit(ctx, 0)` never overwrites a recorded failure code;
  `SetExit` on a context with no exit state does not panic;
  concurrent invocations each track their own code;
  `Execute` returns the recorded code on success and 1 with a JSON error envelope on an unknown subcommand;
  `ExecuteIn` makes an injected cwd readable via `CwdFrom(cmd.Context())` inside a `WrapRunCtx` handler;
  `GroupRunE` prints help for empty args and errors for an unknown subcommand.
  The `ExecuteIn` test is TDD candidate 4's cwd-seam coverage at the boundary the seam actually arrives through, and it must also assert that no code path calls `os.Chdir`.
- **Commit:** `feat(cli): port the clihelp exit-state and cobra execution seam`

### Card 8: config and state path resolution

- **Context:**
  - `internal/scoutengine/load.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/toolchain.go`
  - `internal/scoutcli/cli.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `paths.go` in `package cli` declaring the two machine-global seams `var userConfigDir = os.UserConfigDir` and `var userCacheDir = os.UserCacheDir`, each with a doc comment saying it exists so tests can redirect resolution at a `t.TempDir()` without touching the real machine-global directory — the same seam and the same reason `toolchain.go:30` already documents in the engine.
  Declare `resolveConfigPath(flagValue string) (string, error)` implementing the precedence `flagValue` -> `$QUARRY_CONFIG` -> `filepath.Join(userConfigDir(), "quarry", "servers.yaml")`, returning the first non-empty tier.
  It resolves a path and never reads the file — an absent file is not an error at this layer, because `LoadRegistry` is what falls back to the built-in registry.
  When `userConfigDir()` itself errors, return that error rather than a path rooted at the empty string.
  Declare `workspaceKey(targetDir string) string` returning `filepath.Base(targetDir) + "-" + <first 12 hex characters of the SHA-256 of the cleaned absolute targetDir>`, and `resolveStateDir(flagValue, targetDir string) (string, error)` implementing the precedence `flagValue` -> `$QUARRY_STATE_DIR` -> `filepath.Join(userCacheDir(), "quarry", workspaceKey(targetDir))`.
  The returned state directory is the leaf directory the engine is told;
  it carries no `<lang>` segment, because `DaemonStateFile`/`DaemonLock` join that themselves.
  Write `paths_test.go` first.
  It covers TDD candidates 1 and 2: table-driven over all tiers of both chains, with the default tier exercised by assigning a `t.TempDir()` to `userConfigDir`/`userCacheDir` and restoring the original in a `t.Cleanup`, never by setting `$XDG_CONFIG_HOME` and never by `t.Chdir`;
  the flag tier beating the environment tier and the environment tier beating the default;
  `workspaceKey` deterministic across repeated calls for the same absolute directory;
  `workspaceKey` distinct for two different directories that share a basename;
  and an assertion that `filepath.Join(resolveStateDir(…), "go", "daemon.sock")` for a realistically deep target path stays under 108 bytes, the Linux `sockaddr_un` limit the supervised strategy's socket path is subject to.
  Use `t.Setenv` for the environment tiers so the test process's environment is restored automatically.
- **Commit:** `feat(cli): resolve config and state paths with testable machine-global seams`

### Card 9: record batch 2 in the port log

- **Context:** none
- **Edits:**
  - `docs/research/quarry-port-log.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a `## Batch 2 — quarry-cli-infra` section to the port log in this worktree, listing the three files added to quarry's `internal/cli/` package, the exact set of `clihelp` symbols carried across versus left behind, and the quarry commit SHAs from cards 6 through 8 obtained via `git -C /home/knatte/Code/quarry/wts/quarry log --oneline`.
  Note explicitly that `lyxcwd.Resolve` was not ported and why, so the deletion of `lookupContext`'s hub branch in batch 4 has a written antecedent.
- **Commit:** `docs: record batch 2 in the quarry port log`

## Batch Tests

`verify:` runs `go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...`, which covers the three new test files this batch writes (`cwdcontext_test.go`, `exec_test.go`, `paths_test.go`) alongside batch 1's three leaf-package suites.
The scope is right: `internal/...` is exactly what this batch touches, and the `quarry/` engine package does not exist yet.

The tests are written before their implementations, per the discussion's TDD candidates 1 through 4.
Two of them are load-bearing beyond ordinary coverage.
The `exec_test.go` `SetExit` cases pin the exit-code carrier's semantics before batch 4 repoints 21 call sites at it, so a semantic drift shows up here rather than as a wrong process exit code in the equivalence comparison.
The `paths_test.go` socket-length case guards a real pre-existing exposure: the supervised daemon's socket path is derived from the state directory, and `os.UserCacheDir()` plus a workspace key plus `<lang>/daemon.sock` can approach the 108-byte Linux limit.
