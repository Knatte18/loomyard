# Batch: test migration and guard

```yaml
task: Unblock t.Parallel on hub-fixture tests that currently t.Chdir
batch: test migration and guard
number: 3
cards: 6
verify: go test ./cmd/lyx/... && go test -tags integration ./internal/fabriccli/... ./internal/perchcli/... ./internal/configcli/... ./internal/webstercli/... ./internal/idecli/... ./internal/reedcli/... ./internal/loomengine/...
depends-on: [2]
```

## Batch Scope

This batch is the mechanical half: it migrates all 39 removable `.Chdir` occurrences across the eight integration-tagged target files onto the seam batch 2 built, adds `t.Parallel()` to the three files whose only blocker was chdir, adds the regression guard banning both chdir spellings across a named per-file subject set, and records the timing row and the `LYX_TRACE=1` disposition.
It is one batch because every card performs the same transformation against the same new API and shares the same governing rule — a migration must never silently weaken an assertion — and because the guard in card 17 can only be written once every file it names is migrated.
It consumes batch 2's interface and exposes nothing new to a later batch.
Batch-local decision differing from `## Shared Decisions`: card 17's guard subject set is per-file and never per-package, because the eleven packages gaining a seam change carry eleven further non-smoke chdir-using test files this task deliberately does not touch, plus twelve deferred smoke files;
a per-package subject would make the allowlist larger than the guarded set, which inverts the point of a guard.

## Cards

### Card 13: migrate `internal/fabriccli/cli_test.go`

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/clone.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Remove all 18 `t.Chdir` calls in `internal/fabriccli/cli_test.go` — at `:41`, `:89`, `:165`, `:315`, `:354`, `:404`, `:493`, `:579`, `:609`, `:659`, `:680`, `:716`, `:738`, `:770`, `:801`, `:829`, `:851`, and `:905` — replacing each with the equivalent explicit cwd, and never weaken an assertion in the process.
  The plain lookup sites — `:41`, `:165`, `:770`, `:829`, `:851`, `:905`, plus `:315` and `:354` — pass `h.PrimeWorktree()` as the first argument to `fabriccli.RunCLIIn` in place of the chdir.
  The five clone-destination sites — `:493`, `:579`, `:609`, `:659`, and `:716` — move onto the new `--into` flag instead, passing `cloneParent` as its value, because at those sites the directory is where the hub is created rather than a lookup.
  `:659` and `:716` each use both seams and need one `--into` argument and one per-call `RunCLIIn` cwd, not a choice between them: `:659` chdirs into `cloneParent` for a `clone`, then `:680` chdirs into `filepath.Join(hubPath, "reconcilecli-warp")` for a `reconcile`;
  `:716` and `:738` are the same pairing against `reconcilecli-fail-warp`.
  Pass the cwd per call rather than hoisting it to a per-test variable — proving cwd is a per-call value and not a per-process one is exactly what these two tests exist to demonstrate after this change.
  At `:801` the cwd's classification is the assertion: the test drives the weft-sibling refusal path through `fabricengine.RequireWarpWorktree`, so pass `h.PrimeWeft()` as the `RunCLIIn` cwd argument.
  The refusal must still fire and the assertion is preserved, never weakened to a generic error check.
  At `:89` and `:404` the chdir moves the process into a bare `t.TempDir()` to exercise an error path — the `unknown` subcommand and clone's usage error.
  At `:89` the `PersistentPreRunE` guard returns before any resolution, so that chdir is already dead weight and is deleted outright with no replacement;
  at `:404` pass the temp directory as the `RunCLIIn` cwd so the usage-error path is still reached from a non-hub directory.
  Do not add `t.Parallel()` anywhere in this file: `:324` and `:363` call `t.Setenv("WEFT_SKIP_PUSH", "1")`, which panics under `t.Parallel()` exactly as `t.Chdir` does.
  Add a comment at the top of each of those two test functions naming the `WEFT_SKIP_PUSH` `t.Setenv` call as the remaining blocker, so the deferred work is documented where the next reader will look for it.
  Add one test to this file covering the scenario that proves the fallback and injected paths agree: `fabriccli.RunCLIIn(h.PrimeWorktree(), …)` and `fabriccli.RunCLI(…)` driven from a process standing in that same directory produce identical output and exit code for one read-only verb.
  Add one test covering clone with `--into <dir>` creating the hub at that directory, and one covering clone without `--into` still creating it at the resolved cwd, or the flag's default is untested.
  The file stays `//go:build integration`;
  add no untagged fixture use anywhere.
- **Commit:** `test(fabriccli): drive the CLI at an explicit cwd instead of moving the process`

