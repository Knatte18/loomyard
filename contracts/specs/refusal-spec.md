# Refusal contract: every refusal names its way forward

> **Status: Contract — kept on landing.** This doc defines the way forward a refusal names, the classes a refusal falls in, and which refusals a test must reach; `PATTERN-refusal-way-forward` names the modules it binds.
> The refusal messages themselves live in the code, each ending in its `way forward:` clause, and the test that reaches a refusal pins its message; this doc lists no refusal.
> It is plain markdown, not registered in `contracts/specs/specs.go`.

## What a way forward is

A refusal has a way forward when its message names one of these:

1. a `lyx` verb that makes progress from the refused state;
2. a transition the run takes by itself: a re-step, the next Master retry, a failure ladder rung;
3. a plain `git` command in the task worktree, when no `lyx` verb covers it.

A way forward never names a command the agents' settings deny (`PATTERN-no-denied-recovery` holds the list); when a step needs one, lyx performs it itself, as `lyx webster reset` does.

Hand-editing a lyx-owned state file (`state.json`, `status.json`, `seed.json`, report files) never counts, and neither does a code fix plus a deploy.
Waiting for a lock holder counts when the message names how to see the holder (`status`).
A way forward that names a `lyx fabric` verb the role guard refuses from a task session, and that such a session can reach, ends with the clause `; a session lyx refuses the verb from reports status: FAILED and the orch runs it`.
The message carries it as a trailing `way forward: <verb | transition | git command>` clause.
A message that named its way forward in its own wording before the trailing clause became the rule keeps that wording.

## Classes

Every refusal sits in one class, and the class fixes its disposition.

| Class | Meaning | Disposition |
|---|---|---|
| correctness halt | proceeding could land wrong code, forge a judgment, or corrupt run state | halt; the message names the way forward |
| policy guard | steering or hygiene; proceeding lands nothing wrong | warn and record; never halt |
| transient | a condition that clears by itself or by a named action: lock held, merge in progress, I/O | halt without mutating state; the message names the retry |
| wiring guard | nil deps, an invalid producer list, empty paths; unreachable from any on-disk state a run can produce | fail loud; no reaching test |

A validation verb's findings envelope, such as `lyx webster validate`'s, is that verb's verdict, not a refusal, and so is a test run that ends in go's own exit code.

## Reaching tests

Every refusal a verb can return has a test that reaches it and asserts its message, way forward included; one table-driven test may reach many refusals.
Two kinds of refusal need no test of their own:

- a raw I/O failure (`stat`, `mkdir`, `read`, `write`, `rename`, `remove`) propagated with its context, which is transient;
- a fail-loud wiring guard, which no on-disk state a run can produce reaches.
