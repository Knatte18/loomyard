# Batch: repoint-and-delete-twins

```yaml
task: "planparser owns the plan-directory path"
batch: "repoint-and-delete-twins"
number: 2
cards: 12
verify: go test ./internal/planparser/... ./internal/loomengine/... ./internal/webstercli/... ./internal/websterengine/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
depends-on: [1]
```

## Batch Scope

This batch repoints every caller of `loomengine.PlanDir`/`loomengine.PlanOverview` onto the `planparser` functions batch 1 added, deletes the two `loomengine` twins outright, closes the anchor-always coverage gap the move opens, updates the four stale comments the change falsifies, and lands the `CONSTRAINTS.md` and `docs/overview.md` doc obligations.
Its headline outcome is that `internal/webstercli` no longer imports `internal/loomengine` at all — production and test alike — so a producer CLI stops depending on an orchestrator engine.
It is one batch because the deletion and the repoints are one atomic compile unit: split across batches, the tree would be red at a batch boundary.
There is no external interface for a later batch to consume — this batch is the task's terminal state.

**Batch-local decision — card order keeps every intermediate commit compiling.**
Cards 3–10 repoint callers while both the `loomengine` twins and the `planparser` functions exist side by side, which compiles at every step (the twins simply become unused exported functions for a few commits).
Card 11 then deletes the twins, once nothing references them.
Cards 12–14 are comment/doc-only and carry no compile risk.
Do not reorder card 11 earlier;
doing so makes every commit between it and the last repoint uncompilable.

**Batch-local decision — the two `webstercli` fixture files are edited by two cards each in the case of `verbs_test.go`.**
Card 7 does the mechanical repoint, import swap, `AnchorRel` flip and header-comment fix;
card 8 adds the new subpath-anchored `PersistentPreRunE` case and the helper parameterization it needs.
They are separate cards because they are separate kinds of work — one mechanical, one the batch's load-bearing new coverage — and because card 8's case is the single most likely place for the batch to need iteration.

**Batch-local decision — `internal/websterengine/beginbatch.go` stays out of scope.**
The `lyxcwd-resolved` grep (see below) returns it, but its comment names only `WebsterDir`/`ReportsDir`/`PromptsDir`/`ScratchDir`, all of which genuinely remain `lyxcwd`-resolved after this task.
T7 is what falsifies those;
touching them here would be scope creep.

## Cards

### Card 3: repoint `internal/loomengine/plan.go`'s two plan-path calls

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/loomengine/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `PlanSpec`, replace the two in-package calls that currently read

  ```go
	planDir := PlanDir(layout)
	overviewPath := PlanOverview(layout)
  ```

  with `planparser.PlanDir(layout.AnchorPath())` and `planparser.PlanOverview(layout.AnchorPath())` respectively, keeping the same two local variable names and the same two lines.
  Leave the `decisionRecordPath := DiscussionDecisionRecord(layout)` line above them exactly as it is — `loomengine` keeps owning the discussion paths.

  Add `github.com/Knatte18/loomyard/internal/planparser` to this file's import block.
  The `internal/loomengine` package already imports `planparser` (in `internal/loomengine/config.go`), so this introduces no new package edge and no cycle;
  it is a per-file import addition only.

  Change nothing else in the file: `Spec.OutputFiles` stays `[]string{overviewPath}`, `composePlanPrompt`'s argument list is unchanged, and the file's header comment stays as it is (it names `_lyx/plan` as a path, not a declaring package, and remains true).
- **Commit:** `refactor(loomengine): repoint PlanSpec onto planparser plan paths`

