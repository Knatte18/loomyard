# Discussion: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
task: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name
slug: pattern-into-lyx-consolidation
status: discussing
parent: main
```

## Problem

lyx wires anchor-level junctions per host worktree.
Today two are wired: `_lyx` (durable, fabric-synced, git-tracked storage, structural and injected in code) and `_pattern` (the PATTERN constraint-injection surface, owned solely by `internal/pattern`, sourced from `fabric.yaml`'s optional `pathspec` key).

`_pattern` earns nothing from being its own top-level directory.
It is one hand-authored file (`PATTERN.md`) plus a small set of detail docs, owned by exactly one module, with no recursive or mirrored structure.
Being a separate junction costs a `pathspec` entry, `HostJunctions` wiring, reconcile and status handling, a private `patternDirName` copy in `fabricengine`, a row in the geometry-literal owner map, and roughly forty test files' worth of wiring assertions — for zero benefit.
Folding it into `_lyx` removes an entire junction and its whole support surface.

`_raddle` is a smaller, related cleanup.
No `_raddle` junction has ever been wired: `fabricengine.HubReservedNames()` reserves the name only, grouping it with `_board`/`_portals`/`_launchers` as **hub-level** structural geometry a worktree slug must never collide with.
That grouping is now known to be the wrong geometry.
Raddle has converged on an **anchor-level** design — every raddle file lives under `_lyx/raddle/` inside each worktree, mirroring that worktree's own code tree, resolved by plain path lookup, with no junction and no hub-level presence at all.
Since raddle will be ordinary content inside the already-wired `_lyx`, the hub reservation and the `_raddle`-specific pathspec and status-scan entries are dead scaffolding for a superseded design.

**Why now:** removing it before raddle is implemented is what stops the next person building raddle on top of the wrong assumption.
Both cleanups converge with the parallel `dotlyx-scratch-hygiene` task toward one end state: exactly two anchor-level directories, `_lyx` tracked and `.lyx` untracked.

## Scope

**In:**

- Move the PATTERN content model into `_lyx`: `PATTERN.md` at `_lyx/PATTERN.md`, detail docs under `_lyx/pattern/`.
- Rework `internal/pattern`'s path API onto `lyxdirs.LyxDirName`; drop its `DirName` const and `Dir()` accessor.
- Rewrite the three agent-facing directive constants' literal relative pointers.
- Delete the `_pattern` junction end to end: `template.yaml`'s `pathspec` default, `fabricengine/pull.go`'s `patternDirName`, `fabricengine/status.go`'s `_pattern` pathspec entry and tracked-prefix branch, and every `_pattern` junction assertion across the fabricengine test suite.
- Re-scope `fabricengine`'s PATTERN-residue detection from the `_pattern` directory to the new `_lyx/PATTERN.md` + `_lyx/pattern/` paths; keep the `PatternResidue` feature.
- Remove `"_raddle"` from `HubReservedNames()`, from `status.go`'s host-pollution `ls-files` pathspec and its `HasPrefix` branch, and from the geometry-literal owner map.
- Delete `PollutionEntry.ReportOnly` and both its writers, now that no report-only pollution class remains.
- Un-reserve `_pattern` as a slug name too: with `pathspec` empty it leaves `IsReservedHubName`'s junction-name union, so `internal/lyxcwd/lyxcwd_test.go`'s `TestIsReservedHubName_Pattern` inverts.
- Widen the Pattern Leaf Invariant's allowlist to admit `internal/lyxdirs`.
- Update every doc, code comment, and design doc that describes either the `_pattern` junction or `_raddle` as hub-level/junction-reached geometry.
- Delete `manifest/designs/pattern.md` (overdue Documentation Lifecycle deletion — the module has landed).
- Record the settled anchor-level raddle geometry in `manifest/designs/raddle.md`.

**Out:**

- **Raddle's actual implementation** — the `_lyx/raddle/` shadow tree and its path-lookup logic. This task only stops reserving a hub-level name for a geometry raddle will not use. `manifest/designs/raddle.md` is updated to *record* the geometry, not to build it.
- **`internal/loomengine/coherence.go`'s `"raddle"` entry** — an unrelated phase-name enum value (`discussion`/`plan`/`builder`/`raddle`/`finalize`/`done`). Do not touch.
- **`.lyx` ephemeral hygiene** — that is the separately scoped `dotlyx-scratch-hygiene` task on the untracked side. Disjoint call sites; not blocking either way.
- **Any migration mechanism** — no code, no CLI verb, no reconcile branch, and no operator-facing migration paragraph in the docs. See the "No migration" decision.
- **The actual PATTERN.md content migration out of `CONSTRAINTS.md`** — `manifest/roadmap.md:39` records that as still outstanding and gated on loomyard-init-via-lyx. This task moves the *location*, never the content.
- **Roadmap status changes** — `manifest/roadmap.md` entries are corrected for the new paths only. No item is marked complete or added; per `CLAUDE.md`, the roadmap moves only on completing or adding a planned item, and this is consolidation.

## Decisions

### PATTERN content layout: `_lyx/PATTERN.md` + `_lyx/pattern/`

- **Decision:** `PATTERN.md` lands flat at `_lyx/PATTERN.md`.
  The detail docs land under `_lyx/pattern/`, not flat alongside it.
- **Rationale:** the three directive constants each tell the agent to read "every detail doc under `_pattern/` that PATTERN.md points to".
  Fully flat turns that into "every detail doc under `_lyx/`", which is `_lyx/config/fabric.yaml`, loom's orchestration status, and the future `_lyx/raddle/` tree — machine-owned territory the agent must not be pointed at.
  The flat option therefore forces the directive text to become either vague or to name a subdirectory anyway.
  `PATTERN.md` itself stays at the `_lyx` root because it is the entry point and must stay discoverable.
- **Rejected:** fully flat (the task brief's original wording) — poisons the directive text as above.
  Fully nested at `_lyx/pattern/PATTERN.md` — buries the entry point one level deeper for no gain.

### `internal/pattern` drops `DirName` and `Dir()`

- **Decision:** delete the `DirName` const and the `Dir(baseDir)` accessor.
  `File(baseDir)` builds `filepath.Join(baseDir, lyxdirs.LyxDirName, "PATTERN.md")`.
  `FileHere(l)` keeps its current shape — `File(filepath.Join(l.WorktreePath(), l.AnchorRel))`.
- **Rationale:** the `_pattern` token disappears from the repo entirely, so the geometry-literal owner map loses its `_pattern` row rather than trading one duplicated token for another.
  `internal/pattern` must not declare `_lyx`: `TestEnforcement_GeometryLiterals` registers `"_lyx": {"internal/lyxdirs"}` as sole owner.
- **Rejected:** re-exporting `DirName = lyxdirs.LyxDirName` — preserves call sites but re-introduces a second name for `_lyx` under a misleading identifier.
  Keeping `Dir()` pointed at `_lyx/pattern/` — nothing needs the detail-docs directory as a path; the directives name it as literal prompt text only.

### `internal/pattern` imports `internal/lyxdirs`; the Pattern Leaf Invariant widens

- **Decision:** add `internal/lyxdirs` to `internal/pattern/leaf_enforcement_test.go`'s `allowedImports`, and to `CONSTRAINTS.md`'s Pattern Leaf Invariant prose, in the same commit.
- **Rationale:** the invariant's load-bearing half is real and permanent — `builderengine/spawn.go`, `burlerengine/engine.go`, `loomengine/plan.go`, and `websterengine/render.go` all import `internal/pattern`, so a reverse import is a genuine cycle.
  The allowlist *form* is stricter than that reason requires, but `internal/lyxdirs` is stdlib-only and a zero-import leaf (its own invariant — `CONSTRAINTS.md:36`'s Lyxdirs Single-Declarer Invariant), so it cannot participate in a cycle by construction.
  Keeping the allowlist means a stray feature-package import is still caught with no list maintenance as new feature packages appear;
  the one-line cost is paid only when a genuinely-safe leaf is added, and paying it forces the conscious cycle-safety check.
- **Rejected:** relaxing to a banlist — needs maintenance every time a feature package is added.
  Dropping the invariant — the cycle would surface only as a compile error, which is late.
  Routing the token through `internal/lyxcwd` — `lyxcwd` does not import `lyxdirs` and exposes no `_lyx` accessor, so this means adding both a dependency and a path constructor to the package the Cwd Resolution Invariant deliberately keeps config-blind.

### `template.yaml`'s `pathspec` becomes empty, not `_lyx`

- **Decision:** `internal/fabricengine/template.yaml`'s `pathspec` value becomes empty.
  The key itself stays present, with its explanatory comment retained.
- **Rationale:** the task brief said `pathspec: _lyx`, which is wrong on both ends.
  The current value is `pathspec: _pattern`, not `pathspec: _lyx _pattern`.
  And `_lyx` may never appear in `pathspec` at all — `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant makes `_lyx`/`.lyx` structural, injected in code via `structuralCommittedDirs`/`structuralNeverCommittedDirs`, precisely so no operator-editable config value can tear the durable tree away.
  `Config.Dirs()` is `strings.Fields(c.Pathspec)`, so an empty value yields a nil slice cleanly and `pathspecNames`/`junctionNames` degrade to the structural sets alone.
  The key must stay present: `configengine.Load` reports a missing-key error and tells the operator to run `lyx config reconcile`.
