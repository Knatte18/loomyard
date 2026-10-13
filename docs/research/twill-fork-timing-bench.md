# twill fork timing bench: spawn before writing against spawn after writing

2026-10-13, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-fork-timing`.
Segment: Discussion-Write, twill `opus[medium]`, today's hub setting.

## Setup

- Why Discussion: the Discussion bench (`docs/research/twill-discussion-bench.md`, rounds 1, 4 and 5) has a spawn-before twill `opus[medium]` arm with a 13-finding grading key (`reference/round-{1,2,3}-review.md`), token reports, transcripts, and a setup that still reproduces: base `f66644766` (`50cf6a425` plus the bench-doc deletion) and the note body `docs/research/twill-discussion-bench/input-solo-arms.md`.
  The Plan bench's Round 2 was not needed.
- Reference: `webster-stability`, the only Discussion reference with a key, so "per reference" means this one reference.
- Pairs `twb-ftb-{1,2,3}` (before arm, today's directive unchanged) and `twb-fta-{1,2}` (after arm), each from `lyx fabric add` with a `twb-*` note carrying `input-solo-arms.md`, code branch reset to `f66644766`.
- Pair `_lyx/config/loom.yaml` (sparse): `discussion: opus[medium]`, `discussion_producer: twill`, `parent_review_wait_min: 2`, `alignment_timeout_min: 1`, `skip_alignment: true`.
  The wait could no longer be 1 as in round 4, since the alignment timeout must sit below it.
- Run: `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, `lyx loom step`, `lyx loom pause --after Discussion-Write`, `lyx loom step --until-stop`.
- Prod `lyx` was `e17b29219` for every run (deployed 10:50 local, before the first run).
- After arm: pair `_lyx/config/shuttle.yaml` sets `claude:` to a wrapper (`twill-fork-timing/wrap-after.sh`) that rewrites the run's `prompt.md` before it execs `claude`; the exact change is `twill-fork-timing/after-arm-prompt.diff`.
  - Spawn point: "When the design is settled, run this procedure once. You spawn the forks only after the reviewed files are written, pass the self-check and are marked ready:".
  - Steps reordered to: read the rubric; write the files and run `lyx loom validate-discussion` until it passes; write the ready marker ("Spawn no fork before that file exists."); spawn the six forks in one message; then steps 5-8 unchanged.
  - Fork boilerplate's read line: "After `ready: true`, review <files> through your lens as an outsider: your context already holds them as the writer wrote and edited them, so read one only where your context lacks its final text."
  - Model, effort, the six `twill-discussion` lenses, the rejection rule, the await line, records and base are the same as the before arm.
- Wave 1 (ftb-1, fta-1) ran in parallel from 10:52 local; wave 2 (ftb-2, ftb-3, fta-2) in parallel from 11:05, once wave 1's cost was known.
- Measures: per-session usage from the transcripts (`twill-fork-timing/usage.sh`, deduplicated by message id, the writer's spawn message excluded from each fork, priced with `tools/tokencount/cost.go`'s Opus 5.5 row); `go run ./tools/tokencount` reports in each run's `tokens.md`; writer time is first assistant message to the parent-review delivery prompt (`wtime.sh`); lens counts and re-reads from `lens.sh`; spawn order from `seq.sh`.
- Records, support logs, transcripts and token reports: `.scratch/twill-fork-timing/{ftb-1,ftb-2,ftb-3,fta-1,fta-2}/`.

## Spawn point actually taken

| Run | Arm | Files written | Self-check | Marker | Forks spawned |
|---|---|---|---|---|---|
| ftb-1 | before | 08:57:05-22 | 08:57:59 | 08:58:01 | 08:57:32-57 |
| ftb-2 | before | 09:11:32-52 | 09:11:52 | after | 09:10:14-37 |
| ftb-3 | before | 09:13:12-30 | 09:13:30 | after | 09:11:44-12:08 |
| fta-1 | after | 08:59:32-51 | 08:59:52 | 08:59:56 | 09:00:02-26 |
| fta-2 | after | 09:10:25-41 | 09:10:42 | 09:10:46 | 09:10:52-11:17 |

- ftb-1 broke today's directive: it wrote both files, then spawned, then self-checked and wrote the marker, and said so in its support log.
  Its forks therefore inherited the written files but re-read them after the marker, a half-way arm; it is reported apart.
