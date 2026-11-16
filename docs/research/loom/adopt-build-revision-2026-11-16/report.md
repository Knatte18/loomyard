# Revising the remaining adopt-build members against what landed

2026-11-16, board task `adopt-build-revision`.
A read-only study of the beam `adopt-build` at the commit this branch forked from, `973c9025`,
after `orch-resume-own-session` and `run-fsm-tracked-records` landed.
No test ran and no code, board entry or decision record changed.
The board entries were read with `lym board get`, and the Sketch `adopt-redesign` and its split from the raddle record under Bolt.
A claim marked *not read* names code this study did not open, so the member's own run has to settle it.

The revised decision records sit beside this report:
[push-before-act-takeover](record-push-before-act-takeover.md),
[bolt-holds-catch-up](record-bolt-holds-catch-up.md),
[adopt-foreground-leftovers](record-adopt-foreground-leftovers.md) and
[leftover-pairs-teardown](record-leftover-pairs-teardown.md) (new).
The darn members have no record, so their revised bodies below are their whole specification.

## Short answer

- The three raddle members keep their order, dependencies and recipe.
  Their `Takes` and `Delivers` need edits where the landed code differs from what the Sketch assumed (section 1).
- Two Sketch decisions sit in the wrong member.
  The upgrade's first read of standing pause entries is written into both `bolt-holds-catch-up` and `adopt-foreground-leftovers`;
  it belongs in `bolt-holds-catch-up`, since the hold setter needs a production caller there.
  The leftover-pair teardown (D11) shares no code or decision with the foreground adopt, and I recommend splitting it into `leftover-pairs-teardown`.
- `prime-is-root-beam` narrows to what the board prints.
  Changing what `hubgeom.ParentOf` returns for the prime would reach every owner read the three raddle members rewrite.
- `hub-stop-button` does not build on the run intents.
  A per-slug `lym tender pause` now writes a tracked intent and pushes it, which a stop must not do.
  It builds on hibernate's pause-and-confirm sequence and `beam stop`, and it has to end the orch's registered Claude process.
- `machine-restart-shortcut` must not start a hub through `lym s`.
  That shortcut begins with `lym adopt --here`, which takes over a hub another machine owns.
  `lym beam start --all` already starts the prime and the beams this machine owns, and the launcher builds on it.
- Nothing is left of the orch transcript mirror (F12) in the six members.
  The orch staying up during a hold (F14) matters to the stop button and the restart launcher; both bodies say so.

## 1. What landed, against what the plan assumed

`run-fsm-tracked-records` (`973c9025`):

- `internal/runfsm` declares one state more than the Sketch lists, `none` for a slug no run holds.
  It also exports `Judge` and `Aside`: a verb names what it resolves itself (its own pause, a cancel under way, a hold, a shed pause, an owed push),
  and `Judge` sets those aside before the act rule answers.
  The Sketch's act rule has no such parameter.
- The inputs are not "passed in by callers".
  One reader, `pairteardown.ReadRun` (`internal/pairteardown/runread.go`), fills them.
  It fills the owner, the pause coverage and the past-parent-merge fact.
  `Inputs.Held`, `Inputs.PushOwedOffline` and `Inputs.CodeTipMissing` are declared and nothing sets them.
  `push-before-act-takeover` and `bolt-holds-catch-up` fill them in that reader, so their `Takes` name it.
- The owner input is `boardengine.OwnedElsewhere(owner, here)`, a string comparison.
  `dobbyengine.OwnedWorktrees` and `EnsureLive` compare an owner with the machine's name directly, outside that function.
  The registry sweep in `bolt-holds-catch-up` has to reach both.
- The act rule's `Act` already lets a `landing` run past its parent-side merge act under a hold, before it tests `Held`.
  The `adopt-fix` hold's fix task has no such case, so the reader must leave `Held` false for it.
  The refusal for `ReasonHeld` names no machine and no "ask the operator when that machine has stopped" way forward, which the hold decision requires.
