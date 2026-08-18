# Discussion: scoutengine told-geometry (optional uniformity pass)

```yaml
task: scoutengine told-geometry (optional uniformity pass)
slug: scout-told-geometry
status: discussing
parent: standalone-producers
```

## Problem

`internal/scoutengine` is the last producer engine that takes a `*lyxcwd.Location` instead of told, already-resolved absolute paths.
Its two path constructors — `DaemonStateFile(l *lyxcwd.Location, lang string)` and `DaemonLock(l *lyxcwd.Location, lang string)` in `internal/scoutengine/daemonstate.go` — read `l.AnchorPath()` and nothing else, and `Options.Layout` exists solely to carry that `Location` down to them.
Because scout must keep working outside a lyx hub, `internal/scoutcli/cli.go`'s `resolveLocation` (lines 440-468) *mints a fictional `Location`* whenever `lyxcwd.Resolve(cwd)` fails: `HubPath` is merely the parent of the target directory and `RepoName` is left zero.
That fiction is documented and deliberately bounded, but it is the one place in the repo that fabricates a `Location`, and the whole `producers-standalone` design names it as the rejected alternative to told-geometry.

**Why now.** Waves 1-3 of `manifest/designs/producers-standalone.md` have landed: `shuttleengine`, `reedengine`, `pattern`, `burlerengine`, `perchengine`, `websterengine` all now take plain strings or a per-engine `Geometry` struct, and `internal/hubgeom`/`internal/standalonegeom` are the hub-mode and told-mode tellers that build those from a resolved `Location`.
Scout is the outlier.
T9 is explicitly **optional** and delivers **no new capability** — `lyx scout` already runs against an arbitrary folder with zero lyx setup today.
It buys uniformity, and it deletes the fictional-`Location` synthesis outright, which is the cleanest available outcome for that site.
T10 (`standalone-docs-and-invariants`) must otherwise record scout's remaining deviation as a documented exception.

## Scope

**In:**

- `internal/scoutengine/daemonstate.go` — `DaemonStateFile`/`DaemonLock` take `anchorRoot string`; drop the `lyxcwd` import; update the file header and both doc comments.
- `internal/scoutengine/refs.go` — `Options.Layout *lyxcwd.Location` becomes `Options.AnchorRoot string`; drop the `lyxcwd` import; update the field doc and `acquireConnection`'s call.
- `internal/scoutengine/ensureserver.go` — `ensureServer` and `ensureSupervised` take `anchorRoot string` in place of `layout *lyxcwd.Location`; drop the `lyxcwd` import; update the socket-path comment that says "a deterministic function of (layout, lang)".
- `internal/scoutengine/doc.go` — the "Daemon state and concurrency" section's "per (layout, lang)" wording.
- `internal/scoutcli/cli.go` — delete `resolveLocation` entirely; `lookupContext` returns `(scoutengine.Registry, string, error)` where the string is the anchor root; `buildOptions` takes `anchorRoot string`; all four call sites (`refs` ~151/177/192, `definition` ~286/312/327, `symbol` ~396/403/426, `assert-no-callers` ~593/605) rename their `layout` local to `anchorRoot`.
- Tests in `internal/scoutengine`: `scoutdaemon_test.go` (rewritten as told-string path math), `ensureserver_test.go:354-355`, `supervised_test.go:65/67-68, 130/132-133, 209/211-212`, `supervised_scout_test.go:26/28, 88/90`, `supervised_integration_test.go:57-58`, `ensureserver_integration_test.go:143-144`, `refs_integration_test.go:84-85/94, 200-201/210, 239-240/249`.
- `internal/scoutcli/cli_test.go` — the `resolveLocation`, `lookupContext`, and `buildOptions` tests (lines ~555-635).
- `internal/scoutcli/cli_integration_test.go` — **new file**, `//go:build integration`, pinning `lookupContext`'s hub-mode branch against a real `hubforge.NewHub` fixture. See Testing.
- `cmd/lyx/constructoranchoring_test.go` — its `scoutengine.DaemonStateFile`/`DaemonLock` rows (102-103, 160-161, 181-182).
- `cmd/lyx/notransients_test.go` — its `transientSet` rows (79-80). **This file is missing from T9's Files list in the design doc and will not compile without the change; treat that as a Files-list correction, not a scope expansion.**