### Card 4: repoint `internal/loomengine/plan_test.go` and add the subpath-anchored `PlanSpec` case

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/planpath_test.go`
  - `internal/planparser/parse.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/modelspec/load.go`
- **Edits:**
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two changes.

  **First, the mechanical repoint.**
  In `TestPlanSpec_PromptFilled`, the two lines

  ```go
	planDir := PlanDir(layout)
	overviewPath := PlanOverview(layout)
  ```

  become `planparser.PlanDir(layout.AnchorPath())` and `planparser.PlanOverview(layout.AnchorPath())`.
  Add `github.com/Knatte18/loomyard/internal/planparser` to the import block.
  Leave the `decisionRecordPath := DiscussionDecisionRecord(layout)` line alone.

  **Second, add a new test function — this batch's anchor-always workhorse on the `loomengine` side.**
  Name it `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath`.
  Build a `*lyxcwd.Location` with a non-`"."` `AnchorRel` (use `"backend"`), so `AnchorPath()` and `WorktreePath()` are distinguishable strings, and construct it the same hand-built way the file's existing tests do — `&lyxcwd.Location{HubPath: ..., WorktreeName: ..., AnchorRel: "backend"}`.
  Load a registry with `modelspec.LoadRegistry(t.TempDir())` and build the spec with `PlanSpec(layout, newTestStencilsDir(t), cfg, reg)` exactly as the existing tests do.

  Assert all of the following:
  - `spec.OutputFiles` has exactly one entry and it equals `filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName, "00-overview.md")`.
  - That entry is NOT equal to the `WorktreePath()`-rooted counterpart, `filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName, planparser.PlanDirName, "00-overview.md")`.
    Assert the inequality explicitly with its own failure message naming the wrong root — this is the assertion that fails if someone later passes `layout.WorktreePath()` at the `internal/loomengine/plan.go` call site.
  - `spec.Prompt` contains the `AnchorPath()`-rooted plan directory, `filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName)`, and does NOT contain the `WorktreePath()`-rooted one.

  Give the test a doc comment saying it is the case that catches a wrong-root argument at `PlanSpec`'s plan-path call sites, and that this is why it builds a subpath anchor rather than reusing the other tests' default `AnchorRel`.
  Note that the file's existing tests build their `layout` with no `AnchorRel` field at all (the zero value, empty string), which `filepath.Join` collapses to the worktree path — do not "fix" those, they are testing field mapping, not anchoring.

  Leave `TestPlanSpec`, `TestPlanSpec_PatternDirectiveOptional` and every other existing test in the file unchanged apart from the two repointed lines.
- **Commit:** `test(loomengine): prove PlanSpec anchors plan paths on AnchorPath`

### Card 5: repoint `internal/webstercli/cli.go` and drop its `loomengine` import

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/loomengine/config.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/validate.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three changes in this one file.

  **The call.** In `Command()`'s `PersistentPreRunE`, the line `c.planDir = loomengine.PlanDir(layout)` becomes `c.planDir = planparser.PlanDir(layout.AnchorPath())`.
  Leave the four sibling assignments beside it (`c.websterDir`, `c.reportsDir`, `c.websterScratchDir`, `c.promptsDir`) untouched — those are `websterengine`'s own constructors and are not this task's business.
  Do not restructure `PersistentPreRunE` in any way;
  exactly one line inside it changes.

  **The imports.** Remove `github.com/Knatte18/loomyard/internal/loomengine` and add `github.com/Knatte18/loomyard/internal/planparser`.
  After this card the `internal/webstercli` production package no longer imports `internal/loomengine` at all — verify with a grep over the package's non-test files.
  The `internal/webstercli` package already depends on `planparser` through `internal/webstercli/validate.go` and its two sibling verb files, so the package's dependency set strictly shrinks by one edge.

  **The stale comment.** The `websterCLI` struct field comment currently reads

  ```go
	// planDir, websterDir, and reportsDir are the lyxcwd-resolved _lyx dirs;
	// promptsDir and websterScratchDir are the lyxcwd-resolved .lyx dirs.
  ```

  Reword it so `planDir` is described as told rather than `lyxcwd`-resolved: it is `planparser`'s own told-anchor path, built from `layout.AnchorPath()` in `PersistentPreRunE`, while `websterDir` and `reportsDir` remain the `lyxcwd`-resolved `_lyx` dirs and `promptsDir`/`websterScratchDir` remain the `lyxcwd`-resolved `.lyx` dirs.
  Keep the comment to two or three lines.

  Leave the file's package header comment alone: its sentence about every `_lyx/plan` and `_lyx/webster` path being anchored at `layout.AnchorPath()` is still exactly true and is the anchor-always rule stated at the right place.
- **Commit:** `refactor(webstercli): resolve planDir via planparser, drop loomengine`

### Card 6: repoint `internal/webstercli/cli_test.go` and flip `newTestCLI` off the `"."` anchor

- **Context:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/validate.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/planparser/parse.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three changes.

  **The call and the import.** In `newTestCLI`, `planDir: loomengine.PlanDir(layout)` becomes `planDir: planparser.PlanDir(layout.AnchorPath())`.
  Remove `github.com/Knatte18/loomyard/internal/loomengine` from the import block and add `github.com/Knatte18/loomyard/internal/planparser`.

  **The anchor flip.** `newTestCLI` builds its layout with `AnchorRel: "."`, where `AnchorPath()` and `WorktreePath()` are the same string, so no test in this file can currently distinguish the two roots.
  Change that one fixture's `AnchorRel` to `"backend"`.
  The fixture creates real directories on disk (via `seedValidPlanDir`, which calls `os.MkdirAll`), so confirm that the plan the tests seed and the directory `c.planDir` names stay the same place after the flip — a fixture that seeds one directory and reads another would pass vacuously.
  All six callers discard the helper's second return value, so nothing downstream depends on it naming the anchor;
  leave the second return as it is.

  **Do not flip the two `fabricSync` layouts.**
  The layouts inside `TestFabricSync_SkipGitBypassNeedsNoFabricWorktree` and `TestFabricSync_NonBypassValidatesPairPaths` keep `AnchorRel: "."`.
  Those tests call `fabricSync(layout, …)` directly and never touch `planDir`;
  re-anchoring them would change what they exercise for no plan-path gain.

  **The weakening comment.** Add a comment at `newTestCLI` recording exactly what the flip does and does not buy: it proves the plan paths behave correctly at a nested anchor, but it does NOT prove anchoring, because `newTestCLI` both computes `planDir` and is the same value the tests seed into, so a `WorktreePath()` slip would stay self-consistent and pass at any `AnchorRel`.
  Point the reader at the subpath-anchored `PersistentPreRunE` case (added by card 8, in `internal/webstercli/verbs_test.go`) as the place anchoring is actually proven for this module.

  Leave the file's header comment alone — its statement that every fixture here builds a `*websterCLI` literal directly, bypassing `Command()`'s `PersistentPreRunE`, is true of this file and stays true.
- **Commit:** `test(webstercli): repoint cli_test onto planparser, anchor the fixture`

### Card 7: repoint `internal/webstercli/verbs_test.go` and flip its plan-dir fixture

- **Context:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/planparser/parse.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three changes.
  This file carries `//go:build integration` on its first line, so it compiles only under `go test -tags integration`;
  a green untagged run proves nothing about this card.

  **The two calls and the import.** In `newVerbsFixture`, both `seedValidPlanDir(t, loomengine.PlanDir(layout))` and the `planDir: loomengine.PlanDir(layout)` struct field become `planparser.PlanDir(layout.AnchorPath())`.
  Remove `github.com/Knatte18/loomyard/internal/loomengine` from the import block and add `github.com/Knatte18/loomyard/internal/planparser`.
  After this card and card 6, no file in `internal/webstercli` — production or test — imports `internal/loomengine`;
  confirm with a grep over the whole package directory.

  **The anchor flip.** `newVerbsFixture`'s layout is built with `AnchorRel: "."`;
  change it to `"backend"` so the two roots are distinguishable strings at every site that consumes `c.planDir` — the plan seeding in this helper and the `testPlanFingerprint` helper.
  The layout's `HubPath`/`WorktreeName` derivation from the scratch repo stays exactly as it is: `verbsFixture.Worktree` must keep naming the real git worktree root, because the tests run `git` against it (`commitFile`, `mustGit`) and those operations are anchor-independent.
  Every other fixture path (`websterDir`, `reportsDir`, `websterScratchDir`, `promptsDir`) is layout-derived and moves down by the anchor consistently, so no test's seeded state and read state can diverge.
  If any existing test in this file fails after the flip, the fix is to make the fixture create the anchored directory it needs — never to revert the flip.

  **The stale header comment.** The file's header comment claims tests "build a `*websterCLI` literal directly (bypassing `Command()`'s `PersistentPreRunE`)".
  That is already false: `seedPersistentPreRunFixture` and its two tests drive the real pre-run through `RunCLIIn`.
  Reword the claim to say that *most* tests here build a `*websterCLI` literal directly, with `seedPersistentPreRunFixture` as the deliberate exception that drives `Command()`'s real `PersistentPreRunE`.
  This comment is the traceable source of this task's own first-draft claim that the pre-run was untestable, which is exactly the cost of leaving a stale comment standing — do not skip it.
