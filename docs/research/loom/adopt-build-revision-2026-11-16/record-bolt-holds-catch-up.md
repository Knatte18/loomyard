## Goal

A machine registry and holds in Bolt, the owner pause caller retired, and a catch-up from origin after an absence

Takes: runfsm's Act rule, its `Held` and `OwnedElsewhere` inputs, `Judge` and `Aside`, and `pairteardown.ReadRun`, the one reader that fills them; the synchronous Bolt writes, the takeover reader and hand-over and the owed-push facts from push-before-act-takeover; `pairteardown.EndSubjectProcesses` and `runproc.EndSubject` from orch-resume-own-session; boardengine's owner setter and OwnedElsewhere, machinename's former names, dobbyengine's OwnedWorktrees, EnsureLive, loop and handover with its PauseOwned seam, tendercli's PauseOwnedElsewhere, pauserecord's callers and SleepWatch, battencli's origin-owed marker, pairteardown's local mode
Delivers: board.json's machines registry and holds list written only by boardengine's setter with a synchronous push; every machine-name comparison through the registry, OwnedElsewhere's callers in every package updated, `pairteardown.ReadRun` and dobbyengine's owner filters included; lym machine name --forget-former; hold slug entries under the new pauserecord caller hold, read by every machine, and `pairteardown.ReadRun` filling runfsm's hold input with the fix task left out; the refusal of a held run naming the hold's machine; the owner caller and its writers retired into hand_over with the subject-process end, standing owner entries dropped at the first read after the upgrade, and standing adopt and adopt-hold entries turned into pause intents and one Bolt hold; the origin-owed marker retired; the catch-up from origin after a sleep, a dobby start or a hand pass, pushing owed commits, handing over runs owned elsewhere and removing their local pairs in local mode, the dobby's handover calling the subject-process end
Exports: boardengine's registry and hold types, setters and readers, OwnedElsewhere taking the registry, and the catch-up entry point the dobby start and adopt call

## Scope

Decision numbers are those of the Sketch `adopt-redesign`.
The run FSM and the tracked run decisions (D1, D3) and the orch's own-session resume and subject-process end (D9, D10) have landed.
The synchronous push and the takeover (D2, D4) come from `push-before-act-takeover`,
and the foreground adopt, which retires the adopt record, its worker and the adopt pause step (D8), from `adopt-foreground-leftovers`.

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

## Decisions

### D5 — A machine registry in Bolt gives each machine one identity

- `board.json` gains a top-level `machines` map: each registered name with the former names it replaced.
  Only `boardengine`'s owner setter writes it, pushing synchronously.
- A machine registers at its first owner write or `lym machine name`, copying the former names of its `machine.json`.
  A registration whose former name is another registered machine's current name is refused, naming that machine;
  its way forward is `lym machine name --forget-former <name>` on this machine, which drops that former name from its `machine.json`.
- `lym machine name` records the rename there, besides its sweep of owners (`internal/machinecli/set.go`).
- `lym machine name --forget-former <name>` (new) drops a former name from this machine's `machine.json` and its registry entry;
  owners recorded under that name then read as the machine whose current name it is.
- Every comparison of a recorded machine name with this machine goes through the registry:
  a recorded name that is a former name of a registered machine reads as that machine.
  `boardengine.OwnedElsewhere` takes the registry, and every owner write writes the current name.
  The comparisons that name the machine directly move too:
  `dobbyengine.OwnedWorktrees` and `EnsureLive` compare an owner with the machine's name today, and `pairteardown.ReadRun` reads the owner through `OwnedElsewhere`.
- `internal/machinename` stays this machine's only name source.

Rationale: A8's `osl46` and `osl-1046` are one machine before and after `lym machine name`;
the former names live only in that machine's `machine.json`, so every other machine reads them as two.

### D6 — After an absence, lym catches up from origin before it acts

An absence is any of:

