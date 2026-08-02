# Batch: fabric-cli-collapse

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
batch: fabric-cli-collapse
number: 6
cards: 9
verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/ ./cmd/lyx/
depends-on: [5]
```

## Batch Scope

Collapse fabric's CLI status surface and finish unexporting `CommitWeft`. Wire a new `lyx fabric diff` verb onto `Fabric.Diff`; replace the weft-only `lyx fabric status` (backed by `StatusWeft`) with the unified `Fabric.Status`, then drop `StatusWeft`; migrate `fabriccli`'s own weft verbs (`commit`/`push`/`sync`) onto `Fabric.Commit` and unexport `CommitWeft`→`commitWeft` (all external callers are now gone). Revise `doc.go`'s status-surface and diff-verb prose, the `helptree_test` verb list, and the observable-surface docs (`fabric-unified-view.md`, `overview.md`, sandbox F3). This is the last batch; after it, fabric's external surface is the unified `Commit`/`Diff`/`Status`/`Bolt`/topology set with no warp/weft-named leak.

## Cards

### Card 22: Wire the lyx fabric diff verb

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `diff` subcommand to the flat `lyx fabric` tree via `addWeftVerbs` (`weft_verbs.go:295`) and add `"diff"` to the `weftVerbNames` map (`weft_verbs.go:27`) so it inherits the shared `PersistentPreRunE` that resolves `fab` (`fabricengine.New(l.WorktreeRoot, l.WeftWorktree())`). Also update `fabric.go`'s package doc comment (`fabric.go:10`, "the flat 14-verb `lyx fabric` tree") and any other verb-count prose in that file to the correct new count of **16** — note the "14" is already stale by one: the tree currently has 15 registered verbs (10 topology in `fabric.go` + 5 content-sync in `weft_verbs.go`, confirmed against `helptree_test.go`'s 15-entry `wantSubs`), and adding `diff` makes 16, not 15. The CLI/Cobra Invariant requires help/doc accuracy to be rechecked whenever observable behaviour changes. The verb takes one required positional `<since-warp-sha>`. Set `Args: cobra.ExactArgs(1)` on the command (matching every other fabric verb's `len(args)` guard — `cmd/lyx` has no `recover()`, so an unguarded `args[0]` panics on zero args instead of emitting the JSON error envelope the CLI/Cobra Invariant requires). Then call `res, err := fab.Diff(args[0])` (`Fabric.Diff` returns `DiffResult{Entries []ChangeEntry, NoWeftCorrespondence bool}`). Emit via `output.Ok(out, map[string]any{"entries": <flattened>, "no_weft_correspondence": res.NoWeftCorrespondence})`, flattening each `ChangeEntry` to `map[string]any{"path": e.Path, "side": string(e.Side)}` (the `ChangeEntry` type carries no json tags, so map-flatten to keep fabric's snake_case/lowercase field convention). Give the command a non-empty `Short` (e.g. "show unified warp+weft diff since a warp SHA") and a `Long` with a concrete example (CLI/Cobra Invariant). Errors go through `output.Err`/`clihelp.SetExit` exactly like the sibling verbs.
- **Commit:** `feat(fabric): add lyx fabric diff verb`

### Card 23: Repoint lyx fabric status onto Fabric.Status

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the `status` verb handler (`weft_verbs.go:142-158`) to call the unified `fab.Status()` (`Fabric.Status() ([]ChangeEntry, error)`) instead of `fab.StatusWeft(pathspec)`. Emit `output.Ok(out, map[string]any{"changes": <flattened entries>})`, flattening each `ChangeEntry` to `map[string]any{"path": e.Path, "side": string(e.Side)}` (same snake_case-preserving flatten as card 22). Keep the `clihelp.ShouldAbort`/`SetExit` envelope handling. The `status` verb no longer references `pathspec` — leave the shared `pathspec` resolution in `PersistentPreRunE` intact (other verbs still use it). Update the `status` command's `Short`/`Long` if they describe a weft-only view, to reflect the unified both-sides change list.
- **Commit:** `refactor(fabric): back lyx fabric status with unified Fabric.Status`

### Card 24: Drop StatusWeft

- **Context:**
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/diff.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `Fabric.StatusWeft(pathspec []string) (map[string]any, error)` method (`weftgit.go:178`) — after card 23 its only caller (the `status` verb) is gone, and recon confirmed no other caller (prod or test). Grep to confirm zero residual references before deleting. Remove any now-unused helper or import that `StatusWeft` alone pulled in. Strip the now-deleted `StatusWeft` from `weftgit.go`'s top-of-file enumeration comment (`weftgit.go:1`, "the weft-git content-sync verbs on Fabric: StatusWeft, …"). Also update the two `diff.go` doc-comment mentions of `StatusWeft` as a live surface (`diff.go:4` and `diff.go:57`, which describe it as a distinct dirty/ahead/behind view) so no comment names the deleted method — reword to the two surviving surfaces (`Topology.Status` / unified `Fabric.Status`). Trim edited comments to the `golang-comments` shape.
- **Commit:** `refactor(fabric): remove weft-only StatusWeft`

### Card 25: Migrate fabriccli weft verbs onto Fabric.Commit

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate the normal-mode `fab.CommitWeft(pathspec, fabricengine.DefaultCommitMessage, opts)` calls in the `commit` (`:183`), `push` (`:222`), and `sync` (`:282`) handlers onto `fab.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, opts)` — the existing `pathspec` (`ScopedPathspec(l.RelPath, cfg.Dirs())`, built in `PersistentPreRunE:128`) is already a positive-only list valid as `Commit`'s `files` arg. Map the `(sha, committed, err)` usage onto `CommitResult` (`res.WeftSHA`/`res.WeftCommitted`). Leave the bypass-mode `fabricengine.CoalescePushBothAt(warpPath, weftPath, ...)` (`:212`) unchanged — it stays exported and is the paired-push path. Per the unexport-commitweft decision, accept that `lyx fabric commit` now also fires `Commit`'s async detached push (a consistency bonus; observable weft-commit behaviour is otherwise unchanged). Update the file-header doc comment (`weft_verbs.go:1-11`) which currently names `StatusWeft/CommitWeft/...` as the driving methods. Preserve each verb's `output.Ok`/`output.Err` envelope shape.
- **Commit:** `refactor(fabric): drive fabriccli weft verbs through Fabric.Commit`

### Card 26: Unexport CommitWeft

- **Context:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/unwire.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the exported `Fabric.CommitWeft(pathspec []string, message string, opts SyncOptions, snapshotTags ...string)` (`weftgit.go:504`) to package-private `commitWeft`, same signature and body. After batches 2 and 5 and card 25, its only remaining callers are in-package: `unwire.go:121` (update to `commitWeft`) and the in-package `internal/fabricengine/*_test.go` files (which call it as a same-package method — update those call sites to the new casing too: grep the `fabricengine` package for `.CommitWeft(` and `CommitWeft` doc mentions and update every hit). Confirm via grep that no caller outside the `fabricengine` package references `CommitWeft` after this card. Update `doc.go`/`fabric.go`/`weftgit.go` doc-comment mentions of `CommitWeft` to `commitWeft` casing (the status-surface/diff `doc.go` prose is handled in card 28; here only fix the `CommitWeft`→`commitWeft` name occurrences). Also reconcile `CONSTRAINTS.md`'s "Fabric Git Invariant" section, whose prose still names now-deleted/unexported symbols as current: in the "Orchestration, not agent" bullet (`CONSTRAINTS.md:92`) remove the deleted `SyncWeft` (from "`fabricengine`'s `SyncWeft`/`CommitWeft`") and fix every remaining exported-name casing in that section — `CommitWeft`→`commitWeft`, `CommitWeftAt`→`commitWeftAt`, `PushWeftAt`→`pushWeftAt` (the board carve-out clause "board's git flows through `fabricengine.CommitWeftAt`/`PushWeftAt`" now routes through `Bolt` over those unexported primitives — reflect that), and `commitWeftLocked`/`CommitWeft` machinery → `commitWeft`. Grep `CONSTRAINTS.md` for any remaining `SyncWeft`/`CommitWeft`/`CommitWeftAt`/`PushWeftAt`/`StatusWeft` name and reconcile it. One line per paragraph, no hard-wrap. Additionally update `weftPathspecFilter`'s doc comment in `weftgit.go` (which still names buildercli/webstercli/perchcli as producers of `:(exclude)` pathspec entries — a claim batch 2 falsified): note that after this task no caller feeds `:(exclude)` magic, so its magic-entry passthrough branch is now unreachable by any production caller; leave the passthrough code in place (gitrepo's general `:(exclude)` handling is a deliberately kept general-layer capability) but correct the comment's stale producer list.
- **Commit:** `refactor(fabric): unexport commitWeft; reconcile CONSTRAINTS invariant`

### Card 27: Update test suite for the commitWeft unexport

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/commit.go`
- **Edits:**
  - `internal/fabricengine/weftgit_exclude_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Card 26 unexports `Fabric.CommitWeft`→`commitWeft`. Two categories of test caller need reconciling. (1) IN-PACKAGE (`package fabricengine`) test files call it as `f.CommitWeft(...)` and survive an in-package rename: grep the `fabricengine` package for `.CommitWeft(` and update every hit to `.commitWeft(` — the affected files are `snapshot_integration_test.go` (`:38,:486`), `weftgit_pathspec_integration_test.go` (surviving positive-pathspec cases `:90,:133,:285,:312` after card 14's `:(exclude)`-case reconciliation), `pull_integration_test.go` (`:50`), `syncweft_integration_test.go` (`:335,:394`, the surviving `TestWeftSHAForWarpSHA_*` tests), `checkout_index_refresh_test.go` (`:65`), `commit_integration_test.go` (`:849`), and `diff_integration_test.go` — this casing sweep is done by card 26's own grep-and-update-every-hit instruction, so these files are NOT separately listed in this card's Edits (a same-package method rename plus grep is the whole fix). (2) EXTERNAL — `weftgit_exclude_test.go` is `package fabricengine_test` and calls `f.CommitWeft(...)` at `:96` and `:184` via the exported name; once `CommitWeft` is unexported, Go visibility makes those references uncompilable and no rename can fix them. Migrate both call sites onto the exported `Fabric.Commit`: replace `f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})` (and the `:184` `pathspec`-variable form) with `f.Commit(<positive file list>, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})`, reading `CommitResult.WeftCommitted` where the old `committed` bool was used. This test validates the `.git/info/exclude` seeding (the surviving transient-exclusion mechanism), so express its pathspec as a positive list and let the exclude file skip transients — if the `:184` `pathspec` variable carried any `:(exclude)` entry, rewrite it to the positive-list-plus-exclude-file model. Run the test to confirm the exclude-file behaviour it asserts still holds.
- **Commit:** `test(fabric): migrate external weftgit_exclude_test off unexported CommitWeft`

### Card 28: Revise doc.go status/diff prose and help-tree

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`: rewrite the "three deliberately-distinct status surfaces" paragraph (`doc.go:80`) to describe TWO CLI surfaces after this task — `Topology.Status` (`lyx fabric pairs`) and the unified `Fabric.Status` (`lyx fabric status`) — dropping `StatusWeft` from the enumeration. Update the `Fabric.Diff` "(there is no CLI verb for it — resolved Go-internal-only …)" parenthetical (`doc.go:78`) to reflect that `lyx fabric diff` now exists. Ensure the `Commit`/`CommitWeft` prose (around `doc.go:70-76`, `:46`, `:52-53`) reads correctly with `commitWeft` unexported. In `cmd/lyx/helptree_test.go`, add `"diff"` to the fabric `wantSubs` slice (`:67-73`) so the new verb is asserted (the existing `status` entry stays — the verb is repurposed in place, not removed). Trim edited `doc.go` comments to the `golang-comments` shape; one line per paragraph in the test's Go source is not applicable (Go file), but keep `doc.go` markdown-free.
- **Commit:** `docs(fabric): revise status/diff surface docs; assert diff verb`

### Card 29: Update observable-surface design and overview docs

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/fabric-unified-view.md`, update the passages that go stale with this task: the slice-2 line naming `CommitWeft` as an external building block (line ~113) — reflect that `CommitWeft` is now the package-private `commitWeft`; and the open-question line "Whether `Fabric.Diff` is a CLI verb or Go-internal only — resolved … Go-internal only; no CLI verb was added" (line ~130) — update to state `lyx fabric diff` now exists. In `docs/overview.md`, add `diff` to the authoritative `lyx fabric …` verb enumeration (lines ~121 and ~172) and note `status` is now the unified both-sides view. One line per paragraph, no hard-wrap, for both markdown files.
- **Commit:** `docs: reflect fabric diff verb and unified status in design/overview`

### Card 30: Update sandbox F0/F3 watch-text for the diff verb and status shape

- **Context:**
  - `internal/fabricengine/diff.go`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update scenario F3 (`SANDBOX-FABRIC-SUITE.md:113-119`) so its watch-text matches the new `lyx fabric status` output: a side-labelled `[]ChangeEntry` list (`{path, side}` per change) instead of the old weft-only branch/dirty/ahead/behind map. Confirm no scripted consumer in the suite relies on the dropped branch/ahead/behind fields; if the F3 goal text implies inspecting those, reword it to the change-list shape. Leave the async-push lag note intact. F3 does not need a `**Covers:** fabric` line added (fabric is already covered by F4/F5/F6, so `sandbox_coverage_test` stays green). Also update scenario F0's watch-text (`SANDBOX-FABRIC-SUITE.md:85`, "Does `lyx fabric` list all 15 verbs (`clone`, …, `unwire`)?") to the new count of 16 and add `diff` to its explicit verb enumeration. One line per paragraph, no hard-wrap.
- **Commit:** `docs(sandbox): update F3 for unified fabric status output`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/ ./cmd/lyx/` runs fabric's CLI wiring plus `cmd/lyx` help-tree/drift/registration guards (the new `diff` verb must carry a `Short` and appear in the help tree) and fabriccli's own verb tests. The module-wide `go build ./...` boundary check confirms `commitWeft`/`StatusWeft` have no stray external caller left. Card 27's `weftgit_exclude_test.go` migration and the in-package `commitWeft` casing sweep are both exercised by the `./internal/fabricengine/` integration run — the suite must compile (proving no test file still references the exported `CommitWeft`) and pass. `docs/`, `manifest/`, and `tools/sandbox/` edits are doc-only. Scope covers the three edited code-bearing packages.