- **Commit:** `test(webstercli): repoint verbs_test onto planparser, anchor the fixture`

### Card 8: add a subpath-anchored `PersistentPreRunE` case covering the production call site

- **Context:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/validate.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/batcher/template.go`
  - `internal/planparser/parse.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This is the batch's load-bearing new coverage: the only test that exercises `internal/webstercli/cli.go`'s own production `planparser.PlanDir(layout.AnchorPath())` call at a nested anchor.
  It needs no refactor of `PersistentPreRunE` — that factoring belongs to a later task and would collide with it.

  **Parameterize the existing fixture helper.**
  `seedPersistentPreRunFixture(t, batcherConfig)` currently hardcodes `hubforge.NewHub(t, ".")`.
  Add an `anchor string` parameter (the value `NewHub` takes, documented as `"." or "backend"`) and thread it through to `NewHub`.
  Update the two existing callers, `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves`, to pass `"."` so their behaviour is byte-identical to today.
  Config seeding stays `hubforge.SeedConfig(t, h, …)`, which writes to `h.WeftBase` — the anchor-joined weft directory.
  Do not switch it to `h.PrimeWeft()`: at a non-`"."` anchor that writes a file no module loader reads, with no error at all.

  **Add the new test.**
  Name it `TestPersistentPreRunE_PlanDirAnchoredAtSubpath`.
  Build the hub with the `"backend"` anchor via the parameterized helper and the default `batcher.ConfigTemplate()`.
  Seed a valid plan under the **anchored** location — `planparser.PlanDir(h.Location.AnchorPath())` — using this file's existing `seedValidPlanDir` helper.
  Then drive the `validate` verb through `RunCLIIn`, passing `h.Location.AnchorPath()` as the cwd, not `h.PrimeWorktree()`: `lyxcwd.Resolve` gates cwd against the anchored directory exactly, so at a `"backend"` hub the anchor directory is what `RunCLIIn` must be given.
  `validate` is the right verb because it calls `planparser.ParsePlan(c.planDir)` and surfaces planparser's own `"plan overview not found: <path>"` when the directory is wrong.

  Assert exit code 0 and that the output carries `"valid":true`.
  Add an assertion that the output does NOT contain `plan overview not found`, with a failure message stating that a `WorktreePath()`-based resolution at `cli.go`'s `c.planDir` assignment would look under the un-anchored worktree root and produce exactly that error.
  That negative assertion is the point of the test: it names the failure mode in the message so a future breakage is legible rather than a bare exit-code mismatch.

  Give the test a doc comment saying it is the one case that covers `cli.go`'s production plan-path call at a nested anchor, and that the fixture flip in `newVerbsFixture` and the `cmd/lyx` guard rows deliberately do not carry that proof.
  Follow this file's existing convention and do NOT add `t.Parallel()` — the file stays serial.