- **Rejected:** removing the `pathspec` key — breaks the missing-key check for every deployed `fabric.yaml`.

### `PatternResidue` is re-scoped, not deleted

- **Decision:** keep `PullResult.PatternResidue`, `PatternResidueEntry`, `patternResidueCommits`, `parsePatternResidueRecords`, and the `fabriccli` JSON field.
  Change only the git pathspec argument: from the bare `_pattern` directory to `_lyx/PATTERN.md` and `_lyx/pattern/`.
- **Rationale:** the feature answers "which post-anchor weft commits touched hand-authored PATTERN content, after a warp history rewrite invalidated the baseline they were written against".
  It is a human-review flag, never an automated fix.
  Its reason to exist — agent-binding text authored by a human needs re-checking when the host history it was written against is rewritten — is untouched by moving the files.
- **Rejected:** deleting the whole surface — throws away a working signal for a relocation.
  Widening to all of `_lyx` and renaming — every config and loom-status commit becomes residue, drowning the signal the narrow pathspec was chosen to produce.
- **How the new pathspec strings are built:** from `lyxdirs.LyxDirName`, never as bare `"_lyx/PATTERN.md"` / `"_lyx/pattern"` literals — e.g. `lyxdirs.LyxDirName + "/PATTERN.md"`.
  `internal/fabricengine` already imports `internal/lyxdirs` (`status.go:25`), so this adds no dependency.
  Git pathspecs are always forward-slashed, so these are string concatenations, not `filepath.Join` calls;
  `"/PATTERN.md"` and `"/pattern"` are not policed tokens, so only the `_lyx` half needs to come from the const.
