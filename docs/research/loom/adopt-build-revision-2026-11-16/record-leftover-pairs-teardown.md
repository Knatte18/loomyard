## Goal

The dobby tears down or reports leftover pairs

Takes: runfsm's `landed` and `cancelled` states and its reading of whether a cancelled run kept its pair; the cancelled batten record that `lym batten cancel` keeps and every pair removal deletes; dobbyengine's ended-run teardown and pairteardown's teardown; the holds and registry from bolt-holds-catch-up; fabricengine's hub workspace regeneration
Delivers: the dobby tearing down landed and unkept cancelled pairs, listing kept pairs, writing kept-cancel records for pre-upgrade kept pairs, and tearing down idle orphans after the orphan grace; lym tender resume pushing no pair; the hub workspace file regenerated after each teardown
Exports: the orphan grace constant

## Scope

Decision numbers are those of the Sketch `adopt-redesign`.
The run FSM (D1, D3) has landed, and the holds and the registry come from `bolt-holds-catch-up`.
This part shares no code with the foreground adopt (D8).

The part changes these packages:

- internal/dobbyengine (about 250 changed lines)
- internal/beamcli (about 80 changed lines)
- internal/pairteardown (about 60 changed lines)
- internal/fabricengine (about 50 changed lines)
- internal/battencli (about 40 changed lines)
- internal/tendercli (about 30 changed lines)
- internal/shedrun (about 20 changed lines)

## Decisions

### D11 — Leftover pairs are torn down or reported

- The dobby tears down, through `internal/pairteardown`, the pair of each run in the FSM state `landed`, or `cancelled` without keep,
  with no pause or hold covering it and no other machine owning it.
  A cancelled run's batten record goes in the same step.
  This widens the ended-run teardown of `internal/dobbyengine/ended.go`,
  whose stranded-run condition reads a board entry that ended, so it takes neither a cancelled run, whose entry is unclaimed, nor a landed run's leftover pair.
  The widened condition reads the FSM state in place of the cancel intent and the pause record.
- A cancelled run whose pair was kept stays: `lym beam status` lists it with its age,
  and each dobby start posts one notice listing kept pairs and naming `lym fabric remove <slug>`.
- A pair kept by a cancel before the upgrade has no batten record left.
  The first dobby start after the upgrade writes, for each pair whose slug has a board entry, no batten record and no live process,
  a batten record holding a finished cancel with keep, so the pair is listed as kept and never read as `queued` or as an orphan.
  The board entry must be unclaimed, as a cancel leaves it, so a pair whose claim stands is never read as kept.
- A pair with neither a board entry nor a batten record, such as a bench's or a trial's, is an orphan.
  The dobby posts a notice at first sight saying when it goes, and tears it down no earlier than the orphan grace after that notice,
  and only when neither branch has had a commit for the orphan grace and no process of its slug lives.
  Uncommitted files are committed onto the archived tip first.
- `lym tender resume` pushes no pair, since a transition's push is synchronous and leaves no deferred push for it to pay;
  the `rerun` pair's failure on every resume ends.
- The hub workspace file is regenerated after each teardown, so a torn-down pair leaves VS Code.

Bound: an orphan in use stays while it commits or runs a process; an idle one goes after the grace, its branches kept under archive tags.
A kept pair's batten record keeps it out of the orphans, also when its board entry is removed.

Rationale: a bench pair, a kept pair and a trial pair stood for days, listed in the operator's VS Code, with nothing reporting them (A14).
A kept pair stays because keeping it was the operator's explicit choice at the cancel.

## Constraints

- `PATTERN-pair-remote`: teardown goes only through `internal/pairteardown`, which holds the slug's teardown lock.
- `PATTERN-owner-machine`: a pair another machine owns is never torn down here.
- `PATTERN-lym-files`: every new tracked file sits under `_lym` and every cache under `.lym`, both named in `internal/lymdirs`.
- `PATTERN-daemon-wakeups`: the orphan check wakes at most once a second and backs off while idle.
- `PATTERN-child-processes`: a pair's processes end only through `internal/runproc`.
- `PATTERN-code-conventions`: the orphan grace is one named constant.
- `PATTERN-test-only-code`: every new function has a production caller.

## Auto-mode assumptions

- The orphan grace is 24 hours.

## Open risks

## Acceptance criteria

- D11: a cancelled run's pair without keep is torn down by the dobby; a kept pair stays and is listed with its age in `lym beam status`, also after its board entry is removed.
- D11: a landed run's leftover pair is torn down by the dobby, and a pair another machine owns or a pause or hold covers is not.
- D11: a pair kept by a cancel before the upgrade is listed as kept after the first dobby start, and a pair whose claim stands is not.
- D11: an orphan with no commit and no live process is torn down no earlier than the orphan grace after its notice, its branches archived; one with a commit inside the grace stays.
- D11: `lym tender resume` pushes no pair.
- D11: the hub workspace file no longer lists a pair after its teardown.

## PATTERN changes