- The pause and cancel intents commit and then spawn a detached push (`SpawnStatusPush` in `internal/tendercli/runintents.go`).
  The landed summary says the PR record and fixer counts push "with today's pushes".
  No owed-push record exists, and the origin-owed marker (`internal/battencli/resumereconcile.go`, `dobbyengine`'s `OriginOwed`) still stands.
  The offline half of D3 (act here at once, owe the push) therefore needs `push-before-act-takeover`, as its `Delivers` says,
  and the cancel path's push was *not read*.
- `lym tender pause <slug>` now writes a tracked pause intent; the hub-wide `lym tender pause` with no slug is still a machine-local hub entry.
  The task status bar's HALT button runs the per-slug form.

`orch-resume-own-session` (`3bc33584`):

- D9 and D10 are done.
  `runproc` has `RoleOrch`, `RegisterOrch` and `IsRegistered`, and an orch start ends its own registered holder and resumes the same session.
- The subject-process end exists as `pairteardown.EndSubjectProcesses(prime, owner, endDobby)` over `runproc.EndSubject`.
  Only the orch watcher's owner check calls it, with the dobby end unset.
  The dobby's handover, `tendercli.PauseOwnedElsewhere` (which pauses under the `owner` caller and ends processes by its own `endOwnedElsewhere`) and `dobbyengine`'s `PauseOwned` seam are untouched.
  `bolt-holds-catch-up` replaces them with the landed function.
- A registered orch Claude process outlives its strand's removal, which is why it is registered.
  `lym beam stop` removes the strand and ends the dobby; it does not end that process.

Nothing else on the board overtook the three members.
`lym loom peers` shows no run in flight beside this one.

## 2. Overlaps and gaps between the members

- **The upgrade's first read.**
  `bolt-holds-catch-up`'s D7 turns standing `adopt` and `adopt-hold` pause entries into pause intents and writes one hold;
  `adopt-foreground-leftovers` lists the same read in its `Delivers` and carries its acceptance criteria.
  The hold setter is a new function, and `PATTERN-test-only-code` (landed after the Sketch) forbids a function only tests call.
  The upgrade's first read is the setter's production caller in `bolt-holds-catch-up`, because the foreground adopt, the other writer, arrives a part later.
  The revised records put the first read and its criteria in `bolt-holds-catch-up` and leave `adopt-foreground-leftovers` the retirement of the writers and readers.
  Today's adopt worker still writes the `adopt` caller until then.
- **The adopt record's readers.**
  The dobby's handover and the orch watcher read the adopt record.
  `bolt-holds-catch-up`'s D7 told them to read a hold, but the record and its worker survive until `adopt-foreground-leftovers`.
  The switch moves to the part that deletes the record, so each part leaves a consistent tree.
- **The same test-only rule on exports.**
  `push-before-act-takeover` exports a takeover entry type and reader "tested with kit-written entries".
  The reader has a production caller in the rejected-push path, and the writer a test needs goes in `internal/testkit`;
  the production writer is adopt's.
- **The dobby's handover.**
  `bolt-holds-catch-up` retires `PauseOwned` and `PauseOwnedElsewhere` into the act rule's `hand_over` and the subject-process end.
  Its `Takes` named only "the dobby's loop and handover"; the revised body names the seam and the tender function so the plan covers both.
- **Dead names from the Sketch.**
  Every record cites D1 to D11 as if one record held them.
  The revised records say once that the numbers are the Sketch's, and name the landed function where they cited a landed decision.
- **The Added-after-Discussion entries.**
  Each of the three records carries the same set of entries.
  Their calls are folded into the decisions above, and the revised records carry none:
  F15 stands in D2's bound, the dropped `--release` and the take-hold are in D8 and D7,
  the transcript mirror is gone with D10, and the orch staying up during a hold is in D7's bound.
  The orch applies the revised records with the fold the decision-record rules describe.

## 3. push-before-act-takeover

What changes and why:

- `Takes` drops "runproc's End for a slug", which landed as `pairteardown.EndSubjectProcesses`, and adds what the landed work left for this part:
  the unfilled `Inputs` fields, `ReadRun`, and the detached pushes of the intents, the PR record and the fixer counts.
- `Delivers` adds the intents' push.
  The Sketch said only that the offline pause and resume intents "ride" the owed push, but the cancel intent, the PR record and the fixer counts push through the same detached path today.
- The hand-over's process end names the landed function, with the dobby end unset when the caller is that dobby's own run.
- The takeover writer is adopt's.
  This part ships the type, the reader and a kit writer.
- Package list: `tendercli`, `pairteardown` and `pullrequest` join; `battencli` and `landingshed` grow for the intent and PR pushes.
  The line counts are the Sketch's estimates adjusted by reading, not measurements.
- Dependencies, recipe and the 30-minute code-tip grace stand.
  The `PATTERN-run-fsm` entry is still to add: the landed part did not write it.

Revised body:

```markdown
Filed from the Sketch `adopt-redesign`.

Takes: runfsm's `Judge`, `Aside` and `Inputs`, whose `PushOwedOffline` and `CodeTipMissing` nothing fills yet, and `pairteardown.ReadRun`, the one reader of a run's inputs; the batten record's cancel and pause intents with their shedrun writers, and the detached commit-and-push that `lym tender pause`, `lym tender resume`, `lym batten cancel`, the pull-request record and the fixer attempt counts make today; fabricengine's PushPairAnchored, SpawnStatusPush, teardown-lock respawn, pushclassified's rejection path, originreads and ArchiveOwnerTips; boardengine's Sync, SpawnSync and pushOwnerCommit; statuscommit and sheddriver's records commit; landingshed's Finalize; pairteardown's EndSubjectProcesses over runproc's EndSubject
Delivers: a shed transition that takes a gate admission, commits its records with the code tip it was made with and pushes records side first, code side after, before the next producer starts; the run intents, the pull-request record and the fixer counts pushing through the same synchronous push; an owed push retried on a backoff and shown in lym beam status, which the offline pause, resume and cancel verbs also ride; pairteardown's ReadRun filling the owed-push and code-tip inputs; the per-transition detached push and its teardown-lock respawn retired; synchronous Bolt writes for a run claim, a landing's done and an owner; Finalize pushing the parent branch before its transition and board done; a rejected records push that reads origin's record and hands over on another machine's takeover entry, ending the run's processes, archiving local tips and the local parent merge; the code-tip wait and its grace
Exports: the takeover entry type and its reader, tested with entries a testkit writer makes; the owed-push and code-tip facts `pairteardown.ReadRun` puts into runfsm's Inputs; fabricengine's records-first PushPairAnchored and its hand-over result

The part changes these packages:

- internal/fabricengine (about 1000 changed lines)
- internal/battencli (about 300 changed lines)
- internal/sheddriver (about 200 changed lines)
- internal/loomcli (about 200 changed lines)
- internal/landingshed (about 250 changed lines)
- internal/boardengine (about 150 changed lines)
- internal/statuscommit (about 100 changed lines)
- internal/tendercli (about 100 changed lines)
- internal/pairteardown (about 80 changed lines)
- internal/shedengine (about 60 changed lines)
- internal/beamcli (about 60 changed lines)
- internal/fabriccli (about 40 changed lines)
- internal/tenderengine (about 40 changed lines)
- internal/pauserecord (about 40 changed lines)
- internal/pullrequest (about 30 changed lines)
- pattern (about 30 changed lines)
```

Revised record: [record-push-before-act-takeover.md](record-push-before-act-takeover.md).

## 4. bolt-holds-catch-up

What changes and why:

- `Takes` names the landed `Held`/`OwnedElsewhere` inputs, `ReadRun`, `Judge`, `EndSubjectProcesses`, and the dobby's `OwnedWorktrees`, `EnsureLive`, `PauseOwned` and `tendercli.PauseOwnedElsewhere`, which the retirement and the registry sweep have to reach.
- `Delivers` takes the upgrade's first read (section 2), the hold input with the fix task left out, and the refusal that names the hold's machine.
  It says the registry sweep covers `dobbyengine`'s owner filters and `ReadRun`.
- The catch-up's process end is the landed function, called with the dobby end set only for the dobby's own handover.
- The origin-owed marker retires here, where the catch-up replaces the reconcile it stood for; the Sketch left that to a sentence.
- `Exports` names the dobby start as a caller of the catch-up, so the entry point has a production caller in this part.
- `internal/runfsm`, `pairteardown` and `tendercli` join the package list.
- Dependencies, recipe and order stand.
  The part uses push-before-act's synchronous Bolt writes and its takeover hand-over.

Revised body:

```markdown
Filed from the Sketch `adopt-redesign`.

Takes: runfsm's Act rule, its `Held` and `OwnedElsewhere` inputs, `Judge` and `Aside`, and `pairteardown.ReadRun`, the one reader that fills them; the synchronous Bolt writes, the takeover reader and hand-over and the owed-push facts from push-before-act-takeover; `pairteardown.EndSubjectProcesses` and `runproc.EndSubject` from orch-resume-own-session; boardengine's owner setter and OwnedElsewhere, machinename's former names, dobbyengine's OwnedWorktrees, EnsureLive, loop and handover with its PauseOwned seam, tendercli's PauseOwnedElsewhere, pauserecord's callers and SleepWatch, battencli's origin-owed marker, pairteardown's local mode
Delivers: board.json's machines registry and holds list written only by boardengine's setter with a synchronous push; every machine-name comparison through the registry, OwnedElsewhere's callers in every package updated, `pairteardown.ReadRun` and dobbyengine's owner filters included; lym machine name --forget-former; hold slug entries under the new pauserecord caller hold, read by every machine, and `pairteardown.ReadRun` filling runfsm's hold input with the fix task left out; the refusal of a held run naming the hold's machine; the owner caller and its writers retired into hand_over with the subject-process end, standing owner entries dropped at the first read after the upgrade, and standing adopt and adopt-hold entries turned into pause intents and one Bolt hold; the origin-owed marker retired; the catch-up from origin after a sleep, a dobby start or a hand pass, pushing owed commits, handing over runs owned elsewhere and removing their local pairs in local mode, the dobby's handover calling the subject-process end
Exports: boardengine's registry and hold types, setters and readers, OwnedElsewhere taking the registry, and the catch-up entry point the dobby start and adopt call

The part changes these packages:

- internal/boardengine (about 600 changed lines)
- internal/dobbyengine (about 450 changed lines)
- internal/pauserecord (about 120 changed lines)
- internal/machinecli (about 150 changed lines)
- internal/orchcli (about 120 changed lines)
- internal/tenderengine (about 100 changed lines)
- internal/battencli (about 140 changed lines)
- internal/beamcli (about 100 changed lines)
- internal/pairteardown (about 60 changed lines)
- internal/tendercli (about 60 changed lines)
- internal/hibernateengine (about 60 changed lines)
- internal/adoptcli (about 40 changed lines)
- internal/machinename (about 40 changed lines)
- internal/loomcli (about 40 changed lines)
- internal/runfsm (about 30 changed lines)
- internal/battenshed (about 30 changed lines)
- internal/landingshed (about 10 changed lines)
- internal/fabricengine (about 10 changed lines)
- internal/shedrecipe (about 10 changed lines)
- internal/shedrun (about 10 changed lines)
- internal/hibernatecli (about 10 changed lines)
- internal/fabriccli (about 10 changed lines)
- internal/fronttask (about 10 changed lines)
- internal/pawlengine (about 10 changed lines)
- pattern (about 10 changed lines)
```

Revised record: [record-bolt-holds-catch-up.md](record-bolt-holds-catch-up.md).

## 5. adopt-foreground-leftovers, and the split of D11

What changes and why:

- **Split D11 out.**
  The leftover-pair teardown is the dobby's ended-run teardown, a kept-pair listing, an orphan sweep and a workspace regeneration.
  It needs the run FSM and the holds, and nothing from adopt's steps, takeover or `--take-hold`.
  Kept together, the part is the largest of the beam: `adoptcli` alone is about 2650 changed lines by the Sketch's estimate, and the title already joins two subjects.
  The split gives each plan one subject and lets the teardown land on the holds without waiting on the adopt rewrite.
  The cost is one more board entry and Discussion record, which this study drafts (`leftover-pairs-teardown`).
  The adopt entry keeps its slug `adopt-foreground-leftovers`, so `depends_on` links stay valid, and takes a new title.
  If the operator declines the split, the merged body is the body below with the leftover entry's `Delivers` and `Exports` appended,
  and the merged record is the adopt record with D11, its constraints and its criteria appended.
- `Takes` names the refusals of the landed act rule that say `lym adopt <subject>`, which the foreground adopt must keep true,
  and the adopt record's readers, which this part now replaces.
- `Delivers` keeps `--here`'s fast outcomes.
  `lym s` runs `lym adopt --here --no-orch` first on every start, so a foreground adopt that did a hold, a pull and a check on each call would slow every start on the owner machine.
  `--here` today ends `owned_here` after one Bolt pull when this machine owns the subject; the record now says so.
- It also drops the upgrade's first read, which moved to `bolt-holds-catch-up`, and adds the dobby's handover and the orch watcher reading a hold.
- The package list drops D11's share of `dobbyengine`, `beamcli`, `pairteardown`, `tendercli` and `fabricengine`.
- Dependencies, recipe and order stand.

Revised body of `adopt-foreground-leftovers`, with the new title `lym adopt runs in the foreground over pushed state, with --take-hold`:

```markdown
Filed from the Sketch `adopt-redesign`.

Takes: the holds, registry, catch-up and owner setter from bolt-holds-catch-up; the takeover entry type, its reader and the synchronous pushes from push-before-act-takeover; runfsm's states and the refusals that name `lym adopt <subject>`; adoptcli's fix loop, check, deploy, archives and here paths; the adopt record's readers in the dobby's handover and the orch watcher
Delivers: lym adopt running hold, owner, pull, takeover, catch-up, deploy, workspace, check and green clear in the caller's process, re-runnable from tracked state, with the takeover entry's production writer and its rejected-push retry, --take-hold denied to agents by a shell-guard row, and --here keeping its owned_here and no_subject outcomes for `lym s`; the worker, adopt record and its readers, verdicts, pause step, the writers of the adopt and adopt-hold pause callers, --keep-local, --drop-local and --discard retired, the dobby's handover and the orch watcher reading a hold in the record's place; adoptcli's and adoptrecord's package docs rewritten to the foreground flow
Exports: adopt's foreground step runner and the takeover entry writer

The part changes these packages:

- internal/adoptcli (about 2650 changed lines)
- internal/adoptrecord (about 400 changed lines)
- internal/dobbyengine (about 150 changed lines)
- internal/fabricengine (about 100 changed lines)
- internal/beamcli (about 80 changed lines)
- internal/orchcli (about 80 changed lines)
- internal/pauserecord (about 80 changed lines)
- internal/battencli (about 60 changed lines)
- internal/shellguard (about 30 changed lines)
- cmd/lym (about 10 changed lines)
- pattern (about 10 changed lines)
```

Revised record: [record-adopt-foreground-leftovers.md](record-adopt-foreground-leftovers.md).

### The new entry `leftover-pairs-teardown`

`depends_on` is `adopt-foreground-leftovers`, so the two parts that rewrite `dobbyengine` do not run together; recipe `tapestry`, with labels `feature`, `dobby`, `fabric`, `cleanup`, `workspaces`.
Title: `The dobby tears down or reports leftover pairs`.
The ended-run teardown's stranded-run condition today reads a board entry that ended (`strandedEnded` in `internal/dobbyengine/ended.go`), which neither a cancelled run, whose entry is unclaimed, nor a landed run's leftover pair satisfies; the record says so.

Body:

```markdown
Filed from the Sketch `adopt-redesign`.

Takes: runfsm's `landed` and `cancelled` states and its reading of whether a cancelled run kept its pair; the cancelled batten record that `lym batten cancel` keeps and every pair removal deletes; dobbyengine's ended-run teardown and pairteardown's teardown; the holds and registry from bolt-holds-catch-up; fabricengine's hub workspace regeneration
Delivers: the dobby tearing down landed and unkept cancelled pairs, listing kept pairs, writing kept-cancel records for pre-upgrade kept pairs, and tearing down idle orphans after the orphan grace; lym tender resume pushing no pair; the hub workspace file regenerated after each teardown
Exports: the orphan grace constant

The part changes these packages:

- internal/dobbyengine (about 250 changed lines)
- internal/beamcli (about 80 changed lines)
- internal/pairteardown (about 60 changed lines)
- internal/fabricengine (about 50 changed lines)
- internal/battencli (about 40 changed lines)
- internal/tendercli (about 30 changed lines)
- internal/shedrun (about 20 changed lines)
```

Revised record: [record-leftover-pairs-teardown.md](record-leftover-pairs-teardown.md).

## 6. prime-is-root-beam

What changes and why:

- The body says every reader of an empty `parent_beam` "(orch role, dobby, board verbs) moves to the named root".
  There are two things called the parent.
  What the board prints is `BriefTask.ParentBeam`, set in `internal/boardengine/store.go` from `ParentBeamOf` or `hubgeom.ParentOf`,
  and carried in the `get` envelope by `internal/boardcli/cli.go`.
  What the code resolves is the same `""`, which `hubgeom.ParentOf`, `boardengine.ParentBeam`, `beamcli`, `battencli`'s start guard and `loomcli`'s project record take as "the prime".
  `PATTERN-owner-machine` makes `ParentOf` the one owner reader, and all three raddle members change code that calls it.
  Moving the code's `""` to a name would change every one of those callers for a display fix.
  The revised body maps at the printing boundary only.
- The agents are the readers of the printed value:
  `contracts/orchrole/orch-role.md` (its release rule and its parent-orch address, which says `<shortname>:orch` when `parent_beam` is empty),
  `plugins/ly/skills/board/SKILL.md` and the `boardcli` help and package doc.
  They move in the same change.
- The name is `root`, the slug `boardengine.RootSubject` reserves for the root project (`validateSlugNotReserved` refuses it as an entry slug),
  which `lym adopt root` and `lym beam stop root` already take.
  `P0` is a handle the board's files and window titles give the root project, not a slug a verb accepts.
  The text listing shows the `P0` row.
  The JSON `tasks` array gets no root row, since the root project has no board entry.
- Dependencies: the change shares `boardengine` with `bolt-holds-catch-up` and nothing with the foreground adopt.
  The chain after `adopt-foreground-leftovers` is an ordering choice, not a code dependency; I keep it (section 9).

Revised body:

```markdown
## Seen (operator, 2026-11-15)

`lym board list` and `lym board get` give `parent_beam` empty for an entry whose nearest beam is the prime, and `lym board list --text` shows no `P0`,
while the board view and window titles show the prime as `P0`.
The prime is a beam, the root one; an empty value reads as "no beam".

## Idea

- `lym board list` and `lym board get` print `parent_beam` as `root` for every entry under the prime, never empty.
  `root` is the slug `boardengine.RootSubject` reserves for the root project, the word `lym adopt root` and `lym beam stop root` already take.
- `lym board list --text` shows the root project as a `P0` row among the projects, with the title the board files give it,
  so every view of the board shows the same tree.
  The JSON `tasks` array gets no root row: the root project has no board entry.
- Change what the board prints, not what the code resolves.
  `boardengine.ParentBeam`, `Board.ParentBeamOf` and `hubgeom.ParentOf` keep returning the empty slug for the prime,
  since every owner read, adopt subject, dobby and beam verb takes it as "the prime" and `PATTERN-owner-machine` names `ParentOf` as the one owner reader.
  Map the empty slug to `root` where `BriefTask.ParentBeam` is set and where the `get` envelope is written.
- The readers of the printed value are text, and they move in the same change:
  `contracts/orchrole/orch-role.md` (the entries an orch reads and releases, and the parent orch's address),
  `plugins/ly/skills/board/SKILL.md`, and the `boardcli` help and package doc.
  The parent orch of an entry whose `parent_beam` is `root` is `<shortname>:orch`.
```

## 7. hub-stop-button

What changes and why:

- **The run intents are the wrong base.**
  The body says a stop "builds on the run FSM and its pause and cancel intents, so a stop leaves every run in a pushed state".
  `lym reed down` in a task pair refuses a running run until a pause covers it and names `lym tender pause <slug>`,
  and that verb now writes a tracked pause intent and pushes it.
  A stop that called it for every run would write and push an intent per run, which another machine reads as `paused`, and would need origin.
  Each transition is on origin already once `push-before-act-takeover` lands, so a run cut anywhere goes on from its last transition.
  The stop is machine-local and writes nothing tracked.
- **A hub pause already exists, and so does the sequence to confirm it.**
  The hub-wide `lym tender pause` is a machine-local entry that covers every pair, so `lym reed down` accepts a pair under it.
  `hibernateengine`'s carrier pauses the hub with a grace, confirms every session idle and no network call in flight, and lifts the entry it owns by identity.
  The body asks the writer to "decide whether it first pauses the hub"; the answer is yes, and the writer reuses that sequence instead of writing a second one.
- **The orch's Claude process.**
  `lym beam stop` removes the orch strand and ends the dobby.
  Since `orch-resume-own-session`, the orch's Claude process is registered under `RoleOrch` and can outlive its strand, so the stop ends it through `runproc`.
  After a stop, `lym beam start` resumes the orch's own session, because the transcript stays on the machine.
- **The button exists.**
  Reed's `prime` button group takes an entry with a `confirm` prompt.
  A hub that sets `buttons.prime` in its own layer replaces the group whole and adds the entry by hand.
- **The click runs inside the session it ends.**
  The verb ends its own pane last and runs detached from the click.
- **Placement.**
  No code from `prime-is-root-beam` is used, so that dependency is ordering only (section 9).
  The orch up during a hold (F14) means the stop has to work under a standing hold and leaves the hold alone.
- **Open choice, recommended.**
  The stop keeps its pause entry until the restart verb or `lym tender resume` lifts it, as a hibernate that keeps its wake pause does.
  The alternative lifts the entry at once, after everything is ended.
  What decides it is whether `lym tender resume` restarts a run whose driver was ended: *not read*.
  If it does, keep the entry; if not, lift it at once and let the orchs settle the `loop-exited` halts as after a crash.
  Open question 3.

Revised body:

```markdown
## Seen (operator, 2026-11-11)

To stop everything on a machine without a reboot, the operator has to run several verbs by hand:
`lym beam stop` for the orch and dobby of the prime and of each beam, `lym reed down` in each worktree for the tmux sessions, and `lym tender reap` for run processes no run owns.
The operator finds this cumbersome; it should be one simple action.

## Idea

A button in the `prime` group of reed's status-bar buttons, shown only in the prime orch's session, labelled "stop", that takes the whole hub down.
The entry carries a `confirm` text, so reed asks a yes/no before it runs anything.
It runs one verb the writer names; `lym beam stop --all`, beside `lym beam start --all`, fits.
The operator closes terminal windows (Konsole, the VS Code terminal) by hand.

The stop is machine-local and writes no tracked record.
Every transition of a run is on origin before the run acts (`push-before-act-takeover`),
so a run cut at any point goes on from its last pushed transition on this machine or on another.
It writes no per-run pause intent: `lym tender pause <slug>` writes a tracked intent and pushes it, which another machine reads as `paused`.

Read `hibernateengine` first and reuse its sequence: a hub pause with a grace, then a confirmation that every session is idle and no network call is in flight, with the pause entry owned and lifted by identity.
Do not write a second pause-and-confirm.
Under that hub pause, end in this order:

1. every run's processes, through `lym reed down` of each task pair, which accepts a pair a hub pause covers;
2. each worktree's orch strand, the orch's registered Claude process (`runproc` role orch, which can outlive its strand's removal), its dobby and the coverage-map upkeep;
3. the reed sessions of the prime and the beams;
4. run processes no run owns, as `lym tender reap` ends them;
5. the session the click came from, last and detached from the click.

A standing adopt hold is left as it is.
The orch is up during a hold, and the stop works under one.
The stop keeps its hub pause entry until `machine-restart-shortcut`'s verb or `lym tender resume` lifts it, and its output names both.
Read what `lym tender resume` reconciles first: if it does not restart a run whose driver was ended, lift the entry at once instead.
`machine-restart-shortcut` brings every hub and beam back after a stop or reboot, so the two belong together.

## Placement (operator, 2026-11-15)

This runs in the beam `adopt-build`, after the adopt redesign's run FSM, push-before-act, Bolt holds and foreground adopt have landed in it.
The adopt decision record kept a hub-wide pause machine-local; this button needs no shared pause between machines either.
```

## 8. machine-restart-shortcut

What changes and why:

- **`lym s` is the wrong command in a launcher.**
  The shortcut is `lym adopt --here --no-orch`, then `lym beam start`, then `lym reed attach` (`internal/shortcutcli/shortcuts.yaml`).
  `--here` takes over a subject another machine owns; on the owner machine it ends `owned_here` after one Bolt pull.
  The body's rule "starts only what this machine still owns" would break on a machine that does not own the hub.
  The revised verb filters first and starts through `lym beam start --all`, which `dobbyengine.OwnedWorktrees` and `EnsureLive` already limit to the prime and the beams this machine owns.
  After `bolt-holds-catch-up`, those comparisons go through the registry, so a renamed machine still reads as owner.
- **`EnsureLive` claims a prime nobody owns for this machine.**
  A launcher that started a hub with no recorded prime owner would claim it.
  The revised body skips such a hub and says so.
- **Adding and removing launchers.**
  The body has lym remove a hub's launcher "when another machine takes the hub over".
  Another machine cannot reach this machine's desktop.
  The removal runs on the machine that holds the launcher: the verb removes its own launcher when it finds the hub owned elsewhere or its path gone,
  and so does this machine's dobby or catch-up when it hands the hub over.
  A machine that is down finds out when it next runs the launcher or reads Bolt.
  Beams come and go with no launcher change, because the verb lists the hub's live beams when it runs; only hubs add and remove launchers.
- **A hold a crash left.**
  A crash in the middle of `lym adopt` leaves its hold standing on origin, and nothing but the fix task and a landing past its merge runs under it.
  The orchs come up under it (F14), so the verb prints the hold's machine and subjects, and for this machine's own hold names the re-run `lym adopt <subjects>`.
- **The stop's pause entry** is lifted by the verb once the dobbies are up, if the stop kept it (section 7).
- **Recovery after a crash** is better than the body's account.
  A dobby's start is an absence, so `bolt-holds-catch-up`'s catch-up runs first, pushes owed commits and hands over runs another machine took.
  A run whose step was cut off halting `loop-exited` is *not re-read* here; the operator's observation stands.
- **What the body got right.**
  The workspace file lists every pair, named by project handle (`2409c0f7`); one launcher per hub plus one for all hubs; `.desktop` and `.cmd`; resuming left to the orchs.
- Which hubs are "in flight" has no mark: the machine's record of hubs (`internal/hubregistry`, written when a hub's creel daemon starts) lists every hub that ran here.
  The verb starts each listed hub whose path exists and whose prime this machine owns.
  Open question 4 asks whether that is enough.

