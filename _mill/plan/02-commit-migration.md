# Batch: commit-migration

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
batch: commit-migration
number: 2
cards: 8
verify: go test -tags integration ./internal/fabricengine/ ./internal/buildercli/ ./internal/webstercli/ ./internal/perchcli/
depends-on: [1]
```

## Batch Scope

Migrate the three round-loop CLIs (builder, webster, perch) off `CommitWeft` + synchronous `PushWeftAt` onto the unified `Fabric.Commit` (async push), dropping every `:(exclude)` pathspec magic in the process and leaning on the weft's `.git/info/exclude` (deepened to reach perch's two-deep locks) to skip transients. Fix `Commit`'s hardcoded `classifyPaths(".", …)` to use `l.RelPath`. Once the three CLIs stop calling `PushWeftAt`, unexport it (only in-package callers `unwire`/`Bolt` remain). Revise the two now-false `CONSTRAINTS.md` Fabric-Git-Invariant exclusion bullets in the same batch. This batch consumes `Bolt` only indirectly (via the `pushWeftAt` rename touching `bolt.go`). The external interface the next batch consumes: after this batch no caller passes `:(exclude)` pathspec magic anywhere, which is the precondition for removing `git add -f`.

## Cards

### Card 6: Make Commit classify with l.RelPath

- **Context:**
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/fabric.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Fabric.Commit` (`commit.go:126`), change the `classifyPaths(".", wiredNames, files)` call at `commit.go:136` to `classifyPaths(l.RelPath, wiredNames, files)`, reusing the already-resolved `l` from `commit.go:127`. Update the neighbouring doc comment that references the "relpath-is-dot-for-slice-2" behaviour (`commit.go:83`) to state Commit classifies against the resolved `l.RelPath`. No behaviour change at root (`RelPath == "."` yields the same prefixes); the fix routes nested-worktree weft files correctly. Safe: `Fabric.Commit` has zero production callers (confirmed).
- **Commit:** `fix(fabric): classify Commit paths against l.RelPath not "."`

### Card 7: Deepen the weft exclude pattern for two-deep locks

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `crossModuleMachineLocalExcludes` (`weftgit.go:97-101`), deepen the lock pattern from `"**/" + hubgeometry.LyxDirName + "/*/*.lock"` (one module segment deep) to `"**/" + hubgeometry.LyxDirName + "/*/**/*.lock"` so it also matches perch's two-deep locks (`_lyx/perch/<block>/run.lock`). Leave the `pause` and `prompts/` entries unchanged. This pattern is seeded into the weft `.git/info/exclude` by `seedWeftArtifactExcludes` (`weftgit.go:116`, invoked from `ensureWeftLockDirAt:58`), which becomes the sole guardian of these transients once the per-call magic is gone. Verify the deepened glob does not over-broaden (it still requires a `_lyx/<module>/` prefix).
- **Commit:** `fix(fabric): deepen weft lock exclude to reach two-deep locks`

### Card 8: Migrate buildercli.weftCommit onto Fabric.Commit

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/commit.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/builderengine/pause.go`
  - `internal/websterengine/pause.go`
- **Edits:**
  - `internal/buildercli/weft.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/buildercli/weft.go`, rewrite `weftCommit(layout, label)` (`weft.go:116`) to call `Fabric.Commit` instead of `CommitWeft`+`PushWeftAt`. Delete the `builderWeftPathspec` helper (`weft.go:77`) and the `weftPathspecBase` helper if it becomes unused after this. Build the file list as `files := fabricengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName})` (positive only, NO `:(exclude)` entries). Keep the `if !opts.SkipGit { f, err := fabricengine.New(layout.WorktreeRoot, layout.WeftWorktree()); … }` guard (skipping `New` before its stat validation, exactly as today). Inside the guard call `res, err := f.Commit(files, fmt.Sprintf("builder: %s", label), nil, opts)` and set `committed = res.WeftCommitted`. Remove the standalone `fabricengine.PushWeftAt(weftWorktree, opts)` call entirely — `Commit` fires the async detached push itself. Remove now-unused imports (`builderengine`/`websterengine` pause-flag names, `path`). Because the file list is weft-only (`_lyx`), warp-side stays empty so `Commit`'s SkipGit semantics match the old skip-everything behaviour. Trim touched comments to the `golang-comments` shape.
- **Commit:** `refactor(builder): commit weft via Fabric.Commit, drop exclude magic`

### Card 9: Migrate webstercli.weftCommit onto Fabric.Commit

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/commit.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/builderengine/pause.go`
  - `internal/websterengine/pause.go`
  - `internal/buildercli/weft.go`
