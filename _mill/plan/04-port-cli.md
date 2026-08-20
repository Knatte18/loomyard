# Batch: port-cli

```yaml
task: "Extract scout into its own standalone repo"
batch: "port-cli"
number: 4
cards: 9
verify: go -C /home/knatte/Code/quarry/wts/quarry test ./...
depends-on: [3]
```

## Batch Scope

This batch ports the cobra wiring, replaces the one function that cannot be ported — `lookupContext`, where lyx-specific path resolution enters — with resolution built on batch 2's `internal/cli/paths.go`, adds the `--config` and `--state-dir` flags the new precedence chains need, writes the binary's entry point, and rewrites the two test files whose isolation premise or whose subject this task deletes.
It is one batch because every card is in or immediately around `internal/cli/cli.go`, and because the flag surface, the resolver, and the tests that cover them cannot be reviewed apart.

At the end of this batch quarry is a working binary: `quarry refs`, `definition`, `symbol`, and `assert-no-callers` do what `lyx scout` does, and batch 5 proves it.

Batch-local decisions:

- `testmain_test.go` is **not** ported. Its whole body is `gitkit.HermeticGitEnv()` guarding against the operator's global gitconfig leaking into git-spawning fixtures, and quarry spawns no git at all. The file disappears rather than being reduced to a stub.
- `cli_integration_test.go` is **not** ported either. It covers `lookupContext`'s in-hub branch, and that branch is deleted by this task, so a `t.TempDir()` swap would keep a test whose subject no longer exists. Card 31 replaces it with coverage of the resolution that now occupies that seam.
- The four verbs, their flags, their `Long` help semantics, and the JSON envelope keep their exact shape. Only the binary name changes, and the two new path flags are added. A verb rename or an envelope-field change here would invalidate batch 5's comparison.

## Cards

### Card 24: run the port over the CLI package

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/tools/port/main.go`
  - `internal/scoutcli/cli.go`
  - `internal/scoutcli/cli_test.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the port program over Loomyard's `internal/scoutcli` into quarry's `internal/cli/` for exactly two files: `cli.go` and `cli_test.go`.
  Do not port `testmain_test.go` or `cli_integration_test.go`, for the reasons this batch's `## Batch Scope` gives.
  Do not hand-transcribe either file.
  After the run, confirm both files declare `package cli` and that no `Knatte18/loomyard` import path survives in them.
  The package will not compile at the end of this card, because the ported files still reference `clihelp.`, `lyxcwd.`, `output.`, and `scoutengine.` qualifiers that card 25 repoints.
  That is expected.
- **Commit:** `feat(cli): port the scout CLI package mechanically`

### Card 25: repoint the ported CLI at its new dependencies

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/exec.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cwdcontext.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/errors.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the import block and every qualified reference so the file compiles against quarry's packages.
  The `clihelp` and `lyxcwd` imports are dropped entirely: `clihelp.SetExit` becomes a bare `SetExit` at all 21 call sites, `clihelp.GroupRunE` becomes a bare `GroupRunE`, and `lyxcwd.CwdFrom` becomes a bare `CwdFrom` at its four call sites, because those symbols now live in this same package from batch 2.
  Keep the `github.com/spf13/cobra` import.
  Change the `internal/output` import to `github.com/Knatte18/quarry/internal/output`, leaving its 14 `output.Err` and 7 `output.Ok` call sites otherwise untouched.
  Change the engine import to `github.com/Knatte18/quarry/quarry` and rename every `scoutengine.` qualifier to `quarry.`.
  Leave `lyxcwd.Resolve`'s call site alone in this card — card 26 deletes the function that contains it, and touching it here would produce a conflicting edit.
  Do not change any handler body, any error-mapping decision, or any exit code in this card;
  it is an import-and-qualifier repointing only.
- **Commit:** `refactor(cli): repoint the ported CLI at quarry's own packages`

### Card 26: replace lookupContext with told-path resolution

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/load.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/registry.go`
  - `internal/scoutcli/cli.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `lookupContext(cwd, dir string) (quarry.Registry, string, error)` and the last `lyxcwd.Resolve` reference with it, and replace it with `resolveContext(cwd, dir, configFlag, stateDirFlag string) (quarry.Registry, string, string, error)` returning the registry, the absolute target directory, the resolved state directory, and an error.
  Its body: resolve the absolute target directory exactly as the old out-of-hub branch did — `filepath.Abs(dir)`, falling back to `filepath.Clean(dir)` when `filepath.Abs` itself fails, preserving that failure mode byte for byte;
  call `resolveConfigPath(configFlag)` and pass its result to `quarry.LoadRegistry`, returning any load error unchanged so a malformed `servers.yaml` still fails the lookup rather than degrading silently;
  and call `resolveStateDir(stateDirFlag, <the absolute target directory>)` for the state directory.
  The in-hub branch is deleted outright, not preserved behind a condition: quarry has no hub, so the out-of-hub branch becomes the only branch.
  Note that `dir` is already the caller-normalized value — the four call sites pass the resolved cwd when `--target-dir` is empty rather than the raw flag, and that must not change, or an omitted `--target-dir` would resolve the process working directory instead of the seam cwd.
  Rewrite the function's doc comment completely;
  it currently explains hub anchoring, `loc.AnchorPath()`, and why the registry is anchored rather than hub-rooted, none of which survives.
