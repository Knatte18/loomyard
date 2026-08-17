# Discussion: planparser owns the plan-directory path

```yaml
task: planparser owns the plan-directory path
slug: planparser-plan-dir
status: discussing
parent: standalone-producers
```

## Problem

`internal/webstercli` imports `internal/loomengine` for exactly one value: `internal/webstercli/cli.go:194` sets `c.planDir = loomengine.PlanDir(layout)`.
That single call is the whole reason a producer CLI depends on an orchestrator engine.
The value itself does not belong to `loom`: `internal/planparser` already declares itself the sole owner of the on-disk plan format, already declares the `PlanDirName` constant (`"plan"`), and already exports the worktree-relative token `PlanDirRel()` (`_lyx/plan`).
`loomengine.PlanDir(l)` is that same token re-anchored onto `l.AnchorPath()`, and `loomengine.PlanOverview(l)` repeats the pattern while hardcoding the literal `"00-overview.md"` — a duplicate of `planparser`'s own unexported `overviewFileName` constant.

**Why now.** This is T1 of the [producers-standalone](../manifest/designs/producers-standalone.md) decomposition, wave 1, depending on nothing.
T7 (`webster-told-geometry`) depends on it directly: T7 delivers `lyx webster run`'s standalone entry, which needs `planparser.PlanDir(<state>)` for its standalone default and `planparser.PlanDir(l.AnchorPath())` for hub mode.
Until the function exists on `planparser` and takes a plain string, T7's standalone Webster cannot resolve a plan directory without dragging `loomengine` and a `*lyxcwd.Location` into a CLI that must require neither.
The design's one-sentence rule — *"an orchestrator resolves geometry and requires it; a producer is told its paths and requires nothing"* — is what this task applies to a single path helper.

## Scope

**In:**

