# Batch: told-anchor-root-conversion

```yaml
task: "scoutengine told-geometry (optional uniformity pass)"
batch: "told-anchor-root-conversion"
number: 1
cards: 10
verify: go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch is the whole told-geometry conversion: `internal/scoutengine`'s two path constructors and the `Options.Layout` field that exists only to feed them become told strings, `internal/scoutcli`'s fictional-`Location` synthesis is deleted outright, and every test and comment that named a `layout` is converted with them.
It is one batch because a Go signature change is compile-atomic across packages — `DaemonStateFile`'s new signature breaks `ensureserver.go`, `refs.go`, `cli.go`, and two `cmd/lyx` test files the instant it lands, so nothing smaller than this card set leaves the repo building.

**Intermediate card commits do not compile, by construction.**
Cards 1 through 8 walk the call chain bottom-up (engine constructors, engine callers, engine tests, CLI, CLI tests, `cmd/lyx` tables), and the repo-wide build is red from card 1's commit until card 8's.
This is inherent to the change, not a planning defect: there is no ordering in which a caller and its callee can both be green mid-conversion.
The batch `verify:` runs at the batch boundary, where the tree is green again.

The external interface batch 2 consumes is `lookupContext(cwd, dir string) (scoutengine.Registry, string, error)` — its second return becomes the anchor-root string batch 2's hub-mode integration test asserts against.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: the `-tags scout` suite is compile-checked here (`go vet -tags scout`) rather than executed, because those four files spawn a real `gopls` daemon and would be paid on every implementer and fixer round.
The executed `-tags scout` run is batch 2's card 13 gate.

## Cards

### Card 1: told `anchorRoot string` for `DaemonStateFile` and `DaemonLock`

- **Context:**
  - `internal/websterengine/state.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/scoutengine/daemonstate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `DaemonStateFile(l *lyxcwd.Location, lang string) string` to `DaemonStateFile(anchorRoot string, lang string) string` and `DaemonLock(l *lyxcwd.Location, lang string) string` to `DaemonLock(anchorRoot string, lang string) string`, replacing the first `filepath.Join` argument `l.AnchorPath()` with `anchorRoot` in both.
  `scoutDirName`, the `lyxdirs.DotLyxDirName` join, and the `"daemon.json"`/`"daemon.lock"` leaf names are unchanged — only the first join argument changes.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import.
  Rewrite the file header's second sentence, which currently describes the accessors as "built on `*lyxcwd.Location`", to describe them as told, `anchorRoot`-joined accessors.
  Rewrite both doc comments: `DaemonStateFile`'s "rooted at `l.AnchorPath()`" becomes "joined onto the told `anchorRoot`", and both comments keep their existing anchoring rationale, `.lyx`-not-`_lyx` rationale, and daemon-re-keying consequence prose intact.
  Follow `internal/websterengine/state.go`'s `Dir`/`ReportsDir` free functions for the doc-comment shape.
  Add no validation of `anchorRoot`: an empty value must not error, panic, or be rejected.
- **Commit:** `refactor(scoutengine): tell DaemonStateFile/DaemonLock an anchorRoot string`

