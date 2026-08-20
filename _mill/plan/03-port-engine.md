# Batch: port-engine

```yaml
task: "Extract scout into its own standalone repo"
batch: "port-engine"
number: 3
cards: 14
verify: go -C /home/knatte/Code/quarry/wts/quarry test ./...
depends-on: [2]
```

## Batch Scope

This batch writes the port program, runs it over `internal/scoutengine`, and then hand-edits the parts the program deliberately does not touch: the `logger` replacement, the two path-ownership signature changes, the toolchain cache segment, the lyx vocabulary sweep, the build-tag collapse, and the five test files that cannot be ported on a path rewrite alone.
It is one batch because the port program and its output are meaningless apart — reviewing the program without its diff, or the diff without the program, proves nothing — and because every hand edit in it is a direct consequence of a decision the port program is deliberately restricted from making.

The external interface batch 4 consumes: `package quarry` at `github.com/Knatte18/quarry/quarry`, exporting `References`, `Definition`, `Symbol`, `Options` (with `StateDir`, not `AnchorRoot`), `Query`, `Registry`, `Entry`, `BuiltinRegistry`, `LoadRegistry(path string)`, `DaemonStateFile(stateDir, lang string)`, `DaemonLock(stateDir, lang string)`, and the typed error set in `errors.go`.

Batch-local decisions:

- `template.go` and `template.yaml` are **not** ported. `ConfigTemplate()` has zero call sites anywhere and is absent from Loomyard's own config registry;
  porting it would put dead exported API into quarry's public package on day one. Its content already landed as `docs/servers.yaml.example` in batch 1 card 4.
- The 59 `"scoutengine: "` string literals are **not** touched. They reach the operator through the JSON envelope's error field, and batch 5's envelope comparison is only strict if they are identical on both sides. Renaming them to `quarry:` is a follow-up filed as a quarry issue in batch 5.
- `scoutdaemon_test.go` and `supervised_scout_test.go` land under new names (`quarrydaemon_test.go`, `supervised_lsp_test.go`) because `scout` is dead vocabulary in this repo. They are new files in a repo with no history, so this is not a git rename and carries no `Moves:` pair.

## Cards

### Card 10: the port program

- **Context:**
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/lspclient.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/tools/port/main.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write a single-file Go program at the quarry path above, `package main`, that performs the mechanical half of the port.
  It takes a source directory, a destination directory, and an explicit list of filenames, copies each named file, and rewrites exactly two closed categories of token in the copied bytes and nothing else:
  the import paths `github.com/Knatte18/loomyard/internal/scoutengine` to `github.com/Knatte18/quarry/quarry`, `github.com/Knatte18/loomyard/internal/scoutcli` to `github.com/Knatte18/quarry/internal/cli`, and `github.com/Knatte18/loomyard/internal/lock`, `.../internal/proc`, `.../internal/output` to the corresponding `github.com/Knatte18/quarry/internal/...` paths;
  and the package clauses `package scoutengine` to `package quarry` and `package scoutcli` to `package cli`.
  Match the package clause anchored at the start of a line so a `package scoutengine` mention inside a comment or a string literal is left alone.
  It must not rewrite any other token, and specifically must not touch string literals: after running, it prints the count of `"scoutengine: "` literals in the destination so the operator can confirm the number is unchanged.
  It refuses to overwrite an existing destination file unless given an explicit overwrite flag, so a re-run during development cannot silently clobber a hand edit made after the first run.
  Build it with `go -C /home/knatte/Code/quarry/wts/quarry build ./tools/port`. Keep it dependency-free: stdlib only, so it adds nothing to `go.mod`.
  Do not use `sed` for any part of this port — it is banned by the operator's global instructions and by the `mill:conversation` skill because it triggers a permission prompt.
  This program is deleted from quarry in batch 5 once the port is proven.
- **Commit:** `feat(tools): add the mechanical port program`

