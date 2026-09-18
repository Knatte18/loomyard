# Plan repair: card-numbering collision + cross-batch build break

You are working in the `reed-header-selvage` task worktree at
`/home/knatte/Code/loomyard/wts/reed-header-selvage` (a git worktree, branch `reed-header-selvage`,
parent `main`). Stay inside this worktree — never touch any other worktree.

This is a **manual repair task dispatched directly by the human operator**, outside mill-go's normal
per-batch pipeline. You are not being driven by `millpy-implement.py`, there is no `--stage finalize`
waiting for your JSON status line, and you must **not** touch `_mill/status.md` — the orchestrator
handles that afterward based on your report.

Read `CLAUDE.md` and `CONSTRAINTS.md` at the repo root before touching anything.

## Background

mill-go was executing an approved 7-batch plan at `_mill/plan/`. Batch 1 (`render-bottom-band`) and
batch 2 (`vocabulary-and-config`) both had their cards implemented and committed successfully. Batch
2's implementer correctly renamed `reedengine.Engine.HeaderText`/`ValidateHeader` to
`StatusLineText`/`ValidateStatusLine` as card 12/13 required.

That rename broke `go build ./...` for the whole module, because the only caller,
`internal/reedcli/header.go:103` (`text, err := c.eng.HeaderText()`), is **not** touched until batch 5
(`watchdog-daemon`, card 28: "replace the header verb with statusline") — confirmed by
`_mill/plan/03-selvage-pane.md`'s own text: "Leave the `reedcli/header.go`-referencing SIGWINCH and
stdout/stderr entries alone in this card — they are batch 5's, since the verb they name is renamed
there." So the plan, as written, leaves the full module unbuildable for batches 2, 3, and 4.

mill-go enforces a module-wide `go build ./...` gate at every batch's finalize (the overview's
top-level `verify: go build ./...` field), baseline-aware against a pre-task snapshot that was
`clean`. That gate correctly caught this and mill-go attempted to self-resolve by adding a new "keep
the build green" card to batch 2 — but `_plan_validate.compute_next_card_number` raised `PlanDAGError`:
card 15 is already used by `03-selvage-pane.md` (batch 2 currently owns cards 7–14, batch 3 starts
immediately at 15, no room to insert). Per mill-go's own protocol, a numbering collision escalates
immediately rather than being worked around automatically — that's why this is now a manual repair.

## Card numbering rules (from `_plan_validate.py`, already confirmed by reading the source)

- Within one batch file, card numbers must be sequential with **no gaps** (e.g. 7,8,9,...,14 — never
  7,8,10).
- No card number may appear in two different batch files (global uniqueness).
- Consequence: batches occupy contiguous, disjoint numeric ranges, in batch order. There is no way to
  insert a 9th card into batch 2 (currently 7–14) without shifting every subsequent batch's card
  numbers up by one, since batch 3 already starts at the very next integer (15).

## Your task

### 1. Renumber every card ≥ 15 up by one, across every plan file

Current ranges: batch 3 = 15–22, batch 4 = 23–26, batch 5 = 27–40, batch 6 = 41–43, batch 7 = 44–55.
After the shift they become: batch 3 = 16–23, batch 4 = 24–27, batch 5 = 28–41, batch 6 = 42–44,
batch 7 = 45–56.

Do this in **descending order** (56←55, 55←54, ..., 16←15) so a number is never touched twice by a
naive find-and-replace pass. Two things need updating for every shifted number:

- The `### Card N: <title>` heading itself, in `_mill/plan/03-selvage-pane.md` through
  `_mill/plan/07-docs-smokes-and-residue.md`.
- Every **prose cross-reference** to that card number anywhere in `_mill/plan/*.md` — these exist and
  must not be missed. A non-exhaustive list found by grep (re-grep yourself before finishing, do not
  trust this list as complete):
  - `_mill/plan/00-overview.md`: "Card 46 in `docs-smokes-and-residue`", "card 34 in `watchdog-daemon`",
    "(card 37)", "(card 38)", "(card 51)", "(card 49, ...)".
  - `_mill/plan/02-vocabulary-and-config.md`: "this is the one sentence card 8 deliberately left
    alone", "Do not rename `st.HeaderPaneID` ... both are batch 3's" (no number, leave alone), "card
    10 removed `Config.Header`".
  - `_mill/plan/04-status-line-pins.md`: "into the shipped design doc (card 46)".
  - `_mill/plan/05-watchdog-daemon.md`: "the integration test's subject in card 40", "card 28's and
    card 32's non-empty `Short` requirements", "card 36's `watchdog_test.go`", "Card 40's
    integration-tagged assertions".
  - `_mill/plan/06-standalone-watcher.md`: "card 41's field-type change".
  - `_mill/plan/07-docs-smokes-and-residue.md`: "card 46's subject", "the Selvage spellings card 19
    introduced", "beside the Selvage assertions card 50 put there", "allowlist card 53 shrinks",
    "card 33's `suppressWatchdogSpawn`", "card 49's rewrite of `SANDBOX-REED-SUITE.md`", "Card 55 runs
    no test of its own".

  Every one of these numbers is ≥ 15 and must shift by exactly +1 to keep pointing at the same card.

