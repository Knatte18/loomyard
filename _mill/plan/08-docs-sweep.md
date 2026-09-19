# Batch: docs-sweep

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: docs-sweep
number: 8
cards: 3
verify: go build ./... && go test ./cmd/lyx/... ./internal/loomcli/...
depends-on: [5, 6, 7]
```

## Batch Scope

This batch carries the cross-cutting prose that could not be written until the whole shape existed: the design doc's driver section moving from expected to as-built, the module and execution-stack prose, the roadmap item moving to shipped, the help text that is now driver-specific, and the sandbox suite's recorded reason for not scripting this path.
It is one batch because all of it describes the finished surface and would be rewritten twice if split across the batches that built it.
Per the overview's `per-module-docs-ride-their-own-card` decision, no `CONSTRAINTS.md` amendment lands here — the one invariant this task adds rode batch 4, the batch whose change makes it true.

Batch-local decision beyond the overview's: the sandbox suite gets a **prose note with no mechanical enforcement**, and loom stays a covered module.
The gate is module-keyed, with no sub-module slot for one path within a covered module, so the obvious-looking move — adding loom to the excluded list — would both fail the gate's own rule that an exclusion names a registered module and silently drop loom's existing coverage.

## Cards

### Card 24: the bootstrap's own help text

- **Context:**
  - `internal/loomcli/driverlaunch.go`
  - `internal/shedrun/seed.go`
  - `internal/loomcli/bootstrap.go`
- **Edits:**
  - `internal/loomcli/start.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Amend the long help text of the command `startCmd` builds in `internal/loomcli/start.go` so it describes what the verb now does rather than what it did.
  The numbered steps must say that the driver step reads this run's seed and either spawns the detached Go runner or boots an ly-drive session in this worktree's own reed session, and that which one happens is the seed's recorded choice rather than a flag on this command.
  Rewrite the paragraph describing the no-attach flag, which today names the run lock unconditionally: its documented meaning is unchanged on both paths — perform every bootstrap step, confirm the driver is up by **that path's own** readiness signal, and return without the terminal handover — but the readiness signal differs, so say so per driver rather than naming one mechanism for both.
  Keep the sentence about the detached driver's own output going to its log, and scope it to the Go driver, which is the only path that writes one.
  Keep the command's short description non-empty, per the CLI/Cobra Invariant, and keep every example line valid.
  Add no flag to this command: a driver flag here would be a second source of truth whose only behaviour is to disagree with the seed.
  In `cmd/lyx/helptree_test.go` confirm the pinned command tree still matches and update it only if this card's edits changed a name or a short description — the help-tree gate is where an accidental rename surfaces, and this card changes prose, so the expected outcome is no change to the pinned set.
- **Commit:** `docs(loomcli): describe the seed-driven driver choice in start's help`

### Card 25: the design doc and the roadmap

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/driverspec.go`
  - `internal/shedrun/seed.go`
  - `internal/shedcli/table.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/seeded-shed.md`
  - `manifest/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/seeded-shed.md`, move the driver section from expected to as-built: state that a recipe's own bootstrap verb is the site that reads its run's driver, that loom's is the only such verb today, and that a recipe without one cannot be seeded for an LLM driver until it grows one.
  Record the two accepted residuals this task ships rather than leaving them to be rediscovered — a driver strand that dies mid-run is not detected, reported or recovered by anything here, and a driver that finishes normally leaves its strand and its run directory behind until the next bootstrap's corpse removal unregisters the strand.
  Correct the design's own "one changed command in the existing spawn seam" phrasing to name the child's own bootstrap rather than the parent's spawn seam, and say why the substance holds either way: the command the parent's seam runs is itself a bootstrap, and the branch belongs at the innermost point that knows whose run it is.
  State plainly that no default flips in this task — the driver still defaults to the Go runner everywhere, including for a child — and that the design's expected default is a config decision for a later pass, once an llm-driven run has been watched end to end.
  In `manifest/roadmap.md`, move this task's planned item to shipped, per this repo's roadmap discipline that the file moves only on completing or adding a planned item.
  In `docs/overview.md`, update the module table and execution-stack prose only where this task changed them, and leave the rest alone.
  Every inline link added or touched in all three files must resolve, file part and anchor, per the Markdown Link Integrity invariant; add an allowlist entry only if a link genuinely cannot resolve, and name this task as its owner.
  Follow this repo's markdown rule throughout: one sentence per line, semantic breaks, no fixed-column hard wrap.
- **Commit:** `docs(manifest): record the seeded driver choice as shipped`

### Card 26: the sandbox suite's recorded disposition

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/loomcli/start.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a prose note to `tools/sandbox/SANDBOX-CORE-SUITE.md` recording that the llm-driven bootstrap is deliberately not scripted as a sandbox scenario, and why: exercising it end to end spawns a live, billed provider session that runs for as long as the task takes, which a sandbox scenario can neither bound nor assert against.
  Say which mechanics are covered instead and where — the branch, the conjunction predicate, the spec composition, and the strand count across relaunches, all against a stubbed binary in this task's own Tier 1 and smoke tests.
  The note carries **no mechanical enforcement** and must not try to acquire any.
  Do not add loom to the excluded-modules list in `cmd/lyx/sandbox_coverage_test.go`, and do not edit that file at all: the gate is module-keyed with no sub-module slot, loom is already covered and stays covered, and an exclusion entry would both fail the gate's own rule that an exclusion names a registered module and silently drop loom's existing coverage.
  State that reasoning in the note itself, briefly, so the next reader who notices the gap does not reach for the exclusion list.
  Keep the note's placement consistent with how the document already records per-scenario decisions, and follow this repo's markdown rule: one sentence per line, semantic breaks, no fixed-column hard wrap.
- **Commit:** `docs(sandbox): record why the llm bootstrap is not a scripted scenario`

## Batch Tests

`verify: go build ./... && go test ./cmd/lyx/... ./internal/loomcli/...` runs the command tree's and the loom CLI's untagged suites plus a whole-module build.
The command tree is in scope because card 24 touches help text the help-tree gate walks, and because card 26's subject is that package's own coverage gate — which must stay green with loom still covered and no exclusion added, the one mechanical outcome this otherwise prose-only batch has.
The loom CLI is in scope because card 24 edits a command's long text, which its own command tests assert against.

This batch is prose apart from card 24's help text, so its real verification is the two gates it must not break.
The sandbox coverage gate must still report loom as covered, which is the assertion that catches the tempting wrong move card 26 exists to forbid.
The markdown link gate must still pass over three changed documents, and it is keyed by file and target, so a moved anchor in the design doc fails there rather than at review.