### Card 11: run the port over the engine package

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/tools/port/main.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/definition.go`
  - `internal/scoutengine/detect.go`
  - `internal/scoutengine/doc.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/errors.go`
  - `internal/scoutengine/load.go`
  - `internal/scoutengine/lspclient.go`
  - `internal/scoutengine/position.go`
  - `internal/scoutengine/probe.go`
  - `internal/scoutengine/refs.go`
  - `internal/scoutengine/registry.go`
  - `internal/scoutengine/symbol.go`
  - `internal/scoutengine/toolchain.go`
  - `internal/scoutengine/daemonstate_test.go`
  - `internal/scoutengine/definition_test.go`
  - `internal/scoutengine/detect_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/load_test.go`
  - `internal/scoutengine/lspclient_guard_test.go`
  - `internal/scoutengine/lspclient_test.go`
  - `internal/scoutengine/position_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/refs_test.go`
  - `internal/scoutengine/registry_test.go`
  - `internal/scoutengine/scoutdaemon_test.go`
  - `internal/scoutengine/seam_enforcement_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/scoutengine/symbol_test.go`
  - `internal/scoutengine/toolchain_integration_test.go`
  - `internal/scoutengine/toolchain_test.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/definition.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/definition_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/detect.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/detect_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/doc.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/errors.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/load.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/load_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient_guard_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/position.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/position_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/probe.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/quarrydaemon_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/registry.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/registry_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/seam_enforcement_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_lsp_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/symbol.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/symbol_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the port program over Loomyard's `internal/scoutengine` into quarry's `quarry/` directory for all 34 files listed above.
  Two files land under a different name than their source: `scoutdaemon_test.go` becomes `quarrydaemon_test.go` and `supervised_scout_test.go` becomes `supervised_lsp_test.go`.
  Two files are deliberately excluded from the run and are not created at all: `template.go` and `template.yaml`, per this batch's `## Batch Scope`.
  Do not hand-transcribe any of these files;
  the point of the program is that the copy is deterministic.
  After the run, confirm three things and report the numbers:
  `grep -rc 'scoutengine: ' /home/knatte/Code/quarry/wts/quarry/quarry/` totals 59 across nine production files, unchanged from the source;
  `grep -rl 'Knatte18/loomyard' /home/knatte/Code/quarry/wts/quarry/quarry/` returns nothing;
  and every ported file's first `package` line reads `package quarry`.
  The package will not compile at the end of this card — `lyxdirs`, `configengine`, and `logger` imports have no quarry equivalent yet, which cards 12 through 16 resolve.
  That is expected;
  do not paper over it by stubbing those packages.
- **Commit:** `feat(quarry): port the scout engine package mechanically`

### Card 12: replace internal/logger with log/slog

