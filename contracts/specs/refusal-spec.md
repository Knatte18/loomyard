# Refusal contract: every refusal and its way forward

> **Status: Contract — kept on landing.** This is the table of refusals that `lyx webster`, `lyx shed` and `lyx loom` can reach, each with the way forward its message names, a durable reference doc, not deleted on landing.
> It is the lookup the ly-drive driver and the operator use for an escalation, and it is plain markdown, not registered in `contracts/specs/specs.go`.

## What a way forward is

A refusal has a way forward when its message names one of these:

1. a `lyx` verb that makes progress from the refused state;
2. a transition the run takes by itself: a re-step, the next Master retry, a failure ladder rung;
3. a plain `git` command on the warp worktree, when no `lyx` verb covers it.

Hand-editing a lyx-owned state file (`state.json`, `status.json`, `seed.json`, report files) never counts, and neither does a code fix plus a deploy.
Waiting for a lock holder counts when the message names how to see the holder (`status`).
The message carries it as a trailing `way forward: <verb | transition | git command>` clause, and the Way forward cell of each row quotes that clause, so the cell and the message cannot disagree.

## Classes

Every refusal sits in one class, and the class fixes its disposition.

| Class | Meaning | Disposition |
|---|---|---|
| correctness halt | proceeding could land wrong code, forge a judgment, or corrupt run state | halt; the message names the way forward |
| policy guard | steering or hygiene; proceeding lands nothing wrong | warn and record; never halt |
| transient | a condition that clears by itself or by a named action: lock held, merge in progress, I/O | halt without mutating state; the message names the retry |
| wiring guard | nil deps, an invalid producer list, empty paths; unreachable from any on-disk state a run can produce | one grouped row per section; no per-row test |

Raw I/O failures (`stat`, `mkdir`, `write`) form one grouped transient row per section.

## webster

A validation verb's findings envelope (`lyx webster validate`) is that verb's verdict on the plan, not a refusal, so it has no row here.
The audit rows below follow the fork-audit severity split: a fork-contract write, and a parent write into the warp's tracked tree or under the run's `_lyx`, are correctness; every other finding is policy.
A finding is dispositioned once per run, by the first `record-batch` or run-exit audit that reports it.