- **Commit:** `test(webstercli): cover the anchored plan-dir pre-run at a subpath anchor`

### Card 9: rewrite `cmd/lyx/constructoranchoring_test.go`'s plan rows

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/plan_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three changes.

  **The four rows.** In both `TestConstructorAnchoring_Unanchored` and `TestConstructorAnchoring_SubpathAnchored`, the two `_lyx`-durable-group plan rows become:

  ```go
	assertPath(t, "planparser.PlanDir", planparser.PlanDir(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName))
	assertPath(t, "planparser.PlanOverview", planparser.PlanOverview(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName, "00-overview.md"))
  ```

  Keep them in the `_lyx`-durable group, in the same position (first two rows of that group), in both fixtures.
  The file already imports `planparser`, so no import change is needed here.
  Do not remove the `loomengine` import — the file's other five `loomengine` rows (`DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, `LoomStatusFile`, `LoomStatusLock`) are untouched by this task.
  Change no other row in either fixture, and leave the `dotLyxConstructors` prefix-exclusion guard at the bottom of the subpath-anchored test exactly as it is.

  **The weakening comment.** Add a comment immediately above the two rows in each fixture recording precisely what they now do and do not prove: they still pin the join arithmetic and the `_lyx`-vs-`.lyx` group placement, but because they pass `l.AnchorPath()` in and compare against an `anchor`-derived expectation, they are now tautological with respect to anchoring and can no longer catch a production call site that passes the wrong root.
  Name where that proof now lives — the subpath-anchored `PlanSpec` case in `internal/loomengine/plan_test.go` and the subpath-anchored `PersistentPreRunE` case in `internal/webstercli/verbs_test.go`.

  **The header framing sentence.** The file's opening line reads "pins every constructor batch 5 relocated out of `internal/lyxcwd` into its owning module", which stops being exact once two rows name `planparser` functions that batch 5 never relocated and that take a plain string rather than a `*lyxcwd.Location`.
  Widen that framing sentence so it covers both kinds of row — relocated `*lyxcwd.Location`-taking constructors and told-anchor path functions — without dropping the batch-5 anchoring-table reference the rest of the header depends on.
  Leave the header's `planparser` entry in the owning-module enumeration as it is;
  it is already listed.