- a long-lived lym process, the dobby loop or the orch watcher, observing a sleep through `pauserecord.SleepWatch` over `pauserecord.SleepTolerance`;
- a worktree's dobby starting, as through `lym s`, `lym beam start` or a hibernate wake;
- a hand pass, which keeps the origin reconcile `PATTERN-owner-machine` gives it, now this catch-up over its run.

Before any act rule answer (D1) after an absence, the catch-up runs:

1. It fetches Bolt, replays local display commits as the board sync does, and reads the owners, the registry and the holds.
2. It fetches each local pair's branches and reads each run's record from the result.
3. For a run this machine owns, local commits on top of origin's tip are this machine's owed pushes, and it pushes them.
   A takeover entry naming another machine is the hand-over `push-before-act-takeover` delivers.
4. For a run another machine owns, a `landing` past its parent-side merge included, it ends this machine's processes for the slug through `pairteardown.EndSubjectProcesses`,
   pushes the code tip the run's last transition on origin names when origin lacks it,
   and archives the local tips as the hand-over does, the local parent branch's unpushed merge of this run included.
   It then removes the local pair through `internal/pairteardown`'s local mode (`Request.Local`),
   which takes no origin step and writes no board: origin's branches and the board claim stay.
5. It writes this machine's pause record entries from the run pause intents of the runs it owns,
   and drops the caches of the runs it touched; each run's first pass after it is a full pass, as the dobby already owes after a sleep.

