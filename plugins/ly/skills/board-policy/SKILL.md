---
name: board-policy
description: The policy for keeping the board on the operator's behalf — one finding per note, a body holds the current design only, propose bundles and wait for approval, release only on a named go-ahead. Used beside `ly:board`, which holds the commands.
---

# board-policy

## What it covers

This is how a session that keeps the board for the operator decides what to write and when.
The commands that read and write the board are in the `ly:board` skill.

## Notes

- A proposal goes on the board as a note at once, never held in your head.
- One finding per note, written as what was seen plus the idea; never append a finding to a note about something else.
- A finding essentially the same as an existing note, or a correction to its subject, goes into that note; any other finding stays its own note, even when its work overlaps another entry.
  Notes are bundled only when the operator promotes them into a task.
- A board body holds the current design only: when a newer design, decision or answer replaces part of an entry's body, rewrite the body so it holds only what stands now, removing the replaced drafts, recommendations and open questions in the same write, never keeping them under a "this replaces everything above" line.
  Bolt's git history keeps the old text.
- An operator decision made in chat on an entry no run holds is written into that entry's body in the same turn.
- Every entry carries the labels of the modules it touches beside its type label, an imported issue included.
- Never set a priority on a board entry unless the operator names one.
- Triage drafts stay off the board: show a proposal for curating the board in chat, keep its draft in `.scratch/`, and change the board only after the operator decides.

## Tasks

- Which notes become a task, and when a task is released for its run, is the operator's call: propose the bundle and its release, and merge, promote or release only after the operator approves that bundle.
- A question is never a go-ahead: release a task only on the operator's explicit go-ahead naming that release, never inferred from a rejected alternative or from an answer to another question.
  A release is `lym board set-status '{"slug":"<slug>","status":null}'`; a new task is `draft` until then.
- Fewer and larger tasks: bundle notes whose fixes touch the same packages into one run.
  A shared theme is not enough; notes about different packages stay in different tasks.
- Never propose one note alone as a loom task: a loom run has a fixed cost of about half its spend whatever its size.
  Fold related notes into one loom task, or propose darn for a small change and let the operator name darn.
  A part of a darn task that looks large raises the darn's profile; it never becomes a loom run of its own.
- Every task runs through loom, bug fixes included: never set `recipe: darn` on a board entry yourself; a task runs darn only when the operator names darn for that task.
- A rename is a task of its own: mechanical, done by script, with nothing else in it.
  It runs alone, with no other run in flight: it depends on every task released before it, and every task made before it lands depends on it.
- A bench, spike or other exploration the operator approves runs as a darn task with profile `docs`, in its own worktree, so its report lands as a commit; the operator naming an exploration for a run counts as naming darn for it.
  Its body states an exact cap on runs: inputs, models, efforts and repeats by number, one or two inputs unless the operator names more, and never an open word such as "many".
- A full board body is no reason to lower a task's Discussion rounds: Discussion explores the code, bounds the scope and weighs the impact, which a body does not do.
- A task that needs other review rounds than its recipe's default profile names a profile of the ladder, `speed`, `light`, `solid` or `wide`, in its board entry's `profile` field, before its batten run starts;
  it gets hand-written `run_settings` for its review bounds only when no profile fits.
- A task's regression scope that the operator decides while discussing it goes in its board entry's `run_settings`, as `{"drift": {"scope": "full"}}`, `{"drift": {"scope": "off"}}` or `{"drift": {"scope": "partial", "skip": ["<test>", ...]}}`, before its batten run starts;
  Drift-Baseline then records it and spawns no agent, which otherwise decides the scope.

## Entries a run holds

- A task whose run has started is frozen: a new finding becomes its own note, even when it fits the task.
- A decision on a task a run holds, and anything about a run in flight, is the orch's: the orch of the task's worktree records it on the run.

## A session opened for board work

A session opened from the prime's `board` status-bar button exists for one or a few board cases, and the board holds all its state.

- Read the board, and the code a case needs, yourself.
- Talk each case through with the operator before writing it, then write, merge or remove notes, promote notes to tasks and release tasks as the sections above say.
- Split an entry into tasks on the operator's approval: each part is a task with its own body, its `depends_on` set, and no two parts that run in parallel changing the same package.
  For a discussed entry, each part's body carries the part of the decision record it builds, which `lym board get` with `--record` prints; only lym flags an entry discussed, so each part still runs its own Discussion.
- A case that belongs to the orch, under Entries a run holds, goes back to the operator naming the orch.
- When the cases are written, say so in one line; the operator closes the session, and nothing needs handing off.
