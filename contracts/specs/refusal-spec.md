# Refusal contract: every refusal and its way forward

> **Status: Contract — kept on landing.** This is the table of refusals that `lyx webster`, `lyx shed` and `lyx loom` can reach, each with the way forward its message names, a durable reference doc, not deleted on landing.
> It is the lookup the ly-drive driver and the operator use for an escalation, and it is plain markdown, not registered in `contracts/specs/specs.go`.

## What a way forward is

A refusal has a way forward when its message names one of these:

1. a `lyx` verb that makes progress from the refused state;
2. a transition the run takes by itself: a re-step, the next Master retry, a failure ladder rung;
3. a plain `git` command in the task worktree, when no `lyx` verb covers it.

Hand-editing a lyx-owned state file (`state.json`, `status.json`, `seed.json`, report files) never counts, and neither does a code fix plus a deploy.
Waiting for a lock holder counts when the message names how to see the holder (`status`).
The message carries it as a trailing `way forward: <verb | transition | git command>` clause, and the Way forward cell of each row quotes that clause, so the cell and the message cannot disagree.
A message that already named its way forward before this audit keeps its own wording, and its cell quotes that wording instead.

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
The audit rows below follow the fork-audit severity split: a fork-contract write, a fork plan write, a fork write under webster's run directory other than the fork's own report, every fabric reference, and a parent write into the task worktree's tracked content, under the run's `_lyx` or `.lyx` state directory, under webster's scratch directory, or into another worktree of the task repository are correctness;
every other finding is policy.
A finding is dispositioned once per run, by the first `record-batch` or run-exit audit that reports it.
The audit sees writes made through the Write, Edit and NotebookEdit tools only, so a write made through Bash is caught only when its command references the fabric repo;
the audit catches slips, not an adversary.
A correctness halt clears only on evidence that HEAD and every suspect path match what the run recorded, never on an acknowledgement.