### Card 14: migrate the two `internal/perchcli` integration files

- **Context:**
  - `internal/perchcli/cli.go`
  - `internal/perchengine/identity.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/perchcli/cli_integration_test.go`, delete the `t.Chdir(h.PrimeWorktree())` call inside the shared `seedPerchFixture` helper at `:46` and the `t.Chdir(h.Location.AnchorPath())` call at `:96`, and convert all four `RunCLI` call sites in the file to `RunCLIIn` with an explicit cwd.
  The helper's four callers pass `h.PrimeWorktree()`;
  `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` passes `h.Location.AnchorPath()`, which is a non-`"."` anchor and is not the same directory as `PrimeWorktree()` — passing the wrong one silently changes what that test asserts.
  `TestRunCLI_Pause_InvalidRunID` asserts `os.Stat(filepath.Join("..", "..", "escaped"))`, deliberately relative to the chdir'd hub, proving `--run-id ../../escaped` did not escape the runs area.
  Rewrite that assertion against an absolute path derived from the hub — `filepath.Join(h.PrimeWorktree(), "..", "..", "escaped")` — so it survives the chdir's removal, and change `seedPerchFixture(t)` to `h := seedPerchFixture(t)` at `:24` so the hub is in scope.
  Confirm the rewritten form is not vacuous by planting a deliberate escape locally once before committing the final form;
  do not commit the planted escape.
  Add `t.Parallel()` as the first statement of every test function in `internal/perchcli/cli_integration_test.go` — this file is one of the three whose only blocker was chdir.
  Write the call explicitly in each test function, never inside `seedPerchFixture`, so the parallelism stays visible at the test it governs rather than hidden in a setup helper.
  In `internal/perchcli/run_integration_test.go`, remove all four chdir calls — `:36`, `:90`, `:157`, and `:225` — passing the equivalent directory to `RunCLIIn` at each corresponding call site.
  `:157` chdirs into `h.Location.AnchorPath()`, a non-`"."` anchor: pass that path, not `PrimeWorktree()`.
  This file stays serial and gains no `t.Parallel()` call, because `:33`, `:87`, `:149`, and `:222` call `t.Setenv("WEFT_SKIP_PUSH", "1")`.
  Add a comment at the top of each of its test functions naming that `t.Setenv` call as the remaining blocker.
  Both files stay `//go:build integration`.
- **Commit:** `test(perchcli): drive the CLI at an explicit cwd and parallelize the pause suite`

### Card 15: migrate the three remaining serial integration files

- **Context:**
  - `internal/configcli/configcli.go`
  - `internal/webstercli/cli.go`
  - `internal/idecli/cli.go`
  - `internal/ideengine/spawn.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/idecli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  These three files get every chdir removed but stay serial, each carrying a comment naming its own remaining blocker.
  In `internal/configcli/configcli_integration_test.go`, delete the `t.Chdir(warpWorktreePath)` call at `:55` and the `os.Chdir`/`defer os.Chdir` pair at `:187` and `:190` together with the `oldCwd` local they use.
  The `:55` site is not a `configcli` cwd dependence at all: `dispatch` is already given an explicit layout at `:78`, and the cwd is consumed inside the `injectedSync` closure at `:72-74`, whose `fabriccli.RunCLI(w, []string{"commit"})` call sits at `:73` — change that one call to `fabriccli.RunCLIIn(warpWorktreePath, w, []string{"commit"})`.
  Keep the verb as `commit` rather than `sync`: the comment at `:70-71` records that `sync` calls a detached push child that cannot run in-process, and changing the verb would change what the test proves.
  At `:187-190` the chdir exists so the CLI's own resolution lands on the temp repo — convert `RunCLI(&reconcileOut, []string{"reconcile"})` at `:192` to `RunCLIIn(tmpDir, &reconcileOut, []string{"reconcile"})`.
  This file stays serial: `:58` and `:59` call `t.Setenv`;
  comment that as the blocker.
  In `internal/webstercli/verbs_test.go`, delete the `t.Chdir(h.PrimeWorktree())` call at `:692` inside `seedPersistentPreRunFixture` and convert that helper's two `RunCLI(&out, []string{"status"})` callers at `:706` and `:729` to `RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})`, returning the hub from the helper where it is not already captured.
  Update the helper's doc comment, which currently says it chdirs into the prime warp worktree.
  This file stays serial: it calls `t.Setenv("WEFT_SKIP_GIT", …)` at `:332`, `:370`, `:401`, `:506`, and `:567`;
  comment that as the blocker on the two affected test functions.
  In `internal/idecli/cli_test.go`, remove the `t.Chdir(h.PrimeWorktree())` calls at `:24` and `:121` and the bare `os.Chdir` at `:95` with its manual restore at `:98`, converting each corresponding `RunCLI` call to `RunCLIIn` with `h.PrimeWorktree()` or the temp directory respectively.
  This file stays serial even though it calls no `t.Setenv`: `TestRunCLISpawnDispatch` swaps the package-level `ideengine.CodeLauncher` at `:27-29` and restores it in a `defer`, which under `t.Parallel()` is both a data race on a production package-level variable and a restore firing while sibling tests still run.
  Comment that seam swap as the blocker on that test function.
  Add no `t.Parallel()` call to any of these three files.
  All three stay `//go:build integration`.