### Card 2: `Options.AnchorRoot` and the `ensureServer`/`ensureSupervised` thread

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/burlerengine/geometry.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/scoutengine/refs.go`, replace the `Options` struct's `Layout *lyxcwd.Location` field with `AnchorRoot string`, and update `acquireConnection`'s `ensureServer` call to pass `opts.AnchorRoot` instead of `opts.Layout`.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import from that file.
  Rewrite the field doc comment: drop "the resolved `*lyxcwd.Location` for the worktree this call operates in" and "required and must be non-nil", and state instead that `AnchorRoot` is required and must be a usable absolute path, and that populating it is entirely the caller's obligation — the wording `internal/burlerengine/geometry.go` and `internal/websterengine/geometry.go` already use.
  In `internal/scoutengine/ensureserver.go`, change `ensureServer`'s `layout *lyxcwd.Location` parameter to `anchorRoot string` and `ensureSupervised`'s `layout *lyxcwd.Location` parameter to `anchorRoot string`, keeping both parameters in their existing positions; update `ensureServer`'s internal `ensureSupervised` call and `ensureSupervised`'s `DaemonStateFile`/`DaemonLock` calls to pass `anchorRoot`.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import from that file.
  Rewrite the file-header sentence naming "the `EnsureServer(lang, layout) -> LSPConn` seam" and the socket-path comment inside `ensureSupervised` saying the socket path is "a deterministic function of (layout, lang)" so both name the told anchor root instead.
  Add no validation of `anchorRoot` in either function: an empty value must not produce an error return or a panic.
- **Commit:** `refactor(scoutengine): thread a told anchorRoot through Options and ensureServer`

### Card 3: `doc.go` — the package's module doc

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/refs.go`
- **Edits:**
  - `internal/scoutengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the "# The EnsureServer seam" section, rewrite the spelled-out signature `ensureServer(ctx, lang, entry, targetDir, layout, timeout)` to name `anchorRoot` in place of `layout`.
  In the "# Daemon state and concurrency" section, rewrite "a JSON state file plus a paired advisory lock per (layout, lang)" and "The daemon's socket path is a deterministic function of (layout, lang)" so both name the told anchor root instead of a layout.
  Leave the "This cache root is deliberately outside the Cwd Resolution Invariant's scope" paragraph's reference to `internal/lyxcwd` exactly as it is — that sentence is about `os.UserCacheDir()` and the toolchain cache, is still true, and is not a `layout` mention.
  Change no other section of this file.
- **Commit:** `docs(scoutengine): retire layout wording from the package doc`

### Card 4: rewrite `scoutdaemon_test.go` as told-string path math

- **Context:**
  - `internal/websterengine/webstergeom_test.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/scoutengine/scoutdaemon_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite this file as pure told-string path arithmetic over three fixtures, mirroring `internal/websterengine/webstergeom_test.go`'s structure: an unanchored worktree root (keep today's `filepath.Join("home", "user", "repo-HUB", "repo")` value, so the expected strings are unchanged from the current file), a subpath-anchored root (`filepath.Join(worktreeRoot, "backend")`), and a plain told directory not derived from any `Location`.
  Keep the existing per-language distinctness coverage — `TestDaemonStateFile_DistinctPerLanguage` and `TestDaemonLock_DistinctPerLanguage`'s `go`-versus-`python` assertions — across the conversion.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import and every hand-built `&lyxcwd.Location{...}` fixture line; pass the root string to `DaemonStateFile`/`DaemonLock` directly.
  Rewrite the file header, which currently says the constructors are tested "over a hand-built `*lyxcwd.Location`", to say they are tested over told root strings; keep its "pure path arithmetic, no spawning, untagged (Tier 1)" claim, which stays true.
  This file must spawn nothing and must call no `exec.Command`, `gitexec.Run`, `gitkit.Copy*`, or `hubforge.NewHub`, so it keeps its Tier 1 status under the Test Tier Purity Invariant.
  Author this card's assertions before card 5's and card 6's mechanical conversions, since these are the tests that pin the byte-identical-paths property.
- **Commit:** `test(scoutengine): rewrite the daemon-path tests as told-string path math`

### Card 5: convert the untagged `scoutengine` test fixtures

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/ensureserver.go`
  - `cmd/lyx/tierpurity_test.go`
- **Edits:**
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/supervised_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In both files, delete every hand-built `l := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot), AnchorRel: "."}` line and pass the existing `worktreeRoot` string to `DaemonStateFile`, `DaemonLock`, `ensureServer`, and `ensureSupervised` in place of `l`.
  In every one of these fixtures `l.AnchorPath()` is exactly `worktreeRoot`, so this is a value-preserving swap.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import from both files.
  Change no assertion, no expected value, and no test name — a behaviour change here means the migration is wrong.
  `internal/scoutengine/supervised_test.go` keeps its `spawnAndHoldSubprocess` PID-liveness fixture and its existing entry in `cmd/lyx/tierpurity_test.go`'s allowlist untouched.
- **Commit:** `test(scoutengine): pass told worktree roots in the untagged fixtures`

### Card 6: convert the `scout`-tagged `scoutengine` test fixtures

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/refs.go`
- **Edits:**
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/refs_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply the same value-preserving swap as card 5 across all four files: delete every hand-built `&lyxcwd.Location{...}` line and pass the existing `worktreeRoot` string to `DaemonStateFile`, `ensureSupervised`, and `ensureServer` directly.
  In `internal/scoutengine/refs_integration_test.go` there are three blocks that each build a `Location` *and* set `Layout: l` inside a `scoutengine.Options` literal — both sites in each block change, with `Layout: l` becoming `AnchorRoot: worktreeRoot`.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import from all four files; the `lyxcwdFile := filepath.Join(root, "internal", "lyxcwd", "lyxcwd.go")` string literals and the test names mentioning `lyxcwd.Resolve` in `internal/scoutengine/refs_integration_test.go` are path and prose strings, not package references, and stay exactly as they are.
  Reword the prose comments that survive the type swap but still say "layout": in `internal/scoutengine/refs_integration_test.go`, "Without an explicit layout this anchors at a relative .lyx/scout/go/", "each subcase anchors its layout at its own", and "layout is an isolated temp dir so the supervised daemon this"; in `internal/scoutengine/supervised_integration_test.go`, "Second call, same layout/lang"; in `internal/scoutengine/ensureserver_integration_test.go`, "call against the same layout must reuse that same daemon" and "A second call against the same layout/lang must reuse the".
  These four files carry a `//go:build scout` constraint and are invisible to an untagged `go test`; they are compile-checked by this batch's `go vet -tags scout` and executed by card 13.
  Change no assertion, no expected value, and no test name.