- **`TestEnforcement_GeometryLiterals` will not catch a violation here.** It matches whole tokens by exact equality, so a bare `"_lyx/PATTERN.md"` literal is not equal to `"_lyx"` and passes unpoliced. Building from `lyxdirs.LyxDirName` is therefore a **review obligation**, not a machine-enforced one — call it out explicitly in the plan's card so it is not assumed covered.

### No migration

- **Decision:** ship no migration mechanism of any kind — no reconcile branch, no `lyx fabric migrate-pattern` verb, and no operator-facing migration paragraph in `docs/overview.md`.
- **Rationale:** lyx is not in production.
  The only fabric-wired repo that exists is the throwaway SANDBOX repo, which is re-clonable.
  loomyard itself is mill-managed, not fabric-wired (`git ls-files _lyx _pattern` is empty here), so there is no self-migration either.
  Writing migration machinery for a population of one disposable repo is exactly the hypothetical-requirement design this repo's YAGNI discipline rejects.
- **Rejected:** documenting manual operator steps — there is no operator to document them for.
  A reconcile migration branch — `reconcile` deliberately never moves weft content, and `_lyx` specifically holds the hard refusal against fabric touching what might be hand-authored.
  A one-shot CLI verb — a whole new command surface for a one-time event that will never fire.
- **Note for the implementer — the free teardown applies to fresh clones only.** `applyStaleRemoval` (`internal/fabricengine/reconcile.go:391-434`) removes any on-disk junction absent from `RepoWiredNames`, so a `_pattern` junction tears itself down on the next `lyx fabric reconcile` — **but only once that repo's recorded `pathspec` is actually empty.**
  It will not be.
  `yamlengine.applyExistingOverrides` (`internal/yamlengine/reconcile.go:117`) copies each existing leaf value onto the template, so a deployed `<BoardDir>/_lyx/config/fabric.yaml` keeps `pathspec: _pattern` through any number of `lyx config reconcile` runs.
  `Config.Dirs()` therefore still yields `_pattern`, `RepoWiredNames` still contains it, and `applyStaleRemoval` never classifies the junction as stale.
  Changing `template.yaml` governs **newly cloned repos only**.
  That is accepted, not a defect to fix: the sole deployed repo is the throwaway SANDBOX, which is re-cloned rather than migrated.
  Do not write a test asserting teardown-on-upgrade, and do not describe an upgrade path in the docs — neither exists.

### `PollutionEntry.ReportOnly` is deleted; the scan-error entry relies on an empty `Remedy`

- **Decision:** remove the `ReportOnly` field from `internal/fabricengine/status.go:30`'s `PollutionEntry` type, along with **both** its writers: the `_raddle` classification branch (`status.go:222-223`) and the synthetic scan-error entry (`status.go:149-152`).
  The scan-error entry keeps its existing shape otherwise — `Path` holding the `<scan error: %v>` text, `Remedy` left empty.
  No replacement error field, and no change to `Status`'s non-fatal-and-continue behavior.
