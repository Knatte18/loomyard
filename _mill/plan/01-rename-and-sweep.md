# Batch: rename-and-sweep

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: rename-and-sweep
number: 1
cards: 3
verify: go build ./... && go test -tags integration ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/... ./internal/loomengine/... ./internal/batcher/... ./cmd/lyx/...
depends-on: []
```

## Rename mechanic

_Include this section in any batch that contains at least one non-empty `Moves:` field.
The `move-mechanic-missing` validator check enforces this requirement.
For each `Moves:` pair the implementer MUST:_

1. _Run `git mv <old> <new>` FIRST, before making any other change to the moved file._
2. _Make ONLY surgical edits -- touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits)._
3. _Use a full-file `Creates:` entry only for genuinely new files that have no predecessor._
4. _Never write the relocated file from scratch and delete the original -- that breaks git rename history and inflates review diffs._

## Batch Scope

This batch delivers the mechanical half of the task: the file rename recorded by git as a rename, and the scripted six-pattern sweep across the 30 files that carry a pattern hit.
It is one batch because the rename must land before the sweep (so the sweep also rewrites references *inside* the renamed file), and because both steps are mechanical — the sweep is executed by a program, never by hand.
The external interface the later batches consume is the post-sweep tree: `docs/reference/plan-format.md` exists at its new name, every pattern hit outside the exclusion set is gone, and the only residue left is prose that no pattern can reach (bare `v3` labels and plan-format-**v2** references), which batches 2 and 3 own.

Batch-local decision beyond `## Shared Decisions`: the git-mv is its own card and its own commit (`rename-is-a-separate-git-mv`), so `git log --follow` and the review diff stay readable across the rename boundary.
Letting the sweeper perform the rename inline was rejected — it would write a new file and unlink the old one, which git reports as an unrelated add/delete pair.

## Cards

### Card 1: Rename the pinned plan-format doc, recorded as a git rename

- **Context:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `docs/reference/plan-format-v3.md` -> `docs/reference/plan-format.md`
- **Requirements:** Run `git mv` for the single pair above and make **no other change** in this card — not one byte of the file's content.
  The file's own internal references (its title, its `## Related` links, its self-link) are rewritten by card 2's sweep, not here.
  Confirm afterwards that `git status --porcelain` reports the change with an `R` status prefix, not a `D`+`??` pair;
  if it does not, reset and redo the move with `git mv` rather than editing around it.
  This card needs no file reads: the move is path-level only, which is why `Context:` is `none`.
- **Commit:** `docs(plan-format): rename plan-format-v3.md to plan-format.md`

### Card 2: Author the temporary Go sweeper and run the six-pattern sweep

- **Context:**
  - `.gitignore`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `.scratch/sweep/main.go`
  - `README.md`
  - `docs/overview.md`
  - `docs/reference/model-spec.md`
  - `docs/reference/plan-format.md`
  - `docs/reference/webster-contract.md`
  - `internal/batcher/doc.go`
  - `internal/loomengine/plan-template.md`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/plantemplate.go`
  - `internal/planparser/doc.go`
  - `internal/planparser/normalize.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/sections.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/validate.go`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/integration-template.md`
  - `internal/websterengine/master-template.md`
  - `internal/websterengine/runlevel_test.go`
  - `manifest/designs/loom.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
  - `manifest/designs/webster-parallel-execution.md`
  - `manifest/roadmap.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Overwrite `.scratch/sweep/main.go` — it currently holds a `package main` probe stub whose `main` prints `sweep-probe-ok`;
  replace the whole file.
  Write a `package main` program run as `go run .scratch/sweep/main.go` from the git root.
  **Before writing it, re-derive the hit inventory** as this batch's first action and print it, so the work is bounded by observed reality rather than by the list above:

  ```
  grep -rniE 'plan-format-v3|plan_format_v3|plan-format v3|plan format v3|plan-v3' . -c --exclude-dir=.git --exclude-dir=_mill --exclude-dir=.scratch
  ```

  The sweeper must:
  1. Walk the tree from the git root, skipping the directories `.git`, `_mill`, `.scratch`, and skipping the whole file `manifest/designs/shed-followups.md`.
  2. Process text files only. Skip anything with a NUL byte in its first 8 KiB, and skip every `go.sum`.
  3. Apply this **ordered, longest-pattern-first** replacement table with **case-insensitive** matching, so `plan-format-v3` is consumed before `plan-v3` could partially match it. The leading character's case is preserved from the matched text, so `Plan-format v3` becomes `Plan-format` and `plan-format v3` becomes `plan-format`.

     | pattern | replacement |
     | --- | --- |
     | `plan-format-v3` | `plan-format` |
     | `plan_format_v3` | `plan_format` |
     | `plan-format v3` | `plan-format` |
     | `plan format v3` | `plan format` |
     | `plan-v3` | `plan` |

     Only the leading character carries case; the rest of the replacement is written lowercase as shown.
     There is no separate sixth entry for `Plan-format v3` — case-insensitive matching of `plan-format v3` already covers it, which is why the table has five rows for the six patterns named in the task.
  4. Exclude **line 18 of `manifest/roadmap.md` only**, not the whole file — that one line reads "mechanical rename sweep, `plan-format-v3.md` → `plan-format.md`" and sweeping it collapses both halves to the same name. Match the exclusion on the file's line index or on that line's distinctive `mechanical rename sweep` substring; the file's other five hits are swept normally.
  5. Print one line per changed file with its hit count, plus a total, so the run is auditable and can be pasted into the commit message.

  The program names **no** exclusion for `gopkg.in/yaml.v3`: every pattern above requires a `plan` prefix, so that import string is unmatchable by construction (`yaml-v3-is-structurally-unreachable`).
  Run it once, review the printed file list against the re-derived inventory, and commit the resulting content changes.
  Do **not** stage `.scratch/` — it is gitignored, and passing it to `git add` errors rather than succeeding quietly.