- **Commit:** `test(cmd/lyx): pin planparser plan paths in the anchoring table`

### Card 10: rewrite `cmd/lyx/notransients_test.go`'s `durableSet` plan rows

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/plan_test.go`
  - `internal/webstercli/verbs_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Four changes.

  **The two rows.** In `durableSet`, the first two entries become `{"planparser.PlanDir", planparser.PlanDir(l.AnchorPath())}` and `{"planparser.PlanOverview", planparser.PlanOverview(l.AnchorPath())}`.
  They stay in `durableSet` — the plan directory is durable (`_lyx`, git-tracked), never ephemeral (`.lyx`) — and stay in their current position.
  Do not touch `transientSet`, and do not touch `TestNoTransientsUnderLyx`'s assertion logic or its two `AnchorRel` fixtures: the durable-vs-transient assertion is unchanged and must still hold at both geometries.

  **The import.** Add `github.com/Knatte18/loomyard/internal/planparser` to the import block.
  **Keep** the `internal/loomengine` import: `durableSet` still carries `loomengine.DiscussionDir` and `loomengine.LoomStatusFile`, and `transientSet` still carries `loomengine.LoomStatusLock`, none of which this task touches.
  So this file gains an import and loses none.

  **The weakening comment.** Add a one-line comment above the two rewritten rows recording that they share the weakening annotated on `cmd/lyx/constructoranchoring_test.go`'s plan rows: they still pin that no durable plan path resolves as a transient at both `AnchorRel` fixtures, but because the row now builds `l.AnchorPath()` itself instead of calling a constructor that anchored internally, neither row can catch a production call site that passes the wrong root.
  Name where that proof lives — the subpath-anchored `PlanSpec` case in `internal/loomengine/plan_test.go` and the subpath-anchored `PersistentPreRunE` case in `internal/webstercli/verbs_test.go`.

  **The header enumeration.** The file's header comment lists the owning modules it may import at once (`loomengine`, `websterengine`, `perchengine`, `treadleengine`, `scoutengine`, `logger`).
  Add `planparser` to that enumeration, in the same commit as the import.
- **Commit:** `test(cmd/lyx): pin planparser plan paths in the transient guard`

### Card 11: delete `loomengine.PlanDir`/`PlanOverview` and their test file

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/planpath_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:**
  - `internal/loomengine/planpath_test.go`