- **Rationale:** the field's own doc comment defines it as "true when no automated remedy is available", and `Remedy`'s doc comment already says "Empty when the entry is report-only" — the two carry identical information, so `Remedy == ""` is an exact substitute for both writers.
  `Remedy` is `json:"remedy,omitempty"`, so a JSON consumer already sees the key absent on a report-only entry;
  dropping `ReportOnly` removes a redundant `report_only` key rather than a signal.
  Once `_raddle` leaves, every genuine pollution class (`_lyx`) carries a remedy, so a boolean whose only remaining writer is a synthetic error placeholder is dead scaffolding of exactly the kind this task removes.
- **Rejected:** keeping the field for future report-only classes — hypothetical requirement, no current user.
  Adding a dedicated error field or returning the scan error instead — turns a non-fatal inline report into new error-propagation scope for a case the existing `Path` text already describes.
- **Correction to an earlier framing:** the `_raddle` branch was **not** the field's only writer. The scan-error entry is a second one, and the plan must handle it.

### PATTERN content joins `_lyx` commit routing — intended

- **Decision:** accept that hand-authored PATTERN content becomes ordinary `_lyx` content for commit-routing purposes.
  No carve-out, no exclusion mechanism.
- **Rationale:** every round-loop caller commits with `ScopedPathspec(l.AnchorRel, ["_lyx"])` (`internal/fabriccli/weft_verbs.go:102`; the shape is pinned by `committed_lyxonly_integration_test.go:29` and `weftgit_exclude_test.go:293`).
  Before this task, `_pattern` sat outside that pathspec, so an automated weft commit could not sweep up a PATTERN edit.
  After the move it can, and will.
  That is the direct consequence of the decision to collapse `_pattern` into `_lyx` — PATTERN becomes durable, git-tracked weft content like `_lyx/config`, which is already auto-committed the same way.
  Building an exclusion to keep it out would re-create the separation this task exists to remove, at higher cost than the junction it replaces.
- **Consequence to expect:** PATTERN edits now land in weft commits authored by the round loop rather than only in deliberate operator commits, so `PatternResidue` will flag more commits after a warp rewrite than the `_pattern`-era version did. That is more signal, not noise — each flagged commit genuinely touched PATTERN content against the invalidated baseline.
- **Rejected:** a `_lyx/PATTERN.md` + `_lyx/pattern/` carve-out in `ScopedPathspec` or `classify.go` — the Fabric Git Invariant's cross-module exclusions are positive-only by design, and a negative carve-out would be a new mechanism built for one directory.

### `_raddle` un-reservation keeps a positive guard test

- **Decision:** rewrite each `_raddle` assertion across the test suite to the new truth, and repurpose `internal/lyxcwd/raddle_guard_test.go` as a guard asserting `_raddle` is **not** a reserved hub name.
- **Rationale:** a positive test pinning the removal is what stops someone re-adding the reservation later.
  Deleting the guard file removes the only artifact that records the decision in executable form.
- **Rejected:** deleting `raddle_guard_test.go` outright, with or without keeping the other files' expectation changes.

### `manifest/designs/pattern.md` is deleted

- **Decision:** delete `manifest/designs/pattern.md` in this commit.
- **Rationale:** the Documentation Lifecycle (`docs/overview.md`) says module-design docs are mechanical drafts for planned, not-yet-built modules, deleted when the module lands — the implementation and tests become the source of truth.
  `internal/pattern` has landed, so the deletion is already overdue.
  This task is what makes the file's content actively wrong: it describes `_pattern` as "`_lyx`'s first sibling junction" and "the *second* junction the pathspec wires".
- **Rejected:** rewriting its `_pattern`-geometry passages — preserves a file the lifecycle says should not exist.
  Leaving it untouched — leaves a design doc that contradicts the shipped code.

### Every `_raddle`-as-hub/junction reference is corrected

- **Decision:** update every doc, code comment, and design doc that describes `_raddle` as hub-level structural geometry or as a junction-reached weft directory, to the settled anchor-level `_lyx/raddle/` design.
  `manifest/designs/raddle.md` records the geometry;
  the three cross-referencing design docs are corrected in the same pass.
- **Rationale:** the anchor-level decision currently lives only in the task brief.
  Un-reserving the name while leaving five design docs describing the junction geometry guarantees the raddle implementation task starts from the superseded design.
- **Rejected:** updating `raddle.md` only and deferring the cross-references — leaves three docs actively contradicting it.
  Touching no design content — the reason for the un-reservation would be unrecorded anywhere durable.

## Technical context

### The `_pattern` surface, enumerated

`internal/pattern` (the owner):

