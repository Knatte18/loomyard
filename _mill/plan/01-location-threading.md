# Batch: location-threading

```yaml
task: "Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)"
batch: "location-threading"
number: 1
cards: 9
verify: go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch delivers the whole task as one unit: `scoutengine.DaemonStateFile`/`DaemonLock` stop taking a `string` worktree path and take a `*lyxcwd.Location`, `scoutengine.Options.WorktreeRoot string` becomes `Options.Layout *lyxcwd.Location`, the `Location` is threaded typed down to `ensureSupervised`, and `scoutcli` becomes the sole resolver via a new `lookupContext` seam that also closes the `assert-no-callers` empty-root bug.

It is one batch because it cannot be split into separately-compiling pieces.
Re-signaturing the two accessors breaks `internal/scoutengine`, `internal/scoutcli`, and `cmd/lyx/constructoranchoring_test.go` simultaneously;
any batch boundary drawn inside cards 1-8 leaves `go build ./...` red at a batch verify gate.
Card 9 is the one part that would compile green if deferred — the four `//go:build scout` files are built by no pipeline gate — which is exactly the argument for keeping it here rather than trusting a later batch to remember them.

There is no external interface for a next batch to consume;
this batch is the whole change.

Batch-local decisions beyond the overview's `## Shared Decisions`: none.
The overview's `anchor-stays-worktreepath`, `field-named-layout`, `no-nil-layout-check`, `byte-identical-except-one-delta`, `never-touch-lspclient`, `out-of-scope-files`, and `stale-citation-left-alone` all bind every card here.

## Cards

### Card 1: Re-signature DaemonStateFile/DaemonLock onto *lyxcwd.Location

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/logger/sink.go`
- **Edits:**
  - `internal/scoutengine/daemonstate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `DaemonStateFile(worktreePath, lang string) string` to `DaemonStateFile(l *lyxcwd.Location, lang string) string`, and `DaemonLock(worktreePath, lang string) string` to `DaemonLock(l *lyxcwd.Location, lang string) string`.
  In both bodies substitute `l.WorktreePath()` for the `worktreePath` argument to `filepath.Join`;
  the `dotLyxDirName`, `scoutDirName`, `lang`, and filename segments are unchanged, and so is the argument order.
  Add `github.com/Knatte18/loomyard/internal/lyxcwd` to the import block.
  Leave the `dotLyxDirName` and `scoutDirName` consts and their comments untouched — they stay `scoutengine`-private per the Cwd Resolution Invariant's module-owned-subdirectory rule.
  Mirror `logger.WorktreeLogsDir`'s doc-comment shape: both accessor doc comments must keep stating the anchoring choice and the `.lyx`-ephemeral-versus-`_lyx`-durable reason, reworded so they no longer name a `worktreePath` parameter.
  `DaemonLock`'s comment currently refers to `DaemonStateFile(worktreePath, lang)` — retarget that reference to the new signature.
  Reword the file header's second sentence, which names `DaemonStateFile/DaemonLock` as ".lyx-anchored path constructors", so it does not imply a string parameter.
  Add a `// TODO(dotlyx):` marker to each accessor's doc comment recording that it is a candidate for the `WorktreePath` → `AnchorPath` migration when `.lyx` gets a single owner, per the `anchor-stays-worktreepath` decision.
- **Commit:** `refactor(scoutengine): take *lyxcwd.Location in DaemonStateFile/DaemonLock`

### Card 2: Replace Options.WorktreeRoot with Options.Layout

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/scoutengine/daemonstate.go`
- **Edits:**
  - `internal/scoutengine/refs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `Options` struct, replace the field `WorktreeRoot string` with `Layout *lyxcwd.Location`.
  Keep `TargetDir string` as a separate field — `--target-dir` is an independent user-facing flag that legitimately differs from the worktree root.
  Keep the field's position in the struct and every other field unchanged.
  Add a doc comment on `Layout` alone stating it is required and must be non-nil, per the `no-nil-layout-check` decision;
  `Options` has no per-field docs today, so document only `Layout`, not the other five fields.
  In `acquireConnection`, change the `ensureServer` call to pass `opts.Layout` in place of `opts.WorktreeRoot`, leaving `opts.TargetDir` and `opts.Timeout` in their existing argument positions.
  Add `github.com/Knatte18/loomyard/internal/lyxcwd` to the import block.
  Add no nil check for `Layout` anywhere.
