# Batch: shuttle-told-strings

```yaml
task: "shuttleengine + reedengine + tokenvocab told-geometry"
batch: "shuttle-told-strings"
number: 4
cards: 2
verify: go test ./internal/shuttleengine/... ./internal/websterengine/... ./internal/shuttlecli/... ./internal/webstercli/... ./internal/burlercli/... ./internal/perchcli/... ./internal/tokenvocab/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/... && go vet -tags smoke ./internal/shuttlecli/... ./internal/treadleengine/... ./internal/burlerengine/...
depends-on: [3]
```

## Batch Scope

This batch converts `internal/shuttleengine` to told strings: `NewRunner` and `runDirRoot` take `anchorPath` and `worktreeRoot`, `FindRun` takes `anchorPath`, and `internal/lyxcwd` leaves the package's production imports.
It is one batch, and card 11 is one card, because `NewRunner` and `FindRun` are exported and the no-additive-twins rule means every caller — four CLI construction sites, two `websterengine` production files, and seven test files across five packages — changes in the same commit as the signature.
It depends on batch 3 because the four CLI sites source their two arguments from the `reedengine.Geometry` local `hubgeom.ReedGeometry(layout)` already binds on the adjacent line.

Card 12 is the task's terminal structural gate: it verifies by inspection that the three import-graph facts the whole task exists to deliver actually hold.

Batch-local decisions beyond `## Shared Decisions`:

- No `shuttleengine.Geometry` struct and no `hubgeom.ShuttleGeometry`. Two values do not warrant a struct;
  what keeps the anchor/worktree pairing decided in one place is that both are read off the `Geometry` the adjacent `reedengine.New` call already uses, not a second struct.
- Shuttle test fixtures give `anchorPath` and `worktreeRoot` distinct values rather than the same temp dir twice, for the same swap-detection reason as reed's fixtures in batch 3.

## Cards

