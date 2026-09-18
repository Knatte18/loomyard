# Batch: vocabulary-and-config

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "vocabulary-and-config"
number: 2
cards: 8
verify: go test ./internal/tokenvocab/ ./internal/reedengine/ ./internal/hubgeom/ ./internal/standalonegeom/ ./internal/configsync/
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch delivers the text-and-configuration half of the task: the `worktree` token joins `internal/tokenvocab`'s registry, `reedengine.Geometry` gains the `WorktreeName` field that feeds it, both geometry tellers fill it, `reed.yaml`'s `header:` block is replaced by `status_line:` and `selvage:`, and the rendering pipeline behind it is renamed from Header to StatusLine (`Engine.StatusLineText`, `Engine.ValidateStatusLine`, the embedded `status-line.md` asset).
It is one batch because every one of those changes is a link in a single chain — a token needs a `Ctx` field, which needs a `Geometry` field, which needs both tellers, which the default template then names, which the renamed `StatusLineText` renders from the renamed config block — and splitting it anywhere leaves the tree uncompilable.

The external interface batches 3, 4 and 5 consume: `Config.StatusLine.Template`, `Config.Selvage.HeightRows`, `Engine.StatusLineText() (string, error)`, `Engine.ValidateStatusLine() error`, `StatusLineTemplate() []byte`, and `Geometry.WorktreeName`.

Batch-local decisions beyond `## Shared Decisions`: `standalonegeom.ReedGeometry` fills `WorktreeName` from the **raw** `filepath.Base(target)` — the identical expression that file already uses for `RepoName` — not from the normalized spelling `SessionName`'s readable half takes, so `{{.repo}}` and `{{.worktree}}` render byte-identical strings in standalone mode even for a symlinked spelling.
No removal logic is written for a stale `header:` block in an existing `reed.yaml`: `yamlengine.Reconcile` already strips leaves the template no longer declares, and an un-reconciled file carrying `header:` alongside the new blocks is inert because nothing unmarshals it into `Config` any more.
`ReedState.HeaderPaneID` is **not** renamed here — that is batch 3.

## Cards

### Card 7: add the worktree token to tokenvocab

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/doc.go`
  - `internal/tokenvocab/render.go`
  - `internal/tokenvocab/tokenvocab_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `WorktreeName string` field to `tokenvocab.Ctx` in `internal/tokenvocab/tokenvocab.go`, with a doc comment reading that it feeds the "worktree" token, placed after the existing `HubPath` field. Append one entry to the unexported `registry` slice: `{Name: "worktree", Resolve: func(c Ctx) string { return c.WorktreeName }}`. Change nothing else in that file — `Build` iterates the registry and picks the new token up automatically. In `internal/tokenvocab/doc.go`, change the package-doc phrase "today reed's header text pipeline, later loom's prompt templates" so it names reed's status-line pipeline instead, and change "the token registry (currently \"repo\" and \"hub\", both plain fields on Ctx)" so it names all three tokens. In `internal/tokenvocab/render.go`, change the file's leading comment phrase "reed's header pipeline" to name reed's status-line pipeline. The Tokenvocab Leaf Invariant is unaffected and must stay so: `WorktreeName` is a plain told string field and must never resolve a path, and no import is added to either file. In `internal/tokenvocab/tokenvocab_test.go`, add assertions that the `worktree` token resolves from `Ctx.WorktreeName` and that `Build` returns all three token names, and update any existing test that asserts the registry's exact length or key set.
- **Commit:** `feat(tokenvocab): add the worktree token to the registry`

### Card 8: add WorktreeName to reedengine.Geometry