- Of the ten Discussion twill runs on record with the spawn-before directive (tw-om-2, twc-1, twc-2, twp-1, twp-2, twh-1, twh-2, ftb-1, ftb-2, ftb-3), three (twc-1, twp-1, ftb-1) wrote their files before spawning.
- Every run spawned its six forks in one API message, all six returned, and every fork returned before its writer ended.
- No after-arm fork read the record or the support log; every before-arm fork read one or both after the marker (ftb-1 five of six).

## Results

Cost from the transcripts, orch excluded.
Fork output is counted from each message's last transcript line; see Deviations for why tokencount's fork figures are lower.

| Run | Arm | Writer | Forks | Total | Writer time | Writer msgs | Fork msgs |
|---|---|---|---|---|---|---|---|
| ftb-2 | before | $3.73 | $2.09 | $5.82 | 10m13s | 77 | 33 |
| ftb-3 | before | $3.41 | $1.62 | $5.03 | 11m36s | 70 | 29 |
| ftb-1 | half-way (wrote, spawned, then marker) | $3.01 | $1.52 | $4.53 | 8m18s | 62 | 29 |
| fta-1 | after | $3.23 | $1.09 | $4.32 | 9m00s | 68 | 22 |
| fta-2 | after | $3.07 | $1.49 | $4.56 | 8m34s | 62 | 28 |
| twc-2 (2026-10-11) | before | $3.61 | $1.80 | $5.41 | 9m23s | 70 | 28 |
| tw-om-2 (2026-10-11) | before | $2.63 | $1.76 | $4.39 | 7m52s | 52 | 32 |
| twc-1 (2026-10-11) | half-way | $3.40 | $1.62 | $5.02 | 9m30s | 65 | 29 |

Tokens (output / cache write / cache read / input):

| Run | Seat | Output | Cache write | Cache read | Input |
|---|---|---|---|---|---|
| ftb-2 | writer | 53,226 | 186,739 | 8,649,592 | 166 |
| ftb-2 | 6 forks | 31,987 | 85,561 | 5,085,151 | 78 |
| ftb-3 | writer | 48,120 | 179,788 | 7,749,767 | 152 |
| ftb-3 | 6 forks | 17,530 | 80,081 | 4,345,553 | 70 |
| ftb-1 | writer | 45,537 | 163,046 | 6,419,097 | 136 |
| ftb-1 | 6 forks | 24,587 | 47,669 | 3,959,857 | 70 |
| fta-1 | writer | 50,372 | 167,474 | 6,937,883 | 148 |
| fta-1 | 6 forks | 15,338 | 27,600 | 3,167,810 | 56 |
| fta-2 | writer | 47,415 | 170,384 | 6,327,647 | 134 |
| fta-2 | 6 forks | 23,952 | 40,848 | 4,092,548 | 68 |
| twc-2 | 6 forks | 24,712 | 85,330 | 4,355,014 | 72 |
| tw-om-2 | 6 forks | 27,497 | 84,667 | 3,976,307 | 76 |

| Arm | Runs | Mean writer | Mean forks | Mean total | Fork cache write per fork | Fork wall (tokencount) |
|---|---|---|---|---|---|---|
| before, fresh | ftb-2, ftb-3 | $3.57 | $1.86 | $5.43 | 13.8k | 3m8s-3m18s |
| before, fresh and earlier | + twc-2, tw-om-2 | $3.35 | $1.82 | $5.16 | 14.0k | |
| after | fta-1, fta-2 | $3.15 | $1.29 | $4.44 | 5.7k | 1m8s-1m21s |

Per fork: before $0.27-0.35, after $0.18-0.25.
Bench spend $24.26 (writers $16.45, forks $7.81), orch excluded; tokencount attributes $1.03-1.99 of orch per run, mostly the orch's own hub traffic and the parent-review approvals.

## Lens counts

Severity-tagged lines in each fork's final message; dispositions from each support log's lens-review list.