- **Context:**
  - `internal/logger/logger.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `github.com/Knatte18/loomyard/internal/logger` import in both files with `log/slog`, and repoint all seven call sites: `logger.Warn` at `ensureserver.go`'s wedged-daemon kill site and `lspclient.go`'s five shutdown/exit/kill sites become `slog.Warn`, and `logger.Info` at `ensureserver.go`'s spawn site becomes `slog.Info`.
  Keep every message string and every structured key/value pair byte-identical, including the `"scoutengine: "` prefixes — those are part of the operator-visible surface batch 5 compares.
  Add a package-level default handler so quarry's own logging goes to stderr at `slog.LevelWarn` unless the process has configured otherwise: declare it in `ensureserver.go` next to the other package-level state, and give it a doc comment explaining that `slog.Info` output is therefore suppressed by default and that this matches Loomyard's own logger defaulting to warn-level stderr.
  `lspclient.go` must end this card importing nothing outside the standard library.
  That is stronger than the file-scoped guard it carries today, which permits stdlib plus `internal/logger`, and card 22 tightens the guard test to assert it.
- **Commit:** `refactor(quarry): replace internal/logger with log/slog`

### Card 13: daemon state paths are told, not derived

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `DaemonStateFile(anchorRoot string, lang string) string` to `DaemonStateFile(stateDir string, lang string) string` returning `filepath.Join(stateDir, lang, "daemon.json")`, and `DaemonLock(anchorRoot string, lang string) string` to `DaemonLock(stateDir string, lang string) string` returning `filepath.Join(stateDir, lang, "daemon.lock")`.
  Delete the `scoutDirName` constant and the `github.com/Knatte18/loomyard/internal/lyxdirs` import;
  the `.lyx` and `scout` segments are removed outright, not relocated, because they are lyx vocabulary with no meaning in quarry and the engine derives no path structure it was not told.
  Both functions stay exported: quarry's own supervised-daemon tests use them, and a Go-API consumer needs to locate a daemon it did not spawn.
  Rewrite both doc comments completely. They currently explain anchor-joining, the durable `_lyx` sibling relationship, and the deliberate daemon re-keying in subpath-anchored repos — all three concepts cease to exist. The replacements must say the function is told a leaf state directory, joins only the language segment and the filename, and that the daemon remains a per-state-directory, per-language singleton.
  Also rewrite the file header comment and the `supervisedProtocolVersion` doc comment, both of which say "lyx".
  Leave `readDaemonState`, `writeDaemonState`, `daemonState`, and `daemonStale` untouched, including their `"scoutengine: "` error prefixes.
- **Commit:** `refactor(quarry): tell the engine its state directory instead of deriving it`

### Card 14: rename Options.AnchorRoot to Options.StateDir

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the exported field `Options.AnchorRoot` to `Options.StateDir` in `refs.go`, and rewrite its doc comment: it currently says the field "is required and must be a usable absolute path" with the caller obliged to populate it, and must instead name what the directory is — the leaf directory under which the supervised daemon's per-language `daemon.json`, `daemon.lock`, and `daemon.sock` live — while keeping the statement that it is required, absolute, and the caller's obligation.
  Repoint the threading site in `refs.go`'s `acquireConnection`, which passes `opts.AnchorRoot` into `ensureServer`.
  In `ensureserver.go`, rename the `anchorRoot` parameter to `stateDir` on `ensureServer` and `ensureSupervised` and at every use inside them, including the `DaemonStateFile`/`DaemonLock` calls at the top of `ensureSupervised` and the comment that describes the socket path as deterministic in `(anchorRoot, lang)`.
  Rewrite `ensureserver.go`'s file header comment, which names the seam as `EnsureServer(lang, anchorRoot)`.
  Do not change `ensureNative`, which takes no such parameter.
  The socket path derivation stays exactly as it is — `filepath.Join(filepath.Dir(statePath), "daemon.sock")` — because it is already told-geometry-correct: it derives from the state file path it was handed.
- **Commit:** `refactor(quarry): rename Options.AnchorRoot to StateDir`

### Card 15: LoadRegistry is told a file path

- **Context:**
  - `internal/configengine/config.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/load.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `LoadRegistry(baseDir string) (Registry, error)` to `LoadRegistry(path string) (Registry, error)`, delete the `configengine.ConfigFile(baseDir, "servers")` call and the `github.com/Knatte18/loomyard/internal/configengine` import, and use the told `path` directly in the `os.ReadFile` call and in both error-wrapping sites.
  Preserve every behaviour exactly: an absent file returns `builtins()` with no error;
  an empty or comments-only file returns `builtins()` via the existing `errors.Is(err, io.EOF)` special case;
  `KnownFields(true)` strictness stays;
  `validateEntry` failures are wrapped with the file path prefixed;
  and a file entry replaces the built-in entry whole, with no field-level merge.
  Keep the `"scoutengine: "` prefixes on the read and parse error messages unchanged.
  Rewrite the file header comment, which cites `internal/modelspec`, `configengine.ConfigFile`, and the Cwd Resolution Invariant — none of which exist in quarry;
  the replacement must say the loader is told a resolved absolute path and that resolution happens in `internal/cli`.
  Leave `registry.go`'s `builtins`, `BuiltinRegistry`, `Entry`, and `validateEntry` untouched by this card.