- **Commit:** `test(scoutengine): pass told worktree roots in the scout-tagged fixtures`

### Card 7: delete `resolveLocation`, fold its job into `lookupContext`

- **Context:**
  - `internal/scoutengine/refs.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/scoutcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `resolveLocation` function and its entire doc comment block.
  Change `lookupContext(cwd, dir string) (scoutengine.Registry, *lyxcwd.Location, error)` to `lookupContext(cwd, dir string) (scoutengine.Registry, string, error)`, where the second return is the anchor root.
  It performs exactly one `lyxcwd.Resolve(cwd)` and derives both results from it: on success, `registry = scoutengine.LoadRegistry(layout.AnchorPath())` (unchanged, its error still propagates) and the anchor root is `layout.AnchorPath()`; on failure, `registry = scoutengine.BuiltinRegistry()` (unchanged) and the anchor root is `filepath.Abs(dir)`.
  When `filepath.Abs(dir)` itself returns an error, the anchor root is `filepath.Clean(dir)` — this reproduces the deleted synthesis's `AnchorPath()` on that branch, which was `filepath.Join(filepath.Dir(dir), filepath.Base(dir))`, and preserving it is what keeps the documented failure mode from changing silently.
  Do not return bare `dir` on that branch and do not drop the branch.
  Change `buildOptions`'s third parameter from `layout *lyxcwd.Location` to `anchorRoot string` and its `Options` literal from `Layout: layout` to `AnchorRoot: anchorRoot`.
  Rename the `layout` local to `anchorRoot` at all four command call sites — the `refs`, `definition`, `symbol`, and `assert-no-callers` `RunE` bodies, each of which calls `lookupContext` once and `buildOptions` once or twice.
  Rewrite `lookupContext`'s doc comment: "the servers.yaml overlay load and the Location resolution" becomes the overlay load and the anchor-root derivation, and "degrades to `scoutengine.BuiltinRegistry()` plus the synthesized Location" drops the synthesis.
  Keep its "dir is the already-defaulted directory, never the raw --target-dir flag value" paragraph verbatim — that trap is unchanged and the reshaped test in card 8 pins it.
  Rewrite `buildOptions`'s doc comment, which currently says it ensures all construction sites "thread Layout consistently", to name `AnchorRoot`.
  Keep the `lyxcwd` import: `lyxcwd.Resolve` and `lyxcwd.CwdFrom` both remain in this file.
  Add no new flag and change no command's `Short` or `Long`.
- **Commit:** `refactor(scoutcli): derive a told anchor root and delete resolveLocation`

### Card 8: reshape the `scoutcli` unit tests

- **Context:**
  - `internal/scoutcli/cli.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/registry.go`
- **Edits:**
  - `internal/scoutcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `TestResolveLocation_OutsideHubFallsBackToAbsoluteTargetDir` along with the function it covers.
  Replace `TestLookupContext_OutsideHubReturnsSynthesizedLocationAndBuiltinRegistry` with a test whose name describes the told shape (for example `TestLookupContext_OutsideHubReturnsAbsoluteAnchorRootAndBuiltinRegistry`) asserting the out-of-hub return is `scoutengine.BuiltinRegistry()` plus an absolute-path string, spot-checked on the `"go"` entry exactly as the current test does rather than by a whole-map comparison.
  Keep the existing setup shape: two `t.TempDir()` values passed as the `cwd` and `dir` arguments, with no process-wide chdir — the chdir-into-a-non-git-temp-dir setup belongs to the separate `RunCLI_*_NoLanguageError` tests in this file and must be left alone.
  Cover two cases: an explicit `dir`, where the anchor root is `filepath.Abs(dir)`; and a `dir` defaulted from `cwd`, where the anchor root is `filepath.Abs(cwd)` and not `filepath.Abs("")`.
  Do not feed an uncleaned path such as a trailing-separator value and treat the result as a behaviour pin — the old synthesis and `filepath.Clean` diverge there, and that divergence is unreachable from production.
  Reshape `TestBuildOptions_ThreadsEveryFieldFromItsArguments` to build no `Location` at all: pass an anchor-root string literal and assert `got.AnchorRoot` by plain string comparison in place of the `got.Layout.WorktreePath()` round-trip.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import if nothing else in the file still needs it; the comments at the `RunCLI_*_NoLanguageError` tests that mention `lyxcwd.Resolve` degrading are prose about a still-live behaviour and stay.
