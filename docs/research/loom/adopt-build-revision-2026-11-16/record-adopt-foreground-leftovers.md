## Goal

lym adopt runs in the foreground over pushed state, with --take-hold

Takes: the holds, registry, catch-up and owner setter from bolt-holds-catch-up; the takeover entry type, its reader and the synchronous pushes from push-before-act-takeover; runfsm's states and the refusals that name `lym adopt <subject>`; adoptcli's fix loop, check, deploy, archives and here paths; the adopt record's readers in the dobby's handover and the orch watcher
Delivers: lym adopt running hold, owner, pull, takeover, catch-up, deploy, workspace, check and green clear in the caller's process, re-runnable from tracked state, with the takeover entry's production writer and its rejected-push retry, --take-hold denied to agents by a shell-guard row, and --here keeping its owned_here and no_subject outcomes for `lym s`; the worker, adopt record and its readers, verdicts, pause step, the writers of the adopt and adopt-hold pause callers, --keep-local, --drop-local and --discard retired, the dobby's handover and the orch watcher reading a hold in the record's place; adoptcli's and adoptrecord's package docs rewritten to the foreground flow
Exports: adopt's foreground step runner and the takeover entry writer

## Scope

Decision numbers are those of the Sketch `adopt-redesign`.
The run FSM (D1, D3) and the orch's own-session resume (D9, D10) have landed,
the synchronous push and the takeover (D2, D4) come from `push-before-act-takeover`,
the registry, the catch-up and the holds, with the upgrade's first read, (D5 to D7) from `bolt-holds-catch-up`,
and the leftover pairs (D11) are a part of their own.

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

## Decisions

### D8 — `lym adopt` runs in the foreground: hold, owner, pull, takeover, check

`lym adopt <subject>...` runs every step in the caller's process and prints each step as it ends.
It starts no detached worker and writes no adopt record.
Each step reads tracked state, so a re-run after an interruption goes on from where the state stands.

1. One Bolt commit writes an `adopt` hold, this machine as the subjects' owner, and this machine's registry entry, and pushes it.
   A hold another machine wrote refuses the adopt, unless it runs with `--take-hold` (below), naming the writer machine and the hold's time,
   and as way forward waiting for that machine's adopt to clear it, or asking the operator when that machine has stopped.
   A rejected push reads Bolt again; an owner of these subjects, or any hold, that another machine wrote since the read
   refuses the adopt naming that machine; otherwise it replays and pushes.
   A registration the registry refuses refuses the adopt with the registry's way forward.
2. For each moved run, it fetches the run's branches, archives this machine's local tips that origin lacks as the takeover's hand-over does, resets them to origin's,
   and adds the pair from origin when this machine has none.
3. It writes the takeover entry on each moved run's records branches and pushes it.
   Once it lands, it fast-forwards each moved run's local records branches to origin's tips, which the takeover entry now heads,
   and its local code branch to origin's code tip.
   A code tip the last transition names that origin still lacks is the owner's dobby's to fast-forward to once it lands.
4. The catch-up over the whole hub.
5. The deploy, as today.
6. The cache it rebuilds: the hub workspace file.
7. The check: the hub's `adopt` gate runs in each subject's worktree, in the foreground, with its output on the terminal.
8. A green check clears the hold and runs `orchcli.BeamUp` for each subject; the runs go on by the act rule, with no hand lift.
   A failing check files a fix task through the existing fix loop (`internal/adoptcli/fixloop.go`, `fronttask.File`),
   replaces the hold with an `adopt-fix` hold naming it, runs `orchcli.BeamUp`, and exits non-zero naming the task.
   The task's landing runs the check again; green clears the hold, and failing files the next task within `adopt_fix_tries`.
   Each subject's try count lives in the `adopt-fix` hold, rewritten with each filing,
   so a re-check on any machine reads the same count against that subject's limit.
   An adopt that took over an `adopt-fix` hold keeps its standing fix task instead (below).
   At the limit the hold stays and the dobby's notice names the failing tests and the fix task;
   the hold clears only once the failing tests are fixed and a re-run of `lym adopt` checks green.

`lym adopt --take-hold <subject>...` (new) takes over a hold another machine wrote, such as one whose adopter stopped.
Its step 1 refuses a hold another machine wrote unless that hold names exactly these subjects, naming the hold's subjects as the way forward.
With no standing hold, or one this machine wrote, it runs as `lym adopt` does.
Otherwise its Bolt commit rewrites the hold to name this machine, keeping its kind, fix task and try counts,
writes this machine as the subjects' owner, and pushes; the adopt then runs steps 2 to 8.
Under an `adopt` hold its green check clears the hold.
Under an `adopt-fix` hold the fix task stands, a failing check files no second task,
and the hold clears at the adopt's green check or the fix task's green re-check.
An adopt re-reads its own hold from Bolt before each step from step 2 on,
and stops once the hold is gone or names another machine, naming the subjects' owner it reads from that Bolt and starting nothing,
so a taken hold leaves one adopter, and the stopped adopter's subjects go on under the adopt that took them.
A shell-guard row denies `--take-hold` to every agent, and no refusal or notice an agent reads names it;
adopt's help names it.