| Refusal | Trigger | Class | Way forward |
|---|---|---|---|
| unknown batch | a batch verb names a number the plan's execution batches do not contain | correctness halt | `lyx webster status` lists the run's batches, name one of those |
| plan drifted | begin-batch's re-resolution of the plan against the tree finds a blocking defect | correctness halt | edit the plan so the named cards match the tree, run `lyx webster rebaseline`, then begin-batch again |
| rebaseline card set changed | `lyx webster rebaseline` finds a begun batch's cards removed or changed | correctness halt | restore those cards in the plan, or reset the branch to the run's start commit with git and run `lyx webster run --fresh` |
| plan fingerprint mismatch | begin-batch, record-batch or run sees a plan edited since the run recorded it | correctness halt | `lyx webster rebaseline` accepts the edit and keeps the batch records |
| model switch injection failed | begin-batch cannot assert the batch's model | transient | transient, re-run `lyx webster begin-batch NN` |
| report malformed | record-batch cannot decode the batch's report | correctness halt | `lyx webster recover-batch NN` archives the malformed report and re-drives the batch |
| report unattributable | record-batch finds no begin record, zero fork transcripts, or no transcript directory | correctness halt | `lyx webster begin-batch NN` re-drives the batch; the report is archived |
| batch already terminal | record-batch names a batch already `done` or `failed` | correctness halt | the message names the verb for the batch's state: move on when done, `lyx webster recover-batch NN` when failed |
| audit: fork-contract-write | a fork's transcript wrote one of the run's two contract files | correctness halt | `lyx webster recover-batch NN`; the batch fails with its report archived |
| audit: parent-write, tracked or `_lyx` | Master wrote into the warp's tracked tree or under the run's `_lyx` | correctness halt | `lyx webster recover-batch NN` at record-batch; at run exit, revert or re-derive the named paths on the warp with git, then re-step the Webster row (`lyx webster run`) |
| audit: parent-write, elsewhere | Master wrote a path outside the worktree, or a git-ignored warp path outside `_lyx` | policy guard | warn and record once, after the card verify commands re-run and pass; a failing verify makes the finding correctness |
| audit: fabric-reference | a fork or Master ran a fabric-referencing command | policy guard | warn and record once; no action needed |
| audit: named-spawn | Master spawned a named subagent | policy guard | warn and record once; no action needed |
| audit: nested-agent | a fork attempted an Agent call | policy guard | warn and record once; no action needed |
| card not done | the batch's own done-checks find declared work missing | correctness halt | `lyx webster recover-batch NN`; the batch fails with its report archived |
| later-card drift | a done-check finding concerns a later card, not the batch's own | policy guard | warn and record once; the later card's own begin-batch re-checks it |
| recovery terminal, no record | `recover-batch` persists a terminal with no recorded state for the batch | transient | re-run `lyx webster recover-batch NN` |
| recovery strand start failed | `recover-batch` cannot start its strand | transient | transient, re-run `lyx webster recover-batch NN` |
| run busy | another `lyx webster run` holds `run.lock` | transient | wait for it to finish, or check `lyx webster status` |
| plan not approved | the plan's frontmatter `approved:` is not true | correctness halt | approve the plan through its review, then re-run `lyx webster run` |
| zero execution batches | the plan produced no batch | correctness halt | fix the plan's cards, run `lyx webster rebaseline` when state.json already records the run, then re-run `lyx webster run` |
| plan validation refused | validation of the plan finds blocking findings | correctness halt | fix the named cards in the plan, run `lyx webster rebaseline` when state.json already records the run, then re-run `lyx webster run` |
| quarry unavailable | quarry cannot answer plan validation | transient | transient, re-run `lyx webster run` once quarry answers |
| Master start failed | the Master session cannot start | transient | transient, re-run `lyx webster run` |
| Master ended early | Master ends without a valid outcome, or `outcome.yaml` is malformed or stale | correctness halt | re-run `lyx webster run`; a fresh Master resumes from state.json and the stale file is archived |
| run-exit audit correctness | Master reports done over an undispositioned correctness finding | correctness halt | revert or re-derive the named paths on the warp with git, then re-step the Webster row (`lyx webster run`); done is demoted to stuck |
| fabric sync failed | a bracket verb cannot commit its state to the fabric | transient | the state is saved locally; `lyx fabric commit` commits it, or the next bracket verb's own sync carries it |
| re-baseline persist failed | `validate` or a bracket verb cannot persist the plan-fingerprint re-baseline | transient | re-run the same verb, or `lyx webster rebaseline` |
| standalone reed boot failed | the standalone reed session does not come up | transient | transient, re-run the verb |
| wiring guards | nil deps, empty paths and an invalid batcher or geometry, unreachable from any on-disk state a run can produce | wiring guard | none per row; grouped |
| raw I/O | `stat`, `mkdir`, `read` or `write` of a run file fails | transient | re-run the refused verb; nothing is mutated |

## shed