- Add `planparser.PlanDir(anchorPath string) string` and `planparser.PlanOverview(anchorPath string) string` to `internal/planparser/parse.go`.
- Delete `loomengine.PlanDir` and `loomengine.PlanOverview` from `internal/loomengine/config.go`: the two function bodies at lines 32-34 and 40-42, together with their doc comments at 29-31 and 36-39.
- Repoint every caller: `internal/loomengine/plan.go:67-68`, `internal/webstercli/cli.go:194`, `internal/webstercli/cli_test.go:172`, `internal/webstercli/verbs_test.go:221,259`, `internal/loomengine/plan_test.go:85-86`, `cmd/lyx/constructoranchoring_test.go:71-72,120-121`, `cmd/lyx/notransients_test.go:50-51`.
- Remove the `internal/loomengine` import from `internal/webstercli/cli.go`, `cli_test.go` and `verbs_test.go`.
- Port `internal/loomengine/planpath_test.go`'s cases into a new `internal/planparser/planpath_test.go` and delete the loomengine file.
- Close the anchor-always coverage gap (see the **anchor-always** decision): flip `webstercli`'s test layouts off `AnchorRel: "."`, and add a subpath-anchored `loomengine.PlanSpec` case.
- Reword the [Planparser Sole-Parser Invariant](../CONSTRAINTS.md#planparser-sole-parser-invariant)'s false `lyxcwd` bullet in the same commit.
- Add a path-ownership paragraph to `internal/planparser/doc.go`.
- Extend `docs/overview.md`'s planparser entry (lines 285-286) to name path ownership alongside grammar ownership.
- Update three stale comments the code change falsifies: `internal/webstercli/cli.go:57-58` (calls `planDir` one of "the lyxcwd-resolved `_lyx` dirs"), `internal/webstercli/verbs_test.go:12-13` (asserts tests always bypass `PersistentPreRunE`, already false and the source of this task's own earlier mistaken premise), and `cmd/lyx/notransients_test.go:6-8`'s module enumeration.

**Out:**

- No behaviour change of any kind.
  Every path this task touches resolves to the byte-identical string it resolves to today, in a real worktree and in a subpath-anchored one alike.
- No `lyxcwd` change.
  `planparser` does not import `lyxcwd` today and must not start; the caller resolves geometry and hands over a string.
- No twin, no wrapper, no deprecation shim.
  `loomengine.PlanDir`/`PlanOverview` are deleted outright in the same commit that adds their replacements — the design's **no additive twins** decision.
- No new import for `planparser`.
  `filepath` and `internal/lyxdirs` are both already imported by `parse.go`.
- No touching of `loomengine`'s other path constructors (`DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, `LoomStatusFile`, `LoomStatusLock`) — `discussionDirName` is genuinely loom's own segment and stays.
- No `--plan-dir` flag, no standalone mode, no `webstercli` pre-run refactor.
  Those are T7. This task leaves `internal/webstercli/cli.go`'s `PersistentPreRunE` structurally as it is and changes one line inside it.
- No new machine-check guard (no import guard on `planparser`, no grep guard on call sites).
- No change to `PlanDirRel()`, `PlanDirName`, or `overviewFileName`'s visibility.

## Decisions

### two functions, matching the design's pinned signatures

- **Decision:** Add both `PlanDir(anchorPath string) string` and `PlanOverview(anchorPath string) string`.
  `PlanOverview` is implemented as `filepath.Join(PlanDir(anchorPath), overviewFileName)`, reusing the existing unexported constant.
- **Rationale:** The signatures are pinned by the design doc and consumed verbatim by T7, so deviating would break a downstream task's brief.
  Routing `PlanOverview` through the existing `overviewFileName` is the point of the move: it deletes `loomengine.PlanOverview`'s hardcoded `"00-overview.md"` duplicate, leaving one declaration of the filename in the repo.
- **Rejected:** `PlanDir` only plus an exported `OverviewFileName` constant — re-scatters into callers the exact join `loomengine` just stopped doing, and `internal/loomengine/plan.go:81` needs the overview path as `Spec.OutputFiles`' sole entry.
  `PlanDir` only with the overview path kept internal — breaks `plan.go:68` outright.

### plain string, never a `*lyxcwd.Location`

- **Decision:** Both functions take a plain absolute directory path.
- **Rationale:** `internal/planparser`'s only first-party import today is `internal/lyxdirs`;
  taking a `*lyxcwd.Location` would add an import to a package that has none, and would make the sole parser of a file format depend on cwd-resolution geometry.
  The [Cwd Resolution Invariant](../CONSTRAINTS.md#cwd-resolution-invariant) already mandates this shape: *"a module's own durable-storage subdirectory (e.g. `_lyx/plan`, `_lyx/webster`) is that module's own private relative-path constant, joined onto `AnchorPath()` directly — never a `lyxcwd` function call."*
  This task moves the plan path from the wrong module onto the right one without changing the anchoring rule at all.
- **Rejected:** Keeping the `*lyxcwd.Location` parameter and merely moving the functions — preserves the type coupling the whole producers-standalone design exists to remove, and leaves T7's standalone Webster with nothing to pass.

### anchor-always — every call site passes `AnchorPath()`, never `WorktreePath()`

- **Decision:** This is a hard rule, stated loudly in the function doc comments and enforced by call-site tests.
  Every production call passes `l.AnchorPath()`.
  A `*lyxcwd.Location` with a non-`"."` `AnchorRel` must produce a plan directory under the anchored subpath, exactly as today.
  Neither `l.WorktreePath()`, nor cwd, nor a git-root value is ever an acceptable argument.
- **Rationale:** The move trades a compile-time guarantee for a convention.
  Today `loomengine.PlanDir(l)` does the anchoring itself, so no caller *can* get it wrong.
  After the move the parameter is an untyped string, and passing `l.WorktreePath()` compiles, runs, and silently relocates the plan directory to the worktree root whenever `AnchorRel != "."`.
  Every existing `webstercli` test fixture uses `AnchorRel: "."`, where `AnchorPath()` and `WorktreePath()` are the same string — so that mistake is invisible to the suite as it stands.
  The guard has to move to the call sites, deliberately, in this task.
  Coverage lands at three levels: the two consuming call sites (`loomengine.PlanSpec`, `webstercli`'s plan-dir-consuming fixtures) **and** `webstercli`'s `PersistentPreRunE` itself, which — contrary to an earlier reading of this task — *is* already driven by tests (see the technical context below).
- **Rejected:** Relying on `cmd/lyx/constructoranchoring_test.go` alone.
  After the rewrite its rows read `planparser.PlanDir(l.AnchorPath())` compared against an `anchor`-derived expectation — tautological with respect to anchoring.
  Also rejected: a grep-style `cmd/lyx` guard asserting no production call passes anything but `AnchorPath()` — new guard infrastructure outside this task's scope, and string-matching guards rot.
  Also rejected: factoring `webstercli`'s pre-run wiring into a testable function now — that factoring is T7's job and would collide with it, **and is not needed for coverage**: `verbs_test.go`'s `seedPersistentPreRunFixture` already drives the real `Command()` pre-run through `RunCLIIn`, so `cli.go:194` is reachable today without any refactor.

### path helpers live in `parse.go`

- **Decision:** The two functions go in `internal/planparser/parse.go`, directly beside `overviewFileName`, `PlanDirName` and `PlanDirRel`.
- **Rationale:** The design names this file, and all five symbols form one cohesive plan-location block.
  Splitting them into a new `paths.go` would either separate `PlanDirRel` from its own constant or force it to move too, for no gain.
- **Rejected:** A new `internal/planparser/paths.go` with its own file-header comment.

### `filepath.Join` on segments, not on `PlanDirRel()`

- **Decision:** `PlanDir` is `filepath.Join(anchorPath, lyxdirs.LyxDirName, PlanDirName)`.
- **Rationale:** Byte-identical to `loomengine.PlanDir`'s current body, so the no-behaviour-change claim is inspectable rather than argued.
  `PlanDirRel()` is deliberately built with `path.Join`, i.e. always forward-slash, because it stamps `Card.SourcePath` (`parse.go:250`) — a document token, not an OS path.
  Building an absolute OS path on top of it would work only by leaning on `filepath.Clean`'s Windows slash conversion, coupling two functions that have deliberately different separator contracts.
- **Rejected:** `filepath.Join(anchorPath, PlanDirRel())`.

### garbage in, garbage out — no validation branch

- **Decision:** No guard on an empty or relative `anchorPath`.
  Both functions stay `func(string) string`, pure `filepath.Join`, with the doc comment stating the contract: the caller supplies an absolute anchor path, which in a lyx worktree is `lyxcwd.Location.AnchorPath()`.
- **Rationale:** Identical to today's behaviour through `l.AnchorPath()`;
  every caller already holds an absolute path.
  A validation branch would be the first error-returning path helper in the package and would force error handling into `webstercli`'s pre-run and into `loomengine.PlanSpec`, changing signatures the design pinned.
- **Rejected:** Returning `(string, error)` on a non-absolute anchor.

### CONSTRAINTS reword — sole declarer of the plan path

- **Decision:** Replace the Planparser Sole-Parser Invariant's bullet *"Resolves `_lyx/plan/` via `lyxcwd`, never string literals"* with a told-anchor ownership bullet, in the same commit.
  It states that `planparser` is the sole declarer of the plan directory's path — `PlanDirName`/`PlanDirRel()` for the worktree-relative token, `PlanDir`/`PlanOverview` for the absolute form — that the package never resolves cwd and never imports `lyxcwd`, and that the caller supplies the anchor path (`AnchorPath()`, never `WorktreePath()`).
- **Rationale:** The existing bullet is already false — `planparser` does not import `lyxcwd` — and after this change the package is *told* an absolute anchor path, making it false in a second way.
  Stating sole-declarership is the point of the task;
  deleting the bullet outright would leave the invariant silent on it.
- **Rejected:** Deleting the bullet and relying on the Cwd Resolution Invariant's generic per-module-subpath rule.
  Also rejected: the minimal edit *"via a told anchor path"*, which fixes the falsehood but drops the ownership claim.

### the reworded invariant stays a review obligation

- **Decision:** No machine check is added.
  The invariant's `**Enforced by**` line stays *"review obligation today (candidate future import/grep guard)"*.
- **Rationale:** The bullet already names a guard as a future candidate;
  building one is scope the design did not ask for, and a leaf-style import guard on `planparser` would be new infrastructure landing in a task whose entire point is a mechanical signature move.
- **Rejected:** Adding a `leaf_enforcement_test.go`-style allowlist guard (stdlib + `lyxdirs` + yaml) to `planparser`, which would mechanically pin the never-imports-`lyxcwd` half.

### docs updated in the same commit

- **Decision:** `internal/planparser/doc.go` gains a short path-ownership paragraph, and `docs/overview.md`'s planparser entry (lines 285-286) is extended to name path ownership alongside grammar ownership.
- **Rationale:** The package's public contract changed — it now owns where the plan directory *is*, not only what is inside it.
  CLAUDE.md's Documentation Lifecycle requires the module doc to move in the same commit as the change that alters it.
- **Rejected:** Leaving `doc.go` to the function comments alone;
  leaving `docs/overview.md` untouched on the grounds that no module-table row and no CLI behaviour changed.

**`manifest/designs/fabric-unified-view.md` is deliberately left alone.**
It names `PlanDir` twice — at line 49 in a constructor inventory, and at line 68 in the as-built anchoring table recorded from that task's own Shared Decisions.
Both are historical records of what that task built, not live statements about where the function lives now, and a design doc's as-built section is not maintained forward as the tree moves.
Rewriting it would falsify the record of what `fabric-unified-view` actually delivered.
The anchoring it describes is unchanged by this task in any case: the plan directory is still `AnchorPath`-anchored and still in the `_lyx`-durable group;
only the declaring package moves.

**Three stale comments are updated, not left.**
`internal/webstercli/cli.go:57-58` describes `planDir` as one of "the lyxcwd-resolved `_lyx` dirs" — after this change it is told, not resolved.
`internal/webstercli/verbs_test.go:12-13` states tests "build a `*websterCLI` literal directly (bypassing `Command()`'s `PersistentPreRunE`)", which `seedPersistentPreRunFixture` and its two tests already falsify;
that comment is the traceable source of this task's own first-draft claim that `cli.go:194` was untestable, which is exactly the cost of leaving a stale comment standing.
`cmd/lyx/notransients_test.go:6-8`'s "may import every owning module at once" enumeration gains `planparser`.

## Technical context

### The two functions being deleted

`internal/loomengine/config.go:29-42`:

```go
func PlanDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName)
}

func PlanOverview(l *lyxcwd.Location) string {
	return filepath.Join(PlanDir(l), "00-overview.md")
}
```

Both carry the doc-comment line *"Per the Cwd Resolution Invariant, no other package may construct this path."*
That sentence moves with the functions and is reworded for the told-anchor form.
`discussionDirName` and the four `Discussion*`/`LoomStatus*` constructors in the same file are untouched.

### What `planparser` already has

`internal/planparser/parse.go:24-39`:

- `const overviewFileName = "00-overview.md"` (unexported, used at `parse.go:64` by `ParsePlan` and at `validate.go:110` to skip the overview when walking card files).
- `const PlanDirName = "plan"` — already documented as *"planparser is this segment's sole declarer, per the module-owned-constructors per-segment join rule."*
- `func PlanDirRel() string` — `path.Join(lyxdirs.LyxDirName, PlanDirName)`, forward-slash by contract, consumed at `parse.go:250` for `Card.SourcePath` and at `internal/websterengine/template_test.go:333`.

Package imports: `fmt`, `os`, `path`, `path/filepath`, `regexp`, `strconv`, `strings`, `internal/lyxdirs`, `gopkg.in/yaml.v3`.
`filepath` and `lyxdirs` are both present, so the new functions add nothing.

### Import direction

`internal/loomengine` already imports `internal/planparser` (`config.go:20`), so repointing `loomengine/plan.go` introduces no new edge and no cycle.

The `internal/webstercli` **package** already depends on `planparser` — `awaitbatch.go:18`, `beginbatch.go:22` and `validate.go:22` all import it and call `planparser.ParsePlan(c.planDir)`.
`cli.go` itself does not (imports at lines 22-31), so `cli.go` gains the `planparser` import and loses `loomengine`;
the package's dependency set strictly shrinks by one edge, which is the task's headline outcome.

`internal/webstercli/validate.go:73` already calls `planparser.Validate(plan, c.layout.AnchorPath())` — a told-anchor `planparser` call in production today.
The new functions match that established shape exactly, including the anchor-always argument.

### Every call site, enumerated

Production:

| Site | Today | After |
|---|---|---|
| `internal/loomengine/plan.go:67` | `planDir := PlanDir(layout)` | `planparser.PlanDir(layout.AnchorPath())` |
| `internal/loomengine/plan.go:68` | `overviewPath := PlanOverview(layout)` | `planparser.PlanOverview(layout.AnchorPath())` |
| `internal/webstercli/cli.go:194` | `c.planDir = loomengine.PlanDir(layout)` | `c.planDir = planparser.PlanDir(layout.AnchorPath())` |

Tests:

| Site | Note |
|---|---|
| `internal/loomengine/plan_test.go:85-86` | in-package call, drops the receiver |
| `internal/loomengine/planpath_test.go` | deleted; cases ported to `internal/planparser/planpath_test.go` |
| `internal/webstercli/cli_test.go:172` | fixture field; also drops the `loomengine` import at line 26 |
| `internal/webstercli/verbs_test.go:221,259` | fixture + seed call; also drops the `loomengine` import at line 42 |
| `cmd/lyx/constructoranchoring_test.go:71-72,120-121` | rewritten in place, both fixtures |
| `cmd/lyx/notransients_test.go:50-51` | `durableSet` rows, rewritten in place |

`internal/webstercli/run.go:66` passes `c.planDir` into `RunDeps` and does not change.
No other package references either symbol — verified by a repo-wide grep for `PlanDir`/`PlanOverview`/`overviewFileName`/`PlanDirRel`.

### The `cmd/lyx` guard tables, and what they still prove

`cmd/lyx/constructoranchoring_test.go` runs every relocated constructor against two synthetic `*lyxcwd.Location` fixtures: `AnchorRel: "."` (`TestConstructorAnchoring_Unanchored`) and `AnchorRel: "backend"` (`TestConstructorAnchoring_SubpathAnchored`).
The plan rows sit in the `_lyx`-durable group in both.
After the rewrite they read:

```go
assertPath(t, "planparser.PlanDir", planparser.PlanDir(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName))
assertPath(t, "planparser.PlanOverview", planparser.PlanOverview(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName, "00-overview.md"))
```

where `lyxBase` is `filepath.Join(anchor, lyxdirs.LyxDirName)`.
**These rows are now tautological with respect to anchoring** — they pass `AnchorPath()` in and compare against an `anchor`-derived expectation, so they can no longer catch a production call site that passes the wrong root.
They still prove the join arithmetic, the `_lyx`-vs-`.lyx` group placement, and (via `notransients_test.go`) that no durable plan path is a transient.
A comment must record precisely this weakening at the rows, so the next reader does not re-derive it or over-trust the table.

**Import churn differs between the two files.** `constructoranchoring_test.go` already imports `planparser` (line 40) and needs no import change.
`cmd/lyx/notransients_test.go` does **not** — its import block (lines 16-29) has `loomengine`, `logger`, `lyxcwd`, `lyxdirs`, `perchengine`, `scoutengine`, `treadleengine`, `websterengine` and no `planparser` — so it gains the `planparser` import.
Whether it *loses* `loomengine` depends on its remaining rows (`DiscussionDir`, `LoomStatusFile`, `LoomStatusLock` all stay), so it keeps the import;
verify rather than assume.

Both files' header comments enumerate the owning modules the file may import at once — `constructoranchoring_test.go:5-6` names `planparser` already, `notransients_test.go:6-8` does not.
Add it to `notransients_test.go`'s enumeration in the same commit as the import.
`constructoranchoring_test.go`'s header also describes the anchoring groups and names the `_lyx`-durable group without naming `loomengine.PlanDir` by symbol, so check whether it needs a touch-up when the rows move.

### Where anchoring is actually proven after this task

- `internal/loomengine/plan_test.go` — a new subpath-anchored `PlanSpec` case, asserting the composed plan dir and `Spec.OutputFiles[0]` land under `AnchorPath()` and not under `WorktreePath()`.
- `internal/webstercli/cli_test.go` / `verbs_test.go` — fixtures flipped off `AnchorRel: "."` so the two roots are distinguishable strings at every site that consumes `c.planDir`.
- `internal/webstercli/verbs_test.go` — **a new subpath-anchored `PersistentPreRunE` case that covers `cli.go:194` itself.**
  Most of `verbs_test.go` does build a `*websterCLI` literal directly, but `seedPersistentPreRunFixture` (`verbs_test.go:695-705`) is the deliberate exception: it builds a real `hubforge.NewHub(t, ".")` hub and drives `Command()`'s real pre-run through `RunCLIIn(h.PrimeWorktree(), …)`, consumed today by `TestPersistentPreRunE_UnknownBatcherFailsFast` (`:721`) and `TestPersistentPreRunE_DefaultBatcherResolves` (`:749`).
  `hubforge.NewHub`'s second parameter is the anchor itself (`internal/hubforge/hub.go:215`, documented as `"." or "backend"`), and `Hub.Location` exposes both roots — so a `"backend"`-anchored pre-run case is available with no refactor at all.
  See the Testing section for its shape.

### Sequencing against sibling wave-1 tasks

T1 is parallel-safe with T2 and T3, and depends on nothing.
The one adjacency: T3 (`shuttle-reed-told-geometry`) edits `internal/webstercli/cli.go:179-181` while this task edits line 194 — separate hunks in the same block, so a textual merge conflict is possible but mechanical.
Whichever lands second rebases.
Nothing in T1 requires or anticipates T3.

## Constraints

From `CONSTRAINTS.md`:

- **[Cwd Resolution Invariant](../CONSTRAINTS.md#cwd-resolution-invariant)** — the load-bearing one.
  *"A module's own durable-storage subdirectory (e.g. `_lyx/plan`, `_lyx/webster`) is that module's own private relative-path constant, joined onto `AnchorPath()` directly — never a `lyxcwd` function call. Adding a module's own subdirectory is never a `lyxcwd` change."*
  Also: `root` always means the git worktree/repo root and `cwd` the current working directory — never name a parameter `root` for a value that is a cwd, or vice versa.
  The new parameter is named `anchorPath`, which is neither and is deliberate.
- **[Planparser Sole-Parser Invariant](../CONSTRAINTS.md#planparser-sole-parser-invariant)** — reworded by this task (see the decision above).
  Its other bullets stand: no other package parses `00-overview.md`/`NN-<card-slug>.md`, and consumers read plan-level sections only from the `planparser.Plan` model.
- **[Lyxdirs Single-Declarer Invariant](../CONSTRAINTS.md#lyxdirs-single-declarer-invariant)** — `_lyx` comes from `lyxdirs.LyxDirName`, never a literal.
  The new `PlanDir` body honours this exactly as `loomengine.PlanDir` does today.
- **[Durable-vs-Ephemeral State Invariant](../CONSTRAINTS.md#durable-vs-ephemeral-state-invariant)** — enforced by `cmd/lyx/notransients_test.go`, whose `durableSet` carries both plan rows.
  The plan directory is durable (`_lyx`, git-tracked), never ephemeral (`.lyx`).
  Rewriting the rows must not move them out of `durableSet`.
- **Documentation Lifecycle** (CLAUDE.md) — the module doc and `CONSTRAINTS.md` change in the same commit as the code.
  `manifest/roadmap.md` does **not** move: this is a planned decomposition item being executed, and the roadmap's producers-standalone entries were already synced.

Project rules that bind the commit:

- Markdown uses semantic line breaks — one sentence per line, no fixed-column hard-wrap — in every `.md` file this task edits (`CONSTRAINTS.md`, `docs/overview.md`).
- Go comments follow `golang:golang-comments`;
  the two new exported functions need doc comments starting with the function name, and each must state the anchor-always contract explicitly.
- This is a task worktree — commit and push on the `planparser-plan-dir` branch only.
  Its merge target is its recorded parent, `standalone-producers` (see this file's frontmatter and `_mill/status.md`), not `main`;
  `mill-merge` lands it there.
  Direct pushes to `main` are barred from a task worktree regardless.

## Testing

Verification baseline for the whole task: `go test ./...` from the worktree root.
Task-specific: `go test ./internal/planparser/... ./internal/loomengine/... ./internal/webstercli/... ./cmd/lyx/...`, plus confirming `internal/loomengine` no longer appears in `internal/webstercli`'s import set (production **and** test — all three files lose it).

**The tagged invocation is not optional here.** `internal/webstercli/verbs_test.go` carries `//go:build integration` on line 1, so neither `go test ./...` nor the untagged `go test ./internal/webstercli/...` compiles it.
Two named in-scope test edits live in that file — the `verbs_test.go:220-221,259` repoint plus its `AnchorRel` flip, and the new subpath-anchored `PersistentPreRunE` case — and both are verified only by `go test -tags integration ./internal/webstercli/...`.
Run it alongside the untagged baseline;
a green untagged run proves nothing about either edit.

### `internal/planparser` — new `planpath_test.go`

Ported from `internal/loomengine/planpath_test.go`, rewritten to plain strings, no `lyxcwd` import.
The original's third case (`TestLocationPlanDir_UnanchoredEqualsWorktreePath`) collapses — with a told string there is no unanchored-vs-anchored distinction left to assert inside the package — so two cases remain:

- `PlanDir` joins `_lyx` and `plan` onto the given directory, using `lyxdirs.LyxDirName` rather than a literal.
- `PlanOverview` is `PlanDir` plus the overview filename, and agrees with what `ParsePlan` reads (`parse.go:64` joins the same constant) — worth asserting rather than assuming, since the whole point of routing through `overviewFileName` is that the two can never diverge again.

A nested-directory input (the ported fixture used `filepath.Join("sub", "dir")`) is kept as the argument shape, so the functions are exercised on something that is not a bare root.

### `internal/loomengine`

- `planpath_test.go` deleted.
- `plan_test.go:85-86` repointed to `planparser.PlanDir/PlanOverview(layout.AnchorPath())`.
- **New, and the anchor-always workhorse here:** a `PlanSpec` case built on a `*lyxcwd.Location` with a non-`"."` `AnchorRel`, asserting the composed plan directory and the overview path (which is `Spec.OutputFiles`' sole entry, `plan.go:81`) resolve under `AnchorPath()` and are *not* equal to their `WorktreePath()`-rooted counterparts.
  This is the case that fails if someone later passes the wrong root at `plan.go:67-68`.
- Existing `loomengine` coverage of the untouched `Discussion*`/`LoomStatus*` constructors must keep passing unchanged.

### `internal/webstercli`

- `cli_test.go` and `verbs_test.go` drop the `loomengine` import and use `planparser.PlanDir(layout.AnchorPath())`.
- **Fixtures flip off `AnchorRel: "."`.** `cli_test.go:168` and `verbs_test.go:220` build layouts with `AnchorRel: "."`, where `AnchorPath()` and `WorktreePath()` are the same string, so no test can currently distinguish them.
  A non-`"."` anchor makes the two roots distinguishable at every site that consumes `c.planDir` (the plan seeding at `cli_test.go:201,252` and `verbs_test.go:221`, and the fingerprint helper at `verbs_test.go:275`).
  Whether `cli_test.go:134,152`'s layouts also need flipping depends on what those cases assert — check before changing them;
  the target is the plan-dir-consuming fixtures, not a blanket rewrite.
  These fixtures create real directories on disk, so re-anchoring must keep the seeded plan files and the CLI's `planDir` pointing at the same place — a fixture that silently seeds one directory and reads another would pass vacuously.
- **New subpath-anchored pre-run case, covering `cli.go:194` itself.**
  Parameterize `seedPersistentPreRunFixture` (`verbs_test.go:695`) with the anchor it passes to `hubforge.NewHub` — `"."` for the two existing callers, `"backend"` for the new one — and drive a verb whose behaviour depends on `c.planDir`.
  `validate` is the natural choice: it calls `planparser.ParsePlan(c.planDir)` (`internal/webstercli/validate.go:67`) and surfaces planparser's own `"plan overview not found: <path>"` when the directory is wrong.
  Seed a valid plan under the **anchored** location (`h.Location.AnchorPath()` + `_lyx/plan`), then run `validate` through `RunCLIIn` with the anchored cwd — `lyxcwd.Resolve` gates cwd against `AnchorPath()` exactly, so the anchor directory is what `RunCLIIn` must be given at a `"backend"` hub, not `h.PrimeWorktree()`.
  A `WorktreePath()`-based resolution then looks in `<worktree>/_lyx/plan`, which does not exist, and the test fails with a legible error naming the wrong root.
  Config seeding goes through `hubforge.SeedConfig` against `h.WeftBase`, which is already anchor-joined — writing to the un-anchored weft root at a `"backend"` anchor produces a file no loader reads, with no error at all (`internal/hubforge/hub.go:154-161`).
  This case is the one that would actually catch a future `WorktreePath()` slip at `cli.go:194`;
  the other two levels catch it at the consumers.

### `cmd/lyx`

- `constructoranchoring_test.go` — plan rows rewritten in place in both fixtures, staying in the `_lyx`-durable group, with a comment recording that they now pin the join and the group placement but no longer prove call-site anchoring.
- `notransients_test.go` — `durableSet` rows rewritten in place;
  the durable-vs-transient assertion is unchanged and must still hold at both `AnchorRel` fixtures.
- The help-tree and `Short`/`Long` CLI obligations are untouched — no command, flag, or help text changes in this task.

### TDD candidates

The `internal/planparser` path tests are the natural TDD entry: write `planpath_test.go` against the not-yet-existing `PlanDir`/`PlanOverview`, watch it fail to compile, add the functions, then delete the `loomengine` twins and let the compiler enumerate the remaining call sites.
The subpath-anchored `loomengine.PlanSpec` case is the second TDD candidate — it should be written *before* the repoint, so it is seen passing against the current `loomengine.PlanDir` implementation and then again after, which is what makes the no-behaviour-change claim real rather than asserted.

## Q&A log

- **Q:** Keep both `PlanDir` and `PlanOverview` on planparser, or collapse to one function? **A:** Both, exactly as the design pins them; `PlanOverview` reuses the unexported `overviewFileName`.
- **Q:** Where do the two functions live inside planparser? **A:** `parse.go`, beside `PlanDirName`/`PlanDirRel`/`overviewFileName`.
- **Q:** How is `PlanDir` implemented — segment join, or `filepath.Join(anchorPath, PlanDirRel())`? **A:** Answered from the code rather than by the operator: segment join, because `PlanDirRel()` is a forward-slash document token built with `path.Join` for `Card.SourcePath`, not an OS path.
- **Q:** What happens to `internal/loomengine/planpath_test.go`? **A:** Port its cases into a new `internal/planparser/planpath_test.go` taking plain strings, delete the loomengine file, and keep loomengine-side coverage that `plan.go` still anchors on `AnchorPath()`.
- **Q:** How are the `cmd/lyx` guard-table rows rewritten? **A:** In place as `planparser.PlanDir(l.AnchorPath())`, staying in the `_lyx`-durable group in both fixtures and in `durableSet`.
- **Q:** How is the Planparser Sole-Parser Invariant bullet reworded? **A:** Replace the false `lyxcwd` bullet with a told-anchor sole-declarer bullet.
- **Q:** Does the reworded invariant get a machine check? **A:** No — stays a review obligation, as its `Enforced by` line already says.
- **Q:** Does `planparser/doc.go` gain a path-ownership paragraph? **A:** Yes, one short paragraph.
- **Q:** What does `PlanDir("")` or a relative anchor path do? **A:** Nothing special — pure `filepath.Join`, documented as "caller supplies an absolute anchor path". **And the operator's emphasis:** the anchor path is not an incidental parameter — it is *always* present and *always* must be accounted for; a call site that reaches for `WorktreePath()` or cwd instead is a defect, which is why anchor-always is a named decision with its own test coverage rather than a doc-comment aside.
- **Q:** How hard do we close the anchor-always gap? **A:** Subpath-anchored fixtures at both production call sites (`webstercli` tests off `AnchorRel: "."`, plus a subpath-anchored `loomengine.PlanSpec` case), and state plainly that `cli.go:194`'s own line stays uncovered until T7.
- **Q:** Do the `cmd/lyx` rows keep their expectations, given they become tautological? **A:** Yes, with a comment saying what they now do and do not prove.
- **Q:** (Discussion review r1, BLOCKING) The claim that `webstercli`'s `PersistentPreRunE` is untestable is false — `verbs_test.go`'s `seedPersistentPreRunFixture` drives the real pre-run through `RunCLIIn`, and `hubforge.NewHub` takes the anchor as a parameter. Re-decide the coverage question. **A:** Confirmed against source and re-decided: add a `"backend"`-anchored pre-run case driving `validate`, which covers `cli.go:194` itself with no refactor and no T7 dependency. The earlier deferral was made on a wrong fact, sourced from a stale comment at `verbs_test.go:12-13` — that comment is now an in-scope fix.
- **Q:** (r1 NIT) Do the named verify commands actually compile `verbs_test.go`? **A:** No — it is `//go:build integration`. `go test -tags integration ./internal/webstercli/...` is now named as required, with the two edits it alone covers spelled out.
- **Q:** (r1 NIT) Do both `cmd/lyx` guard files already import `planparser`? **A:** No — `constructoranchoring_test.go` does, `notransients_test.go` does not. The latter's import and its header module enumeration are now in scope.
- **Q:** (r1 NIT) What is the disposition of `manifest/designs/fabric-unified-view.md`'s two `PlanDir` mentions? **A:** Left alone deliberately, as a historical as-built record of what that task delivered; the anchoring it describes is unchanged by this task.
- **Q:** (r1 NIT) What about the stale comments at `cli.go:57-58` and `verbs_test.go:12-13`? **A:** Both in scope, plus `notransients_test.go:6-8`.
