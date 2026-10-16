# Stencil intent audit — flagged findings

These are the design assumptions in `contracts/stencils/` that look doubtful, picked out of the full list in `catalogue.md`.
Line numbers refer to `main` at `aa925bdcc`.
Each entry says what the stencil assumes and why the auditor doubts it; the operator reviewed them on 2026-10-16, and the table below records the outcome.
The decisions themselves are in the board notes, not here.

| Entry | Outcome | Board note |
|---|---|---|
| F0 | Fixed by `webster-master-no-code` (`675728800`) | none |
| F1 | Fixed by `webster-master-no-code` before the review | none |
| F2 | Fixed by `webster-master-no-code` before the review | none |
| F3 | Rejected; only the frozen inline layout still carried the sentence | `webster-drop-inline-pack` |
| F4 | Rejected | `pattern-change-declared` |
| F5 | Rejected | `webster-fork-spawn-wording` |
| F6 | Rejected | `rework-findings-not-operator` |
| F7 | Rejected | `webster-rename-script` |
| F8 | Rejected | `arbiter-filters-orch` |
| F9 | Rejected | `orch-no-peer-compare` |
| F10 | Confirmed | none |
| F11 | Confirmed: kept for the planned Hardener | none |
| F12 | Rejected, together with F13 | `fork-deviation-in-commit` |
| F13 | Rejected, together with F12 | `fork-deviation-in-commit` |
| F14 | Rejected | `orch-hub-rules` |
| F15 | Rejected | `stencil-wording-fixes` |
| F16 | Rejected | `stencil-wording-fixes` |
| F17 | Rejected | `stencil-wording-fixes` |

The trigger is the operator decision of 2026-10-15 on task `webster-master-no-code`: the Webster master reads only what every fork shares and what stays stable during the run, such as the plan, `CLAUDE.md` and `PATTERN.md`, and never source code, since the forks rewrite the code and the master's reads go stale for them.

## F0 — master reads the codebase up front (already in progress)

- `webster/webster-template-master.md:3`, `:17-18`, `:27-29`, `:216`
- Restatement: the master reads the codebase's structure and conventions once, and every fork inherits those reads; it reads files with Read and Grep.
- Flag reason: the case that started this audit.
  Task `webster-master-no-code` is fixing it, so it is listed for completeness only.

## F1 — fork prefix inherits the codebase orientation

- `webster/webster-prefix-fork.md:4`, `:13`
- Restatement: the fork inherits Master's whole context, "the codebase orientation" included, so its prompt is deliberately thin.
- Flag reason: the same wrong design as F0.
  The `webster-master-no-code` board entry names the fork prefix in its idea, so this is likely covered there.

## F2 — recovery prefix assumes forks hold orientation from Master

- `webster/webster-prefix-recovery.md:3`, `:26`
- Restatement: the cold recovery strand must earn its own orientation, "exactly like an in-session fork already holds it from Master".
- Flag reason: carries the F0 design into the recovery prompt.
  The `webster-master-no-code` entry names only the master template and the fork prefix, so this file may be missed.

## F3 — fresh-read fragments assume Master read code, and the inline ones are frozen

- `webster/webster-fragment-fresh-read-plain.md:7`, `webster-fragment-fresh-read-pack.md:7`, `webster-fragment-fresh-read-pack-card.md:7`, `webster-fragment-fresh-read-pack-inline.md:8`
- Frozen baselines: `webster-fragment-fresh-read-pack-inline.md:4`, `webster-fragment-locate-pack-inline.md:4`, `webster-fragment-pack-section-inline.md:4`
- Restatement: "A file you looked at during orientation — your own, or one Master read before forking you — is not necessarily the version on disk right now."
  The inline-layout fragments are the measurement baseline, whose text "changes only when the layout comparison itself is meant to change".
- Flag reason: under the 2026-10-15 decision Master reads no code, so the sentence describes the old design.
  The frozen inline fragments keep that design by rule, so fixing them needs an operator call on whether the baseline may change.

## F4 — PATTERN.md and CLAUDE.md assumed stable during a run

- `pattern/pattern-directive-orchestrator.md:17-19`, rendered into `webster/webster-template-master.md:23`
- Restatement: the master reads `PATTERN.md` and the `pattern/` background files once, and every fork inherits them as binding constraints.
- Flag reason: the 2026-10-15 decision names `PATTERN.md` and `CLAUDE.md` as stable during the run, but this repository's `CLAUDE.md` requires a new invariant to go into `PATTERN.md` in the same commit as its code, and docs land with the code too.
  A card can therefore change `PATTERN.md` or `CLAUDE.md` mid-run, and every later fork works from the master's stale copy.
  The pattern directive is rendered once from the worktree root at run start, so the rendered text goes stale the same way.

## F5 — stale `await-batch` wording and fork-trigger phrase

- `webster/webster-prefix-fork.md:3`, `:19`, `:24`; `webster/webster-template-master.md:12`
- Restatement: the fork is told Master's `await-batch` sees its report and takes over; the master prompt identifies a fork by the instruction "Read this file and follow it exactly: <path>".
- Flag reason: `internal/websterengine/doc.go:412` says `await-batch` no longer sits in Master's loop, which now waits on the fork's completion notification.
  Master sends a different instruction, "Read this file and do exactly and only what it says" (`webster-template-master.md:71`, `:166`), so the trigger phrase at `:12` matches nothing Master sends.