The dobby's periodic Bolt pull for the owner (`internal/dobbyengine/handover.go`) stays; a hold it reads applies at once (D7).
The dobby's own handover of a subject calls `EndSubjectProcesses` with the dobby end set, and a catch-up the dobby runs leaves it unset.
The origin-owed marker (`internal/battencli/resumereconcile.go`, `dobbyengine`'s `OriginOwed`) retires here:
no pass acts without origin, and this catch-up is the reconcile it stood for.

Bound: a catch-up that cannot reach origin acts on nothing and retries on the owed push's backoff; `lym beam status` shows it waiting on origin.

Rationale: the operator requires lym to check pushed state before acting after the machine slept, hibernated or was off.
The dobby already detects a sleep (`internal/dobbyengine/loop.go`), but only owes full passes and pulls Bolt up to 5 minutes later.

### D7 — Adopt holds live in Bolt; the owner pause entry retires

- `board.json` gains a top-level `holds` list.
  A hold has a kind, `adopt` or `adopt-fix`, the machine, the time, its adopt's subjects,
  and for `adopt-fix` the fix task's slug and each subject's try count (D8).
  Only `boardengine`'s setter writes it, pushing synchronously.
- A hold covers the whole hub: while one stands, the act rule answers `wait` for every run on every machine.
  `pairteardown.ReadRun` sets `runfsm.Inputs.Held` for a run, and leaves it false for the `adopt-fix` hold's fix task;
  the act rule already lets a run `landing` past its parent-side merge act under a hold.
  The refusal of a held run names the hold's machine, so `runfsm` carries it in its inputs and verdict.
  Its way forward is waiting for the hold to clear, or asking the operator when that machine has stopped.
- Every verb that asks the act rule outside the dobby's pass, such as `lym loom resume` and `lym tender resume <slug>`,
  fetches Bolt first when origin answers, and reads the local Bolt only when it does not.
- A machine that reads a hold writes no hub entry.
  It writes a slug entry in its own pause record, under one new caller `hold`,
  for each run of this machine the act rule answers `wait` for under the hold, and none for the `adopt-fix` hold's fix task,
  so the network gate and the turn interruption hold those runs' processes.
  It drops those entries once it reads no hold.
- The `hold` entries hold nothing outside those runs.
  The orch, `lym adopt`'s own process and the children it spawns for the deploy and the check,
  and the operator's `lym batten cancel`, which today only a hub entry refuses, go on under a hold.
- The `hold` entries keep apart from the operator's hub entry and hibernate's.
  Dropping them drops only entries of the `hold` caller,
  and the hibernate wake, which skips its reconcile under a hub entry, never reads a `hold` slug entry as one.
  A slug that already has a live entry gets no `hold` entry, since the pause record keeps one live entry per scope.
  `lym tender resume` lifts no `hold` entry while the Bolt hold stands;
  when it lifts a slug's own entry under a standing hold, it writes the `hold` entry in its place.
  `lym tender resume <slug>` of a slug only a `hold` entry covers refuses, naming the hold's machine and the way forward above.
- A run owned by another machine is not paused: the act rule answers `hand_over`, and `pairteardown.EndSubjectProcesses` ends its processes.
  So the `pauserecord` caller `owner` retires, with every writer of it:
  the dobby's owner pause (`internal/dobbyengine/owned.go`, its `PauseOwned` seam),
  `tendercli.PauseOwnedElsewhere` and the process end beside it,
  the hibernate wake's hold of runs owned elsewhere, and the owner entries adopt's pause step reads.
  Each becomes the act rule's `hand_over` with the subject-process end; a Finalize past its parent-side merge hands over too, and its owner finishes it.
  The `adopt` and legacy `adopt-hold` callers stay written by today's adopt until `adopt-foreground-leftovers` retires that worker.
- At the first read after the upgrade, a standing `adopt` or `adopt-hold` entry of a run this machine owns in a state that is not terminal
  becomes a run pause intent (D3), so no run resumes unasked.
  Every other standing entry of those callers, and every standing `owner` entry, is dropped.
  When at least one entry became a pause intent or belonged to a released `queued` task, that first read also writes one Bolt `adopt` hold
  naming this machine and the subjects of those runs and tasks, so no queued task starts;
  a re-run of `lym adopt` of those subjects here clears it at its green check.
  That first read posts one notice to the operator naming the hold, its machine,
  and as the way forward a re-run of `lym adopt` on that machine of the hold's subjects.
  A first read whose entries all belong to landed runs or runs owned elsewhere writes no hold, so every upgrade hold names subjects to re-run.
  A first read that finds another machine's hold standing writes none, so one machine holds the hub at a time;
  its notice names that hold's machine, and the re-run here once that hold clears.
  Like every notice an agent reads, neither names `--take-hold`.
  A released task's entry with no batten record is dropped, and the task stays `queued` under the hold.

Bound: a machine other than the adopter learns of a hold at its next Bolt pull or catch-up:
its dobby's pull within the dobby's pull interval, and a verb's before it asks the act rule.
Until then its dobby goes on with its runs as before, for at most the dobby's Bolt pull interval; the takeover's bound in `push-before-act-takeover` covers the transitions it pushes.
A verb that cannot reach origin reads the local Bolt, and any transition it leads to waits on origin.
A run that holds its parent lock is exempt from slug entries, so the gate does not hold its step in flight;
the act rule still starts no producer for it.
A hold stands until a green check clears it: that of the adopt holding it, or the fix task's re-check under `adopt-fix`;
a hold whose adopter stopped stands until the operator takes it over with `lym adopt --take-hold` on another machine (D8).
The orch is up while a hold stands, and every verb the act rule guards refuses except on the fix task and on a landing past its parent-side merge.

Rationale: the operator decided nothing happens in the hub until adopt is done and its failing tests are fixed;
the pause record is machine-local, so no other machine sees a hold kept there (state map 3.2, 6.4 items 7 and 11).

## Constraints

- `PATTERN-owner-machine`: the owner stays on roots and beams, written only by `boardengine`'s setter, read through `hubgeom.ParentOf` and Bolt's machine registry.
  The rest of a Finalize past its parent-side merge finishes on the run's owner.
- `PATTERN-pair-remote`: a catch-up's local pair removal goes through `internal/pairteardown`'s local mode and deletes no origin branch.
- `PATTERN-dobby-runs`: runs are still started only by the parent's dobby, which starts only with its orch bar `--no-orch`.
- `PATTERN-lym-files`: every new tracked file sits under `_lym` and every cache under `.lym`, both named in `internal/lymdirs`.
- `PATTERN-tolerant-records`: the Bolt `machines` and `holds` keys read tolerantly and are written through the board store.
- `PATTERN-fabric-boundary` and `PATTERN-gitrepo-client-boundary`: the catch-up's fetches and pushes run in `internal/fabricengine` through `gitexec` and `gitrepo`.
- `PATTERN-child-processes`: the subject-process end goes through `internal/runproc`.
- `PATTERN-daemon-wakeups`: the catch-up's retry waits wake at most once a second and back off while idle.
- `PATTERN-refusal-way-forward`: every refusal the act rule, the holds, the registry's clash and the owner refusal add names its way forward and a test reaches it;
  none an agent reads names `lym adopt --take-hold`, which a shell-guard row denies agents (D8).
- `PATTERN-test-only-code`: each exported setter and reader has a production caller in this part,
  the hold setter's being the upgrade's first read and the catch-up entry point's the dobby start.

## Auto-mode assumptions

## Open risks

## Acceptance criteria

- D5: a registration whose former name is another machine's current name is refused naming that machine and `lym machine name --forget-former <name>`; after that verb, the registration lands.
- D5: an owner recorded as a former name of a registered machine reads as owned here on that machine and as that machine on another, in `dobbyengine.OwnedWorktrees` and `pairteardown.ReadRun` too.
- D5: a machine's first registration carries the former names of its `machine.json`; `lym machine name <new>` puts the old name among them on origin when it returns.
- D6: a dobby that observes a sleep runs the catch-up before its next pass; a run another machine took meanwhile gets no pass, its processes end, and its local pair is removed with its origin branches left standing.
- D6: a run owned here whose local records are ahead of origin with its own commits has them pushed by the catch-up, and none is reset.
- D6: a run owned elsewhere at `Finalize` past its parent-side merge is handed over like any other run: its processes end here, and its owner finishes the Finalize.
- D6: with origin unreachable, the catch-up acts on no run and retries.
- D6: no code path reads or writes the origin-owed marker.
- D7: under an `adopt` hold no queued task is claimed and no run but a landing past its parent-side merge gets a pass, on the adopting machine and on another machine after its Bolt pull; under `adopt-fix` the fix task gets passes too.
- D7: a machine reading a hold holds its runs' processes through `hold` slug entries in its pause record, writes no hub entry, and drops only those entries once the hold clears, leaving an operator's hub pause standing.
- D7: under an `adopt-fix` hold the fix task's agent turns and network calls pass the gate, and the orch is not interrupted.
- D7: under a hold a run `landing` past its parent-side merge finishes, and the try count of an `adopt-fix` hold reads the same on every machine.
- D7: a verb refused by a hold names the hold's machine and a way forward that names no `--take-hold`.
- D7: `lym loom resume` on a worktree whose dobby is down refuses a run under a hold another machine wrote since that worktree's last Bolt pull.
- D7: a subject read as owned elsewhere leaves no process of its runs, its orch or its dobby running on this machine, and no code path writes the `owner` caller.
- D7: a standing `adopt` pause entry from before the upgrade becomes a pause intent on its run, which stays paused; a standing `owner` entry is dropped.
- D7: a standing `adopt` pause entry from before the upgrade writes one Bolt `adopt` hold naming this machine and posts one notice naming the hold, its machine and a re-run of `lym adopt` there, and no notice names `--take-hold`; a landed run's standing entry is dropped, an upgrade whose standing entries all belong to landed runs or runs owned elsewhere writes no hold, one with a released queued task's entry writes a hold naming that task's subject, and a second machine's upgrade under a standing hold writes no hold.

## PATTERN changes

- `PATTERN-owner-machine` — changes: Starting, resuming, pushing or landing a run, a Finalize past its merge included, or picking a pair's parent: only on the entry's owner machine, read through hubgeom's one `ParentOf` and Bolt's machine registry; after an absence, a catch-up from origin first.