- **Commit:** `test(scoutcli): assert a told anchor root instead of a synthesized Location`

### Card 9: `cmd/lyx` anchoring and transient tables

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/constructoranchoring_test.go`, change every `scoutengine.DaemonStateFile(l, "go")` and `scoutengine.DaemonLock(l, "go")` call to `scoutengine.DaemonStateFile(l.AnchorPath(), "go")` and `scoutengine.DaemonLock(l.AnchorPath(), "go")`, matching the `websterengine.PromptsDir(l.AnchorPath())` rows directly beside them.
  There are three such pairs: the unanchored fixture's `.lyx` group, the subpath-anchored fixture's `.lyx` group, and the `dotLyxConstructors` map used by the prefix-exclusion regression guard.
  Expected paths are unchanged in every row — that is the point of the change.
  In `cmd/lyx/notransients_test.go`, apply the same swap to `transientSet`'s two `scoutengine` rows.
  Neither file drops its `lyxcwd` import: both still take a `*lyxcwd.Location` and call `l.AnchorPath()` on it.
  Change no expected value and add no row.
  `cmd/lyx/notransients_test.go` is the machine guard for the Durable-vs-Ephemeral State Invariant and must keep passing at both its `AnchorRel == "."` and `AnchorRel == "backend"` fixtures.
- **Commit:** `test(lyx): call the scout constructors with a told anchor root`

### Card 10: closing grep gate for the surviving-`layout`-prose rule

- **Context:**
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/doc.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/scoutdaemon_test.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutcli/cli.go`
  - `internal/scoutcli/cli_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This is a zero-diff verification gate for the closed documentation rule in the overview's Shared Decisions: every mention of `layout` or `*lyxcwd.Location` in scout comments, production and test, is rewritten or deleted by this task.
  Run `grep -rn "layout\|Location" internal/scoutengine internal/scoutcli` and confirm every surviving hit is intentional.
  The known-intentional survivors are: `internal/scoutengine/doc.go`'s toolchain-cache paragraph naming `internal/lyxcwd`, which is about `os.UserCacheDir()` and stays; `internal/scoutcli/cli.go`'s live `lyxcwd.Resolve` and `lyxcwd.CwdFrom` calls and the comments describing them; `internal/scoutengine/refs_integration_test.go`'s `lyxcwd` path-string literals and its test names citing `lyxcwd.Resolve` as the symbol under query; and `internal/scoutcli/cli_test.go`'s comments about `lyxcwd.Resolve` degrading outside a hub.
  Also confirm `grep -rn "Options.Layout\|\.Layout\b" internal/scoutengine internal/scoutcli` returns nothing.
  A surviving unintentional hit means an earlier card in this same batch was implemented incompletely; correct it in the card that owns that file before the batch is done, rather than deferring it.
- **Commit:** none

## Batch Tests

`verify:` is `go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/...`.

- `go build ./...` is the cheap guard that no package outside the three named here referenced the changed symbols.
- `go vet -tags scout ./internal/scoutengine/...` compile-checks card 6's four `//go:build scout` files, which are invisible to the untagged `go test` and would otherwise sit broken until batch 2.
  It is `vet` rather than `test` deliberately: executing that suite spawns a real `gopls` daemon per test and, on a cold machine, a `go install` of the pinned `gopls`, which would be paid on every implementer and fixer round.
  The executed run is batch 2's card 13.
- `go test ./internal/scoutengine/...` covers card 4's rewritten `scoutdaemon_test.go` — the byte-identical-paths pin — plus card 5's `ensureserver_test.go` and `supervised_test.go`, and the package's untouched `seam_enforcement_test.go` and `lspclient_guard_test.go`, both of which must stay green.
- `go test ./internal/scoutcli/...` covers card 8's reshaped `cli_test.go`, which is the automated evidence for the out-of-hub half of the acceptance property.
- `go test ./cmd/lyx/...` covers card 9's `constructoranchoring_test.go` and `notransients_test.go`, plus the repo-wide `tierpurity_test.go` and `hermeticenv_test.go` guards.

The hub half of the acceptance property has no coverage in this batch — that is batch 2's entire purpose, and the `depends-on: [1]` edge is what sequences it.