| Lens | ftb-2 | ftb-3 | ftb-1 | fta-1 | fta-2 |
|---|---|---|---|---|---|
| premises | 4 | 4 | 9 | 4 | 3 |
| goal-scope | 7 | 4 | 6 | 7 | 6 |
| board-fit | 6 | 4 | 5 | 5 | 6 |
| pattern-fit | 6 | 5 | 6 | 6 | 6 |
| scenarios | 11 | 8 | 7 | 8 | 8 |
| leanness | 7 | 11 | 7 | 7 | 8 |
| Total | 41 | 36 | 40 | 37 | 37 |
| Disposition lines | 33 | 24 | 27 | 29 | 27 |
| Rejected, each with a one-line reason | 2 | 1 | 2 | 5 | 3 |
| Fork tool calls after the wait | 21 | 17 | 17 | 10 | 16 |

- The after-arm forks made fewer calls because none re-read the files; the extra calls in the before arm are exactly those re-reads plus code reads.
- After-arm premises forks found the fewest (3-4), as did the fresh before-arm premises forks (4 each); ftb-1's premises fork found 9.
- Rejections run higher in the after arm (5 and 3 against 2 and 1); each names its reason, and ftb-2 alone cites code (`len(gate) == 0`) in one.
  With two runs this is a signal to watch, not a measured effect.

## Grading

Key, letters and method as `twill-discussion-bench.md`: R repeated, P partly, A avoided, n/a another route.
One grader graded all five fresh records with the same reading, so fresh runs compare with each other; round 4's grades came from another session and read R2-F2 and R1-F6 more leniently, so twc-2's 1 / 2 does not compare cell for cell.