- **Commit:** `refactor(quarry): tell LoadRegistry a resolved config path`

### Card 16: rename the toolchain cache segment

- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `goToolchainCacheDir`, change the returned path from `filepath.Join(dir, "lyx", "tools", "go", version)` to `filepath.Join(dir, "quarry", "tools", "go", version)`, and in `goToolchainInstallLock` change `filepath.Join(dir, "lyx", "tools", "go", "install.lock")` to `filepath.Join(dir, "quarry", "tools", "go", "install.lock")`.
  Update the comment inside `goToolchainCacheDir` that names the resulting root as `"lyx/tools/..."`.
  Keep the `var userCacheDir = os.UserCacheDir` seam exactly as it is — it is the same pattern `internal/cli/paths.go` adopted in batch 2, and this axis stays engine-derived because no caller has a reason to override where quarry stashes a `gopls` it installed itself.
  Also fix the dangling citation in this file's comments to a `_mill/discussion.md` path, which will not exist in quarry: replace it with a prose statement of the reasoning it was citing, since the reasoning is still the reasoning behind the code.
- **Commit:** `refactor(quarry): rename the toolchain cache segment from lyx to quarry`

### Card 17: sweep lyx vocabulary out of the ported engine files

- **Context:**
  - `internal/scoutengine/doc.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/doc.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/registry.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/definition_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/symbol_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite every remaining case-insensitive `lyx` occurrence in the files above.
  `doc.go` carries eighteen and needs the most work: it is the module's as-built design record, and it cites lyx-specific invariants (the Cwd Resolution Invariant, `internal/modelspec`, `internal/webstercli`) as the reasoning behind design choices.
  Rewrite those citations into quarry's own terms — the reasoning they support is still the reasoning behind the code, so do not delete the passages.
  Where `doc.go` describes the config or state path shape, update it to the three-axis model this task introduces rather than leaving the `_lyx/config/servers.yaml` and `.lyx/scout/<lang>/` descriptions standing.
  Where it points at the research documents, repoint at their quarry `docs/` locations.
  The remaining files carry one to six occurrences each, mostly in comments and fixture names.
  Do not change any test's assertions or subjects in this card — it is a comment and identifier-naming sweep only, and a behavioural change made here would be invisible in the port diff.
  Two production files are in this card's `Edits:` even though cards 14 and 16 already touched them, because those cards' scopes were narrower than the vocabulary they leave behind.
  In `ensureserver.go`, card 14 rewrote only the parameter rename and the file header, leaving six doc-comment mentions untouched — at the source's lines 40, 160, 257, 260, 357, and 496, describing a daemon lyx spawned to outlive the call, a lyx-owned supervised daemon, this worktree's future lyx invocations, the `.lyx/scout/<lang>/` directory, and a Windows Job Object lyx itself created.
  In `toolchain.go`, card 16 rewrote the two path joins and the citation, leaving the same line's mention of routing through `internal/lyxcwd` and the Cwd Resolution Invariant, which quarry does not have.
  Finish by running `grep -ric 'lyx' /home/knatte/Code/quarry/wts/quarry/quarry/` and confirming every remaining hit is zero;
  files card 18 through 22 rewrite are allowed to still carry hits at the end of this card, so run the grep again after card 22 rather than treating a non-zero count here as a blocker.
- **Commit:** `docs(quarry): sweep lyx vocabulary out of the ported engine`

### Card 18: collapse the scout build tag onto lsp

- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_lsp_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain_integration_test.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the first line of each of the five files from `//go:build scout` to `//go:build lsp`.
  Then rewrite each file's header comment where it names the tag or the tier: the tag now names its actual precondition, which is a real language-server binary on `$PATH`, not a tool called scout.
  Three of these files — `ensureserver_integration_test.go`, `refs_integration_test.go`, and `toolchain_integration_test.go` — carry a comment mentioning `gitkit.HermeticGitEnv` to explain why they do *not* need it. Quarry has no such package, so reword those comments to state the underlying fact directly: the test spawns no git and needs no git-environment isolation.
  Confirm afterwards that `go -C /home/knatte/Code/quarry/wts/quarry vet -tags lsp ./quarry/` type-checks all five files, since a plain `go test ./...` never compiles them.
  Do not change any assertion in this card.
