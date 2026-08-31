# Batch: status-merge-in-progress

```yaml
task: "Surface merge-in-progress in fabric status"
batch: "status-merge-in-progress"
number: 1
cards: 2
verify: go test ./internal/fabriccli/... && go test -tags integration ./internal/fabriccli/...
depends-on: []
```

## Batch Scope

This batch delivers the whole task: `lyx fabric status`'s success envelope gains one always-present boolean key, `merge_in_progress`, sourced from `fabricengine.Fabric.MergeInProgress()`, plus the integration test that pins it and every doc artefact that describes what `status` reports.
It is one batch because the change is a single edit inside one existing `RunE`, one new test file, and five prose edits that all share the same `Context:` set — splitting it would produce batches sharing well over 80% of their context.
There is no external interface for a next batch to consume;
this batch is the whole DAG.
No batch-local decisions differ from `## Shared Decisions` in `00-overview.md`.

Card ordering is TDD: card 1 writes the failing test, card 2 makes it pass and lands the docs in the same commit.
Card 1's commit is expected to be red on its own — that is the point of writing it first — and card 2 is what returns the batch to green before `verify:` runs.

## Cards

### Card 1: Integration test pinning `merge_in_progress` on the `status` envelope

- **Context:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/merge_cli_integration_test.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabriccli/status_mergeinprogress_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the new file in package `fabriccli_test` with `//go:build integration` as line 1, followed by a blank line and a file-header comment in the style of `internal/fabriccli/merge_cli_integration_test.go`'s own header, stating that the file pins the `merge_in_progress` key on `lyx fabric status`'s success envelope across the no-merge and parked-merge cases, and that the key answers the this-pair question only.
  The tag is mandatory: the test calls `hubforge.NewHub`, which `CONSTRAINTS.md`'s Test Tier Purity Invariant bars from untagged files.
  Do not add a `TestMain` — `internal/fabriccli/testmain_test.go` already provides the package's hermetic-git `TestMain`.
  Write two top-level functions, matching this package's one-`TestRunCLI_*`-per-behavior style;
  they build separate hubs and share no expensive setup, so no `t.Run` subtests are needed.
  First, `TestRunCLI_StatusReportsNoMergeInProgressOnACleanPair`: build the hub with `hubforge.NewHub(t, ".")` then `hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")`, matching the read-only-verb exemplar `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` in `internal/fabriccli/cli_test.go` that this scenario sits closest to.
  Drive `fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})`, require exit code 0, and decode with `decodeResult(t, &out)`.
  Assert three separate things: that `merge_in_progress` is **present** in the decoded map, reported as a missing key rather than as false when absent;
  that its value is `false`;
  and that the pre-existing `changes` key is still present, so a regression that drops or renames `changes` while adding the new key is caught here rather than by a distant test.
  Second, `TestRunCLI_StatusReportsMergeInProgressWhileAMergeIsParked`: build the hub with `hubforge.NewHub(t, ".")` alone and do **not** call `hubforge.SeedFabricConfig`, matching the merge exemplars whose flow it reuses.
  Park a conflicted merge with the established sequence from `TestRunCLI_MergeStageRejectsAPathThatIsNotConflicted` in `internal/fabriccli/merge_cli_integration_test.go`: `setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")`, then `branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")`, then `fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"})`, requiring exit code 1 (a conflict envelope, not a failure to re-run differently).
  Then drive `status` from `h.PrimeWorktree()`, require exit code 0, and assert `merge_in_progress` is present and `true` — the assertion that proves the field reads the real record rather than a hardcoded constant.
  Both functions must use the comma-ok decode form the package already uses for booleans, plus a **separate** presence check, because `decodeResult` returns `map[string]any` and a bare type assertion on a missing key panics instead of failing cleanly;
  follow `internal/fabriccli/cli_test.go`'s existing `if ok, _ := envelope["ok"].(bool); !ok` and `if _, present := result["mutations"]; present` shapes.
  Do not call `t.Parallel` in either function — the hub-driving tests in this package do not.
  Do not modify `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` or any other existing test in this package.
  Do not add an error-path scenario for `MergeInProgress()` returning a non-nil error, and do not add an engine seam or test hook to make one reachable;
  that branch is recorded as intentionally untested in `00-overview.md`.
  This card is expected to fail the batch's `verify:` on its own — the key does not exist yet.