Kept: `--here`, `--archives`, `--no-orch` and `--all`.
`--here` stays behind `lym s`, now in the foreground, and keeps its fast outcomes:
in a worktree with no subject it ends ok with `no_subject`,
and when no other machine owns the subject it pays one Bolt pull and ends ok with `owned_here`, writing no hold and starting nothing.
Retired: `--keep-local`, `--drop-local`, `--discard`, `lym adopt worker`, the adopt record, the held-back project verdicts, the adopt pause step,
and every writer of the `adopt` and `adopt-hold` pause callers.
The dobby's handover and the orch watcher, which ask the adopt record whether an adopt here covers a worktree, read a hold this machine wrote instead.
The two caller names stay declared only for the upgrade's first read.

Bound: adopt works from pushed state alone and never asks about another machine's local state;
the old owner's unpushed work reaches the adopter only through the takeover's archive tags, which `--archives` lists,
besides the code tip of a transition already on origin.
While a hold stands the orch is up, but every verb the act rule guards refuses except on the fix task and a landing past its parent-side merge.
The hold's pause entries cover only held runs,
so the orch, adopt's deploy and check with their children, and the operator's `lym batten cancel` go on.
`--take-hold` skips no check and lets no run go on before a green check: it moves the hold and its adopt to this machine.
Only the operator's own shell runs it.

Rationale: the operator decided adopt runs in the foreground and that nothing happens in the hub until it is done and every failing test it found is fixed.
The synchronous push and the takeover leave no local state for adopt to reconcile, which removes the stops A2, A3, A5, A12 and A13.

## Constraints

- `PATTERN-owner-machine`: the owner stays on roots and beams, written only by `boardengine`'s setter, read through `hubgeom.ParentOf` and Bolt's machine registry.
- `PATTERN-dobby-runs`: runs are still started only by the parent's dobby, which starts only with its orch bar `--no-orch`.
- `PATTERN-lym-files`: every new tracked file sits under `_lym` and every cache under `.lym`, both named in `internal/lymdirs`.
- `PATTERN-tolerant-records`: the takeover entry reads tolerantly and is written through `internal/state`.
- `PATTERN-fabric-boundary` and `PATTERN-gitrepo-client-boundary`: the takeover entry's push and the archive run in `internal/fabricengine` through `gitexec` and `gitrepo`.
- `PATTERN-refusal-way-forward`: every refusal this part adds names its way forward and a test reaches it;
  none an agent reads names `lym adopt --take-hold`, which a shell-guard row denies agents.
- `PATTERN-shell-guard-table`: the deny of `lym adopt --take-hold` is a row of `internal/shellguard`'s table.
- `PATTERN-test-only-code`: the takeover entry writer, the foreground step runner and every other new function have a production caller.

## Auto-mode assumptions

## Open risks

## Acceptance criteria

- D8: `lym adopt <beam>` from a second machine's hub, with origin as the only shared state, ends with the beam's runs in the same FSM states as on the first machine and this machine as owner, with no `--keep-local` or `--drop-local`.
- D8: an adopt run on a machine with no local pair of a moved run adds the pair from origin before it writes the takeover entry.
- D8: two machines adopting at once end with one owner; the other's adopt refuses naming the winner.
- D8: a failing adopt check files one fix task, leaves an `adopt-fix` hold naming it, and exits non-zero; the task's green re-check clears the hold and the runs go on with no hand lift.
- D8: `lym adopt --take-hold` of the hold's subjects on another machine rewrites the hold to name that machine, and the hold clears only at that adopt's green check or, under `adopt-fix`, the fix task's green re-check; with other subjects it refuses naming the hold's subjects; an agent's shell is denied it.
- D8: a hold written by a machine that then stops is taken over by `lym adopt --take-hold` on another machine, whose adopt then runs its steps and check.
- D8: an adopt killed after its owner write goes on from the tracked state on its re-run and ends as an uninterrupted adopt does.
- D8: a transition the old owner pushed between the adopter's fetch and its takeover is on the adopter's local branches before the run acts, and the run goes on from it.
- D8: an adopter whose hold another machine took over stops at its next hold re-read, naming that machine, and starts no dobby.
- D8: a registration the registry refuses at step 1 refuses the adopt with the registry's way forward, and the adopt goes on after it.
- D8: a second adopt with `--take-hold` landing between an adopter's step 2 and step 3 stops the first adopter at its takeover retry, naming the second's machine, and the second adopter drives the runs.
- D8: `lym adopt --here` on the machine that owns the subject ends ok with `owned_here` after one Bolt pull, writes no hold and starts no run.
- D8: no code path writes the `adopt` or `adopt-hold` pause caller, and the dobby's handover and the orch watcher read a hold where they read the adopt record.

## PATTERN changes

- `PATTERN-dobby-runs` — changes: Starting or holding a board task's run: only its parent's dobby, started with its orch bar `lym adopt --no-orch`; front tasks only via `fronttask`; an adopt holds the hub through Bolt's holds until an adopt's check is green.