Revised body:

```markdown
## Seen (operator, 2026-11-08 and 2026-11-11)

OSL-1046 and Neve crash often.
After a crash, bringing lym back takes the operator several manual steps:
finding every hub and project that was in flight, often three or four, with beams under them,
and starting each orch session by hand.
Once an orch is up, it and its dobby take the runs up again by themselves;
a run whose step was cut off halts (`loop-exited`) and the orch settles it.
Finding what was running is the hard part, not resuming it.
VS Code needs nothing: on Kubuntu it reopens the windows it had open by itself.

## Idea (operator, 2026-11-08 and 2026-11-11; revised by adopt-build-revision, 2026-11-16)

A simple way to start again every project that was in flight on the machine, such as launcher scripts lym puts on the desktop:

- A `lym` verb does the work, so it also runs from a terminal; the desktop launcher only calls it.
  It starts one hub, or every hub the machine's record of hubs (`internal/hubregistry`) lists whose path still exists.
- For a hub it starts what `lym beam start --all` starts: the prime's dobby and orch and those of every local beam this machine owns.
  It never starts a hub through `lym s`: that shortcut begins with `lym adopt --here`, which takes over a hub another machine owns.
- It opens the hub's one VS Code workspace file, `<hub>/<repo>.code-workspace`, which lists every worktree of the hub, the pairs under beams too.
  For the prime and each beam it started, it opens a terminal outside VS Code in the worktree running `lym reed attach`.
  A running task's window opens from the orch or the status bar's terminal button, so the launcher opens none.
- The orchs and dobbies then take up the runs; the launcher resumes no run itself.
  If `hub-stop-button` left its hub pause entry standing, the verb lifts that entry once the dobbies are up.
- A launcher starts only what this machine still owns, through the registry's reading of the owner.
  It skips every hub or beam another machine owns, saying which machine holds it.
  It skips a hub with no recorded prime owner and says so, since starting one would claim it.
- When a hold stands on the hub, the verb prints the hold's machine and subjects.
  For a hold this machine wrote, which a crash during `lym adopt` leaves behind, it names the re-run `lym adopt <subjects>`.
  The orchs come up under a hold; nothing else on the hub runs until the hold clears.
- The launcher is the form the machine's desktop runs: a `.desktop` file on Linux (the freedesktop standard, so KDE, GNOME and others), a `.cmd` file on Windows.
  Both call the same `lym` verb.
- Fixed by the operator: one launcher per hub plus one that starts every hub; beams are included; resuming interrupted runs is left to the orchs;
  nothing another machine owns is started; `.desktop` on Linux and `.cmd` on Windows, added and removed by lym.
- lym adds a hub's launcher when the hub's prime dobby first starts on this machine, and the all-hubs launcher with the first hub.
  It removes a hub's launcher when this machine's verb, dobby or catch-up finds the hub owned by another machine or its path gone,
  and the all-hubs launcher with the last hub.
  Beams come and go with no launcher change, since the verb lists the hub's live beams each time it runs.
  The operator never makes or deletes one by hand.
```