- **Commit:** `refactor(cli): resolve config and state paths instead of a lyx hub`

### Card 27: thread the new flags and the renamed Options field

- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two persistent string flags on the root command so all four verbs inherit them: `--config`, documented as an explicit path to a `servers.yaml` overlay that overrides `$QUARRY_CONFIG` and the user config directory default;
  and `--state-dir`, documented as an explicit daemon state directory that overrides `$QUARRY_STATE_DIR` and the user cache directory default.
  Both default to the empty string, which means "fall through to the next precedence tier".
  Thread both values into the four `resolveContext` call sites in `refsCommand`, `definitionCommand`, `symbolCommand`, and `assertNoCallersCommand`, replacing the current two-value `lookupContext` destructuring.
  Change `buildOptions`'s third parameter from `anchorRoot string` to `stateDir string` and the struct literal field it sets from `AnchorRoot:` to `StateDir:`, matching batch 3 card 14's rename;
  update its doc comment, which says the function exists to thread `AnchorRoot` consistently.
  Do not add flags for the toolchain cache directory — that axis stays engine-derived, and a flag nobody would set is exactly what the `toolchain-cache-is-a-third-axis-and-stays-engine-derived` decision rejects.
- **Commit:** `feat(cli): add --config and --state-dir and thread Options.StateDir`

### Card 28: rename the command tree from scout to quarry

- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the root command's `Use` from `"scout"` to `"quarry"`, keeping its `Short` text intact apart from any lyx vocabulary.
  Rewrite every `lyx scout <verb>` example inside the four verbs' `Long` help text to `quarry <verb>` — these are operator-visible and there are many of them, concentrated in `refsCommand`'s `Long`.
  Keep every other word of that help text exactly as written: the batch-mode envelope description, the `"resolution":"complete"` trust-marker paragraph, and the gopls interface-method limitation paragraph with its `--within` explanation are all accurate for quarry and are the tool's documentation of its own known limitation.
  Keep `RunCLI(out io.Writer, args []string) int` and `RunCLIIn(cwd string, out io.Writer, args []string) int` exactly as they are, including the empty-cwd branch and its comment explaining that `WithCwd` panics on an empty directory.
  They are what `cli_test.go`'s 79 call sites drive, and they are the in-process seam a second front-end would use;
  rewrite only the comment where it calls them "the scout module CLI".
  Rewrite the package doc comment and the file header comment, which describe the package as wiring the engine into the lyx cobra tree as the "scout" module;
  the replacement describes it as quarry's own command tree.
- **Commit:** `feat(cli): rename the command tree from scout to quarry`

### Card 29: the binary entry point

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/exec.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/cmd/quarry/main.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write a thin `package main` whose `main` calls `os.Exit(cli.RunCLI(os.Stdout, os.Args[1:]))`, importing `github.com/Knatte18/quarry/internal/cli`.
  It contains no flag parsing, no path resolution, and no cobra construction of its own — all of that belongs to `cli.Command()`, and duplicating any of it here would give quarry two places where the CLI surface is defined.
  Give the file a header comment saying so, and naming `cmd/quarry-mcp` as the intended future peer that will consume the same engine through a different front-end rather than by shelling out to this binary.
  Confirm `go -C /home/knatte/Code/quarry/wts/quarry build ./cmd/quarry` produces a binary and that running it with no arguments prints the four subcommands and exits 0.
- **Commit:** `feat(cmd): add the quarry binary entry point`

### Card 30: rewrite cli_test.go's isolation onto the seams

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths.go`
  - `internal/scoutcli/cli_test.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This file calls `t.Chdir(t.TempDir())` at six sites to make `lyxcwd.Resolve` fail so the CLI degrades to built-in servers.
  That premise is deleted by this task, and `os.UserConfigDir`/`os.UserCacheDir` ignore the working directory entirely, so ported unchanged these tests would resolve the operator's real `servers.yaml` and pass or fail depending on the developer's machine.
  Replace every one of the six `t.Chdir` calls with redirection of the `userConfigDir` and `userCacheDir` seams to a `t.TempDir()`, restoring the originals in a `t.Cleanup`.
  Write a small helper in this file that does both redirections at once and returns the temp root, so the six sites share one isolation mechanism rather than six copies.
  Every test's actual subject — the bare and `--help` subcommand listings, each command's `Short`, flag parsing, verb wiring, the `ErrNoLanguage` error-envelope path, and the exit codes — is unaffected by this task and must be preserved exactly.
  Repoint the two `clihelp.` references in the import block and their call sites at this package's own symbols, and the `scoutengine.` references at `quarry.`.
  Rewrite the header comment, which cites the `//go:build integration` tier and "batch 4's measurement" from a Loomyard task.
  Do not add `t.Chdir` anywhere in the rewritten file.
