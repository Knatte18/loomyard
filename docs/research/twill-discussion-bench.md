# twill bench, round 1 (one run per arm)

2026-10-11, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`.
One run per arm, as staged by the coordinator; rounds 2 and 3 await a decision.

## Method

- Input: `webster-stability`'s board body (Bolt `4a0db933`), copied into one temporary note per pair (`twb-tw-om-1`, `twb-so-om-1`, `twb-so-oh-1`); copy in `twill-discussion-bench/input-webster-stability.md`.
- Pairs: `lyx fabric add` from the prime, code branch at `50cf6a425` (the two solo pairs forked from `32a657839` after main moved and were reset to `50cf6a425`).
- Pair `loom.yaml`: `discussion` and `discussion_producer` per arm, `parent_review_wait_min: 1` so the parent-review gate passes with a Warn without an answer.
- Run: `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, one `lyx loom step` (Preflight), `lyx loom pause --after Discussion-Write`, `lyx loom step --until-stop`; one pair at a time.
- Cost: `go run ./tools/tokencount -hub <hub> <slug>` (price table `tools/tokencount/cost.go`, once per message id, sub-agents included, 5 min / 1 h cache writes split); orch cost excluded.
- Wall: writer transcript's first to last assistant message.
- Grading: against the 13 reference Discussion-Review findings (`reference/round-{1,2,3}-review.md`) with the first bench's key (R repeated, P partly, A avoided, n/a); reference final record in `reference/final-decision-record.md`.
- Pairs and notes torn down after copying the records, support logs, transcripts and token reports into `tw-om-1/`, `so-om-1/`, `so-oh-1/`.

## Results

| Arm | Run | Writer cost | Fork cost | Total | Wall | Messages | R | P |
|---|---|---|---|---|---|---|---|---|
| twill `opus[medium]` | `twb-tw-om-1` | $3.18 | $1.45 (6 forks, 33 msgs) | $4.63 | 12m15s | 64 | 0* | 1* |
| twill `opus[medium]`, clean rerun | `twb-tw-om-2` | $2.63 | $1.27 (6 forks, 32 msgs) | $3.90 | 8m01s | 52 | 1 | 3 |
| single `opus[medium]` | `twb-so-om-1` | $1.99 | | $1.99 | 5m13s | 44 | 2 | 4 |
| single `opus[high]` | `twb-so-oh-1` | $2.45 | | $2.45 | 6m19s | 55 | 4 | 3 |

\* Contaminated, see Deviations: the twill writer read `docs/research/discussion-bench.md`, which lists the reference findings.

Total bench spend $12.97 with the rerun, plus orch overhead tokencount attributes to the twill run ($2.66, mostly the base exchange and parent-review delivery, not bench cost).

Tokens (output / cache write / cache read / input):

| Run | Seat | Output | Cache write | Cache read | Input |
|---|---|---|---|---|---|
| tw-om-1 | writer | 47,140 | 171,239 | 6,906,243 | 140 |
| tw-om-1 | 6 forks | 5,936 | 89,173 | 4,443,952 | 78 |
| tw-om-2 | writer | 42,035 | 155,524 | 5,032,580 | 116 |
| tw-om-2 | 6 forks | 2,366 | 84,667 | 3,976,307 | 76 |
| so-om-1 | writer | 28,387 | 130,291 | 3,861,443 | 88 |
| so-oh-1 | writer | 35,028 | 144,418 | 5,129,818 | 110 |

## Grading

