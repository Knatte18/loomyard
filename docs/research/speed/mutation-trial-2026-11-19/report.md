# Mutation trial: `lym gate mutate` on 20 landed diffs

Decides whether the `mutation` gate report step is built.
It records one measurement taken on 2026-11-19 against the code at `49c494cb` and is not rewritten when the code changes.

## Decision

Build the `mutation` report step as a later darn, listed in the template's `diff` keys, after three bound changes (below).

- **Findings.**
  The 20 replays ran 430 mutants.
  374 were killed, 44 survived and 12 were unjudged.
  Of the 44 survivors, 26 are a check a reviewer would act on, and 18 are not (12 no test could reasonably kill, 6 low value).
  The 26 are 59 % of the survivors, a majority, but one reader's judgment and not a reviewer's.
- **Spread.**
  11 of the 20 diffs have at least one such survivor, 3 have only survivors not worth acting on, and 6 have none.
- **Cost.**
  The runs took 10 minutes of wall time in all, 30 s at the median and 80 s at most, with the verb refreshing the maps itself.
  No run came near a bound that would refuse it.
- **Why conditional.**
  The 40-mutant cap and the noise classes below decide whether the step pays on a large diff, so the darn fixes them first.

## Method

- **Landings.**
  20 first-parent landings of `main` from the last 150, each changing Go production code, picked by the diff stat of
  non-test `.go` files: 4 under 100 changed lines, 11 from 100 to 300, 5 over 300 (the largest 5 427 lines).
  They touch `boardengine`, `creelengine`, `dobbyengine`, `fabricengine`, `heddleengine`, `lymcwd`,
  `proc`, `runplan`, `shuttlecli`, `shuttleengine`, `testcover`, `websterengine` and others.
  The landing that built the verb (`49c494cb`) was left out.
- **Replay.**
  One run per landing, no repeats, in a hub pair checked out detached at the landing, with
  `lym gate mutate --base <first parent>`.
  Every tree was clean, so the verb refreshed its own maps of the changed packages.
- **Judging.**
  Each survivor was judged by reading its line in the landing's tree and the tests named as possible killers.
  *Missing check*: a behavior a test could assert, and the landing's own behavior or a contract the doc comment states.
  *Not reasonably killable*: a log field, a log-throttle call, a pure cache, a platform-only branch, a file-descriptor leak or a channel capacity.
  *Low value*: a missing check on formatting, an unread id or an environment-bound branch.
- **Not measured.**
  A reviewer's own judgment of the survivors, and any survivor of a mutant the cap left unrun.

## Counts per diff

`not run` is the mutants the verb planned and left unrun for `mutation_max_mutants`, which is 40 here.
`uncovered` is the changed blocks no test covers.

| # | Landing | First parent | Changed funcs | Run | Killed | Survived | Unjudged | Not run | Uncovered | Not judged | Wall s |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `bd00aab1` | `221c68e3` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0.4 |
| 2 | `5973ebb4` | `bd00aab1` | 1 | 3 | 3 | 0 | 0 | 0 | 1 | 0 | 14 |
| 3 | `a3b16a70` | `6a99d2f5` | 2 | 4 | 4 | 0 | 0 | 0 | 0 | 0 | 2.6 |
| 4 | `f26165a6` | `0b42877c` | 2 | 5 | 3 | 2 | 0 | 0 | 1 | 0 | 6.3 |
| 5 | `99bf612a` | `da4d7efb` | 5 | 14 | 14 | 0 | 0 | 0 | 1 | 0 | 3.0 |
| 6 | `744c720f` | `99bf612a` | 3 | 6 | 1 | 3 | 2 | 0 | 19 | 0 | 49 |
| 7 | `22a2c25b` | `bc7737df` | 10 | 39 | 29 | 9 | 1 | 0 | 1 | 0 | 25.5 |
| 8 | `0291b5bf` | `22b74f24` | 13 | 40 | 34 | 6 | 0 | 10 | 2 | 0 | 17.8 |
| 9 | `64e58795` | `5973ebb4` | 12 | 40 | 39 | 1 | 0 | 1 | 0 | 0 | 28.8 |
| 10 | `4eab123d` | `45d8b10e` | 13 | 29 | 25 | 4 | 0 | 0 | 24 | 0 | 42.6 |
| 11 | `db1c9e2c` | `16881b21` | 9 | 13 | 11 | 2 | 0 | 0 | 5 | 0 | 35.5 |
| 12 | `9ce7233b` | `15cc35e7` | 17 | 40 | 37 | 3 | 0 | 69 | 31 | 0 | 30.2 |
| 13 | `1b405351` | `3bbc92a0` | 10 | 21 | 20 | 1 | 0 | 0 | 5 | 0 | 39.1 |
| 14 | `a8465747` | `1e0ac9b1` | 11 | 14 | 9 | 2 | 3 | 0 | 9 | 0 | 28.5 |
| 15 | `b2141fcb` | `380101d7` | 9 | 13 | 13 | 0 | 0 | 0 | 0 | 0 | 39.2 |
| 16 | `17f374e9` | `0198b955` | 16 | 40 | 37 | 0 | 3 | 3 | 0 | 0 | 67 |
| 17 | `99bec44e` | `85a40118` | 27 | 6 | 5 | 1 | 0 | 0 | over 100 | 0 | 80 |
| 18 | `95418a15` | `96b20f1c` | 195 | 40 | 32 | 5 | 3 | 623 | 214 | 1 | 40 |
| 19 | `e761f0a1` | `a875a70a` | 173 | 40 | 38 | 2 | 0 | 19 | 680 | 0 | 30 |
| 20 | `15de027c` | `99bec44e` | 17 | 23 | 20 | 3 | 0 | 0 | 12 | 0 | 24 |
| | | | | 430 | 374 | 44 | 12 | 725 | | 1 | 604 |

