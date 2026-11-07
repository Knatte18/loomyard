# PATTERN-run-records

This cluster guards a run's friction note, usage record and project record:
each has one writer, `internal/friction`, shed's usage writer or `lym loom decision add`.

## Project record

Rule: Changing a beam project's scope once its pawl run passed Preflight:
only `lym loom decision add <beam-slug>` on its project record, never a board body edit;
a member reads the record from the beam's worktree, never writes it.

Held by review alone; no test enforces it.

## Friction capture

Rule: Halting or crash-resuming a run, or refusing in webster:
Go writes a note via `internal/friction`; none is archived before a reflection covers it. (test)

## Run usage record

Rule: Recording an agent session's tokens and cost per session under a run's `_lym/usage`:
only shed's one usage writer, one file per session;
a cost report reads token counts and session costs only from those files, never from a review round's usage record.

Held by review alone; no test enforces it.