- **Commit:** `refactor(scoutengine): carry *lyxcwd.Location as Options.Layout`

### Card 3: Thread the Location down to ensureSupervised

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/doc.go`
- **Edits:**
  - `internal/scoutengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `ensureServer`'s signature so the combined `targetDir, worktreeRoot string` parameter pair becomes `targetDir string, layout *lyxcwd.Location`, keeping both parameters in their existing positions relative to `ctx`, `lang`, `entry`, and `timeout`.
  Change `ensureSupervised`'s signature the same way: its `lang, targetDir, worktreeRoot string` group becomes `lang, targetDir string, layout *lyxcwd.Location`.
  Update the `ensureSupervised` call inside `ensureServer` to pass `layout`.
  In `ensureSupervised`'s body, change the `statePath` and `lockPath` assignments to call `DaemonStateFile(layout, lang)` and `DaemonLock(layout, lang)`.
  Leave the `socketPath` derivation from `filepath.Dir(statePath)` unchanged — `statePath`'s value is unchanged, so the socket path is too.
  Leave `ensureNative` untouched;
  it takes no worktree root.
  Add `github.com/Knatte18/loomyard/internal/lyxcwd` to the import block.
  Reword two comment sites for the parameter rename: the file header's opening sentence, which names the seam as taking `worktreeRoot` — reword only that, and leave its `manifest/designs/scout-redesign.md` citation exactly as-is per the `stale-citation-left-alone` decision — and the inline comment immediately above `socketPath` describing it as a deterministic function of `(worktreeRoot, lang)`.
  The keying claim itself stays true and must survive;
  only the parameter name in it changes.
  That inline comment is duplicated verbatim in `internal/scoutengine/doc.go` and card 4 rewords the copy — the two must end up saying the same thing.
- **Commit:** `refactor(scoutengine): thread *lyxcwd.Location to ensureSupervised`

### Card 4: Reword the three scoutengine package-doc sites

- **Context:**
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/daemonstate.go`
- **Edits:**
  - `internal/scoutengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reword exactly three sites, all of which name a parameter that no longer exists.
  First, the "# The EnsureServer seam" section, which spells the signature out as `ensureServer(ctx, lang, entry, targetDir, worktreeRoot, timeout)` — update it to the signature card 3 produced.
  Second, the "# Daemon state and concurrency" section's sentence describing the state as a JSON state file plus a paired advisory lock per `(worktreeRoot, lang)` resolved via this package's own `DaemonStateFile`/`DaemonLock`.
  Third, the sentence stating the daemon's socket path is a deterministic function of `(worktreeRoot, lang)`.
  In all three the `(worktreeRoot, lang)` *keying* claim survives on the merits — the key is still worktree-and-language — so reword the parameter name without weakening or deleting the claim.
  The third site is the same sentence card 3 rewords in `internal/scoutengine/ensureserver.go`;
  the two copies must stay consistent with each other.
  Change nothing else in the file — in particular leave the `os.UserCacheDir()` paragraph and the `.lyx`-versus-`_lyx` ephemerality paragraph alone.
- **Commit:** `docs(scoutengine): reword worktreeRoot parameter references`