| Reference finding | tw-om-1 | so-om-1 | so-oh-1 |
|---|---|---|---|
| R1-F1 deadline outcome stated as timeout | A (`OutcomeDone` via `classifyDeadlineExpiry`) | P (bounded by Timeout, outcome unstated) | P ("a run that times out over it") |
| R1-F2 `unix`/GOOS implications | A (`MatchFile` over pinned GOOS/GOARCH list) | P (one-hot GOOS, `unix` implied; android/ios implications missing) | P (same as so-om) |
| R1-F3 recovery writes invisible to contract evidence | A (recovery treated like fork writes) | R (third group; evidence untouched) | R (criterion: evidence "ignores recovery sessions' writes") |
| R1-F4 reset own-path widening unbounded | A (bound + criterion) | P (open risk R2, no criterion) | R (no bound, no reset criterion) |
| R1-F5 re-prompt cannot carry findings path | A (renderer of the path) | A (function of the path) | A (clause after shuttle's path prefix) |
| R1-F6 Scope carries probe history | A | A | A |
| R1-F7 Rename-new done-check / plan-gate criteria | P (Rename new side covered, plan-gate reads not) | P | P |
| R2-F1 Rename of partitioned member undecided | A (old side found-only, Delete + one Create way forward) | A (Rename-old keeps blocking) | R (normalized to multipart in Rename rules, derivation undecided) |
| R2-F2 base `build.Context` fixes built-in tags | A (Compiler neutral, Release/ToolTags emptied) | A (`go/build/constraint`, free tags) | A (same) |
| R2-F3 free-tag cap has no value | A (four) | R ("needs a cap") | R ("a fixed cap") |
| R2-F4 recovery finding says "a fork wrote" | A (message names recovery session) | n/a | n/a |
| R2-F5 `MatchFile` cost claim | A (each file read once, served from memory) | A | A |
| R3-F1 per-file Create way forward | A (one Create) | n/a | n/a |
| **Repeated / partly** | **0 / 1** | **2 / 4** | **4 / 3** |

### Clean twill rerun (`twb-tw-om-2`)

Setup as round 1, plus: the base-800a0e892 instruction in the note from the start (`twill-discussion-bench/input-solo-arms.md`), and `docs/research/discussion-bench.md` deleted in the pair by one commit (`f66644766`) before the run.
No tool input of the writer or any fork read the bench doc. One `git grep` excluded `docs/benchmarks/`, and the other "bench" hits are the record's own text.
The doc is still reachable through git history and the board's bench notes, but nothing read it there.

| Reference finding | tw-om-2 |
|---|---|
| R1-F1 deadline outcome | P (bounded by the deadline; outcome unstated, though the Constraints keep `allOutputFilesExist` first) |
| R1-F2 `unix`/GOOS | P (one-hot GOOS, `unix` exact; android/ios implications missing; `linux` beside `unix` overlaps) |
| R1-F3 contract evidence | R (recovery writes feed only the recovery prompt; contract evidence never sees them) |
| R1-F4 reset widening | n/a (goal-scope fork had the reset widening dropped; reset's own-path set unchanged) |
| R1-F5 re-prompt path | A (builder taking the findings path) |
| R1-F6 Scope probe history | A |
| R1-F7 criteria | P (no Rename-new done-check criterion; Edit at Plan time covered) |
| R2-F1 partitioned Rename | A (old side keeps blocking, new side accepted) |
| R2-F2 built-in tags | A (`go/build/constraint` assignments) |
| R2-F3 free-tag cap | n/a (no enumeration cap named at all; same approach the first bench graded n/a for solo) |
| R2-F4 recovery message | n/a |
| R2-F5 `MatchFile` cost | A |
| R3-F1 | n/a |
| **Repeated / partly** | **1 / 3** |

Lens review: six forks, all returned, about 29 findings after merging duplicates. 28 were fixed and 1 rejected (pattern-fit, perishable GOOS list).
The ones that changed substance:
- goal-scope removed the unrequested reset widening (R1-F4's defect).
- premises caught the `rusageStats` premise: at base it sits in unconstrained `cmd/testtiming/resources.go`, and both solo drafts wrongly say it is split into `rusage_linux.go`/`rusage_other.go`.
- pattern-fit replaced a second, quote-unaware shell splitter with a shared one in `internal/shell`.
- scenarios added the missing-transcript, handle-binding, `+build` and attach cases.

Forks ended between 11:52:51 and 11:53:35; the writer's last message was at 11:55:26. All forks returned before the writer finished.

Verdict: twill repeats 1 of 13 and partly 3. Solo `opus[high]` repeats 4 and partly 3, and solo `opus[medium]` repeats 2 and partly 4. Twill costs $3.90 against $2.45 (1.6x) and $1.99 (2.0x), and takes 8m01s against 6m19s and 5m13s.
One run per arm: this is a direction, not a measured effect.

Departures judged on the merits:

- Gate-shell hold (item 1): twill holds every gated producer's turn end on any outstanding task (the reference's decision); both solo drafts hold only Master on a `lyx gate test` segment match (split on `&&`, `||`, `;`, `|`), the narrower route the first bench judged weaker.
- Item 5: so-om says tmux-tagged steps run through `lyx gate test --tags tmux`; twill and so-oh compile them only (`go vet -tags`), matching the board's "left to verify".
- twill alone adds `D8`, naming the reset route for #511's dead-batch-after-goto point (the solo drafts put it Out of scope).

## Twill run mechanics (PR #541 "To check by hand")

- Forks: six, one per lens of `twill-discussion`, all `isFork: true`, `model: inherit`, spawned 11:15:48-11:16:20 in one API message (all six tool calls share message id `msg_011CfvMVYP52iwsVeJYkYCB2`; the 5-second spacing is streaming, not separate turns), as the directive asks. Round 1's report said six messages; that was wrong. The rerun did the same: one message, 11:50:41-11:51:06.
- Ready marker written 11:18:07, after the validator; every fork returned before the writer finished: forks ended 11:18:48-11:19:44, writer's last message 11:22:52, Discussion-Write done about 13:23:4x local. No fork finished after the writer was marked done.
- Forks made 2-6 Bash calls each (within the 6-call bound); fork wait under the 250 s cap, so no cold wake.
- Forks read the writer's prefix from cache: 4.44 M cache read against 89 k cache write across six forks, $0.24 per fork on average.
- Findings: about 31 lens findings, 30 fixed and 1 rejected (board-fit, `weaver` informational), recorded in the support log's `## Interview` item 8.
  Fixes included the `gc`/`Compiler` premise (reference R2-F2), the missing-transcript case, the `D8` dead-batch route, and leanness cuts.
- Writer phase after the forks: 11:19:56-11:22:52, six exploratory calls, a full record rewrite and three support-log edits.

## Deviations

- Base: the pairs sit at `50cf6a425`, where `1e79825ff` already landed the task. The twill writer noticed (11:11-11:12), messaged `ly:orch`, and got the instruction to face base `800a0e892` via `git show` mid-run; the solo notes carried the same text up front (`twill-discussion-bench/input-solo-arms.md`). The twill writer spent about a minute on that exchange.
- Contamination: while investigating the base mismatch, the twill writer ran `cat docs/research/discussion-bench.md` (11:11:23), which holds the first bench's grading table of all 13 reference findings and the departures. Its 0 / 1 grade is therefore not comparable. The solo writers never read the file (0 tool inputs mention it).
- Binary: the twill run used prod `lyx` at `50cf6a425`; the solo runs used prod at `32a657839`. The drift (reed tmux conf, darn Describe, loom card verbs, plan stencils) touches no Discussion-Write path.
- One run per arm: spread unmeasured.

## Recommendation for rounds 2 and 3

- Repeat the twill arm with the contamination removed: in each pair, commit a removal of `docs/research/discussion-bench.md` (or reset to a base without it and with twill), and put the base-`800a0e892` instruction in every note body up front, as for the solo arms.
- Keep the solo arms as they are; run 2 more each for spread.
- Budget: twill about $4.6, solos about $2.0 and $2.5; rounds 2-3 (6 runs) about $18, total about $27, inside the $30 cap.

# Sonnet round (four arms in parallel)

Setup per pair as for `tw-om-2`:
- a `twb-*` note with the base instruction (`twill-discussion-bench/input-solo-arms.md`);
- the pair at `50cf6a425`, with `docs/research/discussion-bench.md` deleted by one commit;
- `parent_review_wait_min: 1` and a pause after Discussion-Write;
- all six `twill-discussion` lenses.

The four ran in parallel from 12:08 UTC; prod `lyx` was at `32a657839`.
No tool input of any writer or fork names the bench doc.
Cost is from `tools/tokencount`; `claude-sonnet-5-5` is priced at 2 / 2.5 / 4 / 0.20 / 10 USD per Mtok.
Records, support logs, transcripts and token reports are in `tw-sm-1/`, `tw-sh-1/`, `so-sm-1/`, `so-sh-1/`.
Pairs, notes and local branches are torn down.

## All arms

| Arm | Run | Writer | Forks | Total | Wall | R | P |
|---|---|---|---|---|---|---|---|
| twill `opus[medium]` (clean) | tw-om-2 | $2.63 | $1.27 | $3.90 | 8m01s | 1 | 3 |
| single `opus[medium]` | so-om-1 | $1.99 | | $1.99 | 5m13s | 2 | 4 |
| single `opus[high]` | so-oh-1 | $2.45 | | $2.45 | 6m19s | 4 | 3 |
| twill `sonnet[medium]` | tw-sm-1 | $1.22 | $0.68 | $1.90 | 4m16s | 3 | 3 |
| twill `sonnet[high]` | tw-sh-1 | $2.73 | $0.66 | $3.39 | 8m46s | 2 | 2 |
| single `sonnet[medium]` | so-sm-1 | $0.55 | | $0.55 | 1m54s | 3 | 2 |
| single `sonnet[high]` | so-sh-1 | $1.96 | | $1.96 | 4m50s | 1 | 1 |

Sonnet round spend: $7.80.
The contaminated `tw-om-1` ($4.63, 0 / 1) is left out of the comparison.
The solo Opus arms ran without the bench-doc deletion, but neither writer read the doc.

## Grading, Sonnet arms

| Reference finding | tw-sm-1 | tw-sh-1 | so-sm-1 | so-sh-1 |
|---|---|---|---|---|
| R1-F1 deadline outcome | P (timeout "does not read as passed", outcome unstated) | R ("ends the run `OutcomeTimeout`, never `done`") | A (output files stay the return value at a deadline) | A (`classifyDeadlineExpiry` finalizes Done) |
| R1-F2 `unix`/GOOS | P (`unix` derived; android/ios missing) | A (android, ios, illumos, unix implied) | R (tags free; `unix` not derived) | P (`unix` derived; android/ios missing) |
| R1-F3 contract evidence | R (recovery sessions audited "beside the Master sessions") | R (evidence "keeps reading Master's and the forks' fields only") | R (recovery sessions join `loadRunWrites`' audited Master sessions) | R (`recordedSessions` includes them; recovery writes join Master's) |
| R1-F4 reset widening | R (reset widens through `loadRunWrites`, unstated) | A (bound + criterion, after premises fork) | R (unstated) | A (stated, criterion, open risk) |
| R1-F5 re-prompt path | R (Webster text "through the gate's carried text", path undecided) | A (builder taking the path) | A (text built in `gate.go` with the path; Webster text inside shuttle) | A (note after the fixed path head) |
| R1-F6 Scope history | A | A | A | A |
| R1-F7 criteria | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) | A (Rename new side and plan-time Edit) |
| R2-F1 partitioned Rename | A (old side blocks) | P (Rename undecided) | P (old side unstated) | A (old side blocks) |
| R2-F2 built-in tags | A | A | A | A |
| R2-F3 free-tag cap | n/a | A (eight) | n/a | n/a |
| R2-F4 recovery message | n/a | n/a | n/a | n/a |
| R2-F5 cost claim | A | A | A | A |
| R3-F1 | n/a | n/a | n/a | n/a |
| **R / P** | **3 / 3** | **2 / 2** | **3 / 2** | **1 / 1** |

Merit notes outside the 13:
- tw-sm-1's D4 targets a `failed` digest, which already exits non-zero through `batch_failed`, so the stuck/dead exit-0 case stays.
- so-sm-1 holds every outstanding shell rather than gate shells, its D4 omits `stuck`, and its D6 adds a declaration-lookup fallback.
- so-sh-1 holds only a plain `lyx gate test` label and adds a Master stencil rule to start gates that way.
- tw-sh-1 narrows its Goal honestly to wrapped commands still ending `done`.

## Twill lens counts, all clean twill arms

| Lens | tw-om-2 | tw-sm-1 | tw-sh-1 |
|---|---|---|---|
| premises | 6 | 5 | 6 |
| goal-scope | 6 | 9 | 8 |
| board-fit | 4 | 5 | 8 |
| pattern-fit | 6 | 7 | 7 |
| scenarios | 10 | 11 | 7 |
| leanness | 6 | 6 | 10 |
| Raw total | 38 | 43 | 46 |
| Issues raised by 2+ lenses | 4 | 4 | 6 |
| Fixes (distinct) | 28 | 14 | 26 |
| Real / cosmetic | 18 / 10 | 8 / 6 | 14 / 12 |
| Rejected | 1 | 5 | 3 |

Counts are the severity-tagged items in each fork's final message; the tw-sh-1 leanness count includes one finding it marked outside its lens.
Overlap and disposition come from each support log's lens-review list.
- tw-sm-1's cross-lens issues: recovery session ids (premises, scenarios, goal-scope), the label-prefix mechanism (premises, goal-scope, scenarios), `unix` and three candidates (goal-scope, scenarios), and D2's root cause (premises, goal-scope).
- tw-sh-1's cross-lens issues: the reset bound (premises, leanness), predicate vacuity (premises, scenarios), the Master ladder wording (premises, goal-scope, pattern-fit, scenarios), the tag cap (goal-scope, leanness), `PATTERN-gate-slot` (premises, pattern-fit), and Master running gates (goal-scope, board-fit).
- Real fixes in tw-sh-1 include the reset bound (R1-F4) and the tag cap (R2-F3).
  Its premises fork confirmed the deadline path yet left the `OutcomeTimeout` claim (R1-F1) standing.

Both twill Sonnet arms spawned their six forks in one API message, and every fork returned before its writer finished: tw-sm-1's forks ended 12:11:18, writer 12:12:16; tw-sh-1's forks ended 12:15:16, writer 12:16:46.

## Verdict, Sonnet round

- Sonnet twill does not clearly hold Opus twill's quality.
  `sonnet[high]` twill (2 / 2) is close to the clean Opus twill (1 / 3) at 0.87x the cost ($3.39 against $3.90), but repeats the BLOCKING deadline-outcome finding.
  `sonnet[medium]` twill (3 / 3) is cheaper at 0.49x and worse.
- The best draft of the whole bench is solo `sonnet[high]` (1 / 1, $1.96, 0.50x Opus twill).
  With one run per arm, the spread between runs is unknown and likely as large as these gaps; R1-F3 (recovery writes invisible to, or misfiled in, contract-file evidence) was repeated by all seven clean drafts, whatever the model or producer.

# Job A: why the Opus arms missed what they missed

Read only from the saved transcripts.
"Reading error": the code said otherwise and neither the writer nor any of its forks read that code; the check is that the name appears in no tool input or tool result.
"Judgment error": a completeness, scope or preference call made differently from the reference.

| Reference finding | tw-om-2 | so-oh-1 | Code not read |
|---|---|---|---|
| R1-F1 deadline outcome | P, reading | P, reading | `classifyDeadlineExpiry` (`shuttleengine/wait.go`) |
| R1-F2 `unix`/GOOS implications | P, reading | P, reading | go/build's `matchTag` GOOS implications (android, ios, illumos) |
| R1-F3 contract evidence | R, reading | R, reading (false premise "reads Master's writes") | `contractFileStatus` (`websterengine/contractevidence.go`) |
| R1-F4 reset widening | n/a | R, reading | `ownTrackedPaths` (`websterengine/reset.go`), the dirt-check exemption |
| R1-F7 criteria gaps | P, judgment | P, judgment | |
| R2-F1 partitioned Rename | A | R, reading | `renameDeclSource`'s Found-only derivation (`planglyph/handle.go`) |
| R2-F3 free-tag cap | n/a | R, judgment | |
| **Reading / judgment** | **3 / 1** | **5 / 2** | |

- Most misses are code the arm never opened, not design calls made differently.
  None of the six tw-om-2 forks opened any of these files either: forks inherit the writer's reading, so they share its blind spots.
- A reviewer from a different model helps only if it does its own reading of the code the writer skipped.
  A fresh reader of the same model with the same reading would add less than a fresh reader of any model that covers the unread files.
  The seats arms below test this: the leading arm's advisor read `contractevidence.go` and `classifyDeadlineExpiry`, and that record avoids R1-F3.

# Job B: seats, leading questions against blind questions

Two arms: `discussion_producer: seats`, chair `opus[high]`, one advisor `sonnet[high]`.
Setup was the same as `tw-om-2`: a `twb-*` note with the base instruction, the pair at `50cf6a425`, the bench doc deleted by one commit, `parent_review_wait_min: 1` and a pause after Discussion-Write.

- **Leading (`twb-seat-lead-1`):** today's stencils.
- **Blind (`twb-seat-blind-1`):** the board copies of `loom-template-discussion-chair.md` and `loom-template-discussion-advisor.md` were edited before spawn:
  - the chair writes its private pick and alternatives to the ledger before each batch;
  - it sends only open, non-leading questions;
  - it records agree, differ or new option per question;
  - the advisor gets no options, researches itself, and never answers by number.
  The originals are in `seat-stencils-orig/`.
- **Ordering:** the leading arm started at 14:21:51 and the blind arm at 14:32:29, in parallel.
  Both leading seat prompts had been rendered before the edit; the blind prompts carry the edited text.
  No other pair uses seats.
- **Stencil incident:**
  - A board sync triggered by the blind note's upsert committed and pushed the edited stencils to Bolt (`f0b95c70`, 14:31:58).
  - `lyx stencil sync` does not revert board edits, so I reversed both edits by hand once the blind prompts had rendered.
  - The files are identical to the seeded copies, and the restore reached Bolt in `57067049`.
  - `lyx stencil list` shows every stencil untouched.

## Results, all arms

| Arm | Chair / writer | Advisor / forks | Total | Wall | R | P |
|---|---|---|---|---|---|---|
| twill `opus[medium]` (clean) | $2.63 | $1.27 | $3.90 | 8m01s | 1 | 3 |
| single `opus[medium]` | $1.99 | | $1.99 | 5m13s | 2 | 4 |
| single `opus[high]` | $2.45 | | $2.45 | 6m19s | 4 | 3 |
| twill `sonnet[medium]` | $1.22 | $0.68 | $1.90 | 4m16s | 3 | 3 |
| twill `sonnet[high]` | $2.73 | $0.66 | $3.39 | 8m46s | 2 | 2 |
| single `sonnet[medium]` | $0.55 | | $0.55 | 1m54s | 3 | 2 |
| single `sonnet[high]` | $1.96 | | $1.96 | 4m50s | 1 | 1 |
| seats-leading `opus[high]` + `sonnet[high]` | $2.75 | $2.67 | $5.42 | 9m26s | 4 | 2 |
| seats-blind `opus[high]` + `sonnet[high]` | $2.50 | $2.70 | $5.20 | 10m30s | 2 | 3 |

Job B spend: $10.62 (cap $15).
Wall is the chair's first to last message; the advisor's span lies inside it.

## Exchanges and how the advisor moved the chair

| | seats-leading | seats-blind |
|---|---|---|
| Question batches (chair to advisor) | 2 (7 questions) | 2 (6 questions) |
| Advisor messages | 3 (notes summary, 2 answer batches) | 2 (answer batches) |
| Advisor answer differed from the chair's pick | 0 of 7 ("agree on all"); 3 agreed with amendments | 2 of 6 (Q1 in part, Q4 outright); 4 agreed, each adding detail or options |
| Premise corrections from the advisor | 0 | 3: the `background_shell_wait_min` premise, Edit failing at Plan time not record-batch, `runChild` absent at base |
| Answer length | 3.0k-5.3k characters | 11.6k-12.6k characters |

- In the leading arm the amendments carried the weight.
  - **Q3:** a third `Recoveries` slice treated like `Forks` in contract evidence. This is the R1-F3 fix; it came from the advisor reading `contractevidence.go`.
  - **Q6:** normalize in `resolveTargets`.
  - **Q1:** the segment splitter with `VAR=val` skipping.
- In the blind arm the advisor changed two outcomes.
  - **Q4:** the chair's private pick, run tmux steps like any other, contradicted `PATTERN-test-speed`; the advisor's version was adopted.
  - **Q1:** the advisor rejected the chair's planned demotion to stuck and added the discarded-result wording on every outcome.

Did the advisors read the code behind Job A's reading errors? (tool inputs and results of the advisor transcripts):

| Code | Lead advisor | Blind advisor |
|---|---|---|
| `classifyDeadlineExpiry` | read | not read |
| `contractevidence.go` / `contractWritesFor` | read, and flagged the masquerade as Master's writes | read, named it as a reader, did not flag the masquerade |
| `ownTrackedPaths` | read | read |
| go/build GOOS implications | not read | not read; reasoned "`unix` derives from GOOS" and a pinned list |

## Grading, seats arms

| Reference finding | seats-leading | seats-blind |
|---|---|---|
| R1-F1 deadline outcome | A (after a timeout or died exit "the outcome alone can still read `done`") | P (bounded by timeout and liveness, outcome unstated) |
| R1-F2 `unix`/GOOS | R (`MatchFile` over the mentioned GOOS plus a sentinel "other", the reference's own defect: `!windows` beside `unix` reads disjoint) | P (closed pinned list, `unix` derived, android/ios missing) |
| R1-F3 contract evidence | A (`Recoveries` treated like `Forks`, criterion) | R ("a recovery's contract-file write is attributed" through `recordedSessions`) |
| R1-F4 reset widening | A (criterion, reason) | A (bound, open risk, criterion) |
| R1-F5 re-prompt path | A | A |
| R1-F6 Scope history | A | A |
| R1-F7 criteria | P (no Rename-new) | P (no Rename-new) |
| R2-F1 partitioned Rename | R (normalized to multipart, `renameDeclSource` Found-only left undecided) | A (refused with a truthful message naming the files) |
| R2-F2 built-in tags | R (`Context.MatchFile` with no stated base context) | A (`go/build/constraint`, `go1.N` reads not proven) |
| R2-F3 free-tag cap | n/a (no cap) | R ("a fixed cap", no value) |
| R2-F4 recovery message says "a fork wrote" | R (fork-side routing, message unchanged) | n/a |
| R2-F5 cost claim | P ("reads only each candidate file's header", while `MatchFile` reopens per configuration) | A |
| R3-F1 | n/a | n/a |
| **R / P** | **4 / 2** | **2 / 3** |

The leading arm avoided the round-1 defects but took the reference's own `MatchFile`/fork-side route, so it repeats the reference's round-2 findings (R2-F2, R2-F4) and its R1-F2 environment defect.

## Verdict, Job B

- The blind arm drafted better than the leading arm (2 / 3 against 4 / 2) at the same cost ($5.20 against $5.42) and one minute more.
  Its advisor disagreed with the chair on 2 of 6 questions and corrected 3 premises; the leading advisor disagreed with none.
- Both seats arms cost about 1.3-1.4x Opus twill and 2.7x solo `sonnet[high]`, and neither drafted better than those.
  The advisor's value tracks what it reads: the leading advisor fixed R1-F3 by reading `contractevidence.go`, which no other arm managed.
  One run per arm, so this is a direction only.

# Round 4: combo, sharpened premises, twill control (two runs each)

## Setup

Every pair is set up like `tw-om-2`:
- the code branch is reset to `f66644766`, which is `50cf6a425` plus the one commit deleting the bench doc;
- a `twb-*` note carries `twill-discussion-bench/input-solo-arms.md` as its body, created before any edit;
- pair `loom.yaml` sets `discussion: opus[medium]`, `parent_review_wait_min: 1` and a pause after Discussion-Write;
- the run is `lyx fabric commit`, then `lyx shed seed self --recipe loom --driver go --param parent=main`, then `lyx loom step`, then `lyx loom step --until-stop`.
Prod `lyx` was at `a35c8c2b1`.
All six ran in parallel, starting at 13:03-13:04 UTC.

- **combo** (`twb-combo-{1,2}`): `discussion_producer: seats` with advisor `sonnet[high]`.
  - The board copies of the chair and advisor stencils were edited before spawn; the edited copies are in `combo-stencils-edited/` and the originals in `combo-stencils-orig/`.
  - The chair got the blind text from `seat-blind-1`, word for word, plus a `## Lens review by forks` section: the rendered twill directive, the six `twill-discussion` lenses, and the write-scope paragraph, with paths filled through `{{.slug}}`, `{{.decision_record_path}}` and `{{.support_log_path}}`.
  - The advisor got the blind Step 3 lines.
    Its Step 1 altitude line was replaced: "Read the code behind the task in depth: open the functions each part of the task rests on, their signatures, their callers and the build constraints of the files they sit in, and cite the file and function you read. When a question arrives, open the code behind that question the same way before you answer it, even where your notes already cover its module."
  - lyx enables forks only for twill writers (`ForkSubagents`), so two pair-local `shuttle.yaml` changes let the chair fork.
    `claude:` points at a scratchpad wrapper that exports `CLAUDE_CODE_FORK_SUBAGENT=1`, and `claude_deny_agent_tool: false` is set.
    Both seats of the combo pairs therefore had an unrestricted Agent tool; the advisors spawned no subagents.
- **twill-premises** (`twb-twp-{1,2}`): `discussion_producer: twill`; a pair-local `burler.yaml` `premises` lens overrides the template's by name (the hub copy was not touched).
  The lens text: "Focus on the record's premises. List every factual claim the record and the support log make about today's code, config or behaviour: what a function returns or decides, which inputs or sessions a reader covers, what a rule or constraint implies, what a set of paths includes. Open the code behind every such claim, and one hop past what the record cites, such as the caller, the config key or the spawned process a cited piece leans on. Only a verbatim read of that code already in your context counts as opened; a summary or the record's own wording does not. Batch several reads into one tool call to stay inside your call bound. Confirm or refute each claim, citing the file and function you opened, and report a claim you could not open as unverified. Report anything else you notice too — emphasis, never exclusion."
- **twill control** (`twb-twc-{1,2}`): `discussion_producer: twill`, unchanged.

## Results

Cost from `tools/tokencount`, orch excluded (it attributes $4.0-5.1 of orch per run here, mostly shared hub traffic).
Wall is the writer's or chair's first to last assistant message.

| Arm | Run | Writer / chair | Advisor | Forks | Total | Wall | R | P |
|---|---|---|---|---|---|---|---|---|
| combo | combo-1 | $3.62 | $2.12 | $1.73 | $7.47 | 12m57s | 3 | 2 |
| combo | combo-2 | $3.46 | $2.50 | $1.61 | $7.57 | 14m10s | 2 | 2 |
| twill-premises | twp-1 | $3.58 | | $1.03 | $4.61 | 9m27s | 2 | 2 |
| twill-premises | twp-2 | $3.05 | | $1.36 | $4.41 | 8m33s | 1 | 4 |
| twill control | twc-1 | $3.40 | | $1.15 | $4.55 | 9m39s | 2 | 2 |
| twill control | twc-2 | $3.61 | | $1.33 | $4.94 | 9m32s | 1 | 2 |

| Arm | Runs | Mean cost | Mean wall | Mean R / P |
|---|---|---|---|---|
| combo `opus[medium]` + `sonnet[high]` + forks | 2 | $7.52 | 13m34s | 2.5 / 2.0 |
| twill-premises `opus[medium]` | 2 | $4.51 | 9m00s | 1.5 / 3.0 |
| twill control `opus[medium]` | 2 (3 with tw-om-2) | $4.75 ($4.46) | 9m36s | 1.5 / 2.0 (1.3 / 2.3) |
| earlier: seats-blind `opus[high]` + `sonnet[high]` | 1 | $5.20 | 10m30s | 2 / 3 |
| earlier: seats-leading | 1 | $5.42 | 9m26s | 4 / 2 |
| earlier: single `sonnet[high]` | 1 | $1.96 | 4m50s | 1 / 1 |
| earlier: single `opus[high]` | 1 | $2.45 | 6m19s | 4 / 3 |

Round spend: $33.55 (cap $35).
The six runs shared the machine, so walls run about 1.5 minutes above `tw-om-2`'s 8m01s.

## Grading

| Reference finding | combo-1 | combo-2 | twp-1 | twp-2 | twc-1 | twc-2 |
|---|---|---|---|---|---|---|
| R1-F1 deadline outcome | R ("a timeout, never `done`") | A (gate evaluated at the deadline, `finishMasterDone` demotes) | R ("ends the run as a Master timeout") | P (ends on the deadline, outcome unstated) | A (`Done` via `classifyDeadlineExpiry`, webster demotes to stuck) | A (`classifyDeadlineExpiry` answers `done`) |
| R1-F2 `unix`/GOOS | A (closed table, `unix`, android/ios/illumos implied) | A (`MatchFile` over mentioned GOOS plus a Unix and a non-Unix other) | P (`unix` implied; android/ios missing) | P (same) | A (implications listed) | A (implications listed; `_unix.go` no suffix) |
| R1-F3 contract evidence | R (keeps Master and Forks only) | R (same) | R (same; criterion "not counted as Master's") | R (same) | R (same) | R ("a recovery's writes never clear a finding", the reference's own false premise) |
| R1-F4 reset widening | n/a (reset unchanged) | P (open risk only, no criterion) | A (stated, criterion) | P (stated in the bound, no criterion) | A (bound, criterion) | P (stated, no criterion) |
| R1-F5 re-prompt path | R (fixed one-line text naming the path) | A (rendered from the path) | A (builder taking the path) | A (function of the path) | R (fixed text with `<path>`) | A (function of the path) |
| R1-F6 probe history | P (Goal's "Base facts" carry quarry probe results and "#511's premise is wrong") | A | A | A (one-clause reason) | P (D8 rationale carries probes and the #511-is-#500 inference) | A |
| R1-F7 criteria | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) | P (no Rename-new) |
| R2-F1 partitioned Rename | A (Rename-old stays ambiguous at Plan time) | A (out of scope, `rename-old-unresolved`) | A (read as multipart, Found-only) | A (refused) | A (Found-only) | A (Found-only) |
| R2-F2 built-in tags | A (`go/build/constraint`, free tags) | R (`MatchFile` with compiler and `go1.N` "fixed", base context unstated) | A | A | A | A |
| R2-F3 free-tag cap | A (eight) | A (four) | n/a (no cap) | n/a | A (twelve) | n/a |
| R2-F4 recovery message | n/a | n/a | n/a | n/a | n/a | n/a |
| R2-F5 cost claim | A (parsed once, in memory) | n/a | n/a | n/a | n/a | n/a |
| R3-F1 | n/a | n/a | n/a | n/a | n/a | n/a |
| **R / P** | **3 / 2** | **2 / 2** | **2 / 2** | **1 / 4** | **2 / 2** | **1 / 2** |

R1-F3 is now repeated by every clean draft but `seat-lead-1`; R1-F7's missing Rename-new criterion by all six.
On item 1 every draft holds only shells whose label matches `lyx gate test` per command segment; twc-2 adds a hold bound key (`gate_shell_hold_min`), and combo-2 bans Master from running gates.

## Combo: advisor against the chair's private pick

| | combo-1 | combo-2 |
|---|---|---|
| Questions (batches) | 6 (2) | 7 (2) |
| Advisor differed from the private pick | 1 (Q6: done-check only, since `expand` needs `found`; adopted) | 0 outright, 1 in part (Q5 wording) |
| Agreed, adding detail or options | 5 | 6 (Q1 added a `done`-to-`stuck` demotion, adopted and then dropped in the lens review, and the "Master runs no gate" rule) |

- combo-1's chair wrote its private picks into the ledger "at writing time", not before sending: the twill section's "before you write any output file" conflicted with the blind rule's "write the pick first".
- combo-1's advisor read `contractevidence.go` and recommended keeping recovery writes out of it, which is R1-F3's defect.
  combo-1's one disagreement (Q6) narrowed the partitioned reading to the done-checks, against the reference's every-policy reading.

## Reading-error code opened (tool input naming it)

| Code | combo-1 | combo-2 | twp-1 | twp-2 | twc-1 | twc-2 |
|---|---|---|---|---|---|---|
| `classifyDeadlineExpiry` | advisor | chair, advisor, premises, board-fit | (writer saw it in output only) | (writer saw it in output only) | writer, pattern-fit | writer |
| `contractFileStatus` / `contractevidence.go` | chair, advisor | chair, advisor | writer, premises | writer, premises | (writer saw it in output only) | writer |
| `ownTrackedPaths` | chair, advisor, premises | chair, advisor, board-fit | writer | (writer saw it in output only) | writer | writer |
| go/build GOOS rules (`matchTag`, `syslist`) | advisor | premises | none | none | none | premises |

- Unlike `tw-om-2`, where neither the writer nor any fork opened any of these, every run opened at least three of the four.
  The remaining misses are judgment errors, not reading errors: drafts opened the code and drew the wrong consequence.
- R1-F3: both sharpened premises forks opened `contractevidence.go`, confirmed with citations that it reads `writes.Master` and `writes.Forks`, and endorsed leaving recovery writes out.
  twp-2's premises fork even corrected the record's "Master only" to "Master and forks", yet neither saw that an unread recovery write lets the finding clear as Master's.
- R1-F1: combo-1's advisor opened `classifyDeadlineExpiry` and the chair still wrote "a timeout, never `done`".
  twp-1 never opened it and repeated the defect; both controls opened it and avoided it.

## Lens finding counts

Counts are the severity-tagged lines in each fork's final message.

| Lens | combo-1 | combo-2 | twp-1 | twp-2 | twc-1 | twc-2 |
|---|---|---|---|---|---|---|
| premises | 5 | 4 | 5 | 6 | 5 | 5 |
| goal-scope | 7 | 6 | 6 | 5 | 6 | 8 |
| board-fit | 4 | 5 | 6 | 4 | 5 | 5 |
| pattern-fit | 5 | 5 | 5 | 7 | 6 | 5 |
| scenarios | 9 | 9 | 11 | 10 | 11 | 8 |
| leanness | 9 | 10 | 7 | 6 | 7 | 7 |
| Total | 39 | 39 | 40 | 38 | 40 | 38 |

The sharpened premises forks spent their whole call budget on code reads (5-6 calls, each batching several files) and returned a "Confirmed claims" and an "Unverified" list as asked.
The control premises forks also made 5-6 calls, without the lists; twc-2's opened go/build's `matchTag`.

## Verdict, round 4

- The blind deep-reading advisor adds nothing on top of twill: combo averages 2.5 / 2.0 against the control's 1.5 / 2.0, at 1.6x the cost ($7.52 against $4.75) and 4 minutes more; its advisor differed from the chair's pick on 1 of 13 questions.
- The sharpened premises lens does not close the remaining misses (1.5 / 3.0 against 1.5 / 2.0, same cost): its forks opened the code and still confirmed the R1-F3 premise, because the misses that remain are wrong consequences of code that was read, not unread code.
- Two runs per arm: the spread within an arm (1-3 R) is as large as the gaps between arms.

## Deviations, round 4

- Stencil leak: board sync `553aa524` (13:04:03 UTC, triggered by the combo runs' own status write) pushed the edited chair and advisor stencils to Bolt.
  Both board copies were restored by hand once all four combo seat prompts had rendered (13:04:54 UTC).
  The restore reached Bolt in `2be8608b` (13:07:50 UTC); origin's copies and the board copies match `combo-stencils-orig/` byte for byte, and `lyx stencil list` shows every stencil untouched.
- The combo chair's forks needed the fork env and an open Agent tool, which lyx gives only to twill writers; see Setup.
- The combo chair stencil carries the twill directive's own "before you write any output file" next to the blind rule's ledger-first instruction; combo-1 resolved the clash by writing its private picks at writing time.
- Torn down: all six pairs with their local and remote branches (twc-2's local code branch needed a manual `git branch -D`), all six notes and the reed sessions.

# Round 5: twill opus[high]

## Setup

As round 4's twill control (`twb-twc-{1,2}`), with one change: pair `loom.yaml` keeps `discussion: opus[high]` (the hub default) instead of `opus[medium]`.
- Notes `twb-twh-{1,2}` carry `twill-discussion-bench/input-solo-arms.md` as their body, created before any edit; pairs from `lyx fabric add` in the prime at `a35c8c2b1`, code branch reset to `f66644766`.
- Pair `loom.yaml`: `discussion_producer: twill`, `parent_review_wait_min: 1`; then `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, `lyx loom step`, `lyx loom pause --after Discussion-Write`, `lyx loom step --until-stop`.
- Both ran in parallel from 13:35 UTC, with no other bench pair; the writers ran `--model opus --effort high`.
- Parent review went unanswered and passed with a Warn after one minute, as in round 4.
- No tool input of either writer or any fork names the bench doc.

## Results

| Run | Writer | Forks | Total | Wall | R | P |
|---|---|---|---|---|---|---|
| twh-1 | $4.52 | $1.43 | $5.95 | 12m03s | 2 | 4 |
| twh-2 | $4.08 | $1.71 | $5.79 | 11m21s | 1 | 2 |

| Arm | Runs | Mean cost | Mean wall | Mean R / P |
|---|---|---|---|---|
| twill `opus[high]` | 2 | $5.87 | 11m42s | 1.5 / 3.0 |
| twill `opus[medium]`, all five (tw-om-2, twc-1, twc-2, twp-1, twp-2) | 5 | $4.48 | 9m02s | 1.4 / 2.6 |
| twill `opus[medium]` control only (tw-om-2, twc-1, twc-2) | 3 | $4.46 | 9m04s | 1.3 / 2.3 |
| single `opus[high]` | 1 | $2.45 | 6m19s | 4 / 3 |

Round spend $11.74 (cap $12), orch excluded (tokencount attributes $4.77 and $3.79 of orch to the two runs).
The writers ran 65-74 messages against 52-65 for the `opus[medium]` writers; output tokens rose about 20% (62.9k and 62.0k against 52.7k for twc-1), and cache reads to 8.7-10.5 M.
Each writer spawned its six forks in one API message, and every fork returned before its writer finished (twh-1: forks ended 13:45:38, writer 13:47:36; twh-2: 13:44:19, 13:46:53).

## Grading

| Reference finding | twh-1 | twh-2 |
|---|---|---|
| R1-F1 deadline outcome | R (Open risk and criterion: the deadline ends it `OutcomeTimeout`) | A (`classifyDeadlineExpiry` finalizes `done`, verify gate at finalize, shell recorded unread) |
| R1-F2 `unix`/GOOS | P (`unix` derived; android/ios missing) | P (same) |
| R1-F3 contract evidence | R (keeps reading Master and forks, not recovery) | A (recovery writes join the non-Master side, never Master's; criterion 10; fixed after the scenarios fork) |
| R1-F4 reset widening | P (own paths include recovery writes, criterion, discard consequence unstated) | A (reset accepts an earlier recovery's dirty path, criterion 11) |
| R1-F5 re-prompt path | A (function of the path) | A (function of the path) |
| R1-F6 probe history | P (D6 rationale carries "#511's `rusageStats` was a per-GOOS pair", the #511-is-#500 inference, false at base) | A (one-clause reason) |
| R1-F7 criteria | P (no Rename-new) | P (no Rename-new) |
| R2-F1 partitioned Rename | A (found-only, out of scope) | A (`rename-old-unresolved` at Plan time) |
| R2-F2 built-in tags | A (own evaluator, every other tag free) | A (same) |
| R2-F3 free-tag cap | n/a | n/a |
| R2-F4 recovery message | n/a | R (recovery writes on the fork side, refusal still "a fork wrote") |
| R2-F5 cost claim | n/a | n/a |
| R3-F1 | n/a | n/a |
| **R / P** | **2 / 4** | **1 / 2** |

twh-2 is the second clean draft, after `seat-lead-1`, to avoid R1-F3; it then repeats R2-F4, which follows from that fix, as `seat-lead-1` did.
twh-2's scenarios fork asked for an `OutcomeTimeout` criterion at the deadline (R1-F1's defect); the writer, having opened `classifyDeadlineExpiry`, kept `done`.
twh-1 holds every Master background shell (a spawn-level boolean), not only `lyx gate test` labels; twh-2 holds `lyx gate test` per command segment like every round-4 draft.

## Reading-error code opened (tool input naming it)

| Code | twh-1 | twh-2 |
|---|---|---|
| `classifyDeadlineExpiry` | (writer saw it in output only) | writer, pattern-fit |
| `contractFileStatus` / `contractevidence.go` | (writer, premises saw it in output only) | writer, scenarios |
| `ownTrackedPaths` | none | writer |
| go/build GOOS rules (`matchTag`, `syslist`) | none | none |

twh-1 opened none of the four, like `tw-om-2`, and repeated R1-F1 and R1-F3; twh-2 opened three and avoided both.

## Lens finding counts

| Lens | twh-1 | twh-2 |
|---|---|---|
| premises | 4 | 5 |
| goal-scope | 7 | 6 |
| board-fit | 6 | 5 |
| pattern-fit | 4 | 6 |
| scenarios | 7 | 3 |
| leanness | 7 | 8 |
| Total | 35 | 33 |

Fewer than the `opus[medium]` controls' 38-40: forks inherit the writer's model and effort, and a higher-effort record leaves them less to find.

## Verdict, round 5

- `opus[high]` twill averages 1.5 / 3.0 against 1.4 / 2.6 for all five `opus[medium]` twill runs, at 1.31x the cost ($5.87 against $4.48) and 2.7 minutes more.
- The two runs span 2 / 4 to 1 / 2, as wide as the `opus[medium]` spread (1-2 R, 2-4 P); with two runs the arm is indistinguishable from medium.
- What separated the runs is again reading, not effort: the run that opened the deadline and contract-evidence code avoided both defects, and the run that did not repeated them.
- Higher effort does not pay its 31% for twill here.

## Deviations, round 5

- None in setup. Both pairs ran alone, so walls are not inflated by six parallel runs as in round 4; the higher walls are the writer's own.
- Torn down: both pairs with their local and remote branches (twb-twh-2's local code branch needed a manual `git branch -D`, as twc-2's did), both notes and the reed sessions.