- **Commit:** `test(fabriccli): pin merge_in_progress on the fabric status envelope`

### Card 2: Emit `merge_in_progress` from `statusCmd` and land every doc artefact

- **Context:**
  - `internal/fabriccli/envelope.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergestate.go`
  - `internal/fabriccli/status_mergeinprogress_integration_test.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/doc.go`
  - `docs/overview.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabriccli/weft_verbs.go`, change only `statusCmd` — its `RunE` body and its `Long` string.
  Leave `Use`, `Args` (`cobra.NoArgs`), and `Short` exactly as they are, so the help-tree and args-arity tests need no edit.
  In `RunE`, after the existing `fab.Status()` call and its `if err != nil` block, and before the `output.Ok` call, add a second probe: `inProgress, err := fab.MergeInProgress()`, guarded by an `if err != nil` block byte-for-byte the same shape as the `fab.Status()` one immediately above it — `clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))` then `return nil`.
  Probe ordering is load-bearing and must be `fab.Status()` first: it preserves today's error precedence, so a broken pair keeps reporting the `Status()` error it reports today rather than a new merge-state error.
  Then extend the existing `output.Ok` map literal to carry both keys — `"changes"` with its existing `changeEntriesMap(entries)` value, and `"merge_in_progress"` with `inProgress`.
  The key is emitted unconditionally, `false` when no merge is parked;
  do not make it conditional on the value.
  Do not route this through `okWithRecord`;
  `status` stays read-only and must gain neither a `mutations` nor a `partial` key.
  Add no new import — `fab` is the package-local closure already resolved by `addWeftVerbs`'s `PersistentPreRunE`, and `status` is in `weftVerbNames`, so the handle is always non-nil by the time `RunE` runs.
  Add no engine code;
  `fabricengine.Fabric.MergeInProgress()` is used exactly as it ships.
  Next, extend `statusCmd`'s `Long` string with a second paragraph, separated from the existing one by a blank line inside the same raw string literal.
  The new paragraph must say that `status` also reports `merge_in_progress`, that it reports whether **this** pair has a fabric merge parked awaiting `lyx fabric merge --continue` or `lyx fabric merge --abort`, and — explicitly — that it does **not** report whether some other pair in the hub is mid-merge on this pair's branch, so a `lyx fabric remove` refusing on that hub-wide condition can coexist with `merge_in_progress` being `false` here.
  The this-pair qualification is mandatory;
  an unqualified "a merge is in progress" wording would promise the field predicts every `ErrMergeInProgress` refusal, and it does not.
  Keep the paragraph to roughly three sentences and keep the existing first paragraph's wording untouched.
  Next, in `internal/fabricengine/doc.go`, edit the one sentence in the merge section that today asserts an operator who staged some but not all reported paths "cannot re-run `merge-in` to reprint the list (a merge is already in progress), `lyx fabric status` reports a remaining weft-side conflict as an ordinary weft change indistinguishable from any other, and plain `git status` in the visible worktree does not see it at all".
  That claim stays true and must survive intact — the field is pair-level, so `status` still names no conflicted path.
  Add a short clause noting that `status` does now report the parked merge itself, through its `merge_in_progress` key, while still not distinguishing which weft path is conflicted.
  The surrounding argument for why `merge-stage` must exist is unchanged and must read exactly as forcefully after the edit;
  do not weaken or restructure it, and do not touch the two other `status` claims elsewhere in this file — the junction-drift one and the "one side-labelled view" one describing the engine's `Status()` return shape — both of which this task leaves true.
  Next, in `docs/overview.md`, extend in place the Status-section bullet sentence reading "`status` is the unified both-sides uncommitted-change view." so it also names the new field and its this-pair sense.
  Append to that one sentence only.
  Do not reflow the bullet to semantic line breaks: it is today a single physical line carrying four sentences, and reflowing it would turn a one-sentence content change into a whole-line rewrite whose diff hides the actual edit.
  Reflowing that bullet is a separate mechanical pass belonging to its own change.
  Do not touch the friction-asymmetry mention of `lyx fabric status` earlier in the same file, which makes no claim about output shape.
  Next, in `tools/sandbox/SANDBOX-FABRIC-SUITE.md`, edit two `Watch` blocks.
  In F3's `Watch`, which enumerates status's output shape, add the `merge_in_progress` field to the enumeration and note it is `false` in F3's own no-merge scenario.
  Leave F3's `Goal` line alone — it names which verbs the scenario exercises and makes no output-shape claim.
  In F18's `Watch`, add a sentence stating that `status`, run from the **prime worktree** (the pair holding the merge record), is the read-only way to ask whether the merge is parked, and reports `merge_in_progress: true` for the whole live window and `false` again after both `merge --continue` and `merge --abort`.
  That same sentence must carry the this-pair qualification: it must say the field does **not** predict every refusal in the list two lines above it, because `remove` of the merge's own source pair refuses on the hub-wide predicate, and run from *that* pair `merge_in_progress` is correctly `false`.
  Without the qualification, an unqualified "must report true while a merge is live" sitting beside that refusal list would send a suite operator hunting a non-bug.
  Both `Watch` edits follow this file's existing one-sentence-per-line convention.
  Last, in `manifest/roadmap.md`, move the sole **Planned** item — `fabric: surface merge-in-progress in \`lyx fabric status\`` — into **Done**.
  Delete the item from the Planned section but keep the `## Planned` heading and its lead-in line ("This section holds what's committed to next.") in place with no items under them;
  an empty Planned section is acceptable, deleting the heading would break every cross-reference to "the Planned section", and promoting a Someday item to fill the gap would be an unrequested scope decision.
  Insert the Done entry as the first numbered item under the Done section's lead-in line, using the same two-line shape as the existing `fabric: two-sided reset-to-SHA verb` entry in this file: a `1. **fabric: surface merge-in-progress in \`lyx fabric status\`** — ` line with one sentence of what shipped, then an indented continuation line reading exactly `See the \`internal/fabricengine\` package documentation's merge section.` — the same phrasing that entry already establishes for this module, pointing at the passage this very commit edits.
  Do not renumber anything;
  all list items in this file use the `1.` auto-numbering form.
  Do not add or edit a `manifest/designs/fabric.md`;
  none exists, and a shipped module's Done entry points at its package documentation instead.
- **Commit:** `feat(fabriccli): report merge_in_progress in fabric status`

## Batch Tests

`verify:` runs both build tiers over `./internal/fabriccli/...`:

- `go test ./internal/fabriccli/...` — the untagged tier, covering `argsarity_test.go` and `envelope_test.go`, the pure cobra-inspection regression pins that must stay green while `statusCmd`'s `Long` changes.
- `go test -tags integration ./internal/fabriccli/...` — the tagged tier, covering the new `status_mergeinprogress_integration_test.go` plus the whole existing `TestRunCLI_*` suite, including `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` (itself `integration`-tagged), which pins that `status` carries neither a `mutations` nor a `partial` key, and `envelopecontract_integration_test.go`, which is unaffected because `status` is not one of the mutating verbs it pins.

Both tiers are required.
An untagged run alone compiles none of the tagged files, so the new test would silently not run and the pass would be vacuous.

Scope is one package tree, matching this batch's `Edits:` and `Creates:` — `internal/fabriccli` is the only module under test, and no `internal/fabricengine` test changes, since that package's only edit is `doc.go` prose.

Cross-package regressions are caught separately by `pipeline.done_gate`, already configured for this hub as `go test ./... && go test -tags integration ./...`, which mill-go runs from the repo root before marking the task done.