| Refusal | Trigger | Class | Way forward |
|---|---|---|---|
| unknown batch | a batch verb names a number the plan's execution batches do not contain | correctness halt | `lyx webster status` lists the run's batches, name one of those |
| plan drifted | begin-batch's re-resolution of the plan against the tree finds a blocking defect | correctness halt | edit the plan so the named cards match the tree, run `lyx webster rebaseline --card NN` naming each edited card, then begin-batch again |
| rebaseline card set changed | `lyx webster rebaseline` finds a begun batch's cards removed or regrouped, or a begun card whose content changed | correctness halt | restore those cards in the plan, or reset the branch to the run's start commit with git and run `lyx webster run --fresh` |
| rebaseline unnamed card changed | a card file changed since the run recorded the plan and `--card` does not name it | correctness halt | re-run `lyx webster rebaseline` naming every changed card with `--card NN`, or restore the unnamed cards |
| rebaseline overview changed | `00-overview.md` changed since the run recorded the plan | correctness halt | restore `00-overview.md`, or reset the branch to the run's start commit with git and run `lyx webster run --fresh` |
| rebaseline bad card number | a `--card` value is not a positive integer | correctness halt | re-run `lyx webster rebaseline` naming each changed card by its number, such as `--card 5` or `--card 05` |
| plan fingerprint mismatch | begin-batch or run sees a plan edited since the run recorded it | correctness halt | if the edit keeps every begun batch's cards, run `lyx webster rebaseline --card NN` for each changed card to accept it, otherwise reset the branch to the run's start commit and run `lyx webster run --fresh`; when `00-overview.md` changed, restore it or take the `--fresh` route; or restore the plan the run recorded with `lyx webster restore-plan` |
| plan changed before the verb's own rewrite | `validate`, `record-batch` or `recover-batch` finds the plan changed since the run recorded it, or a begun batch's card changed since it began | correctness halt | the `plan fingerprint mismatch` way forward: `lyx webster rebaseline --card NN` for each changed card, `lyx webster restore-plan`, or reset the branch to the run's start commit and run `lyx webster run --fresh` |
| no run in progress | a bracket verb, `rebaseline`, `restore-plan` or `accept-audit` runs before `lyx webster run` has created state.json | correctness halt | run `lyx webster run` first |
| recorded commit missing | `accept-audit` or `run --fresh` finds a batch commit the run recorded missing from the repository | correctness halt | fetch the task branch from the machine that ran those batches with git, then re-run the verb |
| restore-plan: copy missing | `lyx webster restore-plan` finds a plan file to restore with no stored copy | correctness halt | reset the branch to the run's start commit with git and run `lyx webster run --fresh` |
| model switch injection failed | begin-batch cannot assert the batch's model | transient | transient, re-run `lyx webster begin-batch NN` |
| report already present | begin-batch finds a report for the batch it would open | correctness halt | `lyx webster record-batch NN` consumes it (`lyx webster recover-batch NN` for a recovery batch), and a stuck batch escalates via `lyx webster recover-batch NN` |
| report malformed | record-batch cannot decode the batch's report | correctness halt | `lyx webster recover-batch NN` archives the malformed report and re-drives the batch |
| report unattributable | record-batch finds no begin record, zero fork transcripts, or no transcript directory | correctness halt | `lyx webster begin-batch NN` re-drives the batch; the report is archived |
| batch already terminal | record-batch names a batch already terminal | correctness halt | continue with the next batch when it is `done`, `lyx webster recover-batch NN` when it is `failed`, `stuck` or `dead` |
| recovery batch at record-batch | record-batch names a batch recover-batch opened | correctness halt | its report is consumed by `lyx webster recover-batch NN` |
| merge in progress | record-batch or recover-batch finds a git merge in progress in the worktree | transient | conclude it first (`lyx fabric merge --continue` / `lyx fabric merge --abort` in a hub, `git merge --continue` / `git merge --abort` for a standalone run) |
| HEAD moved past the report | a non-merge commit sits between the report's `head_sha` and HEAD | correctness halt | move HEAD back to the report's `head_sha` with git, then re-run the verb |
| unqualified merge after the report | a merge commit between the report's `head_sha` and HEAD is not a clean merge of the run's parent branch | correctness halt | move HEAD back to the report's `head_sha`, re-run the verb, and redo the parent merge-in after the batch is recorded |
| accept-audit: HEAD past last batch head | `lyx webster accept-audit` finds HEAD carries a commit past the last batch head other than a clean parent merge | correctness halt | move HEAD back to the head with git, then re-run `lyx webster accept-audit` |
| audit: fork-contract-write | a fork's transcript wrote one of the run's two contract files | correctness halt | `lyx webster recover-batch NN`; the batch fails with its report archived |
| audit: fork-plan-write | a fork's transcript wrote under the run's plan directory | correctness halt | `lyx webster recover-batch NN`; the batch fails with its report archived and the plan file named as a suspect path; at run exit, `lyx webster restore-plan` (or `lyx webster rebaseline --card NN` for a card no batch has begun), then run `lyx webster accept-audit` |
| audit: fork-state-write | a fork's transcript wrote under webster's run directory other than its own report | correctness halt | at record-batch the batch fails, even over a record already terminal, and `recover-batch` refuses toward the `--fresh` route since the path cannot be checked; at run exit, reset the branch to the run's start commit with git and run `lyx webster run --fresh`, and re-step the Webster row (lyx webster run) |
| audit: parent-write, tracked, `_lyx`, `.lyx` state, scratch or another worktree | Master wrote into the worktree's tracked content, under the run's `_lyx` or `.lyx` state directory, under webster's scratch directory, or into another worktree of the task repository | correctness halt | `lyx webster recover-batch NN` at record-batch, which refuses toward the `--fresh` route when a finding has no path or a path recovery cannot check; at run exit, restore the named paths to the last batch head with git, for a path under the plan directory `lyx webster restore-plan` (or `lyx webster rebaseline --card NN` for a card no batch has begun), then run `lyx webster accept-audit`; for a finding with no path, reset the branch to the run's start commit with git and run `lyx webster run --fresh`, and re-step the Webster row (lyx webster run) |
| audit: parent-write, elsewhere | Master wrote a path outside every worktree of the task repository, or a git-ignored path in the task worktree outside `_lyx`, `.lyx` and the scratch directory | policy guard | warn and record once; at record-batch after the card verify commands re-run and pass, a failing verify making the finding correctness; at run exit with no re-run, the integration stage covering a plan with a `## verify:` section |
| audit: fabric-reference | a fork or Master ran a command that references the fabric repo | correctness halt | `lyx webster recover-batch NN` at record-batch, which refuses toward the `--fresh` route when a finding has no path or a path recovery cannot check; at run exit, restore the named paths to the last batch head with git, for a path under the plan directory `lyx webster restore-plan` (or `lyx webster rebaseline --card NN` for a card no batch has begun), then run `lyx webster accept-audit`; for a finding with no path, reset the branch to the run's start commit with git and run `lyx webster run --fresh`, and re-step the Webster row (lyx webster run) |
| audit: named-spawn | Master spawned a named subagent | policy guard | warn and record once; no action needed |
| audit: nested-agent | a fork attempted an Agent call | policy guard | warn and record once; no action needed |
| card not done | the batch's own done-checks find declared work missing | correctness halt | `lyx webster recover-batch NN`; the batch fails with its report archived |
| later-card drift | a done-check finding concerns a later card, not the batch's own | policy guard | warn and record once; the later card's own begin-batch re-checks it |
| recover-batch over an OK report | `recover-batch` finds a report with `status: OK` for a batch that is not terminal `dead` or `failed` | correctness halt | record it with `lyx webster record-batch NN` instead |
| recovery terminal, no record | `recover-batch` persists a terminal with no recorded state for the batch | transient | re-run `lyx webster recover-batch NN` |
| recovery state vanished | state.json disappears during `recover-batch`'s wait | transient | re-run `lyx webster recover-batch NN` |
| recovery strand start failed | `recover-batch` cannot start its strand | transient | transient, re-run `lyx webster recover-batch NN` |
| run busy | another `lyx webster run` holds `run.lock` | transient | wait for it to finish, or check `lyx webster status` |
| plan-dir override at run | `lyx webster run` is given a `--plan-dir` override | correctness halt | place the plan at the default plan directory the message names and re-run without `--plan-dir` |
| plan not approved | the plan's frontmatter `approved:` is not true | correctness halt | approve the plan through its review, then re-run `lyx webster run` |
| zero execution batches | the plan produced no batch | correctness halt | fix the plan's cards, run `lyx webster rebaseline --card NN` naming each edited card when state.json already records the run, then re-run `lyx webster run` |
| plan validation refused | validation of the plan finds blocking findings | correctness halt | fix the named cards in the plan, run `lyx webster rebaseline --card NN` naming each edited card when state.json already records the run, then re-run `lyx webster run` |
| quarry unavailable | quarry cannot answer plan validation | transient | transient, re-run `lyx webster run` once quarry answers |
| Master start failed | the Master session cannot start | transient | transient, re-run `lyx webster run` |
| Master ended early | Master asks instead of finishing, its pane dies or it times out | correctness halt | re-run `lyx webster run` (re-step the Webster row); a fresh Master resumes from state.json |
| outcome malformed | `outcome.yaml` is malformed or stale at run exit | correctness halt | re-run `lyx webster run`; the stale file is archived and a fresh Master writes a new one |
| run-exit check failed | Master reports done but summary.md is missing or malformed, a batch lacks a done record, the fork audit is missing or short, or the integration report never landed | correctness halt | re-run `lyx webster run`; a fresh Master resumes from state.json and re-drives every batch without a done record |
| run-exit audit correctness | Master reports done over an undispositioned correctness finding | correctness halt | restore the named paths to the last batch head with git, for a path under the plan directory `lyx webster restore-plan` (or `lyx webster rebaseline --card NN` for a card no batch has begun), then run `lyx webster accept-audit`; for a finding with no path, reset the branch to the run's start commit with git and run `lyx webster run --fresh`, and re-step the Webster row (lyx webster run); done is demoted to stuck |
| pending audit findings | run entry finds correctness findings from an earlier run exit still pending | correctness halt | restore the named paths to the last batch head with git, for a path under the plan directory `lyx webster restore-plan` (or `lyx webster rebaseline --card NN` for a card no batch has begun), then run `lyx webster accept-audit`; for a finding with no path, reset the branch to the run's start commit with git and run `lyx webster run --fresh`, then re-step the Webster row; a shed-driven run blocks with this text as its reason |
| accept-audit: suspect path differs | a pending finding's path outside the plan directory differs from the last batch head | correctness halt | restore each with `git checkout <head> -- <path>`, deleting a path the head does not hold, then re-run `lyx webster accept-audit` |
| accept-audit: plan path differs | a pending finding's path under the plan directory differs from the plan the run recorded | correctness halt | `lyx webster restore-plan`, or `lyx webster rebaseline --card NN` for a card no batch has begun, then re-run `lyx webster accept-audit` |
| recover-batch: findings recovery cannot check | `recover-batch` finds a failed batch whose findings have no path or a path recovery cannot check | correctness halt | reset the branch to the run's start commit with git and run `lyx webster run --fresh` |
| accept-audit: finding not checkable | a pending finding has no path, or a path nothing the run recorded can check | correctness halt | reset the branch to the run's start commit with git and run `lyx webster run --fresh` |
| fresh over a differing suspect path | `lyx webster run --fresh` finds a pending finding whose suspect path differs from the run's start commit | correctness halt | reset the branch to that commit with git, then re-run `lyx webster run --fresh` |
| fresh: HEAD not at the start commit | `lyx webster run --fresh` finds HEAD is not the run's start commit while it would drop findings | correctness halt | reset the branch to that commit with git, then re-run `lyx webster run --fresh` |
| fresh over a differing plan path | `lyx webster run --fresh` finds a pending finding's plan path differs from the plan the run recorded | correctness halt | `lyx webster restore-plan`, or `lyx webster rebaseline --card NN` for a card no batch has begun, then re-run `lyx webster run --fresh` |
| recovery suspect path not cleared | `recover-batch` finds a suspect path still holding the flagged content anywhere in the recovery head's tree, or with changes the recovery's head does not hold | correctness halt | `lyx webster recover-batch NN`; the batch fails again with the path named for the next strand, and for a plan suspect path `lyx webster restore-plan` (or `lyx webster rebaseline --card NN` for a card no batch has begun) |
| Webster row paused out of band | the Webster row's run ends paused with no pause loom requested | transient | re-step the Webster row, since lyx webster run clears the pause and resumes |
| fabric sync failed | a bracket verb, `run`, `rebaseline`, `restore-plan` or `accept-audit` cannot commit its state to the fabric | transient | the state is saved locally; `lyx fabric commit` commits it, or the next bracket verb's own sync carries it |
| re-baseline persist failed, bracket verb | begin-batch or record-batch cannot persist the plan-fingerprint re-baseline | transient | re-run the same verb; the re-baseline is recomputed from the plan on disk |
| re-baseline persist failed, validate | `lyx webster validate` cannot persist the plan-fingerprint re-baseline | transient | re-run `lyx webster validate` alone |
| standalone reed boot failed | the standalone reed session does not come up | transient | transient, re-run the verb |
| wiring guards | nil deps, empty paths and an invalid batcher or geometry, unreachable from any on-disk state a run can produce | wiring guard | none per row; grouped |
| raw I/O | `stat`, `mkdir`, `read` or `write` of a run file fails | transient | re-run the refused verb; nothing is mutated |