### Card 5: Add lookupContext and route all four commands through it

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/registry.go`
- **Edits:**
  - `internal/scoutcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `resolveWorktreeRoot(cwd, targetDir string) string` with `resolveLocation(cwd, targetDir string) *lyxcwd.Location`.
  Its in-hub branch returns the `layout` that `lyxcwd.Resolve(cwd)` produced, directly, instead of `layout.WorktreePath()`.
  Its out-of-hub branch synthesizes a `Location` from the absolute target directory as `&lyxcwd.Location{HubPath: filepath.Dir(abs), WorktreeName: filepath.Base(abs), AnchorRel: "."}`, where `abs` is `filepath.Abs(targetDir)`.
  Preserve the existing inner error path exactly: when `filepath.Abs(targetDir)` itself returns an error, today's code returns the bare `targetDir`, so synthesize from `targetDir` instead of `abs` on that path rather than silently changing it.
  `resolveLocation`'s doc comment must state the limit from the `out-of-hub-synthesized-location` decision: the synthesized value is a fiction outside `WorktreePath()` — `HubPath` names no real hub, `RepoName` is left zero, and `AnchorPath()` is meaningless because `AnchorRel` was assumed rather than read from a `.fabric-anchor` marker — so it is contractually consumed by `DaemonStateFile`/`DaemonLock` alone and must never be widened to feed a caller reading `AnchorPath()`, `HubPath`, or `RepoName`.
  Add a new helper `lookupContext(cwd, dir string) (scoutengine.Registry, *lyxcwd.Location, error)` at the bottom of the file, next to `resolveLocation` and `buildOptions`, with `resolveLocation` becoming its private helper rather than a separately-called function.
  `lookupContext` performs both pre-flight derivations every lookup command needs — the `servers.yaml` overlay load and the `Location` resolution — deriving each independently from `(cwd, dir)`.
  Its `dir` parameter is the already-defaulted directory, never the raw `--target-dir` flag value;
  name it `dir` for that reason and say so in its doc comment, because passing the raw flag would make the out-of-hub branch resolve `filepath.Abs("")` — the process working directory — rather than `filepath.Abs(cwd)` whenever `--target-dir` is omitted.
  Its returned `error` carries `scoutengine.LoadRegistry` failures only;
  a `lyxcwd.Resolve` failure is never an error, it is the out-of-hub path and degrades to `scoutengine.BuiltinRegistry()` plus the synthesized `Location`, exactly as today.
  Change `buildOptions` so its `worktreeRoot string` parameter becomes `layout *lyxcwd.Location` in the same position, setting the `Layout` field instead of `WorktreeRoot`;
  its body and field set are otherwise unchanged.
  Reword `buildOptions`' own doc comment, which says it ensures all construction sites thread `WorktreeRoot` consistently, to name `Layout`.
  Replace the duplicated pre-flight block in each of the four commands' `RunE` — the `refs`, `definition`, and `symbol` blocks that call `resolveWorktreeRoot` and then load the registry inside an `if layout, resolveErr := lyxcwd.Resolve(cwd); resolveErr == nil` guard, and `assert-no-callers`' variant that declares `var worktreeRoot string` and assigns `layout.WorktreePath()` inside that same guard — with a single `lookupContext(cwd, dir)` call.
  Each `RunE` keeps its own existing two lines of `dir := targetDir; if dir == "" { dir = cwd }` defaulting ahead of the call;
  the defaulting does not move into `lookupContext`, because `Options.TargetDir` and `filterWithin` need `dir` in the same closure anyway.
  Callers keep the existing envelope mapping unchanged: on a non-nil error from `lookupContext`, call `clihelp.SetExit(ctx, output.Err(out, err.Error()))` then `return nil`.
  Reword or delete the three comment blocks that currently explain why `resolveWorktreeRoot` never leaves `WorktreeRoot` empty outside a hub — they annotate blocks this extraction deletes — and note that the claim now holds for `assert-no-callers` too.
  In `assert-no-callers`, delete the hand-built `scoutengine.Options` literal and the `baseOpts`/`defOpts`/`refOpts` trio, replacing all three with one `buildOptions(registry, dir, layout, lang, query, timeout)` value passed to both `scoutengine.Definition` and `scoutengine.References`.
  This is byte-equivalent to today's code: `query` is parsed before the literal is built, and `defOpts.Query` and `refOpts.Query` are both assigned that same `query`.
  Keep the long explanatory comment above the `Definition` call — the one about UTF-16 column coordinates — and every other line of that `RunE` unchanged.
  Add no new flags and change no `Short` or `Long` text;
  the CLI surface is untouched.
- **Commit:** `refactor(scoutcli): own Location resolution via lookupContext`

### Card 6: Pin the resolution seam in scoutcli tests

