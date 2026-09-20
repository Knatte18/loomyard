# Batch: ly-drive-autonomous

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: ly-drive-autonomous
number: 6
cards: 2
verify: go build ./... && go test ./cmd/lyx/...
depends-on: [4]
```

## Batch Scope

This batch teaches the ly-drive skill how to behave with nobody to ask and nobody to report to, and pins the one number that must agree between the skill's prose and the Go-composed launch prompt.
It is one batch because the skill edit and the drift check are one change: the number is the only thing in this task that lives in two files written in two languages, and a test asserting agreement is meaningless without the section it anchors on.

Four things change in the skill and nothing else does.
The skill keeps its explicit-invocation-only posture: the driver session invokes it from its launch prompt, which is exactly that contract rather than an exception to it.

Batch-local decision beyond the overview's: the skill's own prose is the **sole enforcement** of the no-operator-prompts rule.
The skill asks its questions as numbered text lists in its own text, not through a tool the autonomous posture denies, so the deny that shuttle installs prevents nothing the skill actually does — it is defence-in-depth against a future edit that reaches for that tool, not the mechanism that makes this change work.
Stating this in the plan matters because the obvious-looking reading is that the posture already handles it, which would leave the skill edit looking optional.

## Cards

### Card 21: the autonomous driver section

- **Context:**
  - `internal/loomcli/driverprompt.go`
  - `plugins/ly/skills/INDEX.md`
- **Edits:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `## Autonomous driver` section to `plugins/ly/skills/ly-drive/SKILL.md`, entered when the launch prompt says so, stating the four things that change from the operator-driven path and nothing else.
  First, **no operator choices**: the pane self-check's tracked-absent branch and every other numbered-list prompt in the skill become a line in the report rather than a question, because there is no operator in the session to answer one.
  Second, **the report goes to the file**: every place the skill says to report to the operator or hand back with a report, the autonomous path writes that report to the output-file path named in its launch prompt and then stops.
  Third, **the step cap is a budget, not a check-in**, written on its own line in the fixed phrase `autonomous step cap: 120` so card 22's test has a stable anchor to match on.
  Give the number its derivation in the surrounding prose — loom's own worst case as this skill already computes it, plus a margin — and say that the operator-driven cap of 40 exists so a human can look, which with no human would stop a healthy run three times before it finished.
  On exhausting the budget the driver writes the report and stops, leaving the run exactly as it is.
  Fourth, **stop conditions are otherwise unchanged and remain absolute**: a halting envelope stops, an error envelope stops with the single retry the skill already allows, and the skill still never clears state, never edits the status file, never re-seeds, never pushes, never kills a pane, and never touches git.
  Do not rewrite the skill's existing stop conditions for the autonomous case and do not add a second driving policy — the skill's judgment is the product, and autonomy changes only who reads the output and what to do when there is nobody to ask.
  Keep the skill's existing frontmatter posture unchanged.
  Follow this repo's markdown rule: one sentence per line, semantic breaks, no fixed-column hard wrap.
- **Commit:** `docs(ly-drive): add the autonomous driver section`

### Card 22: pin the step cap against drift

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/loomcli/driverprompt.go`
  - `plugins/ly/skills/ly-drive/SKILL.md`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/drivercap_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `cmd/lyx/drivercap_test.go` asserting that the exported step-cap constant in the loom CLI package and the cap stated in the ly-drive skill are the same number.
  Resolve the repository root through the runtime caller, copying the mechanism `cmd/lyx/sandbox_coverage_test.go` already uses — that is this repo's one working way for a Go test to read a file outside its own package tree.
  Read the skill file, locate the `## Autonomous driver` heading, and match the cap on the fixed phrase card 21 writes, **within that section only**.
  The anchoring is the whole design of this test and must not be relaxed to a bare file-wide search: the skill already contains the operator cap of 40 and a prose aside about loom's worst case landing near a hundred steps, so a search across the whole file would stay green against exactly the wrong number.
  Assert the located value equals the constant's value, and fail with a message naming both sides and both files so a reader of the failure knows which one to move.
  Fail loudly rather than skipping when the heading or the phrase is absent — a skipped test here is indistinguishable from a passing one, and the section's removal is precisely the drift this test exists to catch.
  The test lives in this package rather than beside the constant because no package under the plugins tree compiles Go, and it reads an exported identifier because a test outside the loom CLI package cannot see an unexported one.
- **Commit:** `test(lyx): pin the autonomous step cap against skill drift`

## Batch Tests

`verify: go build ./... && go test ./cmd/lyx/...` runs the command tree's untagged suite, which is where the new drift test lives, plus a whole-module build.
No other package is in scope: card 21 edits a markdown file that no Go package compiles, and card 22 reads it at test time rather than embedding it.

The anchored-match requirement is the load-bearing part of this batch and the easy thing to get wrong.
A test that searched the whole skill file for the constant's value would pass today against the operator cap sitting a hundred lines above, and would keep passing after someone edited the autonomous section's number — which is the exact drift the test is being written for.
The drift it guards is silent in production: the prompt says one number, the skill says another, and the session simply runs a different budget than its launcher believes, with nothing in the run's output to show which one won.
