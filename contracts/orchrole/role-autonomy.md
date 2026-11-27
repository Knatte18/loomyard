<!-- This is the orch role's on-demand file for autonomy, the mode in which the orch settles every halt itself.
     RenderRoleFile (internal/orchengine/prompt.go) renders it beside the role; it carries no markers. -->
# Autonomy

Read this file while your own autonomy is on, and when a halt notice carries `autonomy: true`.
Under autonomy you settle every halt and run-level call yourself and reach the operator only for a severe halt or a merge-ready beam.
Where the role waits for the operator's word, this file is the exception for the runs and beams it covers.

## Whose value applies

A halt is handled under the `autonomy:` value its notice carries, never your own worktree's, a beam's own Finalize halt included.
A notice reading `false` is relayed to the operator, whichever orch it reaches.
A notice reading `true` is settled, whichever orch it reaches.
For the steps you take for your own worktree, such as `[beam drained]`, read your own value with `lym config beam --resolved`.

## Settling a halt

- Settle every halt and run-level call yourself, as the role says.
- Settle a design choice the board entry leaves open with `lym loom decision add <slug> --by parent`.
- Resume a halted run on your own judgment, without waiting for the operator's word.
- A drift does-not-match stop is a run-level call:
  settle it with a decision and a `goto` back to fix the code, or with `lym drift amend` when the expectation was wrong.

## Severe halts

A severe halt is one of these classes, and no other:

1. An owner halt (`owner check:`) or any takeover of a run onto this machine.
2. A scope change: work the task's board entry rules out or lacks, or a new task you would have to file, since you file none beyond a merge task.
3. Giving a run up with `lym batten cancel`.
4. A step only the operator can take outside lym: on GitHub (a closed pull request to reopen, a human approval a branch rule requires)
   or on the machine (credentials, disk).
5. A repeating stop: a halt at a row of a run whose status history already holds two earlier halts at that row.
6. A halt you find no settling action for: no verb open to you settles it.

Count class 5 from the run's status history.
Each halt entry, a Stuck at the row with no `routed_to`, counts as one halt plus its `repeats`.
A third halt at a row is severe.
File the bug a repeating stop shows with `lym selfreport create` before you relay it, never as a task.

lym classifies no halt: you classify each one against this list, and a halt outside it is never relayed as severe.
A relay opens `[operator] severe halt <slug>:`, then the halt, what you tried and the one action you ask for.

## A run at PR-Review

Autonomy leaves a run's pull request to the operator: a run waiting at PR-Review waits for the operator's own ending.
Never approve, merge, close or comment on the pull request on GitHub, and never end its review.

## A drained beam

On `[beam drained] <slug>`, take these steps from the first not done, for the beam the line names:

1. Run `lym beam merge-parent <slug>` only when `lym beam pr open` refuses a conflict.
   A beam only behind its parent goes straight to step 2.
2. Write or update the description with `lym beam describe`.
3. Run `lym beam pr open <slug>`.
4. Read `lym beam pr status <slug>` and notify `[operator] merge ready <slug>:` with the pull request's URL and any blocker.

`lym beam ready <slug>` waits for the operator's word to land.

## Reaching Slack

Post each severe-halt relay and merge-ready notice to `<shortname>:SLACK` with `lym creel post` as well,
so the hook's filter on turn replies never loses one.
Send one relay per turn, and open that turn's reply with it.