- **Commit:** `test(quarry): collapse the scout build tag onto lsp`

### Card 19: rewrite the state-path arithmetic tests

- **Context:**
  - `internal/scoutengine/daemonstate_test.go`
  - `internal/scoutengine/scoutdaemon_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/daemonstate_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/quarrydaemon_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both files exercise the `(anchorRoot, lang)` path arithmetic card 13 replaced, so they need rewriting rather than a path rewrite.
  In `quarrydaemon_test.go`, the source drives `DaemonStateFile`/`DaemonLock` over three fixtures — an unanchored worktree root, a subpath-anchored root, and a plain told directory — and two of those three fixture concepts no longer exist.
  Replace them with a single told-`stateDir` fixture built from `t.TempDir()`, asserting the joined results are `<stateDir>/<lang>/daemon.json` and `<stateDir>/<lang>/daemon.lock`.
  The per-language separation assertion — that two different `lang` values produce paths that do not collide — survives the rewrite and must not be dropped;
  it is the property that keeps concurrent daemons for different languages apart.
  Also assert that neither function introduces a `.lyx` or a `scout` segment, which is a direct regression guard on card 13.
  In `daemonstate_test.go`, keep every subtest covering `readDaemonState`, `writeDaemonState`, and `daemonStale` unchanged — their subjects did not move — and rewrite only the fixture construction where it builds an anchor root and lets the constructors derive the state path;
  those now pass a told directory directly.
  Preserve the subprocess-based confirmed-dead-PID fixture, which is what `daemonStale`'s liveness arm needs.
- **Commit:** `test(quarry): rewrite daemon state-path tests onto the told stateDir`

### Card 20: rewrite the registry overlay loader test

- **Context:**
  - `internal/scoutengine/load_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/load.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/load_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The source builds its fixture paths with `configengine.ConfigFile` and `configengine.ConfigDir` and calls `LoadRegistry(baseDir)`, both of which are gone.
  Retarget the table onto the told-path signature: each case writes its fixture file at a `t.TempDir()`-rooted path of its own choosing and passes that path directly to `LoadRegistry`.
  Keep every case in the existing table and its expectation — absent file returns `builtins()` with no error, empty or comments-only file returns `builtins()`, a valid overlay whole-replaces the built-in entry for that language, an unknown field errors because of `KnownFields(true)`, and an invalid entry errors with the file path prefixed to `validateEntry`'s message.
  Add one case the told-path signature makes newly expressible and newly worth covering: a path pointing at a directory rather than a file must return an error rather than silently falling back to built-ins, since the absent-file fallback keys on `os.ErrNotExist` specifically.
  Delete the `configengine` import.
- **Commit:** `test(quarry): retarget the registry loader test onto the told config path`

### Card 21: rewrite the live references test for the Options field rename

- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs_integration_test.go`
- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This file sets `AnchorRoot:` in three `Options` literals, so it cannot be ported on a path rewrite alone.
  Rename each to `StateDir:` and change the value each passes: today they pass a worktree root and let the engine derive `<root>/.lyx/scout/<lang>/`, and they must now pass a told leaf state directory, which for a test is a fresh `t.TempDir()`.
  Every assertion about the returned references — their files, lines, characters, and ordering — is unaffected and must be preserved exactly;
  this is a fixture-construction change, not a subject change.
  Sweep this file's eighteen lyx occurrences at the same time, since card 17 deliberately left this file to the card that rewrites it.
  This file is `//go:build lsp` after card 18, so `go test ./...` will not compile it;
  check it with `go -C /home/knatte/Code/quarry/wts/quarry vet -tags lsp ./quarry/` and leave running it to batch 5.