- **Commit:** `test(cli): isolate cli tests via the userConfigDir and userCacheDir seams`

### Card 31: cover the resolution that replaced the hub branch

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/paths.go`
  - `internal/scoutcli/cli_integration_test.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/resolve_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write a new untagged test file covering `resolveContext`, the function that now occupies the seam Loomyard's `cli_integration_test.go` covered.
  That source file built a real lyx hub to exercise `lookupContext`'s in-hub branch;
  its subject is deleted, so this is a replacement rather than a port, and it needs no build tag because it spawns nothing.
  Cover: an explicit `--config` value wins over `$QUARRY_CONFIG` and over the user config directory default;
  a `servers.yaml` written at the resolved path whole-replaces the built-in entry for that language in the returned registry;
  an absent file at every tier returns the built-in registry with no error;
  a malformed file returns an error rather than degrading to built-ins, which is the one case where the loader does not swallow the failure;
  an explicit `--state-dir` value wins over `$QUARRY_STATE_DIR` and over the user cache directory default;
  the returned target directory is the absolute form of the `dir` argument;
  and the returned state directory carries no `.lyx` and no `scout` segment.
  Isolate with the `userConfigDir`/`userCacheDir` seams and `t.Setenv`, never `t.Chdir`.
  This is the end-to-end half of TDD candidates 1 and 2 — batch 2's `paths_test.go` covers the resolvers in isolation, and this covers them wired into the registry load.
- **Commit:** `test(cli): cover resolveContext's config and state resolution`

### Card 32: sweep lyx vocabulary out of the ported CLI files

- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli_test.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run `grep -nic 'lyx' /home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go /home/knatte/Code/quarry/wts/quarry/internal/cli/cli_test.go` and rewrite or justify every remaining hit that cards 25 through 31 did not already remove.
  Expect the residue to be comments referencing lyx modules by name, the seam comments describing what `RunCLIIn` does for `cmd/lyx`, and any surviving `lyx scout` string in an error message or a test fixture.
  Then run `grep -ric 'lyx' /home/knatte/Code/quarry/wts/quarry/` across the whole repo and confirm the total is zero, which is the check the `error-prefixes-stay-verbatim-through-the-port` decision requires and which cards 17 and 21 could not complete on their own.
  Do not touch the `"scoutengine: "` literals — they contain no `lyx` token and they must survive verbatim into batch 5's comparison.
- **Commit:** `docs(cli): finish the lyx vocabulary sweep across quarry`

### Card 33: record batch 4 in the port log

- **Context:** none
- **Edits:**
  - `docs/research/quarry-port-log.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a `## Batch 4 — port-cli` section to the port log in this worktree.
  Record the two files the port program copied and the two it deliberately skipped with the one-line reason for each skip;
  the shape of `resolveContext` that replaced `lookupContext`;
  the two flags added;
  the repo-wide `grep -ric 'lyx'` total from card 32, which must be zero;
  and the quarry commit SHAs from cards 24 through 32.
  State that quarry is now a working binary and that its behaviour is unproven until batch 5's comparison.
- **Commit:** `docs: record batch 4 in the quarry port log`

## Batch Tests

`verify:` runs `go -C /home/knatte/Code/quarry/wts/quarry test ./...`, the same hermetic tier batch 3 established, now also covering this batch's two `internal/cli` suites: the rewritten `cli_test.go` and the new `resolve_test.go`.
The scope stays `./...` because this batch adds a new main package (`cmd/quarry`) whose compilation is part of what needs proving, and because `internal/cli` is a package every other package's tests already link against.

`cli_test.go` is the largest single body of coverage in the batch — 79 `RunCLI` call sites over flag parsing, verb wiring, and exit codes — and card 30's rule is that none of its subjects change.
A failure there after card 30 means the isolation rewrite altered a subject, not that the seam swap was wrong.

`resolve_test.go` is the one genuinely new test in the batch, and it is deliberately untagged: the hub fixture that forced its Loomyard predecessor into the integration tier is gone, and nothing it does spawns a process or touches the network.
The live tier is not run here — `refs_integration_test.go` and the four other `//go:build lsp` files stay uncompiled by this gate and are batch 5's job.
