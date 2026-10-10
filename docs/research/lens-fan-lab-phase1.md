# Fan-lab phase 1: measured runs of the master-instruction variants

Measurement report from the `fan-lab` session (2026-10-10); `.scratch/` paths below are in that worktree and were not kept, and phase 0 is `lens-fan-lab-phase0.md`.

Setup: one standalone `lyx burler run` per variant in the `fan-lab` worktree (hub mode), against the round-2 target as the D2 master read it (`.scratch/fixture/target-r2/`), with round 1's review and fixer report as prior rounds.
Rubric and fasit as in round 2, copied into `.scratch/lab/profile-lab.yaml`; round 2's focus file was empty and is omitted.
Master and forks `sonnet[medium]` (a fork inherits its master's model and cache, so they cannot differ); the `discussion` fan (5 lenses) unless marked `std` (the `standard` fan, as in the original round).
`burler run` always starts the fixer too; it ran on `haiku[low]` and is counted only in the phase total.
Code under review: `fan-lab` HEAD, which differs from the round-2 base `b6e1c3601` only in test files on the paths F1 rests on.
Variant texts: `.scratch/phase0.md` section 4, with B made lens-agnostic per the operator (no lens names in step 1); the code is `clusterPrereadBlock` and `clusterRulesBlock` in `internal/burlerengine/prompt.go` behind the lab-only `fanLabVariant` switch, uncommitted.
Each variant ran once; a single run is noisy, so read differences under about 30k fresh as noise.

## Results

"Code pre-reads" counts the master's tool calls over code before the spawn (excluding prompt, instructions, target, stencil and skills).
Review side = master + forks; fresh = input + cache-write + output.

| Run | Fan | Code pre-reads | Fork context at spawn | Fork tool calls (sum) | Fork turns (sum) | Master fresh | Forks fresh | Review side fresh | Review side cache read | Fixer fresh | Verdict | B / M / L / N | F1 found |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| A | discussion | 0 | 55k | 2 | 12 | 88k | 128k | 216k | 1.18M | 84k | APPROVED | 0 / 4 / 14 / 2 | no |
| B | discussion | 2 (board, pattern files) | 64k | 8 | 18 | 93k | 61k | 154k | 2.38M | 90k | APPROVED | 0 / 6 / 9 / 2 | no |
| C | discussion | 2 (board, pattern files) | 62k | 0 | 10 | 86k | 47k | 133k | 1.28M | 111k | APPROVED | 0 / 8 / 8 / 2 | no |
| D | discussion | 12 | 103k | 1 | 11 | 146k | 52k | 198k | 2.88M | 121k | APPROVED | 0 / 7 / 10 / 1 | no |
| D std | standard | 3 (large region reads) | 74k | 1 | 11 | 105k | 48k | 154k | 1.52M | 99k | APPROVED | 0 / 8 / 5 / 2 | no |
| D2 (original) | standard, sonnet[high] | 0 | 56k | 135 | 130 | 134k | 485k | 619k | 14.5M | n/a | BLOCKING | 1 / 9 / 9 / 0 | yes (4 of 5 forks) |

Phase total: 1.36M fresh including fixers (A 299k, B 243k, C 243k, D 319k, D std 253k), within the 1.5M cap; no run reached the 300k review-side abort.

## What the runs show

1. **Only the manifest makes a Sonnet-medium master pre-read.**
   In A, B and C the master read instruction 2 together with instruction 1 and forked after at most the board entry and a few pattern files; B's and C's reading checklist was skipped.
   In both D runs the master read code regions before spawning (12 calls by Decision in D; 3 large `sed -n` reads in D std), wrote a neutral manifest grouped by Decision, and read instruction 2 only after it.
   The having-to-list-it step is what turns the checklist into work.
2. **The master judged nothing before the spawn in every variant.**
   D's pre-fork thinking summary is "I've now reviewed the target pair, … next I'll read the fabric guard, recovery, and refscanner code before sending the reading manifest"; the manifest names files and symbols only.
3. **Pre-reading moves code-grounded findings into the fan.**
   D and D std are the only runs whose kept findings match round 2's code-grounded MEDIUMs: D matches F3 (configcli's nested fabric sync), F4 (D12's cap binding `card_amended`) and F13 (empty `RecoveryStartSHA`), and adds a D7 placement defect (weft verbs carry their own `PersistentPreRunE`); D std matches F13 and F14 (other Rebaseline errors).
   A and B match none of these; their findings are text-level (scope, leanness, missing criteria).
4. **No run found F1, the round's one BLOCKING.**
   F1 needs a hop beyond what the target cites: from the D7 guard on `push` to `Fabric.Commit` → `SpawnDetachedPush`, whose child inherits the strand name.
   D's master read `fabric.go`, `weft_verbs.go`, `agentname.go` and `configcli.go` but not `commit.go` or `spawn.go`, though its checklist says "and the processes it spawns".
   In the original round, four forks found F1 by reading on their own (135 fork tool calls); in every lab run the forks read almost nothing (0-8 calls), so no one made the hop.
5. **At `sonnet[medium]` the forks barely read in any variant, and the cost problem of D2 does not reproduce.**
   Even A's forks made 2 tool calls in total; D2's 485k fork fresh came from `sonnet[high]` forks on the `standard` fan, told by their master to verify against the code.
   So the 6-call budget (C, D) is not what kept forks cheap here; medium effort was.
   What the budget does do is tell forks their context is complete, which in C, with nothing read, produced forks that reviewed the target text alone.
6. **Cost.**
   Review side ran 133-216k, against 619k for D2 and 86-121k for a solo `opus[medium]` round.
   D costs about 50-65k more master fresh than A-C for its pre-read and sits at about 200k, within the board's 200-250k estimate; its forks are the cheapest per fork (7-14k) because they inherit a 100k prefix from cache and write short answers.
   A's forks cost 22-30k each with almost no reading, mostly their own output.

## Cost and time against a solo round

The operator added one solo round on the same fixture and setup: no fan, `opus[medium]`, fixer `haiku[low]`.
Wall time is the review side, from the master's first transcript record to its Write of the review file.
Dollars at API list price per MTok: Opus 5.5 input $4, output $20, cache write $5 (5m) / $8 (1h), read $0.20; Sonnet 5.5 $2 / $10 / $2.50 / $4 / $0.20; Haiku 5.5 $0.10 / $0.50, writes 1.25x / 2x input, read 0.1x.
Every cache write in these runs was 5-minute TTL; none was 1-hour.
Each message is priced on its final usage, each transcript file on its own; a fork file includes its replayed spawn message, so D1 and D2 come out 5-15% above their usage records (D2 703k fresh here against 619k recorded).

| Run | Model / fan | Wall time | Review side $ | Fixer $ | BLOCKING / MEDIUM | F1 found |
|---|---|---|---|---|---|---|
| A | sonnet[medium], discussion fan, today's text | 103 s | 0.98 | 0.03 | 0 / 4 | no |
| B | sonnet[medium], discussion fan | 109 s | 1.08 | 0.03 | 0 / 6 | no |
| C | sonnet[medium], discussion fan | 100 s | 0.75 | 0.04 | 0 / 8 | no |
| D | sonnet[medium], discussion fan | 160 s | 1.40 | 0.03 | 0 / 7 | no |
| D std | sonnet[medium], standard fan | 143 s | 0.92 | 0.03 | 0 / 8 | no |
| Solo | opus[medium], no fan | 225 s | 1.45 | 0.02 | 1 / 2 | yes |
| Solo + body dump (not the quarry pack) | opus[medium], no fan, symbol bodies in a file | 312 s | 1.93 | 0.03 | 1 / 5 | no (its BLOCKING is a different D7 defect) |
| Solo + quarry kick-start pack | opus[medium], no fan, pack in the first prompt | 316 s | 1.94 | 0.03 | 1 / 4 | yes |
| R (recommended D text) | sonnet[high], standard fan | aborted at 288 s active, no review written | 3.19 at abort | 0.01 | none written; forks raised F1 (1 BLOCKING, 2 MEDIUM) | in 3 of 5 forks, not in a review |
| D2 (original round 2) | sonnet[high], standard fan, today's text | 329 s | 5.71 | n/a (opus fixer, transcript not in fixture) | 1 / 9 | yes |
| D1 (original round 1, other target) | sonnet[high], standard fan, today's text | 288 s | 2.53 | n/a | 3 / 6 | n/a |
| Solo opus[medium], webster-replan-state | — | missing: no such round in the fixture | | | | |

The solo round's review side: 135k fresh, 2.19M cache read, 31 turns; it found F1 by reading `fabricengine/commit.go:181` and `spawn.go:40` itself, and kept 1 BLOCKING, 2 MEDIUM and no LOW.
The fanned Sonnet-medium rounds are 65-125 s faster and cost $0.05-0.70 less than solo, and none found the BLOCKING finding.

The first pack run was not quarry's kick-start pack: it gave the solo reviewer whole symbol bodies in a separate file, the treatment quarry's ladder design excludes.
The real pack (`RenderKickstartPack` format: one `<glyph> → <file> <start>-<end>` line and one signature line per glyph, 35 glyphs, 70 lines, 5.3 KB) was then injected into the reviewer's first prompt (`burler-template-review-orchestrator.md`, from a stencils copy outside the worktree) with the ladder card's imperative "Read all listed spans in parallel, in one turn, before doing anything else."
The reviewer did exactly that: its first action after the prompt was 20 parallel Reads of the spans.
It found F1 at BLOCKING and kept 4 MEDIUM, 5 LOW and 2 NIT (plain solo: 1 / 2 / 0), in 316 s for $1.94 review side (plain solo: 225 s, $1.45), with 50 tool calls in all.
The glyph list was still mapped from the target's backticked symbols by hand; a production pack needs that step mechanical.
The body-dump run below is kept for the record.

The body-dump run gave the solo reviewer `.scratch/lab/kickstart-pack.md`: the declarations of the 35 symbols the target cites, mapped to glyphs by hand and resolved with `lyx quarry resolve`, with each span's body cut from the file (1,686 lines, 81 KB, about 20k tokens), named in the profile's target instructions as "read first … and read beyond it only one hop".
For this run the lab source changes sat in a tagged stash, and the phase documents, fixture and earlier lab reviews were moved out of the worktree; all were restored afterwards.
The reviewer read the pack (3 Reads) and then made about 20 tool calls of its own, so the pack did not replace exploration: review side 173k against 135k, $1.93 against $1.45, and 312 s against 225 s.
Its verdict was BLOCKING, but on a different D7 defect: webster refusals name `lyx fabric commit` and merge verbs as their way forward, and D7 refuses them under a webster strand name (round 2's F2, raised there as MEDIUM).
It read `weft_verbs.go`'s bypass branch but not `SpawnDetachedPush`, so it missed F1; the plain solo found F1 by reading `commit.go` and `spawn.go` itself.
On this single run the pack made the solo round slower and dearer without finding more.

R ran the recommended D text (below) on the `standard` fan at `sonnet[high]`, to test whether a pre-read fan matches solo on F1 in less time.
Its master pre-read with about 40 tool calls (rebaseline, recovery, fabric guard, shell wait, plan gate), wrote the manifest, and then ended its turn: "End this step with a reading manifest as a plain message" reads as an end of turn, and shuttle held the idle master for 123 s until `ly:orch` told it to go on; that gap is excluded from the wall time.
After the spawn, three forks used their hop budget (3-4 calls each) on `spawn.go` and `weft_verbs.go` and raised F1, generic B at BLOCKING; the master read `SpawnDetachedPush` too.
The round passed 300k review-side fresh during consolidation, at 288 s of active time, and was aborted (354k, $3.19) with no review written; both strands were then interrupted with `lyx shuttle interrupt`, since stopping `burler run` alone left them working.
So the pre-read and the hop line work at high effort, but the round cost more than twice solo and took longer than solo before writing anything.
One contamination to note: R's master ran `git diff` in the worktree and saw the uncommitted lab code in `prompt.go`.

## Plan-Review: solo with and without the kick-start pack

Plan-Review is a closed check — do the plan's cards carry the decision record — so the pack was tried there too.
The fixture is webster-replan-state's Plan round 1 (transcript `2ad64800`): the 25 cards and the overview rebuilt byte-exact from the master's own `cat` output, the decision record from its Read, the round's rubric and focus file, all under `.scratch/lab/plan/`.
The rubric's two anchor-relative paths point at the fixture; the focus file is named in the target instructions, since standalone burler has no focus key.
The code is `fan-lab`'s HEAD (`b7d4fd0ba`), which postdates the plan's base by one test-suite commit and predates the plan's landing (`5a8be0274`).
The pack is mechanical: every backticked glyph in a card's `Edit`, `Uses` and `Delete` groups, `plan:` glyphs (not yet in code) left out, in card order, located with one `lyx quarry resolve` call and rendered in `RenderKickstartPack` format with the parallel-read imperative: 97 glyphs, 7,065 lines across 54 files.
Both rounds ran solo `opus[medium]`, fixer `haiku[low]`, with the lab source in a tagged stash and the fixture transcripts (which hold the original review) moved out of the worktree.

| Run | Wall time | Review side $ | Fresh | Turns | Fixer $ | Findings (B / M / L / N) | Of the original's 23 |
|---|---|---|---|---|---|---|---|
| Solo | 264 s | 1.74 | 155k | 32 | 0.02 | 1 / 4 / 2 / 1 | 3 (cards 17, 19, 2) |
| Solo + plan pack | 406 s | 3.58 | 372k | 24 | 0.03 | 1 / 5 / 2 / 2 | 6 (cards 17, 14, 21, 13+16, 11, 19) |
| Original round 1 | 633 s | 10.24 | 979k | | n/a | 2 / 12 / 8 / 1 | 23 |

The original was `sonnet[high]` (the transcript's `effort` field) with five lens forks.
The pack round read every one of the 97 spans in its first tool turn, as 11 parallel `awk` calls over merged ranges (7,448 lines); the output overflowed into 9 persisted tool-result files, which it read in the next turn.
It then made about 18 calls of its own, so the pack again added to the exploration rather than replacing it, and the pack's size doubled the cache writes.
What it bought: the original's top finding, card 14 leaving D8 inert under the loom (the loom's `RunDeps` in `loomCLI.wire` never gets the read-only classifier), which plain solo missed, plus card 21's missing `Run` construction sites, the classifier/guard reader-list drift and card 11's unlocatable card files.
Plain solo found four defects the original did not (card 3's batch-order refusal, card 13's dropped positionals, card 16's unreachable `Uses`, card 4's re-pinned refusal); the pack round shares two of those.
Neither found the original's other BLOCKING: card 3's auto-path refusals naming `lyx webster run` and `lyx webster rebaseline`.

The original's 23 findings were then checked against the plan, the decision record and the code by three read-only agents (`.scratch/lab/plan/verify-{1,2,3}.md`).
10 hold as stated, 3 hold at a lower severity (F2, the card-14 finding, is MEDIUM, not BLOCKING: per-batch classing runs in `lyx webster record-batch`, which card 14 wires, so only findings it never dispositioned still halt), 8 hold in part, 1 is false (F19) and 1 is not a defect (F8).
F1, the other BLOCKING, holds only in its `CheckBatchOrder` part (`sequence.go:37` hard-codes `lyx webster rebaseline`, and no card targets it), which is plain solo's card-3 finding.
After the check the original carries 8 findings at MEDIUM or above (F1's held part, F2, F3, F4, F6, F7, F11, F12); plain solo found 4 of them (F1, F4, F6, F7) and the pack round 4 (F2, F3, F4, F7), each with MEDIUM or above findings of its own the original lacks, which were not checked.
So on this fixture the `sonnet[high]` fan's larger count is mostly LOW-level test-row and wording findings; on the findings that matter, each solo round matched half of it for a sixth (plain) or a third (pack) of its cost.

## Recommendation

For Discussion-Review, keep the solo `opus[medium]` round.
On this fixture it is the only configuration that found the round's BLOCKING defect within the 300k abort, at the same price as D ($1.45 against $1.40) and 65 s slower; the original fanned round found it too, but at 4x the price and 100 s slower than solo.
At medium effort the fan buys about a minute per round and costs the BLOCKING finding; at high effort (R) it finds it but is slower and more than twice as expensive as solo.

If the fan is pursued further (Plan-Review, or a code target where lenses aim at code), use **D**, with one change before it lands: the reading checklist names the hop that F1 needed.
D is the only variant that made the master pre-read and keep from judging.
Keep the fork budget as a fallback, not a ban: the measured cost is driven by effort, and the budget's real effect is to stop forks re-deriving the shared read.
It needs a confirming run on this fixture that finds F1 before it replaces solo anywhere.

Text for that case, step 1 (`clusterPrereadBlock`, reviewer's explore file only, cluster rounds only):

```
## Reading for the fork reviewers

This round fans out: in step 2 you spawn fork reviewers, and each fork inherits your context as it stands at the spawn.
Your reading in this step is the forks' reading, so read here what the review will need, once, instead of leaving each fork to read it again.

Before you leave this step, read:

- every file the target names, at the definitions of the symbols it cites;
- for each symbol the target changes, guards or removes: its direct callers and callees, and every path that reaches it from another process, such as a child the code spawns or a command that calls the guarded verb in-process;
- the `pattern/` background file of every PATTERN entry the target names or touches;
- the board entry of the task the target belongs to, through `lyx board get`;
- the failure, restart and resume paths of each change the target makes.

Read definitions and call sites, not whole files: a fork re-reads its inherited context on every turn.
Judge nothing while you read: write no finding, verdict or assessment, not even in a status line or a note to yourself.
A judgement in your context before the spawn colours every fork; your own review starts after the forks are spawned.

Close this step with a reading manifest: one line per file and symbol you read, grouped by the part of the target it serves.
The manifest names what you read and nothing about what you found.
Write it as text in the same turn and go straight on to step 2: the manifest never ends your turn.
```

The last line is new after run R, whose master ended its turn on the manifest; it is untested.

Step 2 (`clusterRulesBlock`), replacing the spawn trigger, the fork boilerplate and the holistic paragraph:

```
This is a cluster round: as soon as step 1's reading is done, spawn ALL of the fork reviewers below in a SINGLE message via the Agent tool with `subagent_type: "fork"`, one per lens listed here — never pass a `name` (named forks silently lose inherited context).

Lenses for this round:

<lenses>

Each fork's prompt is this boilerplate plus that lens's emphasis text, and nothing else: never add a direction to verify against the code or to read files.
Your inherited context already holds the target, the rules and the code the target rests on; review from it.
The reading manifest in your context lists what is already read; check it before any tool call.
You may make at most 6 tool calls, each only for something your lens needs that the manifest lacks, such as a caller or a spawned process one hop beyond it, and never to re-read what your context already shows.
End your final message with one line per tool call you made: what you read and why your context lacked it.
You are READ-ONLY — never Write/Edit/delete any file, never run any git command, never touch the two round output files, and never call the Agent tool yourself (forks cannot nest); return your findings ONLY as your final message, each with a severity, a class, a location, and a one-line summary.

While the forks run, YOU (the handler) do your own HOLISTIC review — architecture, cross-file invariants, PATTERN-fit — from the same context, reading further only where it or a fork's read list shows a gap, and prepare the ground truths and the severity rubric you will judge every finding against.
```

The consolidation paragraph is unchanged.
The two edits against the measured D text are the second checklist item (other-process paths) and "such as a caller or a spawned process one hop beyond it"; both target the F1 miss and are untested.

Evidence: the D and D std rows above; D's master pre-fork calls and manifest in transcript `d1a50543` (project `-home-knatte-Code-loomyard-LYXHUB-fan-lab`); reviews under `.scratch/lab/<run>/round-2-review.md`; token tallies in `.scratch/lab/<run>/tokens.txt`.

## Open items

- **F1 is not recovered by any measured variant.**
  Before the fan goes back on, one run of the recommended text (or D at `sonnet[high]`) on this fixture should show F1; about 300k with the fixer.
- **The fan is still not cheaper than solo.**
  On this fixture the solo `opus[medium]` round used 135k fresh against D's 198k, at about the same price; see the cost-and-time table.
- **Lens colouring.**
  Every fork already inherits instruction 2, which lists every lens, because the master reads it before spawning; this held in D2 and in every lab run, so a lens-agnostic step 1 does not change what the forks see.
- **The `discussion` fan has no code lens.**
  With pre-reading, its findings became code-grounded (D), but no lens is aimed at "a guard's callers"; `scenarios` is the natural owner if a lens text change is wanted.
- **Proposal E: run once as the solo quarry pack (see the cost-and-time table); it did not pay off on this fixture.** The original proposal:
  The target cites 29 files and about 50 symbols; 22 of the 25 files that two or more D2 forks read are named in it by path or by a cited symbol's definition, and the other 3 are one hop away.
  lyx could extract those citations (paths, backticked symbols) and list them, without loading code, as the manifest's starting point in step 1, leaving the choice of what to read to the master; this keeps the operator's rule that lyx does not judge relevance while making D's reading list reproducible.
- **Lab plumbing** (`{{.cluster_preread}}` in `burler-step-1-explore.md`, `clusterPrereadBlock`, the `fanLabVariant` switch, set back to `A`) is uncommitted on `fan-lab`; the landing task implements the chosen text without the switch, with `prompt_test.go` pinning it.

## Conclusion

Discussion-Review stays a plain solo round on `opus[medium]`: on this fixture it alone found the BLOCKING defect, in 225 s for $1.45.
The lens fan is parked: at `sonnet[medium]` it was faster but missed the BLOCKING defect in every variant, and at `sonnet[high]` with the recommended D text it found it in its forks but passed 300k and ran slower and dearer than solo before writing a review.
If the fan is revisited, D's manifest is the one master instruction that made the master pre-read without judging, and its text above, with the same-turn fix, is the starting point.
The quarry kick-start pack, injected into the first prompt with the parallel-read imperative, found F1 and twice the MEDIUM findings of plain solo, at about 90 s and $0.50 more per round; the earlier body-dump run was not the pack and says nothing about it.
On Plan-Review the mechanical pack did not pay off either: after the original's findings were checked, plain solo and the pack round each matched 4 of its 8 MEDIUM-or-above findings, and the pack cost twice as much ($3.58 against $1.74) and took 140 s more.
The `sonnet[high]` fan found more in total, but its surplus is mostly LOW findings, at $10.24 and 633 s; whether its four extra MEDIUM findings are worth six times the cost of a solo round is the operator's call.
A smaller pack (`Edit` and `Delete` glyphs only, or capped in lines) was not run; the weekly quota stood at 91%.
Every comparison here rests on one run per configuration over one fixture.