- `pattern.go` — `DirName`, `Dir`, `File`, `FileHere`, plus the three directive constants (`implementerDirective`, `reviewFixDirective`, `orchestratorDirective`), each of which embeds `_pattern/PATTERN.md` and "every detail doc under `_pattern/`" as **literal, non-interpolated** relative text. `doc.go` explains why the pointer stays relative: an absolute path would vary per worktree and break the fixed-string equality/substring comparisons this package's and its consumers' tests rely on. Preserve that property.
- `doc.go` — the package godoc names `_pattern/PATTERN.md` in the active-check section and the "why the pointer stays relative" section.
- `pattern_test.go`, `patternpath_test.go` — path assertions.
- `leaf_enforcement_test.go` — the `allowedImports` allowlist to widen.

`internal/fabricengine`:

- `template.yaml:2` — `pathspec: _pattern`.
- `pull.go:21-26` — the private `patternDirName` const, existing solely because a git pathspec argument must be a bare string. `pull.go:250-254` calls `patternResidueCommits(anchor.WeftSHA, weftHEADBeforeAnchor)` inside `Pull`'s rewrite-reconcile branch. `pull.go:291-341` is the git spawn plus `parsePatternResidueRecords`. Note the documented relpath-blind limitation on the pathspec, and the record-separator-at-the-start parsing subtlety — both survive the re-scope unchanged.
- `status.go:184` — the `ls-files -- <_lyx> _pattern _raddle` pathspec. `status.go:208-223` — the tracked-prefix classification switch, including the `_raddle` report-only branch. `status.go:30-38` — the `PollutionEntry` type carrying `Path`, `Remedy`, and the `ReportOnly` field to delete. `status.go:149-152` — `ReportOnly`'s **second** writer, the synthetic `<scan error: %v>` entry. `status.go:4,29,72,144,163-178` — doc comments.
- `junctionnames.go` — `HubReservedNames()` at line 123 returns `{BoardDirName, portalsDirName, launchersDirName, "_raddle"}`; doc comments at 111 and 173 enumerate the set.
- **"Four hub-structural tokens" is stated in three places and becomes three.** `internal/fabricengine/structuraldirs_test.go:99-110` — the test is *named* `TestHubReservedNames_StillReturnsExactlyTheFourHubStructuralTokens`, so the function itself is renamed, not just its body. `internal/fabricengine/junctionnames_test.go:73-77` — a sanity loop over the four literals. `internal/fabricengine/doc.go:84` — the package godoc's parenthetical list. The same doc.go passage also says the config-sourced piece is "today just `_pattern`", which becomes "empty today".
- `junction.go:172-175`, `unwire.go:30`, `doc.go`, `reconcile.go:348`, `weftwiring.go:12`, `cleanup.go:81,88` — comments only.

`internal/fabriccli`:

- `weft_verbs.go:93,148,213,302-305` — the pathspec-default doc text and the `PatternResidue` JSON flattening.
- `fabric.go:145,189-192,207-211,262` — command help text describing the junction pair and the pollution scan.

Doc surfaces: `CONSTRAINTS.md:50,104-109,185,199`; `docs/overview.md:125,154,158-167,174,195`; `docs/shared-libs/lyxcwd.md:110`; `README.md:61`; `CLAUDE.md:16`; `docs/research/linux-portability-survey.md:82,87`; `manifest/roadmap.md:37,39`; `tools/sandbox/SANDBOX-FABRIC-SUITE.md:185-186,205`; `manifest/designs/{pattern,raddle,finalize,shed,loom,fabric-unified-view}.md`.

### The geometry-literal owner map

`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` polices which directory may name each geometry token in path-construction context (a `filepath.Join` argument, a `+` operand, or a string const value), matching whole tokens only.
Two rows change: `"_pattern"` (currently dual-owned by `internal/pattern` and `internal/fabricengine`) is deleted outright, and `"_raddle": {"internal/fabricengine"}` is deleted outright.
The `geometryToken` switch at line 254 must drop both tokens too, or the map rows and the switch will disagree.
`docs/shared-libs/lyxcwd.md:110` documents the same token list and must stay in sync.

### Why the junction teardown is free

`RepoWiredNames` is `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())`.
With `pathspec` empty, that collapses to `{_lyx, .lyx}`.
`applyStaleRemoval` compares `scanOnDiskJunctionNames` against that set and removes the difference, so `_pattern` is torn down without a code path dedicated to it.
It is fail-closed: a config-load or scan failure removes nothing.

### `raddleFoldedBack` is a stub

`internal/fabricengine/cleanup.go:83` is `func raddleFoldedBack(_ string) bool { return false }` — no path logic at all.
Its `_raddle` references are comment-only, so the anchor-level geometry change needs a wording update there, not a behavior change.

### Directive-text consumers

