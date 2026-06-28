# Batch: docs-roadmap-constraints

```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "docs-roadmap-constraints"
number: 6
cards: 2
verify: null
depends-on: [3, 4, 5]
```

## Batch Scope

Records the behavior changes in the durable docs, satisfying the Task-completion
documentation discipline: `docs/roadmap.md` notes the CLI-ergonomics landing, and
`CONSTRAINTS.md` gains the two new CLI/Cobra-Invariant rules (JSON-wrapped Cobra errors;
parent groups reject unknown subcommands). Depends on batches 3/4/5 so the docs describe the
final, implemented surface. Pure-docs batch: `verify: null` (no runnable surface). There are
no `docs/modules/{warp,weft,config}.md` design docs, so none need touching.

## Cards

### Card 21: Roadmap note for CLI ergonomics

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a short follow-up note under milestone 21 ("Built-in CLI help —
  self-documenting modules & commands", around line 192) — or a sibling bullet near it —
  recording the 2026-06-28 sandbox-driven CLI-ergonomics pass: a consistent JSON error
  envelope for Cobra-level errors (W14/W15), parent groups now error on unknown subcommands
  (W16), `warp clone --reset` + `Long` (W2/W3), `warp status` renamed to `warp pairs` (W7),
  `warp add` fork-point documented (W6), `weft commit`'s fixed message documented (W4), and
  `lyx config --print` read-only mode + module listing in help (W12/W5). Keep it to a few
  lines consistent with the surrounding roadmap style; do not restructure existing entries.
- **Commit:** `docs(roadmap): note CLI help & error ergonomics pass`

### Card 22: CONSTRAINTS.md CLI/Cobra Invariant additions

- **Context:**
  - `internal/clihelp/exec.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the **CLI / Cobra Invariant** section, add two rules to the existing
  rule list:
  - **Errors are JSON.** Cobra-level errors (unknown command/flag, arg validation) are
    wrapped in the `internal/output` JSON envelope (`{ok:false,error:...}`) on stdout at the
    `clihelp.Execute` seam and at the `cmd/lyx` root, both of which set
    `SilenceErrors = true`. `output.Err` trims the message. Do not reintroduce bare
    plain-text error paths (config's were harmonized in this task).
  - **Parent groups reject unknown subcommands.** Every parent module group sets
    `RunE = clihelp.GroupRunE` (errors `unknown subcommand …` on extra args, else shows
    help); groups with a layout-resolving `PersistentPreRunE` guard it with an early return
    when `cmd.Name()` is the group name, preserving the no-git-repo subcommand listing.
  - Phrase them to match the existing bullet style of that section; keep the rest of the file
    unchanged.
- **Commit:** `docs(constraints): record JSON-error and reject-unknown-subcommand rules`

## Batch Tests

`verify: null` — this batch edits only Markdown docs (`docs/roadmap.md`, `CONSTRAINTS.md`)
with no runnable surface. Correctness is content review by the plan/code reviewer. (The
behavioral assertions for every documented change are covered by batches 1–5; this batch
adds no code.)