### Card 11: Convert `shuttleengine` to told strings, and every caller with it

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/reedengine/geometry.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/doc.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/run_inject_test.go`
  - `internal/shuttleengine/wait_test.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/burlercli/cli.go`
  - `internal/perchcli/cli.go`
  - `internal/webstercli/cli.go`
  - `internal/shuttlecli/cli.go`
  - `internal/shuttlecli/cli_test.go`
  - `internal/shuttlecli/smoke_interrupt_test.go`
  - `internal/treadleengine/smoke_judge_test.go`
  - `internal/burlerengine/smoke_cluster_test.go`
  - `internal/burlerengine/smoke_round_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shuttleengine/run.go`, replace `Runner`'s `layout *lyxcwd.Location` field with two string fields, `anchorPath` and `worktreeRoot`, and change the constructor to `NewRunner(reed ReedOps, engine Engine, anchorPath, worktreeRoot string, cfg Config) *Runner`.
  Update `NewRunner`'s doc comment and the `Runner` type's own doc comment, which both describe the runner as scoped to a layout, to state that the runner is told its two directories and derives neither, and that populating both with usable absolute paths is the caller's obligation.
  Inside `Start`, `spec.validate(r.layout.WorktreePath(), r.cfg)` becomes `spec.validate(r.worktreeRoot, r.cfg)`, the `reedengine.LoadState` join reads `r.anchorPath`, and every `runDirRoot(r.cfg, r.layout)` and `FindRun(r.cfg, r.layout, guid)` call passes `r.anchorPath`.
  Drop the `github.com/Knatte18/loomyard/internal/lyxcwd` import from the file.
  Every `logger.Info` and `logger.Warn` call in `run.go` survives unchanged in intent — same event, same level, same key/value fields.
  In `internal/shuttleengine/rundir.go`, change `runDirRoot(cfg Config, anchorPath string) string` and `FindRun(cfg Config, anchorPath, guid string) (RunState, string, error)`, replacing every `layout.AnchorPath()` read with the parameter.
  Keep both `runDirRoot` branches joining onto the same `anchorPath` base, and keep the doc comment's explanation of why one function must never resolve against two bases when the repo is subpath-anchored — reword it to name the parameter rather than `layout.AnchorPath()`.
  Update the file's header comment, which says the run-dir root is resolved "from Config/lyxcwd", and drop the `lyxcwd` import.
  In `internal/shuttleengine/wait.go`, the `AuditForks` call's second argument becomes `run.runner.anchorPath`.
  In `internal/shuttleengine/doc.go`, add a short paragraph stating that shuttle is told its anchor path and worktree root as plain strings and derives neither, and that `internal/lyxcwd` is consequently absent from the package's production imports.
  In the shuttle test files, replace every `*lyxcwd.Location` fixture with two distinct string locals so a swapped pair fails instead of passing: `newTestRunner` in `run_test.go` returns the runner plus whatever the callers need to recompute `runDirRoot`, `rundir_test.go`'s three `runDirRoot` cases pass an anchor path directly while keeping their present expected values (the default `.lyx/shuttle` branch, the relative-`RunDir` branch, and the absolute-`RunDir` branch), `run_inject_test.go` seeds its run under the same anchor-derived root it passes to `NewRunner`, and `wait_test.go`'s `AuditForks` assertion compares against the runner's told anchor path.
  Keep `rundir_test.go`'s subpath-anchored default-branch case genuinely subpath-anchored: pass an anchor path that is a real subpath of a worktree root, so the assertion still proves the root is anchor-derived and not worktree-derived.
  Drop the `lyxcwd` import from every shuttle test file that no longer needs it.
  Do not change `internal/shuttleengine/seam_enforcement_test.go`.
  In `internal/websterengine/recoverbatch.go` and `internal/websterengine/runlevel.go`, change each `shuttleengine.FindRun(deps.ShuttleCfg, deps.Layout, …)` call's second argument to `deps.Layout.AnchorPath()`.
  These are one-token edits: do not convert `RunDeps.Layout` itself, and do not touch anything else in either file — T7 rewrites both wholesale and must land after this task.
  At the four CLI construction sites — `internal/burlercli/cli.go`, `internal/perchcli/cli.go`, `internal/webstercli/cli.go`, `internal/shuttlecli/cli.go` — pass the `AnchorPath` and `WorktreeRoot` fields of the `reedengine.Geometry` local batch 3 already binds from `hubgeom.ReedGeometry(layout)`, in that order, to `shuttleengine.NewRunner`.
  Do not write `layout.AnchorPath(), layout.WorktreePath()` inline at these sites: that reintroduces the swap hazard beside a struct that already holds the decided pair.
  Leave every `LoadConfig(layout.AnchorPath(), …)` call and every other `layout` use at these sites untouched.
  In the seven test call sites — `internal/shuttlecli/cli_test.go`, `internal/shuttlecli/smoke_interrupt_test.go`, `internal/webstercli/verbs_test.go`, `internal/websterengine/recoverbatch_test.go`, `internal/treadleengine/smoke_judge_test.go`, `internal/burlerengine/smoke_cluster_test.go`, `internal/burlerengine/smoke_round_test.go` — pass the two strings from whatever the fixture already has: the `hubgeom.ReedGeometry(...)` local in the four smoke tests, and `layout.AnchorPath(), layout.WorktreePath()` in the three that build a `Location` for their own other purposes and hold no `Geometry`.
  Leave each fixture's remaining `layout` uses (`seedHubStencils`, `loomengine.PlanDir`, and the like) exactly as they are.
- **Commit:** `refactor(shuttleengine): take told anchorPath and worktreeRoot instead of a lyxcwd.Location`

### Card 12: Confirm the import-graph facts the task promised

- **Context:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/render.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/header.go`
  - `internal/reedengine/geometry.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/wait.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Confirm by inspection, without editing anything, that the three structural facts the discussion lists as part of done actually hold.
  First: `internal/lyxcwd` appears in no non-test `.go` file of `internal/tokenvocab`, `internal/reedengine` or `internal/shuttleengine`.
  Second: `internal/fabricengine` appears in no non-test `.go` file of `internal/reedengine`.
  Third: `internal/tokenvocab/leaf_enforcement_test.go`'s `allowedImports` map contains exactly one entry, `internal/stencil`, and was tightened rather than left permissive.
  A grep across the three package directories for `loomyard/internal/lyxcwd` and `loomyard/internal/fabricengine`, excluding `_test.go` files, is sufficient evidence for the first two.
  If any of the three does not hold, that is a defect in card 4, 8 or 11 — report it rather than fixing it here, since fixing it belongs in the card that introduced it.
- **Commit:** none

## Batch Tests

`verify:` runs the untagged suites of `internal/shuttleengine`, `internal/websterengine`, the four CLI packages that construct a runner, and `internal/tokenvocab`; then the `//go:build integration` tier of `internal/websterengine` and `internal/webstercli`, which is where `recoverbatch_test.go` and `verbs_test.go` live; then a `go vet -tags smoke` type-check of the three smoke packages this batch edits.
Both integration files drive fake reed and fake engine implementations rather than real tmux, so running them is cheap and is the right tier for a `NewRunner` call-site change.

- `internal/shuttleengine/rundir_test.go` — the three `runDirRoot` branches (empty `RunDir`, absolute `RunDir`, relative `RunDir`) must all keep asserting that the base is the anchor path, now passed directly.
  The default-branch case stays subpath-anchored, which is what proves the base is anchor-derived rather than worktree-derived.
- `internal/shuttleengine/run_test.go`, `run_inject_test.go`, `wait_test.go` — the runner fixtures must carry distinct anchor and worktree values, so a swapped `NewRunner` argument pair fails these rather than passing.
  `wait_test.go`'s `AuditForks` case is the one that pins the anchor path specifically.
- `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) is unaffected and must stay green unchanged.
- `internal/websterengine/...` — the two `FindRun` call sites are one-token edits; the package's untagged suite plus `recoverbatch_test.go`'s converted `NewRunner` call are the regression gate.
- `internal/tokenvocab/...` is re-run here as the cheapest available check on card 12's third structural fact, since `TestLeafInvariant_AllowlistOnly` is that fact's machine enforcer.
- The three smoke packages are compile-checked rather than run, per the shared decision on tagged tiers; the only breakage card 11 can introduce there is a `NewRunner` call-site mismatch, which `go vet -tags smoke` catches.
- The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) runs after this final batch and is what covers the packages no batch verify scopes — `cmd/lyx`, `internal/fabricengine`, `internal/reedengine` and the rest — against the cumulative effect of all four batches.