`internal/builderengine/template_test.go`, `internal/burlerengine/template_test.go`, and `internal/websterengine/template_test.go` all assert on rendered prompt text containing `_pattern/PATTERN.md` (websterengine at `:47,54,146,172,618,652`, including a fixture that creates a real `_pattern/PATTERN.md` under `t.TempDir()`).
These are the downstream blast radius of rewriting the three directive constants and must move in lockstep.

## Constraints

From `CONSTRAINTS.md`, in force for this task:

- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd. Each module owns its own relative subpath. `internal/pattern` keeps owning its own path accessors; it never gains cwd-resolution logic, and `lyxcwd` never gains an `_lyx` path constructor.
- **Lyxdirs Single-Declarer Invariant** (`CONSTRAINTS.md:36`) — `internal/lyxdirs` is the sole declarer of `_lyx` (`LyxDirName`) and `.lyx` (`DotLyxDirName`), stays **stdlib-only, a zero-import leaf**, and no other production file may name either literal in path-construction context. Enforced by `TestEnforcement_GeometryLiterals`.
- **Durable-vs-Ephemeral State Invariant** — `_lyx` holds tracked content only; `_lyx`/`.lyx` are structural and never read from `fabric.yaml`'s `pathspec`. The line at `CONSTRAINTS.md:50` cites `_pattern` as its example of an optional dir and needs a new example or a rephrasing once `_pattern` is gone. PATTERN detail docs are tracked content, so `_lyx/pattern/` is correct and needs no `.lyx` mirror.
- **Pattern Leaf Invariant** — widens by exactly one entry (`internal/lyxdirs`), same commit, prose and test together.
- **Fabric Git Invariant** — `CONSTRAINTS.md:185`'s `_lyx`/`_raddle` file-contract line and `:199`'s unwire-preserves line both name the removed geometry and must be corrected.
- **CLI/Cobra Invariant** — no new command is added, so the `Command()`/`RunCLI` seam and help-tree tests are untouched. Existing `fabriccli` help text changes are prose only.
- **Documentation Lifecycle** — this task changes cross-cutting infrastructure, so `docs/overview.md`, `CONSTRAINTS.md`, and the affected `manifest/designs/` docs update in the same commit. `manifest/roadmap.md` is corrected for factual accuracy only, never moved.
- **Fabric Vocabulary Invariant** — `weft`/`warp`/fabric-sense `host` stay confined to their owner set. Any new comment text in `internal/pattern` must avoid that vocabulary; `internal/pattern` is not an owner.
- **Markdown convention** — semantic line breaks, one sentence per line, no fixed-column hard-wrap, in every `.md` file touched.

## Testing

The suite is already dense here — roughly forty files reference `_pattern` and thirty reference `_raddle` — so most of the work is converging existing assertions rather than writing new ones.
Sequence the work so the token-ownership tests are the last thing to go green: they are the repo-wide proof the tokens are gone.

**`internal/pattern` — TDD candidate.**
Update `patternpath_test.go` first: assert `File(base)` yields `<base>/_lyx/PATTERN.md` and `FileHere(l)` yields `<worktree>/<anchorRel>/_lyx/PATTERN.md`, before touching `pattern.go`.
Update `pattern_test.go`'s active-check cases in the same pass — the four existing edge cases (absent, empty file, `PATTERN.md` as a directory, non-`IsNotExist` stat error) are behavior-preserving and must keep passing verbatim, only their fixture paths moving.
Extend `leaf_enforcement_test.go`'s allowlist and confirm it still fails on a deliberately-added feature-package import.

**Directive text — TDD candidate.**
Pin the new pointer strings in `pattern_test.go` before editing the three constants, then converge `builderengine/template_test.go`, `burlerengine/template_test.go`, and `websterengine/template_test.go`.
`websterengine/template_test.go`'s `t.TempDir()` fixture that materializes a real `_pattern/PATTERN.md` must move to `_lyx/PATTERN.md`.
Keep the constants' literal-relative-string property intact: a test that only substring-matches an interpolated path would let the regression through.

**`PatternResidue` re-scope — TDD candidate.**
`pull_integration_test.go:246-248`'s `TestPull_IdentifiesPatternResidue` seeds a synthetic PATTERN-touching weft commit plus a non-PATTERN one and asserts the residue names exactly the former.
Re-point its fixtures at `_lyx/PATTERN.md` and add a case proving a `_lyx/config/fabric.yaml`-only commit is **not** residue — that negative case is the whole point of keeping the pathspec narrow rather than widening it to `_lyx`.
Cover a commit touching `_lyx/pattern/<detail>.md` as residue too.
`parsePatternResidueRecords` is a pure function; its record-boundary parsing tests need no change and should be left alone as a regression anchor.

