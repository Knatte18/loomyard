# Batch: registration-and-guards

```yaml
task: 'loom: session bootstrap'
batch: registration-and-guards
number: 6
cards: 6
verify: go test ./cmd/lyx/ ./internal/loomcli/
depends-on: [2, 5]
```

## Batch Scope

This batch makes the module reachable: it registers `loom` and its `run` alias as two root children and pays every registration cost a thirteenth module plus a second root child forces — the pinned seam-signature slices, the help-tree table, the sandbox-coverage allowlist and its new scenario, the CLI/Cobra Invariant's own seam counts and its new alias clause, and the overview's module table and seam prose.
It is one batch because none of these edits is meaningful alone: the cobra registration and every guard that reads the live cobra root must move together or the package does not compile and the suite does not pass.

It depends on batch 2 for the launcher and record work the docs describe, and on batch 5 for the alias constructor it registers.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- The `run` alias gets a sandbox-coverage allowlist entry rather than its own scenario, because a scenario for it would exercise byte-identical behaviour to the `loom` one; the `loom` module itself gets a real scenario, since an allowlist entry for a newly registered module is not acceptable.
- The new sandbox scenario reaches a seeded state by writing the status file as a hand-authored fixture, because no shipped verb seeds one without going through the tmux bootstrap and the pause verb on an absent file is specified to error.

## Cards

### Card 26: register loom and its alias in the cobra root

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/run.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/registration_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Import the new package, add `loomcli.Command()` and `loomcli.RunAliasCommand()` to the root's `AddCommand` call, keeping the existing entries' order otherwise undisturbed, and append both new root-child names to the root `Long`'s available-modules sentence.
  Both names are required there, not just the module's: the long-list guard iterates every registered non-infrastructure root child and asserts each appears in that prose, and the alias is one.
  Add a short comment beside the alias registration stating it is the same command as the subtree's verb, registered a second time at the root so it is discoverable in help and covered by the help-tree and registration guards, rather than spliced into the argument vector.
- **Commit:** `feat(lyx): register the loom module and its run alias in the cobra root`

### Card 27: extend the pinned seam-signature slices

- **Context:**
  - `internal/loomcli/cli.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/seamsignature_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the new module's `RunCLI` to the first pinned slice and its `RunCLIIn` to the second, keeping both alphabetical, and add the import.
  Update the file-header comment and both slice comments so their counts read thirteen and twelve rather than twelve and eleven, and so the second slice's note still names the single module deliberately absent from it and its reason unchanged.
  Add one sentence to the second slice's comment recording why the new module is on it rather than joining that exception: loom resolves cwd throughout, so a seeded cwd is meaningful to it, which is the exact criterion the existing exception rests on.
  The alias carries no seam function of its own and must not appear in either slice.
- **Commit:** `test(lyx): pin the thirteenth module's RunCLI and RunCLIIn seams`

### Card 28: extend the help-tree tables

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/drive.go`
  - `internal/loomcli/status.go`
  - `internal/loomcli/pause.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add both new root-child names to the required-modules list in the root-help test.
  Add a case to the verb-module subcommand table for the new module, naming all four of its subcommands, so the group listing is asserted to be complete.
  Do not add a case for the alias: it is a leaf root child with no subcommands, so the subcommand-table shape does not apply to it.
- **Commit:** `test(lyx): assert the loom module's help tree and its four verbs`

### Card 29: sandbox coverage for the module and its alias

- **Context:**
  - `internal/loomcli/status.go`
  - `internal/loomcli/pause.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/status.go`
  - `internal/shedengine/status.go`
  - `contracts/specs/loom-status-spec.md`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the coverage guard, add one entry to the exclusion allowlist for the alias's root-child name, with the one-line reason that it is an alias of the module's own bootstrap verb and is covered by that module's scenario.
  Do not add an entry for the module itself.
  In the core suite, add one new scenario after the last existing one and before the paragraph pointing at the dedicated reed suite, following the shape every scenario in that file already uses: a heading with the next sequence number and a short title, a covers tag naming the module, a goal paragraph in the operator's voice, a watch paragraph, and a verdict line.
  The scenario exercises exactly the two verbs that need no tmux — the status verb and the pause verb — and reaches a seeded state by hand-writing the status file as a fixture before invoking either.
  It must state in a fixture note why the fixture is the only tmux-free route: no shipped verb seeds one without going through the bootstrap's tmux handover, and pausing an absent file is specified to error.
  The watch paragraph must ask whether the fixture's own values round-trip through the status verb's envelope, which is what also pins that envelope against the status contract, and whether the pause verb sets the request flag while leaving every other field untouched.
  Add the new scenario's identifier to that file's session-log format block so the log template stays complete.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `test(lyx): cover the loom module in the sandbox suite and allowlist its alias`

### Card 30: update the CLI/Cobra Invariant

- **Context:**
  - `cmd/lyx/seamsignature_test.go`
  - `cmd/lyx/main.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the CLI/Cobra Invariant, move the seam clause's two counts from twelve-and-eleven to thirteen-and-twelve, leaving the named exception and its reason exactly as they are, and move the enforcement clause's identical counts with them.
  Add one new bullet covering the alias shape, which the invariant's opening sentence would otherwise read as forbidding: a module may register an alias command beside its own subtree, as a second root child, via a separately-named exported constructor, and that alias carries no seam function of its own because it delegates into the subtree's verb.
  Add the two new interactive-handoff exception holders to the invariant's own exception bullet, naming the watch mode's self-displays-then-blocks tail and the bootstrap verb's terminal handover, in the same shape the existing reed entries in the overview use.
  Add no new invariant section: nothing this task ships crosses a seam the existing invariants do not already govern.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `docs(constraints): move the seam counts to thirteen and cover the alias shape`

### Card 31: update the architecture overview

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/loomcli/cli.go`
  - `internal/loomshed/loomshed.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the package tree block, add a line for the new CLI package beside the other module lines, describing it as loom's cobra module: the session bootstrap plus the driver, status, and pause verbs.
  In the module-dispatch prose, move the seam-count sentence from eleven-of-twelve to twelve-of-thirteen, leaving the named exception and its reason unchanged.
  In the modules list, add a bullet for loom describing the four verbs, the bootstrap's four steps, the root alias, and the two interactive-handoff exceptions it registers, and marking it implemented, in the same voice and level of detail the reed and webster bullets already use.
  Correct the tree line describing loom's producer list so its row count matches the thirteen rows that package actually carries, since this task is what makes that list reachable and the stale count would otherwise be the first thing a reader checks against it.
  Every link this file already carries must keep resolving, both its file part and its anchor, since the Markdown Link Integrity guard scans this file.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `docs(overview): add the loom module and move the seam counts`

## Batch Tests

`verify: go test ./cmd/lyx/ ./internal/loomcli/` runs the two packages this batch touches.
`cmd/lyx` is the batch's real subject: it holds the registration itself and all four guards that read the live cobra root — the long-list, help-tree, registration, drift, and sandbox-coverage tests — plus the compile-time seam pin, whose assertion is the package compiling at all.
`internal/loomcli` is included because card 26's registration is the first thing to compile that package into the root binary, so a signature mistake there surfaces here rather than at the repo-wide gate.
The three doc cards have no runnable surface of their own; the markdown link guard that covers two of them lives in another package and is reached by the repo-wide done gate at Handoff.