- **Moves:** none
- **Requirements:**
  Run this card only after cards 3–10 have landed;
  it is the card that removes the twins, and every caller must already be repointed.

  **In `internal/loomengine/config.go`,** delete the `PlanDir` and `PlanOverview` function bodies together with their doc comments — four declarations' worth of text in all, the two doc comment blocks and the two functions.
  Delete nothing else in the file: the `discussionDirName` constant and the five remaining constructors (`DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, `LoomStatusFile`, `LoomStatusLock`) stay exactly as they are, as do `Config`, `LoadConfig` and `ConfigTemplate`.

  **Remove the now-unused `github.com/Knatte18/loomyard/internal/planparser` import from `internal/loomengine/config.go`.**
  The deleted `PlanDir` body was this file's only `planparser.` reference, so leaving the import would not compile.
  Grep the file for `planparser.` after the deletion to confirm zero remaining references.
  Leave `path/filepath` and `github.com/Knatte18/loomyard/internal/lyxdirs` in place — both are still used by the surviving constructors.
  Also leave `github.com/Knatte18/loomyard/internal/lyxcwd` in place, still used by every surviving constructor's parameter type.

  The file's header comment describes `Config`/`LoadConfig` and does not mention the plan-path constructors, so it needs no change.

  **Delete `internal/loomengine/planpath_test.go` outright.**
  Its cases were ported to `internal/planparser/planpath_test.go` in batch 1;
  its third case (`TestLocationPlanDir_UnanchoredEqualsWorktreePath`) collapsed deliberately, because a told string has no unanchored-vs-anchored distinction to assert inside `planparser`.
  Do not port anything further from it here.
  Anchoring coverage on the `loomengine` side lives in the new `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` case card 4 added.

  This is a deletion, not a rename: the file's successor lives in a different package with different test names and a different parameter shape, so `Moves:` would be wrong for it.
- **Commit:** `refactor(loomengine): delete the PlanDir and PlanOverview twins`

### Card 12: fix `internal/websterengine/runlevel.go`'s stale `RunDeps` comment

- **Context:**
  - `internal/webstercli/run.go`
  - `internal/webstercli/cli.go`
  - `internal/websterengine/beginbatch.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The `RunDeps` doc comment states that `PlanDir, WebsterDir, ReportsDir, PromptsDir, ScratchDir, WorktreeRoot are lyxcwd- resolved paths` (the hyphenated word is wrapped across a line break in the source).
  `RunDeps.PlanDir` is fed verbatim from `webstercli`'s `c.planDir`, so this task's change lands the same falsehood one layer down.

  Reword the comment so `PlanDir` is described as `planparser`'s told-anchor path — supplied by the caller from `layout.AnchorPath()` — while the remaining five fields keep their existing description.
  Keep the change confined to that one sentence of the `RunDeps` doc comment;
  do not touch the `RunDeps` struct fields, `Run`, `MasterStarter`, or anything else in the file.

  Do not touch `internal/websterengine/beginbatch.go`'s two `lyxcwd-resolved` comments.
  They name only `WebsterDir`/`ReportsDir`/`PromptsDir`/`ScratchDir`, which genuinely remain `lyxcwd`-resolved after this task.

  Before editing, re-run the enumerating grep rather than trusting this list — the tree moves:

  ```
  grep -rn "lyxcwd-resolved\|lyxcwd- resolved" --include="*.go" internal/ cmd/
  ```

  Both spellings are required because `runlevel.go` wraps the hyphenated word across a line break.
  The expected result is 5 lines in 3 files: this file's one line (in scope), `internal/webstercli/cli.go`'s two (fixed in card 5), and `internal/websterengine/beginbatch.go`'s two (out of scope).
  If the grep returns anything else, treat the extra hit as in scope only if its comment names the plan directory.
- **Commit:** `docs(websterengine): describe RunDeps.PlanDir as a told path`

### Card 13: reword the Planparser Sole-Parser Invariant

- **Context:**
  - `internal/planparser/doc.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the `## Planparser Sole-Parser Invariant` section, replace the bullet reading *"Resolves `_lyx/plan/` via `lyxcwd`, never string literals"* with a told-anchor ownership bullet.
  That bullet is already false today — `planparser` does not import `lyxcwd` — and after this task it is false in a second way, because the package is now *told* an absolute anchor path.

  The replacement bullet must state all four of: `planparser` is the sole declarer of the plan directory's path;
  `PlanDirName`/`PlanDirRel()` declare the worktree-relative token and `PlanDir`/`PlanOverview` the absolute form;
  the package never resolves cwd and never imports `internal/lyxcwd`;
  and the caller supplies the anchor path — `AnchorPath()`, never `WorktreePath()`.
  Deleting the bullet outright is wrong: stating sole-declarership is the point of this task, and the generic per-module-subpath rule in the Cwd Resolution Invariant would leave this invariant silent on it.

  Leave the section's other content alone: the opening sentence, the no-other-package-parses bullet, and the `**Enforced by**` line reading *"review obligation today (candidate future import/grep guard)"*.
  No machine check is added by this task — the bullet already names a guard as a future candidate, and building one is scope the producers-standalone design did not ask for.

  Write the new bullet with semantic line breaks: one sentence per line, and a break at an internal independent-clause boundary inside a long sentence.
  Never hard-wrap at a fixed column.

  Do not rename `Validate`'s `worktreeRoot` parameter to match the new wording, and do not add a note about it to this file — the reason is recorded in `_mill/discussion.md` and in the overview's Shared Decisions, and the caller disagreement it describes is a separate follow-up.
- **Commit:** `docs(constraints): reword the planparser plan-path bullet as told-anchor`

### Card 14: extend `docs/overview.md`'s planparser entry

- **Context:**
  - `internal/planparser/doc.go`
  - `internal/planparser/parse.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The module-table entry for **planparser** currently names only grammar ownership — that it is the sole parser of the on-disk flat card-list plan format and that no other package reads that tree directly.
  Extend it to name path ownership alongside grammar ownership: `planparser` also declares where that tree *is*, in both the worktree-relative form (`PlanDirName`/`PlanDirRel`) and the absolute told-anchor form (`PlanDir`/`PlanOverview`), with the caller supplying the anchor path.

  Keep it to one added clause or one added line — this is a module-table row, not a place to restate the invariant.
  Keep the entry's existing `✅ Implemented.` marker and its plan-spec link intact, and keep the existing plan-directory reference.
  Use semantic line breaks for anything added.

  Change no other entry in the table.
  In particular, leave the **loom** entry alone: `loomengine` still composes the Planner producer's prompt and still owns the discussion paths;
  only the declaring package of the plan path moved, which is not a module-table-level fact for that row.
  Do not move `manifest/roadmap.md` — this is a planned decomposition item being executed, and the producers-standalone roadmap entries were already synced.
- **Commit:** `docs(overview): name planparser's plan-path ownership`

## Batch Tests

`verify:` runs two commands chained with `&&`.

The untagged half, `go test ./internal/planparser/... ./internal/loomengine/... ./internal/webstercli/... ./internal/websterengine/... ./cmd/lyx/...`, covers every package this batch edits.
It is scoped rather than repo-wide because the batch touches five packages and nothing outside them;
the task-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is what catches a regression in a package outside this scope.
`internal/websterengine` is in the list even though card 12 is comment-only, so the doc-comment edit is compiled rather than assumed.

The tagged half, `go test -tags integration ./internal/webstercli/...`, is not optional.
`internal/webstercli/verbs_test.go` carries `//go:build integration` on line 1, so neither `go test ./...` nor the untagged `go test ./internal/webstercli/...` compiles it — and three of this batch's edits live in that file alone: card 7's call-site repoint, card 7's `AnchorRel` flip, and card 8's new subpath-anchored `PersistentPreRunE` case.
A green untagged run proves nothing about any of them.

What each new or changed test proves:

- `internal/planparser/planpath_test.go` (batch 1) — the join arithmetic and the `PlanOverview`/`overviewFileName` agreement.
  Re-run here because card 11 deletes the `loomengine` predecessor and the ported successor is the only remaining coverage of the functions themselves.
- `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` (card 4) — the anchor-always proof on the `loomengine` side.
  It fails if `internal/loomengine/plan.go` is ever changed to pass `layout.WorktreePath()`.
- `TestPersistentPreRunE_PlanDirAnchoredAtSubpath` (card 8) — the anchor-always proof for `internal/webstercli/cli.go`'s own production call, driven through `Command()`'s real `PersistentPreRunE` at a `"backend"`-anchored hub.
- `newTestCLI`/`newVerbsFixture`'s `AnchorRel` flips (cards 6 and 7) — nested-anchor path handling only.
  Both fixtures compute `planDir` and are the same value the tests seed into, so a wrong-root slip stays self-consistent;
  both carry a comment saying so, and neither is counted as anchoring coverage.
- `cmd/lyx/constructoranchoring_test.go`'s four rewritten rows (card 9) — join arithmetic and `_lyx`-vs-`.lyx` group placement at both `AnchorRel` fixtures, now tautological with respect to anchoring and annotated as such.
- `cmd/lyx/notransients_test.go`'s two rewritten `durableSet` rows (card 10) — that no durable plan path resolves as a transient, at both `AnchorRel` fixtures.
  Weakened by the same rewrite as the anchoring rows and annotated the same way: the row now builds `l.AnchorPath()` itself rather than calling a constructor that anchored internally, so it too can no longer catch a wrong-root production call site.

Two non-test verifications belong to this batch and are grep obligations rather than assertions, both stated in the cards that own them: `internal/webstercli` must contain zero `loomengine` references after cards 5–7 (production and test), and `internal/loomengine/config.go` must contain zero `planparser.` references after card 11.