**Junction teardown.**
`reconcile_stale_removal_test.go` is the densest `_pattern` file (18 references) and currently proves `_pattern` is wired.
Invert it: with an empty `pathspec`, an on-disk `_pattern` junction is stale and removed, and `_lyx`/`.lyx` are not.
`junction_pattern_integration_test.go` (38 references), `weftgit_pathspec_integration_test.go` (17), `junction_repoint_test.go` (11), `classify_test.go` (16), `config_driven_junctions_integration_test.go`, `remove_junctions_integration_test.go`, and `unwire_test.go` all wire `_pattern` as their second junction — each needs a substitute generic optional name (the existing `_extra` used by `config_driven_junctions_integration_test.go` and `junctionnames_test.go` is the natural choice) so the config-driven-junction behavior stays covered by a name that is not the one being deleted.
Do not simply delete these cases: the generic multi-junction path must remain tested after `_pattern` stops being its exemplar.

**Reserved-name un-reservation — both tokens, and the arithmetic is six → four.**
`junctionnames_test.go:232-246`'s `"default pathspec union reserves exactly six names"` case asserts `{_lyx, _pattern, _board, _portals, _launchers, _raddle}`.
**Two** names leave, not one: `_raddle` because `HubReservedNames()` drops it, and `_pattern` because an empty `pathspec` removes it from the `junctionNames` union `IsReservedHubName` folds in.
So the set drops to **four** — `{_lyx, .lyx, _board, _portals, _launchers}` minus the hub tokens that remain, i.e. rename the sub-test and recount rather than decrementing by one.
`internal/lyxcwd/lyxcwd_test.go:132-141`'s `TestIsReservedHubName_Pattern` asserts the exact opposite of the new truth and must be inverted (and renamed), not merely edited — it is the only test pinning `_pattern` as a reserved slug.
`raddle_guard_test.go` inverts to a positive "not reserved" guard.
`hostjunction_test.go`, `config_test.go`, `fabric_test.go`, `structuraldirs_test.go`, `add_test.go`, `lyxcwd_test.go`, and `cmd/lyx/tierpurity_test.go` each need their `_raddle` expectations converged, plus `structuraldirs_test.go`'s test-function rename off "TheFour".
Add cases proving a worktree slug named `_raddle` **and** one named `_pattern` are now accepted by `IsReservedHubName` — that is the observable behavior change and nothing currently pins it.

**Slug/junction collision risk from un-reserving `_pattern`.**
A worktree slug named `_pattern` is now legal, and a repo that still has a stale on-disk `_pattern` junction would get a worktree directory colliding with it.
This is not worth guarding in code — the un-reservation is deliberate, and per the "No migration" decision the only repo that could hold a stale junction is the re-clonable SANDBOX.
Record it as a known consequence in the commit message;
do not add a guard or a test for it.

**Host-pollution scan.**
Prove the `ls-files` pathspec no longer names `_pattern` or `_raddle`, and that a tracked `_lyx/...` path still reports with its `git rm --cached` remedy.
`ReportOnly`'s removal has **two** call sites to converge, not one — the `_raddle` branch and the scan-error entry at `status.go:149-152`.
Pin that a scan failure still produces its synthetic `<scan error: …>` entry with an empty `Remedy`, and that `Status` still continues rather than failing the pair — the `omitempty` tag means the JSON shape changes only by losing the `report_only` key.

**Config plumbing.**
`internal/configsync/configsync_test.go:480-481` asserts the template default is `pathspec: _pattern` and must assert the empty default instead.
`internal/fabricengine/template_test.go` and `internal/configcli/configcli_integration_test.go:67` wire `{"_lyx", "_pattern"}` explicitly.
Add a case proving an empty `pathspec` yields `Dirs() == nil` and that `pathspecNames`/`junctionNames` degrade to the structural sets without panicking on the nil slice.

**Repo-wide proof.**
`internal/lyxcwd/enforcement_test.go` is the gate: after the `geometryToken` switch and `geometryTokenOwners` map drop both rows, any surviving `_pattern` or `_raddle` path-construction literal in production code fails the build.
Note that the enforcement scan excludes `*_test.go` by design, so test-side occurrences are a review obligation, not machine-caught — a final `grep -rn '_pattern\|_raddle'` across `internal/`, `cmd/`, `docs/`, `manifest/`, `tools/`, `README.md`, and `CLAUDE.md` is the closing check.

## Q&A log

