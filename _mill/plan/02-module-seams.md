# Batch: module seams

```yaml
task: Unblock t.Parallel on hub-fixture tests that currently t.Chdir
batch: module seams
number: 2
cards: 7
verify: go build ./... && go test ./internal/scoutcli/... ./internal/configcli/... ./cmd/lyx/... && go test -tags integration ./internal/loomengine/... ./internal/fabriccli/... ./internal/configcli/... ./internal/perchcli/...
depends-on: [1]
```

## Batch Scope

This batch is the risky half of the task: it moves every production cwd resolution on a CLI path off `lyxcwd.Getwd()` and onto `lyxcwd.CwdFrom(cmd.Context())`, adds `RunCLIIn` to the ten modules that resolve a cwd, threads the context through the four plain handler functions that have none today, rebases the two verbs that take a relative path as an argument (`fabric clone --into` and `scoutcli`'s four `--target-dir` defaulting points), changes `loomengine.Preflight()` to `Preflight(cwd string)`, and lands the docs and the `RunCLIIn` half of the signature-pinning assertion those changes cause.
It is one batch because the seam is only useful once all ten `RunCLIIn` functions exist, and because `Preflight`'s breaking signature change must ship together with its only two call sites or the batch does not compile under `-tags integration`.
The external interface batch 3 consumes is exactly the ten `RunCLIIn(cwd string, out io.Writer, args []string) int` functions, `loomengine.Preflight(cwd string)`, and the `lyx fabric clone --into <dir>` flag.
Batch-local decisions differing from `## Shared Decisions`: `internal/scoutcli/cli_test.go` is edited here despite being a test file, because `inFileQuery` and `parseQuery` gain a `base` parameter and `TestInFileQuery_ResolvesRelativePathToAbsolute` pins the process-cwd behaviour those functions are losing — without that edit the batch does not compile.
That file does not join batch 3's guard subject set, and its other chdir call sites stay exactly as they are.

## Cards

### Card 6: `RunCLIIn` and context-resolved cwd on the seven `PersistentPreRunE` modules

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/lyxcwd/cwdcontext.go`
- **Edits:**
  - `internal/reedcli/cli.go`
  - `internal/shuttlecli/cli.go`
  - `internal/perchcli/cli.go`
  - `internal/webstercli/cli.go`
  - `internal/idecli/cli.go`
  - `internal/boardcli/cli.go`
  - `internal/burlercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  These seven modules share one shape: a cobra `PersistentPreRunE` that calls `lyxcwd.Getwd()`, then `lyxcwd.Resolve(cwd)`, then builds the module's engine or config.
  In each, replace the single `lyxcwd.Getwd()` call with `lyxcwd.CwdFrom(cmd.Context())`, keeping the surrounding error handling — the `output.Err` write plus `clihelp.Abort(ctx, 1)` plus `return nil` — byte-for-byte as it is.
  The seven sites are `internal/reedcli/cli.go:56`, `internal/shuttlecli/cli.go:58`, `internal/perchcli/cli.go:77`, `internal/webstercli/cli.go:123`, `internal/idecli/cli.go:37`, `internal/boardcli/cli.go:71`, and `internal/burlercli/cli.go:59`.
  Where the enclosing closure already binds `ctx := cmd.Context()` above the call, pass that local rather than re-calling `cmd.Context()`.
  In each of the seven files add `RunCLIIn(cwd string, out io.Writer, args []string) int` immediately beside the existing `RunCLI`, branching on the sentinel: `cwd == ""` returns `clihelp.Execute(Command(), out, args)`, any other value returns `clihelp.ExecuteIn(Command(), cwd, out, args)`.
  Rewrite each `RunCLI(out io.Writer, args []string) int` as exactly `return RunCLIIn("", out, args)`.
  Give `RunCLIIn` a doc comment stating that `""` means "read the process cwd" and that the branch exists because `lyxcwd.WithCwd` panics on an empty directory, so a uniform delegation would panic on every existing `RunCLI` call.
  Change no other behaviour in these seven files: no flag is added, no `Short` or `Long` string changes, and every existing `clihelp.WrapRun` registration stays on `WrapRun`, since each of these modules resolves its cwd in the pre-run hook rather than in a handler.
- **Commit:** `feat(cli): add RunCLIIn and context-resolved cwd to the seven PersistentPreRunE modules`

### Card 7: `scoutcli` seam plus the argument-path rebase

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/lyxcwd/cwdcontext.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/refs.go`
- **Edits:**
  - `internal/scoutcli/cli.go`
  - `internal/scoutcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the four `lyxcwd.Getwd()` calls in `internal/scoutcli/cli.go`'s `RunE` bodies — at `:136`, `:266`, `:371`, and `:563` — with `lyxcwd.CwdFrom(ctx)`, where `ctx` is the `ctx := cmd.Context()` local each of those closures already binds.
  Update the three-line comment above each call so it no longer claims `lyxcwd.Getwd()` is the anchor.
  Rebase the four `--target-dir` defaulting points onto that seam cwd — `internal/scoutcli/cli.go:142-145`, `:272-274`, `:377-379`, and `:569-571`, each the `dir := targetDir` / `if dir == "" { dir = cwd }` block.
  After the existing default-to-`cwd` branch, add an else-branch making a supplied relative value absolute against the seam cwd: an absolute `dir` becomes `filepath.Clean(dir)`, a relative one becomes `filepath.Join(cwd, dir)`.
  All four are rebased;
  a missed one does not fail to build, it silently answers against the wrong directory.
  Rebasing here rather than at each `filepath.Abs` occurrence is deliberate: the raw value leaves the package unresolved and is finally absolutised by `rootURIFor` in `internal/scoutengine/ensureserver.go`, so only a rebase at the defaulting point reaches every consumer.
  Change `parseQuery(arg string)` at `:774` to `parseQuery(base, arg string)` and `inFileQuery(inFilePath, name string)` at `:794` to `inFileQuery(base, inFilePath, name string)`.
  In both, replace the `filepath.Abs(…)` call with the same absolute-or-join rule: an already-absolute path becomes `filepath.Clean(path)`, a relative one becomes `filepath.Join(base, path)`.
  Neither function may fall back to the process cwd, so the `filepath.Abs` error branch disappears;
  keep both return signatures as `(scoutengine.Query, error)` so the six call sites need no restructuring, and update both doc comments, which currently state that resolution happens against the process cwd.
  Pass the seam `cwd` as `base` at all five call sites: the two `buildQuery` closures (`:161` and `:163`, and `:291` and `:293`) and the direct `parseQuery(args[0])` at `:580`.
  Add `RunCLIIn` beside `RunCLI` at `:913` using the sentinel branch from card 6, and rewrite `RunCLI` as `return RunCLIIn("", out, args)`.
  In `internal/scoutcli/cli_test.go`, update the three existing call sites for the new signatures: `parseQuery(arg)` at `:219`, and `inFileQuery("internal/foo/bar.go", name)` at `:602`.
  Rewrite `TestInFileQuery_ResolvesRelativePathToAbsolute` at `:622-635` so it passes an explicit base instead of moving the process: delete its `t.Chdir(cwd)` call, keep `cwd := t.TempDir()` as the base value, call `inFileQuery(cwd, "relative/bar.go", "MyFunc")`, and keep the existing `filepath.Join(cwd, "relative/bar.go")` expectation exactly.
  That test may now call `t.Parallel()`;
  the other tests in the file keep their own chdir calls untouched, and this file does not join batch 3's guard subject set.
  Add a test proving the rebase reaches a consumer: `scoutcli.RunCLIIn(cwd, …)` invoked with a relative `--target-dir` resolves against the injected cwd rather than the process cwd, and an absolute `--target-dir` is honoured unchanged.
  Keep every added test untagged, matching the rest of `internal/scoutcli/cli_test.go`, and add no fixture build, no git spawn, and no `exec.Command` to it.
- **Commit:** `feat(scoutcli): resolve target-dir and query paths against the injected seam cwd`

### Card 8: `fabriccli` topology-verb context threading

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/lyxcwd/cwdcontext.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/unwire.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/fabriccli` is the sharp case: it holds one `lyxcwd.Getwd()` call on the topology path but eleven functions that change signature, because `resolveWarpLocation` has ten callers and every one is a `clihelp.WrapRun`-wrapped handler carrying no context today.
  Size the work from that transitive path, not from the `Getwd()` count.
  Change `resolveWarpLocation() (cwd string, l *lyxcwd.Location, err error)` at `internal/fabriccli/fabric.go:397` to `resolveWarpLocation(ctx context.Context) (cwd string, l *lyxcwd.Location, err error)`, replacing the `lyxcwd.Getwd()` call at `:398` with `lyxcwd.CwdFrom(ctx)`.
  Leave the rest of that function — the ungated `lyxcwd.ResolveWorktree` reclassification and the `fabricengine.RequireWarpWorktree` refusal — exactly as it is.
  Thread `ctx` into all ten call sites: `internal/fabriccli/fabric.go:427`, `:459`, `:485`, `:531`, `:563`, `:666`, `:691`, and `:715`;
  `internal/fabriccli/unwire.go:18`;
  and `internal/fabriccli/weft_verbs.go:76`.
  For the nine handler functions reached from a registration site — `runAdd`, `runList`, `runRemoveWithFlag`, `runCheckout`, `runPairs`, `runReconcile`, `runPruneWithFlags`, `runCleanupWithFlags`, and `runUnwire` — add a leading `ctx context.Context` parameter and pass it straight through to `resolveWarpLocation`.
  Migrate their nine registration sites in `internal/fabriccli/fabric.go` from `clihelp.WrapRun` to `clihelp.WrapRunCtx`: `:159`, `:171`, `:196`, `:227`, `:243`, `:264`, `:303`, `:346`, and `:372`.
  Each closure's parameter list becomes `func(ctx context.Context, out io.Writer, args []string) int`, and each flag read through its captured `*cobra.Command` stays exactly as written.
  In `internal/fabriccli/weft_verbs.go`, the scoped `PersistentPreRunE` at `:52` calls no `Getwd()` of its own;
  its migration is to pass `cmd.Context()` into the `resolveWarpLocation()` call at `:76`.
  This is a second, real touch point, never a double-count of the `Getwd()` swap inside `resolveWarpLocation` itself.
  Add `RunCLIIn` beside `RunCLI` at `internal/fabriccli/fabric.go:385` using the sentinel branch from card 6, and rewrite `RunCLI` as `return RunCLIIn("", out, args)`.
  The clone registration site at `:125` is card 9's and stays on `clihelp.WrapRun` until that card converts it.
  Change no `Short` or `Long` text in this card, and add no flag.
- **Commit:** `feat(fabriccli): thread the seam context through the topology verbs`

### Card 9: `lyx fabric clone --into <dir>`

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/lyxcwd/cwdcontext.go`
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Clone's cwd is a destination, not a lookup: at `internal/fabriccli/clone.go:119` the resolved directory is where the hub is created, via `CloneAndWire(cwd, …)`.
  Register a new string flag on `cloneCmd` beside the existing `--reset`, `--subpath`, and `--force-bootstrap` registrations in `internal/fabriccli/fabric.go`: `cloneCmd.Flags().String("into", "", …)`, with a description saying the flag names the directory the new hub is created in and that a relative value resolves against the current working directory, defaulting to it when omitted.
  Change `runCloneWithReset(out io.Writer, args []string, reset bool, subpath string, forceBootstrap bool) int` in `internal/fabriccli/clone.go` to take a leading `ctx context.Context` and a trailing `into string`.
  Replace its `lyxcwd.Getwd()` call at `:119` with `lyxcwd.CwdFrom(ctx)`, then derive the destination before the `CloneAndWire` call at `:133`: an empty `into` yields the seam cwd unchanged;
  an absolute `into` yields `filepath.Clean(into)`;
  a relative `into` yields `filepath.Join(seamCwd, into)`.
  A relative `--into` resolves against the seam cwd and never against the process cwd — under `RunCLIIn` those two differ by construction, and resolving against the process cwd would reintroduce the very dependency this task removes, in the one verb where cwd is a destination.
  The resolution must happen in `runCloneWithReset` before `CloneAndWire` is called, never inside `CloneAndWire`.
  Convert the clone registration site at `internal/fabriccli/fabric.go:125` from `clihelp.WrapRun` to `clihelp.WrapRunCtx`, read `into` from the captured `cloneCmd` flag set exactly as `reset`, `subpath`, and `forceBootstrap` are read, and pass both new arguments through.
  The usage string is duplicated in three places and all three carry `--into` in this card, or the CLI/Cobra Invariant's help-accuracy obligation is violated by whichever one goes stale: the cobra `Use:` line at `internal/fabriccli/fabric.go:64`, the usage-error literal returned on a wrong argument count at `internal/fabriccli/clone.go:125`, and the descriptive comment at `internal/fabriccli/fabric.go:61`.
  Add one `--into` example to the clone command's `Long` block beside the two examples already there.
  All new help text obeys the Fabric Vocabulary Invariant: `host` in any fabric sense is banned, and `warp`/`weft` appear only where the two sides must be told apart.
  Add tests to `internal/fabriccli/clone.go`'s existing coverage in batch 3 rather than here;
  this card changes production wiring only.
- **Commit:** `feat(fabriccli): add --into to fabric clone, resolved against the seam cwd`

### Card 10: `configcli` seam and the nested `fabriccli` invocation

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/lyxcwd/cwdcontext.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/configcli` resolves its cwd in two plain handler functions rather than a cobra hook, so both take the context as a parameter.
  Change `runReconcile(out io.Writer, apply bool) int` at `:255` to take a leading `ctx context.Context`, and replace the `lyxcwd.Getwd()` call at `:257` with `lyxcwd.CwdFrom(ctx)`.
  Change `runConfig(out io.Writer, args []string, printOnly bool, setFlags []string) int` at `:370` the same way, replacing its own `lyxcwd.Getwd()` call with `lyxcwd.CwdFrom(ctx)`.
  Migrate both registration sites from `clihelp.WrapRun` to `clihelp.WrapRunCtx`: the `configCmd.RunE` assignment at `:325` and the `reconcileCmd.RunE` assignment at `:343`.
  Each closure's parameter list becomes `func(ctx context.Context, out io.Writer, args []string) int`;
  the captured flag reads stay exactly as written.
  Change the production `realSync` closure at `:382-384` so its nested call carries the caller's already-resolved cwd: `fabriccli.RunCLI(w, []string{"sync"})` at `:383` becomes `fabriccli.RunCLIIn(cwd, w, []string{"sync"})`, capturing the `cwd` local `runConfig` has already resolved.
  Letting the nested call re-derive cwd from process state is precisely the bug being removed.
  Add `RunCLIIn` beside `RunCLI` at `:356` using the sentinel branch from card 6, and rewrite `RunCLI` as `return RunCLIIn("", out, args)`.
  `dispatch`'s signature is unchanged: it is already given an explicit layout, and the injected-sync closure remains its seam.
- **Commit:** `feat(configcli): resolve cwd from the seam context and thread it into the nested fabric call`

### Card 11: `loomengine.Preflight` is told its cwd

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/export_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `Preflight() (Report, error)` in `internal/loomengine/preflight.go` to `Preflight(cwd string) (Report, error)`, and delete the `lyxcwd.Getwd()` call at `:36` along with the two-line comment above it, using the parameter directly as the input to `lyxcwd.Resolve(cwd)`.
  Every other branch of the function — the `lyxcwd.ErrNotAGitRepo` short-circuit yielding a single `CheckGeometry` failure, the escalating error path, and the `checkResolved(l)` tail — stays exactly as it is.
  Update the doc comment so it no longer implies the function reads the process cwd.
  This is the one breaking signature change in the task, so its two call sites ship in this card or the batch does not compile under `-tags integration`.
  In `internal/loomengine/preflight_integration_test.go`, change `loomengine.Preflight()` at `:192` to pass the `dir` local that test already builds, and `loomengine.Preflight()` at `:229` to pass the `sub` local that test already builds.
  Leave the `os.Chdir` calls at `:188` and `:225`, the `restoreCwd` helper at `:106`, and every other line of that file untouched in this card — chdir removal, `restoreCwd` deletion, and `t.Parallel()` are batch 3's card 16.
  Rewrite the doc comments in `internal/loomengine/export_test.go`: both the file comment and `CheckResolvedForTest`'s own comment justify the shim by `Preflight()`'s `lyxcwd.Getwd()` dependency, which this card removes, so leaving them would be a live contradiction.
  Keep the shim itself — it still serves tests that build a synthetic `*lyxcwd.Location` with no backing directory on disk — and state that as the rewritten rationale.
- **Commit:** `refactor(loomengine): Preflight takes its cwd instead of reading the process cwd`

### Card 12: pin `RunCLIIn` and amend the CLI/Cobra Invariant

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/selfreportcli/cli.go`
  - `internal/reedcli/cli.go`
- **Edits:**
  - `cmd/lyx/seamsignature_test.go`
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Append the second half of the compile-time assertion to `cmd/lyx/seamsignature_test.go`: a `var _ = []func(string, io.Writer, []string) int{…}` listing `boardcli.RunCLIIn`, `burlercli.RunCLIIn`, `configcli.RunCLIIn`, `fabriccli.RunCLIIn`, `idecli.RunCLIIn`, `perchcli.RunCLIIn`, `reedcli.RunCLIIn`, `scoutcli.RunCLIIn`, `shuttlecli.RunCLIIn`, and `webstercli.RunCLIIn` — ten entries, not eleven.
  Carry a comment naming `internal/selfreportcli` as the one module deliberately absent, because it references `lyxcwd` nowhere and a `RunCLIIn` there would accept a cwd argument nothing reads.
  Remove the batch-1 note in that file's doc comment saying the `RunCLIIn` half lands later.
  In `CONSTRAINTS.md`'s CLI / Cobra Invariant, amend the Seam bullet to name both shapes: `RunCLI(out io.Writer, args []string) int` and `RunCLIIn(cwd string, out io.Writer, args []string) int`, with `RunCLI` delegating as `RunCLIIn("", out, args)` and the empty string meaning "read the process cwd".
  State that ten of the eleven seam modules carry `RunCLIIn` and name the exempt one with its reason.
  Do not restate the Cwd Resolution Invariant bullet batch 1 already added.
  In `docs/overview.md`, extend the same "Module dispatch" paragraph batch 1 touched so the stated seam is no longer only `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`, naming `RunCLIIn` and its `clihelp.ExecuteIn` delegation.
  Every edited markdown line uses semantic line breaks — one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary.
  Add no new inline markdown link unless it resolves, anchor included.
- **Commit:** `docs: amend the CLI/Cobra Invariant for the RunCLIIn seam`

## Batch Tests

`verify:` runs three gates in sequence, scoped to this batch rather than repo-wide.
`go build ./...` is the fastest possible check that ten modules' production wiring still compiles after nine handler signatures and one engine signature changed.
`go test ./internal/scoutcli/... ./internal/configcli/... ./cmd/lyx/...` covers the untagged surface this batch edits: `internal/scoutcli/cli_test.go`'s updated `parseQuery`/`inFileQuery` cases and the new relative-`--target-dir` test from card 7, `internal/configcli`'s untagged tests, and `cmd/lyx`'s guard suite including the compile of the amended `cmd/lyx/seamsignature_test.go` and `TestEnforcement_MarkdownLinks` over the two edited doc files.
`go test -tags integration ./internal/loomengine/... ./internal/fabriccli/... ./internal/configcli/... ./internal/perchcli/...` is required rather than optional: card 11 edits the integration-tagged `internal/loomengine/preflight_integration_test.go`, and `-tags integration` is the only way its two updated `Preflight(cwd)` call sites are compiled at all.
`internal/fabriccli`, `internal/configcli`, and `internal/perchcli` are included because their integration suites drive the seam through `RunCLI`, which now delegates through `RunCLIIn` and `clihelp.Execute` — the fastest proof that the `cwd == ""` sentinel path is behaviour-preserving for every existing caller.
The module-wide `go vet -tags integration ./...` in the overview frontmatter runs after this batch's own gate and type-checks every remaining integration-tagged test file repo-wide, catching any call site of a changed signature the four named packages miss.
The scenario proving the injected path and the fallback path agree — `RunCLIIn(cwd, …)` and `RunCLI(…)` from a process standing in that same cwd producing identical output and exit code — is written in batch 3 alongside the migrated fixtures that can build a real hub to stand in.
