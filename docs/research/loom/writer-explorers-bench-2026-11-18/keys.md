# Grading keys

Fixed and committed before any bench output exists.
Each key is a finding of the reference run's own review of the artifact an arm writes, so a key asks whether the arm's artifact repeats the defect the reference's writer left in it.

Grades: R repeated, P partly, A avoided.

## Reference

`pawl-checklist-hidden-in-beam`, its records archived on the weft tag `archive/pawl-checklist-hidden-in-beam/fb0f145ec02c`.
It is the smallest landed tapestry run with a Discussion review and a twill Plan with its review.
The base commit is `f98a1ba0`, the newest commit of `main` before the run's seed at 2026-11-03T09:46:30Z.
Its board entry body is `design-pawl-checklist-hidden-in-beam.md` in the bolt repository at `08c5342d1b`, the last revision before the run started.

The task: while a beam's pawl run lives, the beam's session shows the run's checklist in a second overview window.
The pawl run's own writers worked before `3085f451`, so they carried no explorer instruction; the keys do not depend on that.

## Discussion keys

From Discussion-Burler-Review round 1 (`_lym/reviews/discussion/round-1-review.md` on the tag).
A Discussion arm's artifact is its `decision-record.md`, graded as written by the writer, before any review.
Finding F3 is left out: it faults the reference's own support log, which an arm's log cannot repeat.

| Key | Finding | Avoided when the record |
|---|---|---|
| DK1 | An old-build pawl driver calls `orchcli.BeamUp` in-process and runs the old `ensureRunsStrand`: it can drop the runs strand or replace the status strand again | names that driver and what it can do until the next `lym beam start` under the new build |
| DK2 | The viewed-window-ending path has no acceptance criterion: with two overview windows, a viewer that ends must land on the other overview window | has a criterion that fails if that path yields nothing, the shell or the ending window |
| DK3 | An older status strand tying the runs strand at the same order key puts the checklist first, so the focus fallback lands on it instead of the beam overview | states that consequence of the tie, or removes the tie |

Partly: the record names the case but not its consequence, or its criterion cannot fail on the wrong outcome.

## Plan keys

From Plan-Burler-Review round 1 (`_lym/reviews/plan/round-1-review.md` on the tag).
A Plan arm's artifact is `_lym/plan/` after Plan-Write and before any review, with twill on.
The lens reviews of the reference's twill step (`_lym/plan/lens-review.md`) are not keys: both Plan arms run twill, and its fixes land in the plan before it is graded.

| Key | Finding | Avoided when the plan |
|---|---|---|
| PK1 | `docs/overview.md` carries two statements that `ensureRunsStrand` replaces a loom status strand (the orch entry and the batten entry); the plan rewrites one | rewrites both, in a card that targets `docs/overview.md` |
| PK2 | The sweep of comments saying one overview per session misses `isOverview`'s doc comment, the `runs.go` file header, `runsStrandAddSpec`'s doc comment and the `lym reed follow` help text | names all four, or each in a card's Intent |
| PK3 | A relaunch test row for an `IfAbsent` relaunch says the relaunched pane runs the spec's command, though the relaunch launches the strand's stored command | has no such row, or states the stored command |

Partly: the plan covers some of a key's sites, or names a site without saying what changes there.

## Grading rules

- One grader, the bench's author, reads each artifact once against these keys.
- A grade cites the line of the artifact it rests on.
- An arm is not told its keys, and the keys are not edited after the first arm's output is read.