- **Context:**
  - `internal/tokenvocab/tokenvocab.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/geometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `WorktreeName string` field to `reedengine.Geometry` in `internal/reedengine/geometry.go`, placed after `RepoName` and before `HubPath`, documented as the status-line's "worktree" token passed through `internal/tokenvocab`. Correct the file's own leading comment, which today reads "geometry.go declares Geometry, the eight-field struct reed is told its coordinates through" — it must say nine-field. Correct the `RepoName` and `HubPath` field comments, which today read "the header pane's \"repo\" token" and "the header pane's \"hub\" token" — both must name the status-line instead. The Told-Geometry Invariant is unaffected: this file still declares the type only, adds no constructor, no validator and no default, and `reedengine` still must not import `internal/lyxcwd`.
- **Commit:** `feat(reedengine): add WorktreeName to the told Geometry`

### Card 9: fill WorktreeName in both geometry tellers

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/hubgeom_test.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/standalonegeom_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/hubgeom/hubgeom.go`'s `ReedGeometry`, add `WorktreeName: l.WorktreeName` to the returned `reedengine.Geometry` literal, reading the field `*lyxcwd.Location` already exposes. In `internal/standalonegeom/reedgeom.go`'s `ReedGeometry`, add `WorktreeName: filepath.Base(target)` — the **raw** told spelling, the identical expression the same function already uses one line away for `RepoName`, and deliberately not `standalonestate.Normalize(target)`'s basename nor `readableName`. Extend that function's doc comment with the standalone disposition stated explicitly: standalone mode has no worktree, so `{{.worktree}}` and `{{.repo}}` render the same string byte for byte and the default template reads `foo/foo · <stateDir>`; taking the raw spelling for both is what makes that exact rather than approximate, since normalizing only the new token would make a symlinked target render two different names on one line, and both tokens are display values that never reach a tmux target. In the same file, change the existing `RepoName` comment phrase "it is the header pane's display token" to name the status-line. In `internal/hubgeom/hubgeom_test.go`'s `TestReedGeometry`, assert `WorktreeName` equals the resolved `Location`'s own `WorktreeName`. In `internal/standalonegeom/standalonegeom_test.go`, update the assertion at the site that today quotes "the header pane's display token" to name the status-line, and add an assertion that `WorktreeName` and `RepoName` are byte-identical for both a symlinked and a real spelling of one target — which is what pins the "same string, not merely the same directory" claim.
- **Commit:** `feat(hubgeom,standalonegeom): fill Geometry.WorktreeName in both tellers`

### Card 10: replace the header config block with status_line and selvage

- **Context:**
  - `internal/reedengine/template.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/config.go`
  - `internal/reedengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/config.go`, delete the `HeaderConfig` type and the `Header HeaderConfig \`yaml:"header"\`` field on `Config`. Add two types in their place: `StatusLineConfig` with a single field `Template string \`yaml:"template"\``, and `SelvageConfig` with a single field `HeightRows int \`yaml:"height_rows"\``. Add the two corresponding `Config` fields, `StatusLine StatusLineConfig \`yaml:"status_line"\`` and `Selvage SelvageConfig \`yaml:"selvage"\``, in that order where `Header` used to sit. Document `StatusLineConfig` as configuring the tmux status-line's rendered text and `SelvageConfig` as configuring the Selvage pane's fixed row budget, and state in one of the two comments that these describe two different mechanisms — text in a tmux option, and a row budget for a pane — which is why one block holding both would be misleading. `LoadConfig` is unchanged and stays on `configengine.LoadOrTemplate`, keeping `reedengine` on the degrading side of the Config Strictness Invariant. In `internal/reedengine/config_test.go`, replace every `cfg.Header` assertion with `cfg.StatusLine`/`cfg.Selvage` equivalents, and add a case pinning that an **un-reconciled** `reed.yaml` still carrying a stale `header:` block alongside `status_line:` and `selvage:` unmarshals cleanly with the stale block ignored — which is what makes the no-removal-logic decision safe.
- **Commit:** `feat(reedengine): replace the header config block with status_line and selvage`

### Card 11: update both reed.yaml templates

- **Context:**
  - `internal/reedengine/config.go`
  - `internal/reedengine/template.go`
  - `internal/reedengine/watchdog.go`