| Refusal | Trigger | Class | Way forward |
|---|---|---|---|
| shed busy | another driver holds the run lock, at `run`, `step` or `goto` | transient | `lyx shed pause` asks the live driver to stop at its next producer boundary; check the holder with `lyx shed status`, then retry |
| status file missing | `step` finds no status file; Shed never seeds one | correctness halt | seed the run through its recipe's bootstrap verb, or `lyx shed seed` for a recipe without one |
| current producer missing | the status file's `current_producer` names no row in the list | correctness halt | `lyx shed goto --to <producer>` moves the run onto a row that exists |
| bounce budget exhausted | a segment's bounce budget runs out and the run halts Stuck | correctness halt | `lyx shed goto --to <row>` gives the segment a fresh budget |
| goto on a done run | `goto` names a run whose status is done | correctness halt | seed a new run |
| goto target unknown | `goto` has no `--to`, or the target names no producer | correctness halt | name a valid target; the message lists them |
| seed disagrees | `seed` finds the run-id already seeded with different values | correctness halt | keep the existing seed and drive it (`lyx shed status <run-id>` shows it), or address a different run-id |
| seed refuses here | the seed verb runs outside the worktree the recipe drives | correctness halt | run `lyx shed seed` from the worktree the recipe drives, which the cause names |
| llm driver without bootstrap | `--driver llm` on a recipe with no bootstrap verb | correctness halt | re-run with `--driver go` |
| wiring guards | nil deps, an invalid producer list, empty paths | wiring guard | none per row; grouped |
| raw I/O | `stat`, `mkdir`, `read` or `write` of a status, seed or lock file fails | transient | re-run the refused verb; nothing is mutated |

## loom

`lyx loom` drives the same shed verbs, so the shed section's rows apply to it unchanged, with `lyx loom` in place of `lyx shed`.
The `validate-*` verbs' findings envelopes are each verb's verdict on its artifact, not refusals, so they have no row here.

| Refusal | Trigger | Class | Way forward |
|---|---|---|---|
| approve: not at Publish | `lyx loom approve` runs while the run is not awaiting or blocked at Publish | correctness halt | `lyx loom status` shows where the run is; approve once it halts at Publish |
| approve: no pull request | no pull request from the task branch to its parent exists | correctness halt | `lyx loom step` opens it at Publish |
| approve: pull request not open | the pull request is closed or merged | correctness halt | `lyx loom step` opens a new one at Publish |
| approve: HEAD differs | the local task HEAD differs from the pull request's head | correctness halt | push or sync the task branch (`lyx fabric push`), then re-run `lyx loom approve` |
| approve: lookup failed | branch resolution, the origin URL, the pull request lookup or the HEAD read fails | transient | transient, re-run `lyx loom approve` |
| commit-records: probe or commit failed | the merge-state probe or the commit fails | transient | transient, re-run `lyx loom commit-records` |
| commit-records: not pushed | the commit landed locally but the push failed | transient | `lyx fabric push` pushes the landed commit, or re-run `lyx loom commit-records` |
| commit-records: park marker failed | the park marker cannot be written | transient | transient, re-run `lyx loom commit-records --park <park>` |
| start: driver took no lock | the detached driver exits without taking the run lock | transient | `lyx loom start`; the message names the driver log |
| Loom-Preflight half-finished | the preflight finds a half-finished run | correctness halt | `lyx loom goto --to Discussion-Write` resumes the run past Loom-Preflight, or seed a new run |
| preflight preconditions unmet | the worktree is dirty, fabric is unsynced or a junction is broken | correctness halt | commit or stash the warp with git and `lyx fabric commit` the weft; `lyx fabric checkout` re-syncs; `lyx fabric reconcile` repairs a junction; then re-step |
| invalid history outcome | the status file's history carries an outcome the coherence check does not know | correctness halt | seed a new run |
| batcher misconfigured | `batcher.yaml`'s `active:` key names no batchifier | correctness halt | fix `batcher.yaml`'s `active:` key, then re-step |
| produced artifacts commit failed | the Discussion-Write, Plan-Write or Webster row cannot commit its records | transient | the fault is transient, re-step the row |
| wiring guards | nil deps, an invalid producer list, empty paths | wiring guard | none per row; grouped |
| raw I/O | `stat`, `mkdir`, `read` or `write` of a status, seed, lock or records file fails | transient | re-run the refused verb; nothing is mutated |

## Out of scope

Landing, batten, orch, fabric, burler and board refusals are not in this table; a later audit adds each as its own section.
The fast-forward merge-in limit is out of scope too, since its message already names a git recourse.