## 9. Order, dependencies and recipes

Order after the changes.
Each member's dependency on `adopt-build-revision` clears when this study lands;
the table shows the `depends_on` that stands after that.

| Member | depends_on | Recipe | Change |
| --- | --- | --- | --- |
| `push-before-act-takeover` | `run-fsm-tracked-records` | tapestry | none |
| `bolt-holds-catch-up` | `push-before-act-takeover` | tapestry | none |
| `adopt-foreground-leftovers` | `bolt-holds-catch-up` | tapestry | retitled; D11 leaves |
| `leftover-pairs-teardown` (new) | `adopt-foreground-leftovers` | tapestry | new, if the split is approved |
| `prime-is-root-beam` | `leftover-pairs-teardown` | darn | follows the new entry in place of `adopt-foreground-leftovers`; with no split it keeps its dependency |
| `hub-stop-button` | `prime-is-root-beam` | darn | none |
| `machine-restart-shortcut` | `hub-stop-button` | darn | none |

- Real code dependencies, as far as I read:
  push before bolt (the synchronous Bolt writes and the hand-over), bolt before adopt (holds, registry, catch-up),
  bolt before leftover pairs (hold and registry in the teardown condition),
  stop before restart (the stop's pause entry),
  and restart after bolt (the registry-aware owner filter).
- Ordering only: `prime-is-root-beam` shares `boardengine` with the bolt part but no code with the rest, and `hub-stop-button` uses no code from it.
  I keep the chain because the beam runs one member at a time and a linear order avoids conflicts in `boardengine`, `dobbyengine` and `orchcli`.
  The operator may move `prime-is-root-beam` anywhere after `bolt-holds-catch-up`.
- The recipes hold.
  The three darn bodies now carry decisions the Sketch's records carry for the others, so a darn writer works from the body alone.
  `hub-stop-button` has the most design left in it (the pause entry's lifetime, the verb's name); it stays a darn task because the body settles the rest.
- Labels stay.
  `leftover-pairs-teardown` takes `feature`, `dobby`, `fabric`, `cleanup`, `workspaces`.
  `hub-stop-button` gains `dobby`, since it ends dobbies, and `machine-restart-shortcut` gains `dobby` and `fabric`.

## 10. Open questions for the operator

1. **Split D11 into `leftover-pairs-teardown`?**
   Recommended: yes, for the reasons in section 5.
   If declined, the records merge as section 5 says and nothing else in this report changes.
2. **What does `parent_beam` print for the prime: `root` or `P0`?**
   Recommended: `root`, the slug the code already reserves and accepts in verbs; `P0` stays the handle in the text listing.
3. **Does the stop keep its hub pause entry until the restart or `lym tender resume`, or lift it at once?**
   Recommended: keep it, if `lym tender resume` restarts a run whose driver was ended.
   The writer reads that; the body tells it to choose the other way if it does not.
4. **Which hubs does the restart verb start?**
   Recommended: every hub in the machine's record whose path exists and whose prime this machine owns, with no separate "in flight" mark.
   A hub never stopped or retired but long idle starts too.
   A mark written by the stop button and the dobby would narrow it, at the cost of a new machine-local record.
5. **Keep the linear order for the three darn members?**
   Recommended: yes (section 9); the alternative is to move `prime-is-root-beam` up beside the bolt part.

## Limits of this study

- The cancel path's push, the PR record's and the fixer counts' pushes, what `lym tender resume` reconciles, and the `loop-exited` halt were *not read*.
  Each body or record says so where it matters.
- The changed-line counts in the package lists are the Sketch's estimates adjusted by reading the touched code, not measured.
- The revised records change no decision's content.
  They fold the added entries, replace cross-references to landed decisions with the landed names,
  move the upgrade's first read to `bolt-holds-catch-up`, and add what the landed code requires of each part.