## F6 — PR-Rework calls the rejection findings the operator's

- `loom/loom-template-rework.md:12`, `:29`
- Restatement: the rework writer reads "the operator's findings on a rejected pull request".
- Flag reason: a PR rejection comes from the alignment agent, whose report becomes the findings (`internal/landingshed/pralign.go`, `applyReject`), or from the orch's reviewer fork; `orch/orch-template-role.md:113` says the orch decides at PR-Gate, never the operator.
  A writer told the findings are the operator's may weigh an agent's claim as an operator decision.

## F7 — edit directive bans the rename script the spec and rubric require

- `edit/edit-directive.md:6`; `loom/loom-rubric-webster-review.md:79`; `contracts/specs/loom-plan-spec.md:125`, `:334`
- Restatement: the edit directive forbids any script that rewrites files; the plan spec makes a `Rename` an AST-aware script plus a grep, and the Webster-Review rubric asks the reviewer to confirm that check ran and passed.
- Flag reason: the two rules conflict, the fork prompt never tells a fork to run such a script, and a diff review cannot show whether a script ran.

## F8 — the review-escalation rule lives in three places

- `orch/orch-template-role.md:54-56`; `loom/loom-template-arbiter.md:63-66`; `bouncer/bouncer-template-escalation.md:31-39`
- Restatement: the orch accepts a hand-on "unless a finding makes the design wrong" and hands "a blocker that remains after round 3" to the operator; the arbiter accepts or continues by its own cause rules; the escalation fork settles "on the materials" or reports back.
- Flag reason: one decision is worded three ways, and the orch's fixed "round 3" ignores the configurable hard cap.

## F9 — orch reads both Discussions itself at parent-review

- `orch/orch-template-role.md:94`
- Restatement: for related runs the orch reads both Discussions itself at parent-review, pauses both after Plan-Write to compare plans, and reviews both PRs side by side.
- Flag reason: conflicts with `:40-45` of the same file, where the alignment agent settles parent-review, any escalated review goes to a one-shot fork and the orch's context grows only by the fork's summary; the alignment agent already checks overlap with peer runs.

## F10 — orch reviewer forks inherit the orch's context on purpose

- `orch/orch-template-role.md:43`
- Restatement: reviewer forks inherit the orch's context deliberately, since the orch knows the operator's settled decisions best.
- Flag reason: inherited context, the same mechanism F0 rejected for Webster.
  The orch holds decisions rather than code, so it may be sound; listed so the operator confirms it.

## F11 — treadle judge stencils are unreachable in production

- `treadle/treadle-template-judge-circling.md`, `treadle/treadle-template-judge-milestone.md`, `treadle/treadle-template-targeting.md`
- Restatement: a treadle progress judge reads review files and a handoff ledger and writes a circling or milestone verdict.
- Flag reason: no production package imports `internal/treadleengine`, which reads these stencils; only tests reach them.
  Their ledger and circling rules duplicate the Bouncer judge's in `bouncer/bouncer-template-judge.md`.

## F12 — who reads the fork's report and reply

- `webster/webster-body-implementer.md:93`, `:99`; `webster/webster-template-master.md:101`, `:243-244`; `webster/webster-body-verify-fix.md:53`
- Restatement: the implementer body says "Master reads ONLY these two fields" of the report and tells the fork to name an unrelated failure "in your final reply to Master"; the master never reads raw fork output and says forks write no prose; the verify-fix fork's reply is the one thing Merriam reads.
- Flag reason: Go reads the report through `record-batch`, not Master; and Master is told both that fork replies carry content and that it never reads them.

## F13 — "say so in your report" with no place to say it

- `pattern/pattern-directive-implementer.md:19`
- Restatement: when a constraint conflicts with the prompt, the constraint wins, and the agent says so in its report.
- Flag reason: the directive reaches the Webster recovery strand, whose report holds only `status` and `head_sha`.

## F14 — orch role names `update-plugins.sh`

- `orch/orch-template-role.md:131`, `:135`
- Restatement: while the hub's landing `deploy` key is empty, the orch runs `update-plugins.sh` by hand after a landing, and compares `origin/prod` with `origin/main` at every wake.
- Flag reason: the stencil is hub-generic but names this repository's script, and this repository's `CLAUDE.md` lets the orch run it only after its own pushed fix.
  Dormant in this hub, whose `landing.yaml` sets `deploy`.

## F15 — the operator pushes a hand fix in a task pair

- `shed/shed-template-driver.md:67`
- Restatement: a fix committed by hand outside the run is pushed by the operator before `approve`.
- Flag reason: `orch/orch-template-role.md:99` forbids editing or committing in either worktree of a task pair, so this route has no owner.

## F16 — the discussion advisor reads a record that does not exist yet

- `loom/loom-template-discussion-advisor.md:10`
- Restatement: the advisor reads the decision record "for its format".
- Flag reason: the chair writes the record at the end of the step, after the advisors start, and the format lives in `loom-discussion-spec.md`.

## F17 — the plan writer "never asks" but asks its parent

- `loom/loom-template-plan.md:15`, `:31`
- Restatement: the Plan producer never interviews and never asks, yet a late scope addition is planned only once it asks its parent to record it.
- Flag reason: the two sentences contradict each other; a minor wording fix.