## shed

| Refusal | Trigger | Class | Way forward |
|---|---|---|---|
| shed busy | another driver holds the run lock, at `run`, `step` or `goto` | transient | `lyx shed pause <run-id>` asks the live driver to stop at its next producer boundary; check the holder with `lyx shed status <run-id>`, then retry |
| seed missing | a verb addresses a run-id with no seed | correctness halt | run `lyx shed seed <run-id> --recipe <name>` first |
| status file missing | `step` or `goto` finds no status file; Shed never seeds one | correctness halt | the recipe's own, as the message names it: `lyx loom start` for loom, `lyx batten run <slug>` for batten, `lyx shed seed` otherwise |
| current producer missing | the status file's `current_producer` names no row in the list | correctness halt | `lyx shed goto <run-id> --to <producer>` moves the run onto a row that exists |
| bounce budget exhausted | a segment's bounce budget runs out and the run halts Stuck | correctness halt | `lyx shed goto <run-id> --to <row>` gives the segment or row a fresh budget |
| goto on a done run | `goto` names a run whose status is done | correctness halt | seed a new run |
| goto on a running run | `goto` names a run whose status is running | correctness halt | `lyx shed pause <run-id>` then `lyx shed step <run-id>` leaves the run paused at its next producer boundary, then re-run goto |
| goto target past the current row | `goto` names a row after the run's current row, or a target the awaiting narrowing excludes | correctness halt | re-run goto with --to naming one of: `<admitted rows>` |
| goto target unknown | `goto` has no `--to`, or the target names no producer | correctness halt | re-run goto with `--to` naming one of the producers the message lists |
| status watch as JSON | `status` is given both `--watch` and `--json` | correctness halt | drop `--json`, or drop `--watch` for the JSON envelope |
| seed disagrees | `seed` finds the run-id already seeded with different values | correctness halt | keep the existing seed and drive it (`lyx shed status <run-id>` shows it), or address a different run-id |
| seed refuses here | the seed verb runs outside the worktree the recipe drives | correctness halt | run `lyx shed seed` from the worktree the recipe drives, which the cause names |
| llm driver without bootstrap | `--driver llm` on a recipe with no bootstrap verb | correctness halt | re-run with `--driver go` |
| wiring guards | nil deps, an invalid producer list, empty paths | wiring guard | none per row; grouped |
| raw I/O | `stat`, `mkdir`, `read` or `write` of a status, seed or lock file fails | transient | re-run the refused verb; nothing is mutated |