- **Context:**
  - `internal/scoutcli/cli.go`
  - `internal/scoutengine/registry.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/scoutcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `TestResolveWorktreeRoot_OutsideHubFallsBackToAbsoluteTargetDir` with an equivalent test against `resolveLocation`, renamed to match.
  It must assert the returned `Location`'s `WorktreePath()` equals `filepath.Abs(targetDir)` — deliberately the same expected value the current test asserts, so it proves byte-identical behaviour across the refactor rather than merely exercising new code — and must also assert `AnchorRel` equals `"."`, since a synthesized non-`.` anchor would silently move the daemon state.
  Add a second new test covering `lookupContext` out-of-hub: call it with a `cwd` outside any lyx hub and assert the returned `*lyxcwd.Location` is non-nil with `WorktreePath()` equal to `filepath.Abs(dir)`, and that the returned registry is the built-in one.
  `scoutengine.Registry` is a `map[string]Entry` and so is not `==`-comparable — assert the registry half with a keyed spot-check that the `"go"` entry matches `scoutengine.BuiltinRegistry()["go"]`, never `reflect.DeepEqual` over the whole map, which would couple the test to every future built-in entry.
  This is the test that observes the actual defect: today the equivalent derivation inside `assert-no-callers` yields an empty string on exactly this path, and because all four commands now share `lookupContext`, one test covers all four.
  Retarget the existing `buildOptions` field-threading test's `WorktreeRoot` assertion onto `Layout`, comparing `got.Layout.WorktreePath()` rather than the struct pointer, so the test survives any future `Location` field addition;
  pass a synthesized `Location` as the new argument in place of the `"/worktree/root"` string.
  Do not write a test asserting `buildOptions(..., layout, ...).Layout == layout` — `buildOptions` copies a parameter into a struct field, so such an assertion is true by construction and proves nothing about the resolution that was broken.
  Both new tests inherit the existing test's unstated premise, that `t.TempDir()` does not itself sit inside a git repository;
  that holds for an ordinary `TMPDIR` and is how the current test already passes.
  Do not build on a stronger assumption and do not try to force the out-of-hub branch by other means.
  Do not add a `TestMain` or a `lyxtest.HermeticGitEnv` call to this package — `internal/scoutcli` has neither today, both guards still pass because the git spawn is two packages down inside `lyxcwd.Resolve`, and adding one would be an out-of-scope drive-by.
- **Commit:** `test(scoutcli): pin resolveLocation and lookupContext out-of-hub`

### Card 7: Adapt the untagged scoutengine tests

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/websterengine/audit_test.go`
- **Edits:**
  - `internal/scoutengine/scoutdaemon_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/scoutengine/ensureserver_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapt every affected call site to the new signatures, changing no expected value anywhere.
  The fixture shape is a hand-built `Location` whose `WorktreePath()` equals the directory the test already uses: given an existing `dir`, add `l := &lyxcwd.Location{HubPath: filepath.Dir(dir), WorktreeName: filepath.Base(dir), AnchorRel: "."}`.
  `internal/websterengine/audit_test.go` shows the same idiom if a shape reference is wanted.
  Do not delete the existing string variable when introducing `l`: `ensureSupervised` keeps `targetDir` as a `string`, and several fixtures pass the same directory as both arguments, so those calls become `ensureSupervised(..., lang, dir, l, ...)` and still need the string.
  In `internal/scoutengine/scoutdaemon_test.go`, all four tests build a fixed fake `worktreePath` and call the accessors directly;
  synthesize an `l` from it and pass that, keeping every `want` value byte-identical.
  The two per-language distinctness tests are the ones that would catch a wrong `filepath.Join` argument order, so their assertions must not be weakened.
  In `internal/scoutengine/supervised_test.go`, three fixtures each build a temp dir and call both accessors, and three `ensureSupervised` calls pass that dir twice;
  note the third fixture has two consumers — its accessor pair and one of those calls.
  In `internal/scoutengine/ensureserver_test.go`, one fixture feeds a `DaemonLock` call and one `ensureServer` call that passes a separate `t.TempDir()` as `targetDir`.
  Add the `lyxcwd` import to each of the three files.
  These files must stay Tier 1-pure per the Test Tier Purity Invariant: the fixture shape spawns no git and copies no fixture tree, so add no `gitexec` call, no `exec.Command`, no `lyxtest.Copy*`, and no `TestMain`.
- **Commit:** `test(scoutengine): adapt untagged tests to the Location signatures`

### Card 8: Re-signature the anchoring pin test

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In both `TestConstructorAnchoring_Unanchored` and `TestConstructorAnchoring_SubpathAnchored`, change the two `scoutengine.DaemonStateFile` and `scoutengine.DaemonLock` calls in each from passing the `worktree` string to passing `l`.
  Both tests already have `l` in scope from the existing `anchoringFixture` helper — reuse it, and do not add a second helper.
  This is a pure signature change: because the accessors stay `WorktreePath()`-anchored per `anchor-stays-worktreepath`, every expected `dotLyxBase`-derived path stays byte-identical in both tests.
  Change no expected value, no `assertPath` call for any other constructor, and none of the `.lyx`-group or three-group comments.
  Leave the `worktree` variable itself in place — the other assertions in both tests still use it.
  `TestConstructorAnchoring_SubpathAnchored` is the machine pin on the `anchor-stays-worktreepath` decision: if it starts failing, the implementation drifted to `AnchorPath()` and the fix is in `internal/scoutengine/daemonstate.go`, never in this test's expectations.
- **Commit:** `test(cmd/lyx): pass Location to the scout anchoring assertions`

### Card 9: Adapt the four scout-tagged test files

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/refs.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapt every affected call site in the four `//go:build scout` files, using the same hand-built `Location` fixture shape as card 7 and keeping the existing string variable alongside `l` wherever a call still needs a `string` `targetDir`.
  In `internal/scoutengine/supervised_scout_test.go`, two fixtures each feed one `DaemonStateFile` call and one `ensureSupervised` call that passes the dir twice.
  In `internal/scoutengine/supervised_integration_test.go`, one fixture feeds one `DaemonStateFile` call and three `ensureSupervised` calls that pass a separate `root` as `targetDir`.
  In `internal/scoutengine/refs_integration_test.go`, three fixtures each feed one `DaemonStateFile` call and one `scoutengine.Options` composite literal whose `WorktreeRoot` field becomes `Layout`.
  In `internal/scoutengine/ensureserver_integration_test.go`, one fixture feeds one `DaemonStateFile` call and two `ensureServer` calls that pass a separate `root` as `targetDir`.
  Add the `lyxcwd` import to each of the four files.
  Change no expected value and no assertion.
  Additionally reword the eight prose comments across three of these files that name `WorktreeRoot` or `worktreeRoot` and become false after the rename — three in `internal/scoutengine/refs_integration_test.go`, three in `internal/scoutengine/ensureserver_integration_test.go`, and two in `internal/scoutengine/supervised_integration_test.go` — to the new vocabulary, either `layout` or a phrase such as "the worktree the `Location` resolves to".
  These files are compiled by no pipeline gate, which is why the batch verify runs `go vet -tags scout` and why a missed call site here is the single largest correctness risk in the task.