- **Edits:**
  - `internal/webstercli/weft.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror card 8 for `internal/webstercli/weft.go`: rewrite `weftCommit(layout, label)` (`weft.go:113`) onto `Fabric.Commit`, delete `websterWeftPathspec` (`weft.go:74`) and `weftPathspecBase` if now unused, build `files := fabricengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName})`, keep the `SkipGit`-before-`New` guard, call `res, err := f.Commit(files, fmt.Sprintf("webster: %s", label), nil, opts)`, set `committed = res.WeftCommitted`, drop the standalone `PushWeftAt`. Remove now-unused imports. Trim touched comments.
- **Commit:** `refactor(webster): commit weft via Fabric.Commit, drop exclude magic`

### Card 10: Migrate perchcli block-exit commit onto Fabric.Commit

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/commit.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/perchcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchcli/run.go`, rewrite the inline block-exit weft-commit (the block spanning `run.go:396-461`) onto `Fabric.Commit`. Replace the pathspec built at `run.go:409-425` — currently `append(fabricengine.ScopedPathspec(c.layout.RelPath, []string{hubgeometry.LyxDirName}), ":(exclude)*.lock")` — with the positive-only `files := fabricengine.ScopedPathspec(c.layout.RelPath, []string{hubgeometry.LyxDirName})` (drop the `:(exclude)*.lock` entry entirely). Keep the `if !opts.SkipGit { fab, weftErr = fabricengine.New(c.layout.WorktreeRoot, weftWorktree); … }` short-circuit. Inside, call `res, weftErr := fab.Commit(files, fmt.Sprintf("perch: %s %s", id, outcomeLabel), nil, opts)` and set `committed = res.WeftCommitted`. Remove the `fabricengine.PushWeftAt(weftWorktree, opts)` call (`run.go:445`). Perch's locks (`run.lock`, `state.json.lock`) are now excluded solely by the deepened `.git/info/exclude` pattern from card 7 — this also closes the tracked leading-`*` `:(exclude)*.lock` silent-no-op bug. Trim touched comments.
- **Commit:** `refactor(perch): commit weft via Fabric.Commit, drop unanchored exclude`

### Card 11: Unexport PushWeftAt

- **Context:**
  - `internal/fabricengine/bolt.go`
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/unwire.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the exported package-level `PushWeftAt` (`weftgit.go:544`) to `pushWeftAt`. After cards 8–10 (and batch 1's board/clone migration) its only remaining callers are in-package: `unwire.go:126` and `Bolt.Push` (`bolt.go`). Update both to the new casing. Confirm via grep that no caller outside the `fabricengine` package references `PushWeftAt` after this card. Update any doc-comment mentions to the new casing.
- **Commit:** `refactor(fabric): unexport pushWeftAt`

### Card 12: Revise CONSTRAINTS Fabric-Git-Invariant exclusion bullets

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`'s "Fabric Git Invariant (warp + weft)" section, revise the two now-false bullets. "Anchored exclusions" (the `:(exclude)`-anchoring rule and its live-caller enumeration naming buildercli/webstercli `weftCommit` + perch "still unanchored — carries this bug"): retire the anchoring failure-mode rule and the live-caller list — no caller passes `:(exclude)` pathspecs anymore. "Cross-module exclusions": drop the "live callers pass exclusions" enumeration and the perch caveat; keep the git-exclude-layer mechanism (now the SOLE guardian) and update its stated pattern to the deepened `**/_lyx/*/**/*.lock` form from card 7. Keep the "Known limitation" (`.git/info/exclude` does not untrack already-committed artifacts) text. One line per paragraph, no hard-wrap.
- **Commit:** `docs(constraints): retire pathspec-exclude rules, keep git-exclude guardian`

### Card 13: Exclusion + nested-RelPath regression coverage

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/classify.go`
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
- **Edits:**
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (a) In `commit_integration_test.go` add nested-RelPath coverage for the card-6 fix — a case at `RelPath` of two or more segments (e.g. `wts/some-task`) proving a `<RelPath>/_lyx/...` file routes to the weft side (existing tests only exercise `RelPath="."`). (b) In `buildercli/weft_integration_test.go` and `webstercli/weft_integration_test.go`, confirm `TestWeftCommit_CommitsAtEveryRelPathDepth` still asserts every transient (`*.lock`, builder/webster `pause`, `webster/prompts/*`) stays uncommitted at every depth — now via `.git/info/exclude` through `Fabric.Commit`, not pathspec magic; adjust the setup to the new `Commit` call path if the test drives `weftCommit` internals. Before relying on the exclude file, the tests must confirm each excluded path is genuinely never-tracked (an already-tracked file is not un-tracked by an exclude entry). (c) In `perchcli/run_integration_test.go`, extend `TestRunCLI_Run_WeftCommitExcludesLockFiles` (or add a sibling) to exercise a nested `RelPath` (≥2 deep) proving perch's two-deep `run.lock` is excluded by the deepened pattern — the regression guard perch lacked. Accept that push is now async (assert commit-side effects, not push completion). Add integration build tags matching each file's existing tag.
- **Commit:** `test(fabric): cover exclude-file transients and nested-RelPath commit`

## Batch Tests

`verify` runs fabricengine + the three CLI packages with `-tags integration`. The load-bearing tests are `commit_integration_test.go` (RelPath classification), the two `weft_integration_test.go` depth suites (transient exclusion via the exclude file after magic removal), and `perchcli/run_integration_test.go` (perch's newly-guarded two-deep lock exclusion). These prove the migration preserved the `committed` semantics and the never-commit transient set without any `:(exclude)` magic. Scope is exactly the four edited packages.