**Out:**

- `lookupContext`'s registry fallback to `scoutengine.BuiltinRegistry()` — already the correct shape, stays byte-identical.
- `Options.TargetDir` — already told, unchanged.
- Any new `--anchor-root`, `--state-dir`, or similar CLI flag. Scout needs no new observable CLI surface; the anchor root stays derived exactly as today (resolved `Location`'s `AnchorPath()` in a hub, absolute target dir outside one).
- A `scoutengine.Geometry` struct, a `hubgeom.ScoutGeometry`, or a `standalonegeom` scout teller — see Decisions.
- Adding `internal/lyxcwd` to `scoutengine`'s banned-import list in `seam_enforcement_test.go`, and any `CONSTRAINTS.md` edit — T10 owns that, uniformly across every producer package.
- `docs/overview.md` — the module table and execution-stack description are unaffected by a signature change.
- `manifest/roadmap.md` — T10 moves the wave entries; per `CLAUDE.md` the roadmap moves only on completing a planned item, and T9 alone does not complete the wave.
- `manifest/designs/producers-standalone.md` — T10 deletes it per the documentation lifecycle; this task does not edit it. **This leaves the document knowingly stale, and the staleness is accepted, not overlooked** — see the disposition below.
- The daemon lifecycle, staleness check, wedged-daemon escalation, toolchain manager, and every LSP behaviour — pure signature/plumbing change, zero behavioural change intended.

## Decisions

### told parameter shape — a bare `anchorRoot string`, not a `Geometry` struct

- **Decision:** `DaemonStateFile(anchorRoot string, lang string)` and `DaemonLock(anchorRoot string, lang string)`. No `scoutengine.Geometry` type, no new `geometry.go` file in the package.
- **Rationale:** The design's "told-geometry structs per engine" rule exists to stop positional parameter lists reaching four or five strings (`reedengine` is the named example, at five). Scout is told exactly **one** path. `websterengine.Dir(anchorRoot)`, `websterengine.ReportsDir(anchorRoot)`, `perchengine.RunsDir(anchorRoot)`, and `planparser.PlanDir(anchorRoot)` are all bare-string free functions today, and `cmd/lyx/constructoranchoring_test.go` already calls them as `f(l.AnchorPath())`. Scout's rows become the same shape as its neighbours in that table, which is the uniformity this task is buying.
- **Rejected:** A one-field `Geometry{AnchorRoot string}` struct for symmetry with burler/perch/webster/reed — it adds a type, a file, and a doc comment to wrap a single string, which is the YAGNI this design otherwise enforces. Revisit only if scout later grows a second told path.

### `Options.Layout` becomes `Options.AnchorRoot string`

- **Decision:** `scoutengine.Options` drops `Layout *lyxcwd.Location` and gains `AnchorRoot string`. The value threads unchanged through `acquireConnection` → `ensureServer` → `ensureSupervised`, each renaming its `layout *lyxcwd.Location` parameter to `anchorRoot string`.
- **Rationale:** Direct translation of what the engine reads. `Options.Layout`'s doc says "required and must be non-nil"; the told equivalent is "required, must be a usable absolute path, populating it is entirely the caller's obligation" — the exact wording `burlerengine.Geometry` and `websterengine.Geometry` already use. The engine validates nothing, consistent with every converted sibling.
- **Accepted consequence — the *misuse* failure mode goes from loud to silent.** Today a caller that forgets to populate `Layout` panics at `l.AnchorPath()` (`refs.go:52` says "required and must be non-nil"; nothing validates it, so a nil pointer dereferences). After the change an empty `AnchorRoot` does not panic — `filepath.Join("", lyxdirs.DotLyxDirName, "scout", lang, "daemon.json")` yields the *relative* path `.lyx/scout/<lang>/daemon.json`, and the daemon writes its state wherever the process happens to stand. This is accepted, not overlooked: it is the identical trade every converted sibling already made (`burlerengine`, `perchengine`, `websterengine` all validate no `Geometry` field), and adding a scout-only guard would reintroduce the asymmetry this task exists to remove. **It is explicitly outside the "zero behavioural change" property below**, which covers the two real call paths only, never the never-taken misuse path. If a future task wants told-geometry validated, it must add it uniformly across all four engines, not here.
- **Rejected:** Keeping `Layout` and deriving `anchorRoot` inside the engine — that keeps `lyxcwd` in `scoutengine`'s import graph and leaves the fictional-`Location` synthesis alive in `scoutcli`, i.e. it does none of the work.
- **Rejected:** Having `ensureSupervised` return an error on an empty `anchorRoot` to preserve loudness — it puts validation in the one engine whose siblings have none, and `scoutcli` is the only caller, which always populates it.

### `resolveLocation` is deleted, and its job folds into `lookupContext`'s existing `Resolve`

- **Decision:** Delete `resolveLocation` (`internal/scoutcli/cli.go:440-468`) outright. `lookupContext(cwd, dir string) (scoutengine.Registry, string, error)` performs **one** `lyxcwd.Resolve(cwd)` and derives both results from it:
  - `Resolve` succeeds → `registry = scoutengine.LoadRegistry(layout.AnchorPath())` (unchanged, error still propagates) and `anchorRoot = layout.AnchorPath()`.
  - `Resolve` fails → `registry = scoutengine.BuiltinRegistry()` (unchanged) and `anchorRoot = filepath.Abs(dir)`.
- **Rationale:** Today `lookupContext` calls `lyxcwd.Resolve(cwd)` for the registry and then `resolveLocation` calls it a *second* time for the layout, discarding and re-deriving the same object. With told geometry there is no object left to synthesize, so the second call has nothing left to justify it. One `Resolve`, two derived values, and the repo's only fictional `Location` is gone — which the design names as "the cleanest possible outcome for the one place in the repo that currently mints a fictional `Location`".
- **Rejected:** A standalone `resolveAnchorRoot(cwd, targetDir) string` helper beside `lookupContext` — it preserves the double-`Resolve` and keeps a function whose only caller is `lookupContext`. Rejected as a mechanical translation that leaves the actual smell in place.

### the `filepath.Abs` error fallback is preserved byte-for-byte

- **Decision:** When `filepath.Abs(dir)` returns an error, `lookupContext` yields `filepath.Clean(dir)` as the anchor root.
- **Rationale:** The old code's fallback built `&lyxcwd.Location{HubPath: filepath.Dir(targetDir), WorktreeName: filepath.Base(targetDir), AnchorRel: "."}`, whose `AnchorPath()` is `filepath.Join(filepath.Dir(targetDir), filepath.Base(targetDir))` — which is `filepath.Clean(targetDir)`. `resolveLocation`'s own comment states this branch exists "so the failure mode does not silently change", and this task must honour that. `filepath.Clean(dir)`, not bare `dir`, is the equivalent.
- **Rejected:** Returning `dir` unchanged (drops the `Clean`, a silent change to a documented failure mode), or dropping the branch entirely (`filepath.Abs` only fails when `os.Getwd` fails, but the existing code chose to handle it and this task is not the place to reverse that).

### the anchor-root semantics are unchanged in both modes

- **Decision:** In a hub, `anchorRoot == layout.AnchorPath()`. Outside a hub, `anchorRoot == filepath.Abs(targetDir)`.
- **Rationale:** The old synthesis always set `AnchorRel: "."`, so its `AnchorPath()` coincided with `WorktreePath()` byte for byte — i.e. the absolute target directory. Both branches therefore reproduce today's `DaemonStateFile`/`DaemonLock` inputs exactly. **No daemon is re-keyed by this task**, in either mode, in either an unanchored or a subpath-anchored repo. Any change to a resolved daemon-state path is a regression, not an improvement.
- **Rejected:** Simplifying to "always the target directory" — that re-keys the daemon in every subpath-anchored repo, contradicting `DaemonStateFile`'s doc comment about the deliberate anchor re-keying already in force.

### enforcement and invariants stay with T10

- **Decision:** Do not add `internal/lyxcwd` to `seam_enforcement_test.go`'s banned list, and do not edit `CONSTRAINTS.md`.
- **Rationale:** T6 and T7 made `burlerengine`, `perchengine`, and `websterengine` production code `lyxcwd`-free without adding a per-package ban to any of them. T10 explicitly owns "an import-allowlist test per producer package where one exists, review obligation where none does", landing the three-tier invariant uniformly once every package obeys it. Adding a scout-only ban here would make scout the odd one out again — the opposite of what a uniformity pass is for.
- **Rejected:** Landing the ban plus a `CONSTRAINTS.md` reword in this commit. The repo rule "record any new cross-cutting invariant there, same commit" applies to invariants a commit *introduces*; this commit introduces no new rule, it makes scout conform to one T10 will state.
- **Consequence for T10:** T10's "Record scout's remaining deviation if T9 was skipped" becomes a no-op. T10 should instead include `internal/scoutengine` in whatever per-producer enforcement it lands.

### documentation lands in `doc.go` and file headers, same commit

- **Decision:** Update `internal/scoutengine/doc.go`'s "Daemon state and concurrency" section, `daemonstate.go`'s file header and both function doc comments, `refs.go`'s `Options.AnchorRoot` field doc, `ensureserver.go`'s socket-path comment, and `internal/scoutcli/cli.go`'s `lookupContext`/`buildOptions` comments — all in the implementing commit.
- **Rationale:** `scoutengine` has no `manifest/designs/` module doc (deleted on landing per the documentation lifecycle; `docs/overview.md:408` records that its durable rationale lives in the package documentation). `doc.go` *is* the module doc for this package, so the `CLAUDE.md` "docs land in the same commit" rule points there.
- **Note:** every comment that describes the *fiction* — `resolveLocation`'s whole doc block, `daemonstate.go`'s "built on `*lyxcwd.Location`", `doc.go`'s "per (layout, lang)" — must be deleted or rewritten, not left describing a shape that no longer exists.

### disposition of `producers-standalone.md`'s own staleness

- **Decision:** Leave `manifest/designs/producers-standalone.md` untouched, knowingly stale on three counts, all bounded by T10's deletion of the file:
  - `:198` describes scout's synthesized `Location` as a live "deliberate, documented fiction". After this task it describes nothing that exists.
  - `:641`'s T9 Files list cites `cmd/lyx/constructoranchoring_test.go` rows **91-92/140-141**; the live rows are **102-103/160-161/181-182**, and `cmd/lyx/notransients_test.go` is missing from the list entirely. The Scope section above is the corrected enumeration — where the two disagree, Scope wins.
  - T10's own Files list claims "the `doc.go` of each converted package", which overlaps this task's decision to update `internal/scoutengine/doc.go` here.
- **Rationale:** The "must not leave a comment describing a shape that no longer exists" rule in the Note above governs **Go comments and package docs** — the durable, in-code documentation a reader hits while working. A planning document already scheduled for deletion by a named downstream task is a different artifact with a different lifecycle: editing it would touch a file T6, T7, and T8 all deliberately left alone, and would create merge contention across wave 4 for text that ceases to exist in wave 5.
- **On the `doc.go` overlap:** touching `internal/scoutengine/doc.go` twice is by design, not duplicated work. This task removes the `*lyxcwd.Location` wording because it becomes false the moment the code lands (the same-commit docs rule). T10 later adds the cross-cutting three-tier invariant reference once every package obeys it. Different content, different preconditions.
- **Rejected:** Correcting `:198` and `:641` in this commit — it edits a doomed document, contends with wave-4 siblings, and none of the corrections outlive T10.
- **Carried to T10:** if T9 lands, T10's "Record scout's remaining deviation if T9 was skipped" is a no-op, and `internal/scoutengine` should instead be included in whatever per-producer enforcement T10 lands.

## Technical context

**Current call chain.**
`internal/scoutcli/cli.go` → `lookupContext(cwd, dir)` → `(registry, layout, err)` → `buildOptions(registry, dir, layout, lang, query, timeout)` → `scoutengine.Options{Layout: layout}` → `scoutengine.References`/`Definition`/`Symbol` → `lookup` → `acquireConnection(ctx, lang, entry, opts)` → (only when `entry.HasNativeDaemon`) `ensureServer(ctx, lang, entry, opts.TargetDir, opts.Layout, opts.Timeout)` → `ensureSupervised(ctx, command, lang, targetDir, layout, timeout)` → `DaemonStateFile(layout, lang)` / `DaemonLock(layout, lang)`.
`opts.Layout` is read at exactly one place at the bottom of that chain (`ensureserver.go:300-301`). Nothing else in the engine touches it.

**The four `scoutcli` command call sites** all follow the identical three-line shape and must be updated together:

| command | `lookupContext` | `buildOptions` calls |
|---|---|---|
| `refs` | 151 | 177, 192 |
| `definition` | 286 | 312, 327 |
| `symbol` | 396 | 403, 426 |
| `assert-no-callers` | 593 | 605 |

**Precedent to copy, in order of usefulness.**

- `internal/websterengine/state.go:41/49/58/67` — **this is the file that declares the bare-`anchorRoot string` free functions** (`Dir`, `ReportsDir`, `ScratchDir`, `PromptsDir`). It is the shape to copy for `DaemonStateFile`/`DaemonLock`. Do **not** copy `internal/websterengine/geometry.go` for the shape — that file holds webster's eight-field `Geometry` struct, which is precisely the shape this task's first Decision rejects for scout.
- `internal/websterengine/webstergeom_test.go` — the three-case test for those free functions (unanchored `Location`, subpath-anchored `Location`, pure told directory with no `Location` at all). Scout's rewritten `scoutdaemon_test.go` should mirror this structure directly.
- `internal/burlerengine/geometry.go` and `internal/websterengine/geometry.go` — cited for the **doc-comment wording only** ("populating every field with a usable absolute path is entirely the caller's obligation"), never for the parameter shape.
- `internal/perchcli/cli_integration_test.go` — the `hubforge.NewHub(t, ".")` / `h.PrimeWorktree()` fixture shape for the new hub-mode test below.
- `internal/hubgeom/hubgeom.go` — the hub-mode teller pattern. **Not needed for scout** (one string, read inline as `l.AnchorPath()`), but it is why `cmd/lyx` test rows read `f(l.AnchorPath())`.
- Commit `33018982` (`burlerengine + perchengine told-geometry`) and `3255efa6` (`websterengine + webstercli told-geometry`) — the exact file-set and comment-rewrite discipline of a landed sibling task.

**Gotchas found during exploration.**

1. `cmd/lyx/notransients_test.go:79-80` calls both constructors and is **not** in T9's Files list. It must change or `./cmd/lyx/...` fails to build.
2. Six `scoutengine` test files hand-build `&lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot), AnchorRel: "."}` purely to feed these two functions. In every case `l.AnchorPath()` is just `worktreeRoot`, so the conversion is: delete the `Location` line, delete the `lyxcwd` import, pass `worktreeRoot` directly. Four of those files are `//go:build scout`-tagged (`supervised_scout_test.go`, `supervised_integration_test.go`, `refs_integration_test.go`, `ensureserver_integration_test.go`) and will not compile under an untagged `go test` — they must be verified with `go test -tags scout`.
3. `refs_integration_test.go` sets `Layout: l` inside a `scoutengine.Options` literal at three sites (94, 210, 249) *and* separately calls `DaemonStateFile(l, "go")` at 85/201/240. Both need updating in each block.
4. `internal/scoutcli/cli_test.go` has three tests bound to the old shape: `resolveLocation`'s own test (~555-575, deleted with the function), `TestLookupContext_OutsideHubReturnsSynthesizedLocationAndBuiltinRegistry` (~577-606, renamed and reshaped to assert an absolute-path string), and `TestBuildOptions_ThreadsEveryFieldFromItsArguments` (~609-635, asserting `got.AnchorRoot` rather than `got.Layout.WorktreePath()`). Several other tests in that file chdir into a non-git temp dir specifically to exercise the degraded path — that setup stays valid and should not be disturbed.
5. `daemonstate.go`'s `scoutDirName` constant and the `lyxdirs.DotLyxDirName` join are unchanged; only the first argument of the two `filepath.Join`s changes from `l.AnchorPath()` to `anchorRoot`.
6. `internal/scoutengine/toolchain.go`'s machine-global `os.UserCacheDir()` path is deliberately outside cwd-resolution scope and is **not** part of this task.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone. This task strengthens compliance: after it, `scoutengine` production code performs no cwd resolution and holds no `Location`. `scoutcli` keeps its single `lyxcwd.Resolve(cwd)` (via `lyxcwd.CwdFrom(ctx)` for the seam cwd), which is the sanctioned call site. Raw `os.Getwd` and `git rev-parse` stay banned. A module's own durable/ephemeral subdirectory is its own private relative-path constant joined onto the anchor — `scoutDirName` already is exactly that, and stays.
- **Durable-vs-Ephemeral State Invariant** — scout's daemon state stays under `.lyx/scout/<lang>/`, never `_lyx/`. `cmd/lyx/notransients_test.go` is the machine guard and must keep passing at both `AnchorRel == "."` and `AnchorRel == "backend"` fixtures.
- **Scout Engine-Seam Invariant** (`CONSTRAINTS.md:159-172`) — `internal/scoutengine` never imports `internal/output`, `cobra`, any `internal/*cli` package, or `internal/clihelp`; `scoutcli` → `scoutengine` is the only allowed direction. Unaffected by this task, but `seam_enforcement_test.go` must stay green and its banned list must **not** be extended here.
- **`lspclient.go` file-scoped guard** — stdlib plus `internal/logger` only. `lspclient.go` is untouched by this task; `lspclient_guard_test.go` must stay green.
- **Test Tier Purity Invariant** (`CONSTRAINTS.md:444`) — an untagged test file must not call `exec.Command`, `gitexec.Run`, `gitkit.Copy*`, or `hubforge.NewHub`. The rewritten `scoutdaemon_test.go` stays pure path arithmetic with no spawning, preserving its Tier 1 status. `daemonstate_test.go`'s existing file-level allowlist entry in `cmd/lyx/tierpurity_test.go` is unaffected.
- **CLI/Cobra Invariant** — no new command, flag, `Short`, or `Long` is introduced, so the help-tree tests need no update. If the implementation finds itself adding a flag, that is out of scope and should be flagged rather than landed.
- **Documentation Lifecycle** — `scoutengine`'s module doc was deleted on landing; `internal/scoutengine/doc.go` is its durable home and is the doc that must move in this commit.

Discovered during discussion:

- **Zero behavioural change.** Every resolved daemon-state and lock path must be byte-identical before and after, in hub mode and out-of-hub mode, at both unanchored and subpath-anchored geometries. This is the single acceptance property the whole task hangs on. It covers the two real call paths only; the never-taken *misuse* path (an unpopulated `AnchorRoot`) is explicitly excluded and its loud-to-silent shift is recorded as an accepted consequence under Decisions.
- **Both halves of that property need named automated evidence.** Out-of-hub is covered by the reshaped `lookupContext` test; hub mode is covered by the new `//go:build integration` test in `internal/scoutcli`. Neither half may rest on the manual smoke run.
- **`lyx scout` must keep resolving symbols in a directory outside any hub** — the design doc names this as "the behaviour this task must not regress".

## Testing

**`internal/scoutengine` — TDD candidate: `scoutdaemon_test.go`.**
Rewrite it as told-string path math over three fixtures, mirroring `internal/websterengine/webstergeom_test.go`:

- an unanchored worktree root (today's `filepath.Join("home", "user", "repo-HUB", "repo")` value, so the expected strings are unchanged from the current file),
- a subpath-anchored root (`<worktree>/backend`), proving the accessors move down with the anchor,
- a plain told directory not derived from any `Location` (`/var/lib/lyx-standalone/state` style), proving they need only a string.

Keep the existing per-language distinctness tests (`go` vs `python` for both state file and lock). Drop the `lyxcwd` import. Write these first — they are pure path math and pin the "byte-identical" property before any production edit.

**`internal/scoutengine` — mechanical conversions.**
`ensureserver_test.go`, `supervised_test.go`, `supervised_scout_test.go`, `supervised_integration_test.go`, `ensureserver_integration_test.go`, `refs_integration_test.go`: replace the hand-built `Location` with the bare `worktreeRoot` string it was wrapping. No assertion changes — if any of these tests changes behaviour, the migration is wrong.

**`internal/scoutcli` — TDD candidate: the reshaped out-of-hub `lookupContext` test.**
Replace `TestLookupContext_OutsideHubReturnsSynthesizedLocationAndBuiltinRegistry` with a test asserting the out-of-hub return is `(BuiltinRegistry(), filepath.Abs(dir))` — an absolute-path *string*, no `Location`. Keep the existing setup shape: the current test passes two `t.TempDir()` values as the `cwd`/`dir` arguments and **never chdirs** (`cli_test.go:584-607`); the chdir-into-a-non-git-temp-dir setup belongs to the separate `RunCLI_*_NoLanguageError` tests at lines 82, 165, 205, 798. Do not introduce a process-wide chdir the current test does not have. Cover:

- out-of-hub with an explicit `dir` → anchor root is `filepath.Abs(dir)`, registry is the built-in;
- out-of-hub with `dir` defaulted from `cwd` → anchor root is `filepath.Abs(cwd)`, **not** `filepath.Abs("")`. `lookupContext`'s existing doc comment calls out this exact trap ("dir is the already-defaulted directory, never the raw `--target-dir` flag value") and it must survive the rewrite.

Delete `resolveLocation`'s own test with the function. Reshape `TestBuildOptions_ThreadsEveryFieldFromItsArguments` to assert `got.AnchorRoot` directly — a plain string comparison, simpler than the old `WorktreePath()` round-trip.

**`internal/scoutcli` — the hub-mode branch, in a new `cli_integration_test.go`.**
The out-of-hub tests above cover only half the acceptance property. The hub branch — `anchorRoot = layout.AnchorPath()` when `lyxcwd.Resolve(cwd)` succeeds — has no automated coverage today and must not be left to the manual smoke run, which this section elsewhere forbids as sole evidence. `cmd/lyx/constructoranchoring_test.go` does **not** close this: it exercises the two constructors directly and never calls `lookupContext`.

Add `internal/scoutcli/cli_integration_test.go`, first line `//go:build integration` (mandatory — `hubforge.NewHub` is banned in untagged tests by the Test Tier Purity Invariant), following `internal/perchcli/cli_integration_test.go`'s fixture shape:

- Build a hub with `hubforge.NewHub(t, ".")`.
- Call `lookupContext(h.PrimeWorktree(), <a separate t.TempDir()>)` — deliberately passing a `dir` that is **not** the worktree.
- Assert the returned anchor root is the hub worktree's anchor, **not** `filepath.Abs(dir)`. This is the discriminating assertion: an implementation that wrongly took the out-of-hub branch in both cases fails it, which a same-value fixture would not catch.
- Assert the returned registry is the loaded overlay rather than `BuiltinRegistry()`, pinning that `LoadRegistry(layout.AnchorPath())` still anchors where it did.

This is the one genuinely new test the task adds; everything else is a conversion.

**`cmd/lyx` — the anchoring tables.**
`constructoranchoring_test.go` rows 102-103, 160-161, 181-182 and `notransients_test.go` rows 79-80 become `scoutengine.DaemonStateFile(l.AnchorPath(), "go")` / `scoutengine.DaemonLock(l.AnchorPath(), "go")`, matching the `websterengine.Dir(l.AnchorPath())` rows directly above them. Expected paths are unchanged — that is the point.

**Commands.**

- `go test ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/...`
- `go test -tags scout ./internal/scoutengine/...` — mandatory, not optional: four of the converted test files are `scout`-tagged and are invisible to the untagged run.
- `go test -tags integration ./internal/scoutcli/...` — mandatory: the new hub-mode test is the only automated evidence for the hub half of the acceptance property, and it is invisible to the untagged run.
- `go build ./...` — cheap guard that no other package referenced the changed symbols.
- Manual acceptance, as a smoke check on top of the tests and never as the only evidence: run `lyx scout symbol <name> --target-dir <dir>` from a scratch directory outside any git repository and confirm it still resolves, and run `lyx scout refs` inside this worktree and confirm `.lyx/scout/go/daemon.json` appears at the same path as before the change.

## Q&A log

- **Q:** How is the told anchor passed to `DaemonStateFile`/`DaemonLock` — a bare `anchorRoot string` or a `scoutengine.Geometry` struct? **A:** [auto-pick] Bare `anchorRoot string` positional. **Why:** scout is told exactly one path; the design's geometry-struct rule exists to avoid four- and five-string positional lists, and `websterengine.Dir`/`perchengine.RunsDir`/`planparser.PlanDir` are all bare-string free functions already.
- **Q:** What replaces `Options.Layout *lyxcwd.Location`? **A:** [auto-pick] `Options.AnchorRoot string`, threaded unchanged through `acquireConnection` → `ensureServer` → `ensureSupervised`. **Why:** direct translation of the only value the engine reads; the engine validates nothing, matching every converted sibling.
- **Q:** What happens to `scoutcli.resolveLocation` — delete and fold into `lookupContext`, or keep a `resolveAnchorRoot` helper? **A:** [auto-pick] Delete it; fold into `lookupContext`'s existing `lyxcwd.Resolve(cwd)`, returning `(Registry, string, error)`. **Why:** today's code resolves twice and discards the first result; with no object to synthesize, the second `Resolve` has nothing left to justify it.
- **Q:** Do the `Location`-typed test fixtures inside `scoutengine` survive? **A:** [auto-pick] Convert to plain strings and drop the `lyxcwd` test import. **Why:** `l.AnchorPath()` is just `worktreeRoot` in every one of them; `cmd/lyx/constructoranchoring_test.go` remains the single `Location`-driven table, which is where that coverage belongs.
- **Q:** Does this task ban `internal/lyxcwd` in `scoutengine`'s seam test and reword `CONSTRAINTS.md`? **A:** [auto-pick] No — leave it to T10. **Why:** T6/T7 landed burler/perch/webster without a per-package ban, and T10 explicitly owns per-producer enforcement; a scout-only ban would recreate the asymmetry this task removes.
- **Q:** How is the `filepath.Abs` error fallback preserved? **A:** [auto-pick] As `filepath.Clean(dir)`. **Why:** the old synthesis's `AnchorPath()` on that branch was `filepath.Join(filepath.Dir(dir), filepath.Base(dir))` = `filepath.Clean(dir)`, and the existing comment requires the failure mode not to change silently.
- **Q:** `cmd/lyx/notransients_test.go` calls both constructors but is absent from T9's Files list — include it? **A:** [auto-pick] Include it, as a Files-list correction. **Why:** `./cmd/lyx/...` will not compile otherwise.
- **Q:** Where do the doc updates land, given `scoutengine` has no `manifest/designs/` doc? **A:** [auto-pick] `internal/scoutengine/doc.go` plus the affected file headers and comments, same commit; no `docs/overview.md` and no `manifest/roadmap.md` change. **Why:** the module doc was deleted on landing and `doc.go` is its durable home; the module table, execution stack, and wave completion are all unaffected by a signature change.
- **Q:** [review r1 gap] The acceptance property claims byte-identical paths in *both* modes, but every named test covered only the out-of-hub branch — `lookupContext`'s hub branch (`anchorRoot = layout.AnchorPath()`) had no automated evidence, and `constructoranchoring_test.go` does not call `lookupContext` at all. Name a hub test, or justify the gap? **A:** [auto-pick] Name one: a new `internal/scoutcli/cli_integration_test.go` (`//go:build integration`) driving a `hubforge.NewHub` fixture, calling `lookupContext(h.PrimeWorktree(), <separate t.TempDir()>)` and asserting the anchor root is the hub anchor rather than `filepath.Abs(dir)`. **Why:** the justify-the-gap alternative would rest the hub half of the sole acceptance property on a manual smoke run that the same section forbids as sole evidence; the mismatched-`dir` fixture is what makes the assertion discriminating rather than tautological, and `perchcli/cli_integration_test.go` already supplies the fixture shape at near-zero cost.
- **Q:** What is the test strategy for the new told accessors? **A:** [auto-pick] Rewrite `scoutdaemon_test.go` as told-string path math across unanchored, subpath-anchored, and pure-told-directory cases; add a reshaped `scoutcli` `lookupContext` test; leave the `-tags scout` integration tests behaviourally unchanged. **Why:** mirrors `webstergeom_test.go`, and pins the byte-identical-paths property that is this task's sole acceptance criterion.