- **Edits:**
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/template_windows.yaml`
  - `internal/reedengine/template.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In both `internal/reedengine/template_posix.yaml` and `internal/reedengine/template_windows.yaml`, replace the three-line `header:` block (the `header:` key plus its `template:` and `height_rows:` children) with two blocks: `status_line:` carrying a single `template: ""` child commented as "empty means \"use the embedded default template\"; set to override", and `selvage:` carrying a single `height_rows: 1` child commented as the fixed row count the always-on Selvage pane occupies at the bottom of the window. Give `status_line:` its own trailing comment naming it the tmux status-line's rendered text. Also rewrite the `watchdog:` key's inline comment in both files: it today reads "header pane resize self-heal" and "takes effect on the next header-pane rebuild only — a server restart, a dead-header heal, or \"lyx reed down\" + \"up\"", neither of which survives this task. It must instead describe the per-worktree resize self-heal gate (the watch loop plus the session's `window-resized` hook) and say that the detached per-hub watchdog daemon re-reads a worktree's value when that worktree's session re-enters the watched set, so a `down` + `up` is what makes a flipped value take effect. Keep both files' `${env:LYX_REED_WATCHDOG:-on}` default and every other key byte-identical. In `internal/reedengine/template.go`, correct `ConfigTemplate`'s doc comment, which today reads "The layout-tuning keys (width, height, collapsed_strip_rows, min_full_rows, strand_name) and the header block are plain literals" — it must name the `status_line` and `selvage` blocks instead.
- **Commit:** `feat(reedengine): replace the header block in both reed.yaml templates`

### Card 12: rename the header text pipeline to the status-line

- **Context:**
  - `internal/reedengine/config.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/render.go`
  - `internal/reedengine/geometry.go`
- **Edits:**
  - `internal/reedengine/statusline.go`
  - `internal/reedengine/statuslinetemplate.go`
  - `internal/reedengine/status-line.md`
  - `.gitattributes`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/reedengine/header.go` -> `internal/reedengine/statusline.go`
  - `internal/reedengine/headertemplate.go` -> `internal/reedengine/statuslinetemplate.go`
  - `internal/reedengine/console-header.md` -> `internal/reedengine/status-line.md`
- **Requirements:** After the three `git mv` calls, make only surgical edits. In `internal/reedengine/statusline.go`: rename `Engine.HeaderText` to `Engine.StatusLineText` and `Engine.ValidateHeader` to `Engine.ValidateStatusLine`; read `e.cfg.StatusLine.Template` in place of `e.cfg.Header.Template`; call `StatusLineTemplate()` in place of `HeaderTemplate()`; and extend the `tokenvocab.Ctx` literal to `tokenvocab.Ctx{RepoName: e.geom.RepoName, HubPath: e.geom.HubPath, WorktreeName: e.geom.WorktreeName}`. Rewrite the file's leading comment to describe the status-line's text-rendering pipeline over `internal/tokenvocab` and the eager pre-tmux boot gate, and state that `ValidateStatusLine` is now more load-bearing rather than less, since a template that fails to render would otherwise reach `set-option status-left` where every failure is non-fatal and merely logged. In `internal/reedengine/statuslinetemplate.go`: rename `HeaderTemplate` to `StatusLineTemplate`, change the `//go:embed` directive to `status-line.md`, rename the package-level var `headerTemplate` to `statusLineTemplate`, and rewrite the file comment to name the status-line asset while keeping the "deliberately outside the stencil mechanism" paragraph. In `internal/reedengine/status-line.md`: change the banner comment to describe the default status-line text template, list all three available tokens (`{{.repo}}`, `{{.worktree}}`, `{{.hub}}`), name a `Config.StatusLine.Template` override rather than `Config.Header.Template`, and change the rendered body from `hub: {{.hub}}` to `{{.repo}}/{{.worktree}} · {{.hub}}` per the discussion's `worktree-token-joins-the-vocabulary` decision. In `.gitattributes`, change the `internal/reedengine/console-header.md text eol=lf` line to name `internal/reedengine/status-line.md`.
- **Commit:** `refactor(reedengine): rename the header text pipeline to the status-line`

### Card 13: retarget the engine's remaining config and validate call sites

- **Context:**
  - `internal/reedengine/statusline.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/lifecycle.go`, change the pre-tmux boot gate's call from `e.ValidateHeader()` to `e.ValidateStatusLine()`, and rewrite the comment block directly above it — today it opens "Validate the header template in the same pre-tmux block — it reads only cfg+geometry (HeaderText makes no tmux round trip)" — so it names the status-line template and `StatusLineText`, keeping the whole half-created-session and lost-rebirth-signal rationale that follows it verbatim. Also correct the file comment at the top of the same function's declaration block, which lists what the pre-tmux block validates and today ends "mouse, watchdog, and header template" — it must say status-line template. In `internal/reedengine/apply.go`'s `toRenderInputs`, change `HeightRows: e.cfg.Header.HeightRows` to `HeightRows: e.cfg.Selvage.HeightRows`. Do not rename `st.HeaderPaneID` or the local `headerPaneID` in this card — both are batch 3's.
- **Commit:** `refactor(reedengine): point the boot gate and layout params at the new config`

### Card 14: rename the engine's text-pipeline test and pin the reconcile removal

- **Context:**
  - `internal/reedengine/statusline.go`
  - `internal/reedengine/config.go`
  - `internal/configsync/configsync.go`
  - `internal/reedengine/template_posix.yaml`
- **Edits:**
  - `internal/reedengine/statusline_test.go`
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/reedengine/header_test.go` -> `internal/reedengine/statusline_test.go`
- **Requirements:** After `git mv`, edit `internal/reedengine/statusline_test.go` surgically: retarget every call from `HeaderText`/`ValidateHeader` to `StatusLineText`/`ValidateStatusLine`, every `Config{Header: ...}` construction to `Config{StatusLine: ...}`, and every `HeaderTemplate()` reference to `StatusLineTemplate()`. Rename each test function so it names the status-line rather than the header. Add a case asserting the rendered default template carries all three token values — a `Geometry` with distinct `RepoName`, `WorktreeName` and `HubPath` must render a string containing each of the three. In `internal/configsync/configsync_test.go`, add a new test beside `TestReconcileAll_DropsStaleReedClaudeKey` and modelled on it: seed a `reed.yaml` at `configengine.ConfigFile(tmpDir, "reed")` carrying `header.template` and `header.height_rows`, call `ReconcileAll(tmpDir, true)`, take the `reed` entry out of the results, and assert that `Applied` is true, that `Removed` contains both `header.template` and `header.height_rows` under whatever leaf-key spelling `yamlengine.Reconcile` reports (read `TestReconcileAll_DropsStaleReedClaudeKey`'s own assertion for the exact spelling convention and follow it), that `Added` contains the new `status_line`/`selvage` leaves, and that the merged file on disk no longer carries a `header:` block. This is the test that pins the migration path an operator actually gets from `lyx config reconcile --apply`.
- **Commit:** `test(reedengine,configsync): pin the status-line pipeline and the stale header-key removal`

## Batch Tests

`verify: go test ./internal/tokenvocab/ ./internal/reedengine/ ./internal/hubgeom/ ./internal/standalonegeom/ ./internal/configsync/` names exactly the five packages this batch edits production code in, and each one carries the assertions for its own half: `tokenvocab_test.go` for the third token and the still-passing leaf-enforcement test, `statusline_test.go`/`config_test.go` for the renamed pipeline and the two new config blocks (including the un-reconciled stale-`header:` case), `hubgeom_test.go`/`standalonegeom_test.go` for both tellers filling `WorktreeName` and for standalone's byte-identical `repo`/`worktree` pair, and `configsync_test.go` for the reconcile-removal migration path.
Every one of these is untagged and spawns nothing — `configsync_test.go` writes only into `t.TempDir()`, matching `TestReconcileAll_DropsStaleReedClaudeKey`'s existing shape.
The overview's module-wide `verify: go build ./...` is what catches any package outside these five that still names `Config.Header`, `HeaderText`, `ValidateHeader` or `HeaderTemplate` after this batch.