- **Commit:** `test(scoutengine): adapt scout-tagged tests to the Location signatures`

## Batch Tests

The batch `verify:` is `go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./internal/lyxcwd/... ./cmd/lyx/...`.

`go build ./...` runs first because it is the cheapest way to catch a missed consumer of either changed signature anywhere in the tree, and it fails fastest.

`go vet -tags scout ./internal/scoutengine/...` is not optional and not redundant with the test run.
It is the only thing that proves the four `//go:build scout` files card 9 touches still compile — `go build ./...` and `go test ./...` both skip them entirely, so a missed call site in them fails nothing and stays invisible until someone runs the scout suite by hand.

The test scope is wider than the two packages the batch edits, deliberately, and each addition earns its place:

- `./internal/scoutengine/...` — the accessor tests (`scoutdaemon_test.go`, whose per-language distinctness cases catch a wrong `filepath.Join` argument order), the supervised tests, and both seam guards (`seam_enforcement_test.go` for the banned-import list, `lspclient_guard_test.go` for the file-scoped stdlib-plus-logger guard on `lspclient.go`).
- `./internal/scoutcli/...` — the two new pin tests from card 6 and the retargeted `buildOptions` field-threading test.
- `./internal/lyxcwd/...` — `enforcement_test.go` carries `TestEnforcement_GeometryLiterals` and `TestEnforcement_FabricVocabulary`, which walk production `.go` files under `internal/` and `cmd/`.
  Adding a `lyxcwd` import and new geometry-adjacent prose to `scoutengine` is exactly what those two scan, so the package must be in scope even though no card edits it.
- `./cmd/lyx/...` — `constructoranchoring_test.go` (card 8, and the machine pin on `anchor-stays-worktreepath`), plus `tierpurity_test.go` and `hermeticenv_test.go`, which police the test-fixture shape cards 6 and 7 use, and `helptree_test.go`/`drift_test.go`, which confirm card 5 left the CLI surface untouched.

Deliberately not run: the `scout`-tagged tests themselves.
They spawn a real gopls, are slow and environment-dependent, and this refactor changes no runtime behaviour they would catch beyond compilation — which the `go vet -tags scout` run already covers.

The repo-wide `go test ./... && go test -tags integration ./...` done gate configured in `pipeline.done_gate` covers everything outside this scoped list before the task is marked done.
