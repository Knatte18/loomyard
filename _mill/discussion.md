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
- `manifest/designs/producers-standalone.md` — T10 deletes it per the documentation lifecycle; this task does not edit it.
- The daemon lifecycle, staleness check, wedged-daemon escalation, toolchain manager, and every LSP behaviour — pure signature/plumbing change, zero behavioural change intended.

## Decisions

### told parameter shape — a bare `anchorRoot string`, not a `Geometry` struct

- **Decision:** `DaemonStateFile(anchorRoot string, lang string)` and `DaemonLock(anchorRoot string, lang string)`. No `scoutengine.Geometry` type, no new `geometry.go` file in the package.
- **Rationale:** The design's "told-geometry structs per engine" rule exists to stop positional parameter lists reaching four or five strings (`reedengine` is the named example, at five). Scout is told exactly **one** path. `websterengine.Dir(anchorRoot)`, `websterengine.ReportsDir(anchorRoot)`, `perchengine.RunsDir(anchorRoot)`, and `planparser.PlanDir(anchorRoot)` are all bare-string free functions today, and `cmd/lyx/constructoranchoring_test.go` already calls them as `f(l.AnchorPath())`. Scout's rows become the same shape as its neighbours in that table, which is the uniformity this task is buying.
- **Rejected:** A one-field `Geometry{AnchorRoot string}` struct for symmetry with burler/perch/webster/reed — it adds a type, a file, and a doc comment to wrap a single string, which is the YAGNI this design otherwise enforces. Revisit only if scout later grows a second told path.

### `Options.Layout` becomes `Options.AnchorRoot string`

- **Decision:** `scoutengine.Options` drops `Layout *lyxcwd.Location` and gains `AnchorRoot string`. The value threads unchanged through `acquireConnection` → `ensureServer` → `ensureSupervised`, each renaming its `layout *lyxcwd.Location` parameter to `anchorRoot string`.
- **Rationale:** Direct translation of what the engine reads. `Options.Layout`'s doc says "required and must be non-nil"; the told equivalent is "required, must be a usable absolute path, populating it is entirely the caller's obligation" — the exact wording `burlerengine.Geometry` and `websterengine.Geometry` already use. The engine validates nothing, consistent with every converted sibling.
- **Rejected:** Keeping `Layout` and deriving `anchorRoot` inside the engine — that keeps `lyxcwd` in `scoutengine`'s import graph and leaves the fictional-`Location` synthesis alive in `scoutcli`, i.e. it does none of the work.

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

- `internal/websterengine/geometry.go` + `internal/websterengine/webstergeom_test.go` — the bare-`anchorRoot string` free-function shape and its three-case test (unanchored `Location`, subpath-anchored `Location`, pure told directory with no `Location` at all). Scout's rewritten `scoutdaemon_test.go` should mirror this structure directly.
- `internal/burlerengine/geometry.go` — the "populating every field with a usable absolute path is entirely the caller's obligation" doc-comment wording.
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

- **Zero behavioural change.** Every resolved daemon-state and lock path must be byte-identical before and after, in hub mode and out-of-hub mode, at both unanchored and subpath-anchored geometries. This is the single acceptance property the whole task hangs on.
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

**`internal/scoutcli` — TDD candidate: the reshaped `lookupContext` test.**
Replace `TestLookupContext_OutsideHubReturnsSynthesizedLocationAndBuiltinRegistry` with a test asserting the out-of-hub return is `(BuiltinRegistry(), filepath.Abs(dir))` — an absolute-path *string*, no `Location`. Keep the existing chdir-into-a-non-git-temp-dir setup, which is what forces the degraded branch. Cover:

- out-of-hub with an explicit `dir` → anchor root is `filepath.Abs(dir)`, registry is the built-in;
- out-of-hub with `dir` defaulted from `cwd` → anchor root is `filepath.Abs(cwd)`, **not** `filepath.Abs("")`. `lookupContext`'s existing doc comment calls out this exact trap ("dir is the already-defaulted directory, never the raw `--target-dir` flag value") and it must survive the rewrite.

Delete `resolveLocation`'s own test with the function. Reshape `TestBuildOptions_ThreadsEveryFieldFromItsArguments` to assert `got.AnchorRoot` directly — a plain string comparison, simpler than the old `WorktreePath()` round-trip.

**`cmd/lyx` — the anchoring tables.**
`constructoranchoring_test.go` rows 102-103, 160-161, 181-182 and `notransients_test.go` rows 79-80 become `scoutengine.DaemonStateFile(l.AnchorPath(), "go")` / `scoutengine.DaemonLock(l.AnchorPath(), "go")`, matching the `websterengine.Dir(l.AnchorPath())` rows directly above them. Expected paths are unchanged — that is the point.

**Commands.**

- `go test ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/...`
- `go test -tags scout ./internal/scoutengine/...` — mandatory, not optional: four of the converted test files are `scout`-tagged and are invisible to the untagged run.
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
- **Q:** What is the test strategy for the new told accessors? **A:** [auto-pick] Rewrite `scoutdaemon_test.go` as told-string path math across unanchored, subpath-anchored, and pure-told-directory cases; add a reshaped `scoutcli` `lookupContext` test; leave the `-tags scout` integration tests behaviourally unchanged. **Why:** mirrors `webstergeom_test.go`, and pins the byte-identical-paths property that is this task's sole acceptance criterion.