- **Q:** Where do the PATTERN detail docs land — flat under `_lyx/`, or under `_lyx/pattern/`? **A:** `_lyx/pattern/`, with `PATTERN.md` itself flat at `_lyx/PATTERN.md`. The brief's fully-flat wording was rejected because it makes the agent directives point at machine-owned config.
- **Q:** Does `pattern.DirName`/`Dir()` survive in any form? **A:** No — both deleted; `File` builds from `lyxdirs.LyxDirName`.
- **Q:** What is `PatternResidue`, and does it survive? **A:** It is the human-review flag listing post-anchor weft commits that touched hand-authored PATTERN content after a warp history rewrite invalidated their baseline. It survives, re-scoped to the new paths — not deleted, and not widened to all of `_lyx`.
- **Q:** What does `template.yaml`'s `pathspec` become? **A:** Empty, key retained. Not `_lyx` — that would violate the Durable-vs-Ephemeral State Invariant, and the brief's premise about the current value was wrong.
- **Q:** What migration path ships for already-wired repos? **A:** None. No code, no CLI verb, no doc paragraph.
- **Q:** Are there live fabric-wired repos to worry about? **A:** Only the SANDBOX repo. lyx is not in production, and loomyard itself is mill-managed rather than fabric-wired.
- **Q:** Does `PollutionEntry.ReportOnly` survive `_raddle`'s removal? **A:** No — deleted along with **both** its writers (the `_raddle` branch and the scan-error entry). The type is `PollutionEntry`, not `HostPollution`.
- **Q:** Keep or delete `internal/lyxcwd/raddle_guard_test.go`? **A:** Keep, repurposed as a positive guard that `_raddle` is not reserved.
- **Q:** Is the Pattern Leaf Invariant still justified, or just an old agent's decision? **A:** Its ban on feature packages is load-bearing — four feature packages import `internal/pattern`, so a reverse import is a real cycle. The allowlist form is stricter than needed but cheap, and `internal/lyxdirs` is cycle-safe by construction, so the allowlist stays and widens by one entry.
- **Q:** Does `manifest/designs/pattern.md` get rewritten or deleted? **A:** Deleted — the module landed, so the Documentation Lifecycle already required it.
- **Q:** How far does the `_raddle` documentation sweep reach? **A:** Every reference to the old hub-level/junction geometry, across docs, code comments, and all five affected design docs — not just `manifest/designs/raddle.md`.
- **Q:** Does emptying `template.yaml`'s `pathspec` actually reach a deployed `fabric.yaml`? **A:** [auto-pick, review r1] No. `yamlengine.applyExistingOverrides` preserves existing leaf values, so a deployed repo keeps `pathspec: _pattern` and never sees the junction as stale. **Why:** the operator chose no migration and the only deployed repo is the re-clonable SANDBOX, so the correct resolution is to state the fresh-clone-only limitation explicitly rather than build an upgrade path.
- **Q:** What becomes of the scan-error entry once `ReportOnly` is deleted? **A:** [auto-pick, review r1] It keeps its `<scan error: …>` `Path` with an empty `Remedy`. **Why:** `Remedy`'s own doc comment already defines empty as report-only and the field is `omitempty`, so `Remedy == ""` carries identical information with no new error field and no new error-propagation scope.
- **Q:** Is it intended that PATTERN content now joins `_lyx`'s automated commit routing? **A:** [auto-pick, review r1] Yes, with no carve-out. **Why:** it is the direct consequence of collapsing `_pattern` into `_lyx` — PATTERN becomes durable weft content like `_lyx/config`, which is already auto-committed; a negative carve-out would re-create the separation this task removes, and the Fabric Git Invariant's exclusions are positive-only by design.
- **Q:** How does `fabricengine` construct the new `_lyx/...` residue pathspec strings? **A:** [auto-pick, review r1] From `lyxdirs.LyxDirName` by string concatenation (it already imports `lyxdirs`), never as bare literals — and this is a review obligation, since `TestEnforcement_GeometryLiterals` matches whole tokens by exact equality and would not catch `"_lyx/PATTERN.md"`. **Why:** the Lyxdirs Single-Declarer Invariant applies regardless of whether the enforcement test can mechanically see the violation.
- **Q:** How many names does `IsReservedHubName`'s default-pathspec union reserve after this task? **A:** [auto-pick, review r1] Four, not five — `_pattern` leaves alongside `_raddle`, because an empty `pathspec` removes it from the `junctionNames` union. **Why:** an earlier framing counted only `_raddle`'s departure and would have left `TestIsReservedHubName_Pattern` asserting the opposite of the new truth.
- **Q:** Does un-reserving `_pattern` create a slug/junction collision risk? **A:** [auto-pick, review r1] Yes in principle, recorded in the commit message, guarded nowhere. **Why:** the un-reservation is deliberate and the only repo that could hold a stale `_pattern` junction is the re-clonable SANDBOX, so a guard would be scope built for a population of zero.