- **Commit:** `test(cli): drive the serial integration suites at an explicit cwd`

### Card 16: migrate and parallelize `internal/reedcli` and `internal/loomengine`

- **Context:**
  - `internal/reedcli/cli.go`
  - `internal/loomengine/preflight.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/reedcli/cli_integration_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedcli/cli_integration_test.go`, remove all four `t.Chdir(h.PrimeWorktree())` calls — at `:26`, `:53`, `:76`, and `:112` — converting each test's `RunCLI` call to `RunCLIIn(h.PrimeWorktree(), …)`.
  Add `t.Parallel()` as the first statement of every migrated test function in the file.
  In `internal/loomengine/preflight_integration_test.go`, delete the `os.Chdir(dir)` call at `:188` and the `os.Chdir(sub)` call at `:225` — batch 2 already changed both call sites to pass their directory to `Preflight(cwd)` directly, so both chdirs are now dead.
  Delete the `restoreCwd` helper at `:106` along with its `os.Chdir(orig)` call at `:117` and every `restoreCwd(t)` invocation, which become dead with the last chdir;
  its `t.Cleanup` LIFO doc comment goes with it.
  Add `t.Parallel()` as the first statement of every migrated test function in the file, including `TestPreflight_NotAGitRepo` and `TestPreflight_SubpathAnchoredHubIsNotRejectedForItsAnchor`.
  Preserve both assertions exactly: `Preflight(dir)` against a non-git directory still yields exactly `CheckGeometry`, and `Preflight(sub)` against a subpath-anchored hub still treats the anchor as legal geometry rather than reporting a geometry failure.
  Both files were verified during the discussion's per-file safety audit: neither assigns to a cross-package exported variable and neither calls `t.Setenv` or `os.Setenv`, so the chdir removed here was their only process-global mutation.
  Re-run that same per-file sweep — cross-package variable assignment and environment mutation — before adding `t.Parallel()`, and do not rely on a package-level safety claim, because the blocker can live in a production package the test stubs.
  Both files stay `//go:build integration`.
- **Commit:** `test(reedcli,loomengine): remove the chdirs and parallelize both suites`

### Card 17: the chdir regression guard

- **Context:**
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/perchcli/cli_integration_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/idecli/cli_test.go`
  - `internal/reedcli/cli_integration_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `CONSTRAINTS.md`