- `_mill/status.md` line 54's `blocked_reason` text also mentions "card 15" — leave that file alone,
  the orchestrator will overwrite it.

### 2. Insert the new card into batch 2

After the shift, batch 2 (`02-vocabulary-and-config.md`) still legitimately ends at card 14. Append a
new **Card 15** to its `## Cards` list (after card 14, before any following section), following the
file's existing card format (`Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:`/`Requirements:`/
`Commit:`, matching the style of the surrounding cards). Bump that file's frontmatter `cards: 8` to
`cards: 9`.

The card's scope is deliberately minimal — it must **not** duplicate batch 5's card 29 (was 28,
"replace the header verb with statusline"), which still owns the full rename: `git mv
internal/reedcli/header.go internal/reedcli/statusline.go`, renaming the `header` verb to
`statusline`, deleting the `--blocking` branch, `headerBlockingPayload`, `headerWatch`, `headerPark`,
etc. Card 15 here fixes **only** the compile break so the module builds again in the interim:

- **Edits:** `internal/reedcli/header.go`
- **Requirements:** change line 103's `text, err := c.eng.HeaderText()` to
  `text, err := c.eng.StatusLineText()`. Confirm no other call site in that file (or elsewhere outside
  batch 5's scope) references the old method names — `grep -rn "ValidateHeader\|\.HeaderText(" internal/`
  outside test files and outside `internal/reedengine/` (which batch 2 already retargeted). Do not
  rename the file, the verb, or touch anything else in `header.go` — that is explicitly batch 5's
  scope and must stay untouched here to avoid a merge/scope conflict when batch 5 runs.
- **Commit:** something in the style of the existing card commits (see `git log --oneline -5 --
  internal/tokenvocab/tokenvocab.go internal/reedengine/geometry.go` for examples), e.g.
  `fix(reedcli): update header.go's stale HeaderText call for the status-line rename`.

Actually make this edit, run `go build ./...` from the repo root and confirm it now succeeds, then run
batch 2's own verify command (`go test ./internal/tokenvocab/ ./internal/reedengine/
./internal/hubgeom/ ./internal/standalonegeom/ ./internal/configsync/`) and confirm it still passes.
Commit this single-card change on the current branch (`reed-header-selvage`) with a plain `git commit`
(do not push — per this repo's Board discipline, per-card implementer commits do not push; mill-merge
pushes the full branch at task end).

### 3. Record the self-resolve in batch 2's plan file

Append a `## Prior failure` section to `02-vocabulary-and-config.md`, immediately after its
frontmatter (before `## Rename mechanic`), with one bullet:

```
## Prior failure

- Round 1: module-wide verify failed after batch 2's finalize — `go build ./...` broke on
  `internal/reedcli/header.go:103:23: c.eng.HeaderText undefined (type *reedengine.Engine has no
  field or method HeaderText)`, because the only caller of the renamed `Engine.HeaderText` method is
  not touched until batch 5's card 29. Resolved by inserting card 15 (a minimal one-line compat fix)
  into this batch rather than by editing batch 5, so the module builds again without duplicating batch
  5's full rename scope.
```

Commit this together with the plan-file renumbering from step 1 (a separate commit from the code
fix in step 2 is fine — two commits total is expected: one for the plan-file edits, one for the code
fix — or combine them if that reads more cleanly; your call).

### 4. Validate everything before reporting back

Run, from the repo root:

```
PYTHONPATH="$CLAUDE_PLUGIN_ROOT/scripts" "$MILL_PYTHON" -c "
from pathlib import Path
import _plan_dag, _plan_validate

plan_dir = Path('_mill/plan')
batch_files = sorted(p for p in plan_dir.glob('??-*.md') if p.name != '00-overview.md')
errors = _plan_validate._check_card_numbering(batch_files)
print('numbering errors:', errors)

overview_text = Path('_mill/plan/00-overview.md').read_text(encoding='utf-8')
batches = _plan_dag.extract_batch_index(overview_text)
_plan_dag.validate(batches, [p.name for p in batch_files])
print('DAG validation: OK')
"
```

Both must come back clean (empty `errors` list, no exception). Then run `go build ./...` and batch 2's
verify command one more time from a fully clean state to be sure, and `git status` to confirm the tree
is clean (only your intended commits, nothing stray).

## Report back

End your final message with a concise plain-text report (no JSON envelope needed — this isn't going
through the CLI finalize path) covering:

- Every plan file you edited and a one-line summary of what changed in each.
- The exact new card numbering scheme confirmed (batch 3 now 16–23, etc.).
- The commit SHA(s) you made, and their messages.
- Confirmation that `_check_card_numbering` and `_plan_dag.validate` both passed clean.
- Confirmation that `go build ./...` and batch 2's verify command both pass.
- Anything you found that doesn't fit this brief's description (e.g. an additional stray card-number
  reference not listed above, or a further affected call site) — do not silently skip it, fix it and
  say so.
