# Batch: sabotage-proof-and-docs

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'sabotage-proof-and-docs'
number: 8
cards: 3
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./internal/lyxcwd/ -run 'TestEnforcement|TestEnforcement_MarkdownLinks'
depends-on: [7]
```

## Batch Scope

This batch closes the task: it proves nine named cells fail on demand, records that proof as a durable table in `doc.go`, and lands the documentation the Documentation Lifecycle requires in the same commit as the work it describes.
It is one batch because the sabotage exercise and its artifact are one deliverable — the temporary edits are gone the moment the exercise ends, so the table is the only evidence it happened — and because the doc updates all depend on numbers and outcomes that do not exist until it does.

Batch-local decision: the sabotage edits are **local working-tree changes, reverted immediately and never committed**.
`internal/fabricengine/destroy.go` and every other production file are untouched in the delivered diff.
`git status` must be clean of production-source modifications before the batch's commit, and the implementer verifies that explicitly rather than assuming it.

## Cards

### Card 20: sabotage-prove nine cells and record the table

- **Context:**
  - `internal/fabricengine/fabrictest/matrix_test.go`
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/states.go`
  - `internal/fabricengine/fabrictest/refusal.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Prove **exactly these nine cells**, keyed as (state, verb, anchor) triples, one per evidence-table defect and only those — proving a cell fails on demand costs a manual edit-run-revert cycle each, which is affordable nine times and not affordable 168 times:
  (1) `trackedSymlinkAtWiredPath` × `UnwireJunctions` × `.` — R1, retargeted from `Reconcile` because `Reconcile` has no path-executor gate call at all (batch 7's own omission findings), so the literal `Reconcile` cell cannot be run; `UnwireJunctions` destroyed a tracked symlink instead;
  (2) `dirtyWarpTracked` × `Pull` × `.` — R2, pull discarded uncommitted tracked warp work;
  (3) `dirtyWarpUntracked` × `Remove` × `.` — R3, remove destroyed the warp worktree past a git refusal;
  (4) `clean` × `Cleanup` × `.` — R3, cleanup deleted the primary weft branch;
  (5) `dirtyWarpUntracked` × `Prune` × `.` — R3, prune removed a path git had just refused;
  (6) `foreignDirAtFabricOwnedPath` × `Prune` × `.` — R4, prune removed an ordinary weft-suffixed user directory;
  (7) `unrelatedGitCloneAtWeftNamedPath` × `Prune` × `.` — R4, prune removed an unrelated git clone;
  (8) the `Reset` column's non-hub target × `CloneHub{Reset}` × `.` — R4, `clone --reset` destroyed a non-hub `<derived>-HUB`;
  (9) hostile input `..` × `Remove` × `.` — R5, `remove ..` destroyed an entire hub.
  **Mechanism.** For each, temporarily neuter the specific check that cell depends on — the pre-flight branch or the gate check, whichever the cell's expectation kind names — run **that one cell** with a `-run` filter, and confirm it **fails**.
  Revert the edit immediately;
  it appears in no commit.
  **Row 3 carries an extra requirement** and must not be signed off without it: neutering `Remove`'s pre-flight must make the cell fail on the **refusal half**, not merely on the manifest diff.
  Without `RefusedBefore`'s `"check failed"` exclusion the refusal half stays green — the gate refuses instead and a naive substring still matches — so the proof would rest entirely on the diff catching it incidentally, which it may not once the moved check runs earlier.
  Confirm explicitly that row 3's failure message names the refusal expectation.
  **Durable artifact.** Fill `doc.go`'s sabotage-proof section with a table carrying one row per proved scenario, recording the cell (state, verb, anchor), which check was neutered and where, and the observed failure — the assertion that fired and what it said.
  **Completion gate:** all nine rows present and populated.
  A row that could not be proved is recorded **as such with the reason**, never silently omitted.
  This is not optional and is not a permanent automated gate: proving it continuously would need a per-check injection seam slice 12 deliberately did not build, and adding one now would be production surface introduced by a test-only slice.
  Before committing, run `git status --porcelain` and confirm no production file under `internal/fabricengine/` or `internal/fabriccli/` is modified — the sabotage edits must all be gone.
- **Commit:** `fabrictest: record the nine-cell sabotage proof`

### Card 21: overview and design-doc updates

- **Context:**
  - `internal/fabricengine/fabrictest/doc.go`
  - `internal/fabriccli/clone.go`
  - `CONSTRAINTS.md`
  - `manifest/designs/fabric-windows-verification.md`
  - `manifest/designs/lyxtest-real-hubs.md`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/fabric-crucible-followups.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`'s `## Tests` section, name `internal/fabricengine/fabrictest` alongside `internal/boardengine/boardtest` and state what distinguishes it: `boardtest` holds cross-cutting suites only, while `fabrictest` is a hybrid — a non-test helper package (the hub factory, importable by `fabricengine_test`) that also holds its own integration-tagged suites, and it is the live-state harness that drives real cloned hubs in hostile state.
  In `manifest/designs/fabric-crucible-followups.md`, record **slice 13 as landed**, matching how that file already records slice 12: what shipped (`fabrictest`, the hub factory backed by the extracted `fabriccli.CloneAndWire`, the ten-state × nine-verb × two-anchor cross product with prefix-rooted manifest permits, the two refusal-expectation helpers and the nine-cell sabotage proof), what it deliberately left out (the deferred matrix axes — concurrency between worktrees, the hook surface, `_portals`/`_launchers` as a state axis — plus slice 14's truthfulness assertions, the 102 unconverted `CloneHub(` call sites, and the unclosed Windows verification gap), and that slice 14 is next.
  Keep both files' existing voice and semantic line breaks — one sentence per line, no fixed-column wrapping.
  Every inline markdown link added in either file must resolve, including its `#anchor` if it targets a `.md`;
  `internal/lyxcwd/docslink_test.go` scans both `docs/` and `manifest/` and will fail otherwise.
- **Commit:** `docs: record slice 13's live-state harness in the overview and design doc`

### Card 22: roadmap move

- **Context:**
  - `manifest/designs/fabric-crucible-followups.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move slice 13 from Planned to Done in `manifest/roadmap.md`, in the same shape the file already uses for slice 12's Done entry: one numbered item naming what landed and pointing at `designs/fabric-crucible-followups.md` for the full body.
  Update the Planned entry for the crucible follow-ups so slice 14 is named as next and slice 13 is no longer described as upcoming.
  Two cross-references in that file also need correcting now that `fabrictest` exists: the `lyxtest-real-hubs` entry says it "builds on the fabric campaign's slice 13 above, which creates the `fabrictest` package this needs as a landing zone" — reword so it refers to the landed package rather than a future one;
  and the Windows-verification entry says closing that gap "gets much cheaper once Planned slice 13's live-state harness exists" — reword to say the harness now exists and the gap is a run-and-fix exercise on a Windows host.
  This is a roadmap move for a completed planned item, which is exactly what the Documentation Lifecycle permits;
  do not move anything else.
  Every inline link must still resolve for `TestEnforcement_MarkdownLinks`.
- **Commit:** `docs: move slice 13 to Done on the roadmap`

## Batch Tests

`verify:` runs three commands.
`go build ./...` and `go test -tags integration ./internal/fabricengine/fabrictest/` prove the tree is clean of every sabotage edit — if any neutering survived, the matrix fails, which is the cheapest possible check that the working tree was really reverted.
`go test ./internal/lyxcwd/ -run 'TestEnforcement|TestEnforcement_MarkdownLinks'` covers the two guards this batch's doc edits can break: the fabric-vocabulary walk over `internal/**/*.md`, and markdown link integrity over `manifest/` and `docs/`.
The whole-tree `go test ./... && go test -tags integration ./...` runs once after this batch as the configured `pipeline.done_gate`, which is where a regression in a package no batch-verify scope covered would surface.
