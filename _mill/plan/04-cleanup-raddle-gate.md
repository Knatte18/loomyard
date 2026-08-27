# Batch: cleanup-raddle-gate

```yaml
task: "Add a local-only file category to weft"
batch: "cleanup-raddle-gate"
number: 4
cards: 3
verify: go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

`raddleFoldedBack` is a stub returning `false`, so today every fabric-managed orphan weft branch is gate-protected unless `--force` — which makes routine teardown require the destructive flag.
This batch removes the stub and the `Protected` branch it feeds, so an orphan weft branch is deletable under `--apply` alone.
Raddle does not bring the gate back: its design regenerates fresh against the parent's HEAD and commits directly onto the parent pair inside `Finalize`'s critical section, so it never depended on a merge carrying the child's weft forward and there is no fold-back for a gate to guard.
The project `CLAUDE.md`'s `_lyx/raddle/` clause is corrected in the same batch, because it currently directs durable notes there as "anything versioned and merged into `main`", which is imprecise under this design.

This batch shares no file with batches 1, 2, 3 or 5, so it carries no `depends-on` edge.

Batch-local decision beyond `## Shared Decisions`: the correction to `CLAUDE.md` is a one-clause precision fix, not a redesign.
Raddle's *output* is genuinely meant to land in the parent's weft.
What never travels is the child's copy of it, by git-merge.
Do not restate the clause as "`_lyx/raddle/` is per-branch-local and never merged" — that flattens a regenerate-and-commit step into a merge, and is exactly the misreading the discussion records an earlier reviewer making.

## Cards

### Card 18: remove the raddle fold-back gate from Cleanup

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabriccli/fabric.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/cleanup.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/cleanup.go`:
  (a) delete the `raddleFoldedBack` function and its doc comment;
  (b) delete the `folded := raddleFoldedBack(branch)` block inside `Topology.Cleanup` — the `if !folded && !force` branch that sets `entry.Protected = true` and continues — together with the paragraph-long in-body comment above it explaining why the gate is evaluated in both modes;
  (c) rewrite the package-level flag matrix in the file-header comment so it names only the protections that survive: `apply == false` is report-only, `apply == true` deletes every orphan branch that is not the primary weft branch, not checked out at a worktree, and not unmanaged;
  (d) rewrite the `CleanupBranchEntry.Protected` field comment, deleting its "because raddleFoldedBack returned false and force was not set" clause and keeping the two surviving reasons;
  (e) rewrite `Topology.Cleanup`'s own doc comment, deleting "force bypasses the `_lyx/raddle/` merge-back gate";
  (f) keep the `force` parameter in `Topology.Cleanup`'s signature and document it, in that doc comment, as reserved and currently consulted by no gate in this verb — `deleteWeftBranch` already hardcodes `force: false` for its own request and that stays true.
  Leave `primaryWeftBranch`, `listWeftBranches` and `deleteWeftBranch` bodies unchanged.
  Do not change `Topology.Cleanup`'s exported signature.
  Do not touch `internal/fabriccli/fabric.go`'s `--force` flag registration in this card.
- **Commit:** `refactor(fabricengine): remove Cleanup's raddle fold-back gate`

### Card 19: correct the CLAUDE.md raddle clause and the cleanup flag help

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `manifest/designs/raddle.md`
  - `_mill/discussion.md`
- **Edits:**
  - `CLAUDE.md`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CLAUDE.md`, correct the `## Persistent notes go in git, not file-memory` section's clause directing durable notes to `_lyx/raddle/` as "anything versioned and merged into `main`".
  Replace only that clause: `_lyx/raddle/` content reaches the parent by being regenerated fresh against the parent's HEAD and committed onto the parent pair at landing time, never by a merge carrying the child's copy forward.
  Keep the section's other two destinations — this file and code comments — and its overall shape.
  Follow the repo's semantic-line-break rule.
  In `internal/fabriccli/fabric.go`, correct the two `cleanup` flag help strings and the `Long` description so they stop naming a gate that no longer exists: `--apply`'s help currently reads "delete non-gate-protected orphaned weft branches" and `--force`'s reads "also delete gate-protected task branches (requires --apply)".
  `--force`'s new help must say the flag is reserved and answers no `cleanup` gate today.
  Keep both flags registered, keep `Use: "cleanup [--apply] [--force]"`, and keep `runCleanupWithFlags`' signature and body unchanged — the CLI / Cobra Invariant makes the flag set part of this module's contract, and removing a shipped flag is outside this task's scope.
- **Commit:** `docs: correct the raddle fold-back claim in CLAUDE.md and cleanup help`

### Card 20: rework the cleanup gate tests

- **Context:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/cleanup_primary_integration_test.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/reconcile_stale_registration_test.go`
- **Creates:**
  - `internal/fabricengine/cleanup_raddlegate_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/reconcile_stale_registration_test.go`, rework `TestCleanup_DryRunMatchesApplyVerdict`.
  Its dry-run-matches-apply property survives and must keep being asserted, but its final assertion — `t.Fatalf` when `--apply` deleted the orphan without `--force`, on the grounds that "the gate is not doing its job" — asserts exactly the behaviour card 18 removes.
  Replace it with the inverse: the orphan is now `Deleted` under `--apply` alone and reported unprotected in both the dry run and the apply.
  Rewrite the test's doc comment accordingly, keeping its explanation of why dry-run/apply parity matters and dropping its account of the gate.
  Leave the checked-out-branch protection test around `internal/fabricengine/reconcile_stale_registration_test.go:470-505` unchanged: that protection survives, and it is the proof card 18 removed only the raddle gate.
  Create `internal/fabricengine/cleanup_raddlegate_integration_test.go` in `package fabricengine_test`, opening with the `//go:build integration` constraint and a file-header comment.
  Cover three scenarios, one test function each:
  (1) `Cleanup(l, true, false)` — apply without force — deletes an orphan weft branch outright, with `Deleted` true, `Protected` false, `Error` empty, and the branch genuinely gone from the weft repo;
  (2) the primary weft branch is still protected under `Cleanup(l, true, false)`, so removing the raddle gate did not widen the primary carve-out;
  (3) an unmanaged, non-`-weft`-suffixed weft branch is still reported and still protected under `Cleanup(l, true, false)`.
  Reuse `newFabricFixture`, `findCleanupEntry`, `branchExistsAt`, `mustWeftRepoRoot` and `fabricengine.WeftBranchName` from the package's existing test files rather than adding new fixture helpers.
- **Commit:** `test(fabricengine): an orphan weft branch is deletable without --force`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then the untagged tier over `./internal/fabricengine/...`, then the `integration` tier over the same package.

- The `integration` tier is chained separately because card 20 edits `reconcile_stale_registration_test.go` and creates a new `//go:build integration` file;
  both are invisible to an untagged run.
- Card 20's rework of `TestCleanup_DryRunMatchesApplyVerdict` is this batch's primary proof: it currently fails the build's own expectation in the opposite direction, so a batch that left `raddleFoldedBack` in place fails it.
- `internal/fabriccli` is deliberately absent from `verify:` even though card 19 edits `internal/fabriccli/fabric.go`: that edit is confined to three help/description strings and no `fabriccli` test asserts their text.
  `pipeline.done_gate`'s repo-wide sweep covers the package at the end of the run.
- `CLAUDE.md` is not under `manifest/` or `docs/`, so the Markdown Link Integrity invariant does not bind it and `./internal/lyxcwd/...` is not needed here.