- **Commit:** `docs(plan-format): sweep every plan-format-v3 reference to plan-format`

### Card 3: Confirm the rename and sweep gates

- **Context:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff verification card. Run each gate and report its output;
  if any fails, fix the cause in card 1 or card 2's territory before proceeding — do not paper over a failure here.

  1. Acceptance gate 1 — the move is recorded as a rename. `git log --follow --name-status -1 -- docs/reference/plan-format.md` shows an `R` status for the rename commit.
  2. Acceptance gate 2 — the six-pattern grep returns **zero** lines:

     ```
     grep -rniE 'plan-format-v3|plan_format_v3|plan-format v3|plan format v3|plan-v3' . \
       --exclude-dir=.git --exclude-dir=_mill --exclude-dir=.scratch \
       --exclude=shed-followups.md \
       | grep -v '^\./manifest/roadmap\.md:18:'
     ```

     The exclusions anchor on the path field, never on line content (`exclusions-anchor-on-the-path-field`).
  3. Acceptance gate for the hard exclusion — `grep -rl 'gopkg.in/yaml.v3' --include='*.go' . | wc -l` returns **32**, unchanged from before the sweep.
  4. Acceptance gate 7 — `git status --porcelain` lists nothing under `.scratch/`.
  5. `go build ./...` is clean and the batch `verify:` command is green.

  Note what this card does **not** yet assert: bare `v3` labels and plan-format-**v2** prose still survive at this point by design — no sweep pattern reaches them. Batches 2 and 3 own them, and their gates are asserted there.
- **Commit:** none

## Batch Tests

`verify:` runs `go build ./...` plus the test packages whose files the sweep edits: `internal/planparser`, `internal/webstercli`, `internal/websterengine`, `internal/loomengine`, `internal/batcher`, and `cmd/lyx`.
That is per-batch scoping, not the unbounded suite — the sweep touches no other package, and the whole scoped run completes in about one second.

What each covers against this batch's specific risk:

- `internal/planparser` — `parse_test.go` and `validate_test.go` carry full parse/validate coverage including a golden fixture materialized from the renamed doc's worked example. The fixture is hardcoded in test source rather than read from disk, so the rename cannot break it by path;
  what these tests do catch is a sweep that corrupts the example's structure through an over-broad replacement.
- `internal/webstercli` — `cli_test.go` guards the cobra `Long` strings at `cli.go` and `validate.go`, both of which carry sweep hits, and `cmd/lyx/helptree_test.go` guards the resulting help tree (CLI/Cobra Invariant).
- `internal/websterengine` — `template_test.go` asserts on the embedded prompt templates, two of which (`master-template.md`, `integration-template.md`) carry sweep hits. Its assertions target `oversized`/`chain`/`## Scope`/schema keys, none of which the sweep patterns can reach, so a green run here means the sweep stayed inside its lane.

No new test. The meaningful failure mode is incompleteness, and card 3's grep gates check it directly (`no-new-tests`).