- **Commit:** `test(quarry): retarget the live references test onto Options.StateDir`

### Card 22: retarget the two seam guard tests

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/seam_enforcement_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `seam_enforcement_test.go` scans every non-test file in the package for banned imports;
  retarget its banned list from Loomyard paths to quarry's: `github.com/Knatte18/quarry/internal/output`, any `github.com/spf13/cobra` import, and any import path containing `/internal/` and ending in `cli`.
  The `internal/clihelp` entry has no quarry equivalent — that package's surface was absorbed into `internal/cli` in batch 2, which the `/internal/`-plus-`cli`-suffix rule already covers — so drop the separate entry rather than leaving it matching nothing.
  Keep the zero-files-scanned guard that fails the test when it finds nothing to check;
  it is what stops the invariant going green by accident after a directory rename.
  `lspclient_guard_test.go` today allows stdlib plus `github.com/Knatte18/loomyard/internal/logger`. Card 12 made that file stdlib-only, so tighten the guard: delete the `allowedLyxImport` constant and its use, and assert stdlib-only.
  Rewrite the header comments of both files, which name Loomyard's `CONSTRAINTS.md`, the Scout Engine-Seam Invariant, and "no lyx dependency except logging";
  the replacements state the same property as a quarry-owned invariant — the public `quarry` package never imports the CLI package, cobra, or the output-envelope package, and `internal/cli` is the sole place engine results become JSON.
- **Commit:** `test(quarry): retarget the engine seam and lspclient guards`

### Card 23: record batch 3 in the port log

- **Context:** none
- **Edits:**
  - `docs/research/quarry-port-log.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a `## Batch 3 — port-engine` section to the port log in this worktree.
  Record the count of files the port program copied, the two files renamed on landing, the two files deliberately excluded, the confirmed count of `"scoutengine: "` literals before and after (both must be 59), the final `grep -ric 'lyx'` count across the ported engine directory, and the quarry commit SHAs from cards 10 through 22.
  Record the hand-edited surface explicitly — the seven `logger` call sites, the two path-ownership signature changes, the `Options` field rename, and the toolchain cache segment — so a reviewer can tell at a glance which part of the diff the port program produced and which part a person wrote.
- **Commit:** `docs: record batch 3 in the quarry port log`

## Batch Tests

`verify:` runs `go -C /home/knatte/Code/quarry/wts/quarry test ./...`, which compiles the whole module and runs the hermetic tier: batch 1's three leaf-package suites, batch 2's three `internal/cli` suites, and the fourteen untagged engine test files this batch lands (`daemonstate_test.go`, `definition_test.go`, `detect_test.go`, `ensureserver_test.go`, `load_test.go`, `lspclient_guard_test.go`, `lspclient_test.go`, `position_test.go`, `quarrydaemon_test.go`, `refs_test.go`, `registry_test.go`, `seam_enforcement_test.go`, `supervised_test.go`, `symbol_test.go`, `toolchain_test.go`).
The scope is `./...` rather than a file list because this is the batch that first makes the module compile end to end, and a compile failure in any package is exactly what the gate needs to catch.

The five `//go:build lsp` files this batch touches are not compiled by that command.
Cards 18 and 21 therefore each require a `go vet -tags lsp ./quarry/` type-check as their own verification, and batch 5 is where they actually run.

Nine of the fourteen untagged suites are ported unchanged;
a failure in one of them means the mechanical copy is wrong, not that behaviour changed.
The five that were rewritten (`daemonstate_test.go`, `quarrydaemon_test.go`, `load_test.go`, `seam_enforcement_test.go`, `lspclient_guard_test.go`) are where a genuine design regression would surface, and each card names the specific assertion that must survive its rewrite.