## loom

`lyx loom` drives the same shed verbs, so the shed section's rows apply to it unchanged, with `lyx loom` in place of `lyx shed`,
except that loom's own seed is written by `lyx loom start`, which the seed-missing and status-missing rows below name instead.
The `validate-*` verbs' findings envelopes are each verb's verdict on its artifact, not refusals, so they have no row here.

| Refusal | Trigger | Class | Way forward |
|---|---|---|---|
| seed missing | a loom verb addresses a run-id with no seed | correctness halt | run "lyx loom start" first to bootstrap this task |
| status file missing | `lyx loom status` or `lyx loom approve` finds no status file | correctness halt | run "lyx loom start" first to bootstrap this task |
| approve: not at PR-Gate | `lyx loom approve` runs while the run is not awaiting or blocked at PR-Gate | correctness halt | `lyx loom status` shows where the run is; approve once it halts at PR-Gate |
| approve: no pull request | no pull request from the task branch to its parent exists | correctness halt | `lyx loom goto --to Publish` moves the run back to Publish, then `lyx loom step` opens a new pull request |
| approve: pull request not open | the pull request is closed or merged | correctness halt | `lyx loom goto --to Publish` moves the run back to Publish, then `lyx loom step` opens a new pull request |
| approve: HEAD differs | the local task HEAD differs from the pull request's head | correctness halt | push the task branch with `git push` when the local HEAD is ahead, or pull it with `git pull` when the pull request's head is ahead, then re-run lyx loom approve |
| approve: lookup failed | branch resolution, the origin URL, the pull request lookup or the HEAD read fails | transient | transient, re-run `lyx loom approve` |
| commit-records: probe or commit failed | the merge-state probe or the commit fails | transient | transient, re-run `lyx loom commit-records` |
| commit-records: not pushed | the commit landed locally but the push failed | transient | `lyx fabric push` pushes the landed commit, or re-run `lyx loom commit-records` |
| commit-records: park marker failed | the park marker cannot be written | transient | transient, re-run `lyx loom commit-records --park <park>` |
| start: driver took no lock | the detached driver exits without taking the run lock | transient | `lyx loom start`; the message names the driver log |
| start: driver not parked | the run is halted at a hand-back and its driver is still writing its stop report and committing its records | transient | retry `lyx loom start` in a few seconds |
| Loom-Preflight half-finished | the preflight finds a half-finished run | correctness halt | seed a new run, or `lyx loom goto --to Loom-Preflight` accepts this run as a deliberate re-entry |
| preflight preconditions unmet | the worktree is dirty, fabric is unsynced or a junction is broken | correctness halt | per failed check: commit or stash the code changes with git, and commit the _lyx changes with `lyx fabric commit`, then re-step; `lyx fabric checkout` re-checks out the current branch and re-syncs the _lyx side, then re-step; `lyx fabric reconcile` recreates a missing _lyx worktree and re-points broken junctions, then re-step |
| invalid history outcome | the status file's history carries an outcome the coherence check does not know | correctness halt | seed a new run |
| batcher misconfigured | `batcher.yaml`'s `active:` key names no batchifier | correctness halt | fix `batcher.yaml`'s `active:` key, then re-step |
| produced artifacts commit failed | the Discussion-Write, Plan-Write or Webster row cannot commit its records | transient | the fault is transient, re-step the row |
| wiring guards | nil deps, an invalid producer list, empty paths | wiring guard | none per row; grouped |
| raw I/O | `stat`, `mkdir`, `read` or `write` of a status, seed, lock or records file fails | transient | re-run the refused verb; nothing is mutated |

## Out of scope

Landing, batten, orch, fabric, burler and board refusals are not in this table;
a later audit adds each as its own section.
The fast-forward merge-in limit is out of scope too, since its message already names a git recourse.