- **Creates:**
  - `cmd/lyx/cwdmutation_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `cmd/lyx/cwdmutation_test.go`, untagged, package `main`, following the house pattern every package-walking guard in this repo already uses — resolve the module root via `go env GOMOD`, skip cleanly when the go toolchain is absent, walk with `filepath.WalkDir`, and report every violation rather than stopping at the first.
  Model its structure and its allowlist-with-a-reason style on `cmd/lyx/tierpurity_test.go`.
  The guard bans both `t.Chdir(` and `os.Chdir(` as raw substrings.
  Banning `os.Chdir` as well is what catches the wrapper pattern already present in these files (`restoreCwd`, `mustChdir`), which a `t.Chdir`-only ban would miss.
  The subject set is an explicitly named per-file list, never a package prefix: `internal/fabriccli/cli_test.go`, `internal/perchcli/run_integration_test.go`, `internal/perchcli/cli_integration_test.go`, `internal/configcli/configcli_integration_test.go`, `internal/webstercli/verbs_test.go`, `internal/idecli/cli_test.go`, `internal/reedcli/cli_integration_test.go`, `internal/loomengine/preflight_integration_test.go`, and `internal/fabricengine/coalesce_integration_test.go`.
  Only files on that list are scanned;
  every file off it is outside the guard entirely and carries no allowlist entry, so the guard stays silent about work this task chose not to do.
  The list carries exactly one allowlist entry, `internal/fabricengine/coalesce_integration_test.go`, with the reason `cwd is the assertion: TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd pins gitrepo.New("") against a non-git process cwd`.
  Document the growth rule in the file's doc comment: a file joins the subject set when it is migrated, never by default.
  Add a test proving the guard is not vacuous, mirroring how `cmd/lyx/tierpurity_test.go` carries its banned tokens as its own test data: assert the guard's matcher fires on a planted violation string and stays silent for the allowlisted file.
  Add `cmd/lyx/cwdmutation_test.go` to `allowedSpawners` in `cmd/lyx/tierpurity_test.go` with a one-line reason, because the new file resolves its scan root via `go env GOMOD` and therefore contains `exec.Command` — the same reason four sibling guards on that map already carry.
  In `CONSTRAINTS.md`'s Cwd Resolution Invariant, add a bullet naming `cmd/lyx/cwdmutation_test.go`, its two banned spellings, its named per-file subject set, its single allowlisted exemption, and the growth rule.
  Use semantic line breaks throughout the markdown edit.
- **Commit:** `test(cmd/lyx): guard the migrated files against both chdir spellings`

### Card 18: record the measurements and the `LYX_TRACE` disposition

- **Context:**
  - `internal/logger/sink.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/benchmarks/running-tests.md`
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Run the verification protocol before writing either file: the packages this batch touched, with `-race -count=2`, under both `-tags integration` and untagged, before and after the migration.
  `-race` covers the parallel-safety of the three newly parallelized files;
  it does not catch a cwd dependence removed incorrectly, because the process working directory is not race-detectable memory. `-count=2` catches fixture-teardown ordering bugs that only surface on a second run in the same binary.
  Add a `LYX_TRACE=1` subsection to `docs/benchmarks/running-tests.md` recording the durable-sink disposition honestly.
  `ensureDurableSink` short-circuits under `testing.Testing()` unless `LYX_TRACE=1`, and its `sinkOnce` is process-wide, so the first call to log in a test binary pins the trace directory for every subsequent call in that process.
  Per-test, hub-accurate trace directories were therefore never delivered: before this task the sink resolved against whichever fixture hub happened to log first, which is arbitrary;
  after it, the sink resolves against the repo worktree instead, deterministically.
  State that as a change from one arbitrary answer to one predictable answer, not as a fix and not as a regression, and record that no code change was made to `internal/logger`.
  Add a timing row to `docs/benchmarks/test-suite-timing.md`'s Trend log with the measured Tier 1 and Tier 2 wall-clock and a "What changed" cell naming this task.
  Be honest about the size of the payoff: on bare-metal Linux the chdir-heavy CLI packages sum to roughly 3.3 s and `go test` already runs those packages concurrently, so intra-package parallelism recovers close to zero wall-clock there, and the value of this task is architectural.
  Note that the payoff scales with environment, since Tier 2 measured 4.97 s on bare-metal Linux against 131.7 s on the Cortex-XDR Windows laptop.
  Do not invent numbers: every figure written must come from a run performed for this card, and the machine and tag set must be named beside it exactly as the surrounding rows do.
  Use semantic line breaks in both files, and add no inline markdown link unless it resolves, anchor included.
- **Commit:** `docs: record the chdir-migration timing row and the LYX_TRACE sink disposition`

## Batch Tests

`verify:` runs two gates.
`go test ./cmd/lyx/...` covers the untagged guard suite, which is where card 17's new `cmd/lyx/cwdmutation_test.go` and the `allowedSpawners` edit to `cmd/lyx/tierpurity_test.go` live, and where `TestEnforcement_MarkdownLinks` would fire on a broken link introduced by card 18.
`go test -tags integration` over `internal/fabriccli`, `internal/perchcli`, `internal/configcli`, `internal/webstercli`, `internal/idecli`, `internal/reedcli`, and `internal/loomengine` covers every one of the eight migrated files, all of which are `//go:build integration` and therefore invisible to an untagged run.
That package list is exactly the set this batch edits, scoped per-batch rather than repo-wide.
`internal/scoutcli` is deliberately absent: its one edited test moved to an explicit base in batch 2 and is untagged, so it is already covered by batch 2's own gate and by the module-wide `go vet -tags integration ./...` that runs at every batch boundary.
The verification protocol proper — `-race -count=2` under both tag sets, before and after — is card 18's own requirement rather than part of `verify:`, because `verify:` runs after every implementer and fixer round and a `-race -count=2` sweep is far too slow to run that often.
Explicitly not covered by this batch: anything requiring `-tags smoke`, and the four `t.Setenv` files plus `internal/idecli/cli_test.go`, which stay serial and about which no parallel-safety claim is made.