| Reference finding | ftb-2 | ftb-3 | ftb-1 | fta-1 | fta-2 |
|---|---|---|---|---|---|
| R1-F1 deadline outcome | P (held until Timeout; deadline outcome unstated) | P (`EndedShells` at the deadline, outcome unstated) | P (same) | P (same) | P ("delays a Done", outcome at the cap unstated) |
| R1-F2 `unix`/GOOS | P (`unix` via go/build; android/ios unstated) | A (`unix`, `linux` under `android` named) | A (android, ios, illumos, `unix`) | P (`unix` from GOOS; android/ios missing) | P (same) |
| R1-F3 contract evidence | R ("keeps reading Master's writes only") | R (contract evidence "Master sessions only") | R (keeps Master and forks) | R (unchanged, criterion "no evidence to the contract-file check") | R (contract evidence never sees a recovery) |
| R1-F4 reset widening | A (own-path set widens, discard stated, criterion) | n/a (reset unchanged) | n/a (reset unchanged) | P (a criterion reads recovery evidence "for a reset", the widening unstated) | A (widening, its bound and a criterion) |
| R1-F5 re-prompt path | A (path prefix kept, instruction follows it) | A (function of the path) | A (function of the path) | A (builder taking the path) | A (builder given the path) |
| R1-F6 probe history | P (D8 restates HEAD probes of `tmuxAllowed`, `rusageStats`) | P (A7: #511 inferred from the landed `cmd/testtiming`) | P (D10: quarry probe and #511 inference) | P (assumption: landed `rusageStats` probe) | P (D7: "checked on the pair's tree") |
| R1-F7 criteria | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) |
| R2-F1 partitioned Rename | A (old side present or refused) | A (found-only, out of scope) | A (found-only, out of scope) | A (old side refused, out of scope) | A (old side needs `found`) |
| R2-F2 built-in tags | P (go/build file-matching, base context unstated) | P ("tags the toolchain sets" fixed, base context unstated) | P (go/build's rules, base context unstated) | A (GOOS/GOARCH bound, every other tag free) | A (same, `ignore` free too) |
| R2-F3 free-tag cap | n/a | R ("a fixed cap", no value) | n/a | n/a (no cap; exponential growth an open risk) | n/a |
| R2-F4 recovery message | n/a | n/a | n/a | n/a | n/a |
| R2-F5 cost claim | n/a | n/a | n/a | n/a | n/a |
| R3-F1 | n/a | n/a | n/a | n/a | n/a |
| **R / P** | **1 / 5** | **2 / 4** | **1 / 4** | **1 / 5** | **1 / 4** |

- Before arm, fresh: mean 1.5 / 4.5. After arm: mean 1.0 / 4.5. Half-way ftb-1: 1 / 4.
- R1-F3 is repeated by all five, as by every clean draft but `seat-lead-1` and `twh-2` before; R1-F7's missing Rename-new criterion by all five.
- The one R that separates the arms is ftb-3's free-tag cap with no value (R2-F3), a before-arm draft defect no lens caught.
- Outside the key, both after-arm records hold every finding the forks raised to the same bar: fta-1's forks caught the unlicensed foreground-gate rule for the orch (four lenses merged) and an unread-transcript audit failure (BLOCKING), fta-2's the session-list growth and resume cases.

## Verdict

1. As good a result: yes, within what five runs can show.
   The after arm scores 1 / 5 and 1 / 4 against 1 / 5 and 2 / 4 for the fresh before arm, raises as many lens findings (37 and 37 against 41 and 36), and its forks still opened code their lens needed (10 and 16 calls).
   The "less anchored" benefit claimed for early spawning does not show: the forks inherit the writer's exploration in both arms, and no key finding was caught by a before-arm fork that the after arm missed.
2. Lower cost: yes, in the forks; the writer is unchanged within noise.
   - Fork cost falls from $1.86 to $1.29 per run (31%), $0.53 a run, about $0.09 a fork.
   - Fork cache writes fall from 13.8k to 5.7k tokens per fork (59%), fork cache reads from 4.7M to 3.6M per run, fork messages from 31 to 25, fork tool calls from 19 to 13.
   - Writers cost $3.15 against $3.57 fresh ($3.35 with the earlier runs); the writer does the same work in both arms, and its spread across runs ($2.63-3.73) is larger than the gap.
   - Total per run: $4.44 against $5.43 fresh (18%) or $5.16 with the earlier controls (14%).
   - Writer time: 8m34s-9m00s against 10m13s-11m36s fresh; the fresh before runs ran three in parallel with fta-2, so only part of that gap is the arm.
     After-arm forks run 1m8s-1m21s instead of waiting about 3 minutes on the marker.
- The half-way order ftb-1 and twc-1 took by mistake (write, spawn, then self-check and marker) already gets half the fork cache-write saving (47.7k and 38.2k), but its forks still re-read the files.
- Recommendation: move both spawn points (`twill.go:91`, `planTwillSpawnPoint`) after the marker, and change the fork boilerplate's read line so a fork reviews the files it inherited.
  Three of ten writers already broke the spawn-before order, so the change also removes a step writers do not follow reliably.

## Deviations

- Parent review: the orch approved every bench pair's parent-review request at once, unreviewed (`lyx loom review approve`), so all arms were treated alike; that wait falls inside the run's wall time and is excluded from writer time, which ends at the delivery prompt.
- `parent_review_wait_min: 2` with `alignment_timeout_min: 1` and `skip_alignment: true` replaced round 4's `parent_review_wait_min: 1`, which today's binary refuses beside the 20-minute alignment timeout.
- The after-arm wrapper did not set `claude_deny_agent_tool: false`: lyx already gives a twill writer fork subagents (`ForkSubagents`, the Agent hook admitting forks only), and leaving the deny as is kept both arms' settings equal; the wrapper changes only `prompt.md`.
- The after arm changes the fork boilerplate's read line as well as the spawn point, so forks review the files they inherited; without it a fork would re-read them and the arm would not test inheritance.
- tokencount under-counts fork output: it keeps the first transcript line of each message id, and in a subagent transcript that first line carries a partial `output_tokens` (6 on a thinking block against 4,546 on the final line).
  Its fork figures are therefore low (ftb-2 forks $1.49 against $2.09; twc-1 $1.15 against $1.62, so round 4's fork costs in `twill-discussion-bench.md` are low too); writer figures match exactly.
  All costs here come from `usage.sh`, with tokencount's reports kept beside each run.
- Notes carry the label `bug` only (round 4 had `bug`, `webster`); the note body is byte-identical.
- One `lyx config loom --resolved` read ran in a bench pair to check the layer stack, against the no-`lyx config` without `--set` rule; it wrote nothing.
- ftb-1 broke the before arm's order (see "Spawn point actually taken"), so the before arm has two clean fresh runs, plus twc-2 and tw-om-2 from 2026-10-11 for cost; drift since then is small (fork cache writes 80-86k then and now, writer $2.63-3.73 throughout).
- Torn down: all five pairs with `lyx fabric remove --force --remote` (twb-fta-2's local code branch needed a manual `git branch -D`, as in rounds 4 and 5), all five `twb-*` notes, no reed strand left; `lyx stencil list` shows every stencil untouched, and no committed file, stencil or Bolt entry was edited.