The uncovered counts are the entries of the envelope's list, a function once per block.
The counts of #6, #10 and #12 were tallied by eye from the printed list, and the list of #17 was cut off when read, so its count is a lower bound.
Landing #1 (`bd00aab1`) removes code and changes no function, so the verb mutated nothing and printed `the diff changes no function`.

## Survivors

26 survivors a reviewer would act on, by diff:

| # | Survivor | Why it is a missing check |
| --- | --- | --- |
| 6 | `fabricengine/pushanchored.go:221` retry counter start | no test asserts how many retries a records push makes |
| 7 | `shuttleengine` `hold.go:206`, `run.go:885`, `904`, `905`, `951`, `952`, `1014`, `send.go:625` | the prompt-offset and deadline hand-off to a send, which the lost-prompt retype reads, is unasserted |
| 8 | `heddleengine/overview.go:102`, `153`, `208` | the overview's table header, its engine wording and an entry's trimmed first line are unasserted |
| 9 | `boardengine/intake.go:181` match limit | no test has one text naming several paths |
| 10 | `dobbyengine/loop.go:698` | no test shows a tick clearing a done run |
| 11 | `shuttlecli/cli.go:88` | no test shows await-forks resolving from below the anchor |
| 12 | `testcover/mutant.go:258`, `554`, `577` | the innermost-block pick and the field-name filter of variable reads are unasserted |
| 13 | `shuttleengine/editloop.go:58` | a clone of the edit-loop watch may lose its history unnoticed |
| 18 | `creelengine` `fold.go:212`, `859`, `machinemail.go:303` | a returned machine message's status, a carried event kind and a retry reset are unasserted |
| 19 | `runplan/report.go:73`, `websterengine/recordbatch.go:540` | a report with an unknown key is accepted, and the OK flag of the post-batch checks is unasserted |
| 20 | `heddleengine/store.go:100`, `153` | the store version a newer binary wrote is not kept, and an upgraded store's version is not set |

The 6 low-value missing checks: `proc/memorycap_linux.go:163` (needs a real systemd scope), `heddleengine/overview.go:277`,
`307`, `308` (JSON formatting), `creelengine/server.go:340` (an id no client reads) and `heddleengine/target.go:78`
(a defensive slice copy).

The 12 not reasonably killable: `proc/memorycap_linux.go:179` (descriptor leak); `fabricengine/pushanchored.go:218`, `219`
(real-sleep fallback); `shuttleengine/run.go:911` (log field); `dobbyengine/donerun.go:52`, `55`, `77` (log throttle and log);
`lymcwd/anchor.go:147` (Windows-only branch); `creelengine/server.go:434` and `deliver.go:73`, `outbound.go:150`
(a write failure between two writes, a channel capacity, a logged error); `testcover/blockkey.go:174` (a pure cache).

## Unjudged mutants and packages not judged

- **Unjudged.**
  12 of 430 mutants (2.8 %), all "a covering test ran past its time bound with the mutant on".
  The diffs: #6 (2, slow fabric push tests), #7 (1, a dropped mutex unlock), #14 (3, creel write and wait paths),
  #16 (3, a head and tail cut that stops shrinking) and #18 (3, creel scheduler waits).
  Several look like a mutant that hangs its covering test, which would in effect be a kill; this was not checked.
- **Packages not judged.**
  One in 20 runs: `internal/testkit/creelfake` in #18, with
  `block table run: exit status 127 ... cover.test: not found` under `/tmp/lym-1000`.
  The failure is of the build's temp directory, not of the diff, and no other run printed one.

## What the bounds did

- **The mutant cap.**
  `mutation_max_mutants` stopped 6 of the 20 runs (#8, #9, #12, #16, #18, #19) at 40 mutants and left 725 unrun, 623 of them in #18.
  The cap takes the changed functions in turn, so on a large diff the first functions are mutated and the rest never.
  The survivors in the table above say nothing about the unrun ones.
- **Time and memory.**
  The slowest run took 80 s, and no envelope reports a stop of a whole run for its time or memory bound.
- **Coverage.**
  Large and moved-code diffs (#17, #18, #19) have hundreds of uncovered changed blocks, which the verb lists but cannot mutate.
  The list is long enough to bury the survivors in the printed envelope, so the report step prints counts and writes the list to `mutation.json`.

## Changes to make before or with the step

1. Spread the mutants across the changed functions, such as one per function first, so a large diff is not judged on its first functions only.
2. Skip as a mutation site a statement that only logs and a branch that tests `runtime.GOOS` for another platform:
   they are most of the 12 survivors no test could reasonably kill, and the 12 are 27 % of all survivors.
3. Keep `mutation.json` for the full list and have the step print one summary line:
   mutants run, killed, survived, unjudged, uncovered changed blocks, packages not judged.

The step feeds a `mutation_report` marker of the code review rubric with the survivors, so a reviewer reads the missing-check ones first.
The 18 survivors that are noise today are the reason to make change 2 before the marker is trusted.

## Data

The full envelopes of the three largest runs are in `data/`: `run-18-95418a15.json`, `run-19-e761f0a1.json` and `run-20-15de027c.json`.
The other counts above are read from the printed envelopes.
