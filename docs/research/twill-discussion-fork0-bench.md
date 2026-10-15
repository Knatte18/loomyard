# twill fork0 bench, Discussion-Write: today's twill, the critical fixer, and fork0 with the critical fixer

On these three references, today's twill (arm a) is the arm to keep for Discussion-Write.
The critical fixer (arm c) and fork0 with the critical fixer (arm f) both cost more and repeated more key items.
In Discussion-Write the critical fixer never disputed a finding, so the gain it gave at Plan-Write did not appear here.

This ran on 2026-10-15 from the prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, with one run per arm per reference and one grader for all nine runs.
The raw evidence is in the git-ignored `.scratch/twill-discussion-fork0-bench/`:
- `setup/`: fragments, wrapper, wave, harvest, teardown and usage scripts, spliced prompts, step and wrap logs, and the `a-to-c.diff` and `c-to-f.diff` proofs.
- `runs/<run>/`: record, support log, prompt, transcripts, fork reports, traces and `usage.tsv`.
- `keys/`: `keys.md`, plus each reference's board body, review rounds, and draft and final records.
- `finding-grades.md` and `tokencount.md`.

## Setup

- **Arms:** all three use twill `opus[medium]`, `discussion_producer: twill`, and the six `twill-discussion` lenses.
  - **a, today:** the prompt is unchanged.
  - **c, critical fixer:** steps 4–5 of the lens review are replaced by the strict critical-judge text from `twill-fork0-bench/setup/critical-judge.md`.
    It was adapted to Discussion only where the Plan text names Plan things:
    - "the plan worse" became "the record worse";
    - `validate-plan` became `validate-discussion`;
    - the lens review is recorded in the support log's last `## Interview` turn, which is where today's Discussion host records it;
    - "or the board entry" was added to the `premise false` ground and to the evidence list, as the Discussion counterpart of Plan's decision record.
  - **f, fork0 plus critical fixer:**
    - The spawn point is replaced by a fork0 preamble.
      The master does Steps 1–4 (board, exploration, auto-picked questions) and states every settled question with its pick and rationale in a text reply before spawning.
      It must not draft either file.
    - fork0 writes both files and passes `validate-discussion`.
    - The master then reads both files fresh, runs `validate-discussion`, and spawns the six lenses.
    - The lens read line is adjusted as in the Plan bench.
    - The plan-only lines of the Plan preamble (the "opening line" override and the per-fork evidence line) were dropped, because today's Discussion host has neither.
- **References:** three tasks that ran twill `opus[medium]` at Discussion (hub default since 2026-10-12) and whose Discussion-Review rounds produced real findings.
  The reference fixers fixed every finding and disputed none.

  | Reference | Letter | Code base | Review rounds | Key items (NITs excluded) |
  |---|---|---|---|---|
  | `hibernate-trigger` | h | `d4824a3d6` | 2 | KH1–KH13 (2 B, 4 M) |
  | `plan-gate-gaps` | g | `f0e753ba2` | 2 | KG1–KG11 (1 B, 6 M) |
  | `claimed-scope-edit` | s | `e2366eca7` | 3 | KS1–KS12 (3 B, 6 M) |

  - The base is main at the moment each reference pair was created, taken from the main reflog against the records' "fabric: record parent branch" commit.
  - Each board body is taken from `_board` history at the Discussion-Write start, and was unchanged through that row.
- **Pairs:**
  - Nine pairs `twb-d0-{h,g,s}{a,c,f}`, each with a board note carrying the reference body, created by `lyx fabric add` and reset to the base.
  - Pair `loom.yaml`: `discussion: opus[medium]`, `discussion_producer: twill`, `parent_review_wait_min: 2`, `alignment_timeout_min: 1`, `skip_alignment: true`.
  - Pair `shuttle.yaml` sets `claude:` to `setup/wrap-d0.sh`.
  - Then `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, `lyx loom step`, `lyx loom pause --after Discussion-Write`, `lyx loom step --until-stop`.
  - All nine ended "paused after Discussion-Write, as requested", with exit code 0.
- **Waves:** wave h ran from 07:26, and waves g and s ran in parallel from 07:35 once wave h's cost (about $2–3 a run) was known.
- **Proof that the spliced prompts differ only by the arm fragments:**
  - Per reference, the three unspliced prompts are byte-identical once the slug is normalized.
  - Arm a's prompt equals the original apart from the trailing arm marker.
  - a to c differs in 18 lines, the critical-judge block only (`setup/a-to-c.diff`).
  - c to f differs in 29 lines: the preamble, the read line and the fork0 block (`setup/c-to-f.diff`).
  - Both counts are identical for all three references.
- **Contamination:**
  - No tool input of any writer or fork names a reference slug, the records archive tags of the references, or the keys.
  - The g runs read `tender`'s plan in the records repo, which is background the reference also used.
  - Mentions of the bench directory come from board entries, flagged by board-fit lenses as sibling notes.

## Per run

Cost is writer, fork0 and lens forks from the transcripts (`usage.sh`, deduplicated, Opus 5.5 prices), with the orch excluded.
Span is the writer's first to last message.

| Run | Arm | Cost | Writer (+fork0) | Lenses | Span | Findings | Real (B/M/L) | Wrong | Noise | Distinct real | Disputed/rejected | Wrong disp. | Key R | Key P | n/a |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| ha | a | $1.98 | $1.44 | $0.54 | 5.3 min | 41 | 27 (1/11/15) | 0 | 14 | 21 | 7 | 0 | 1 | 3 | 3 |
| hc | c | $2.71 | $1.97 | $0.74 | 7.0 min | 42 | 32 (1/10/21) | 0 | 10 | 27 | 0 | 0 | 3 | 4 | 0 |
| hf | f | $3.08 | $1.96 + $0.29 | $0.83 | 7.6 min | 40 | 29 (0/9/20) | 0 | 11 | 25 | 0 | 0 | 3 | 1 | 2 |
| ga | a | $3.09 | $2.32 | $0.77 | 8.3 min | 37 | 22 (0/10/12) | 0 | 15 | 19 | 2 | 2 | 1 | 1 | 3 |
| gc | c | $3.01 | $2.11 | $0.90 | 7.2 min | 34 | 23 (0/5/18) | 0 | 11 | 20 | 0 | 0 | 3 | 1 | 2 |
| gf | f | $3.22 | $1.97 + $0.19 | $1.06 | 6.1 min | 32 | 21 (0/6/15) | 0 | 11 | 19 | 0 | 0 | 2 | 0 | 2 |
| sa | a | $2.90 | $1.98 | $0.92 | 5.8 min | 39 | 24 (0/11/13) | 0 | 15 | 20 | 5 | 0 | 0 | 4 | 1 |
| sc | c | $3.05 | $2.21 | $0.84 | 6.5 min | 36 | 23 (0/6/17) | 0 | 13 | 21 | 0 | 0 | 1 | 5 | 2 |
| sf | f | $3.45 | $2.15 + $0.37 | $0.93 | 8.6 min | 33 | 22 (0/9/13) | 0 | 11 | 19 | 0 | 0 | 5 | 2 | 2 |

## Per arm

| Arm | Cost/discussion | Writer (+fork0) | Lenses | Span | Findings | Real | Real M+ | Distinct real | Wrong + noise | Wrong disp. | Key R / P (n/a) | Key items avoided through a lens |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| a, today | $2.66 | $1.91 | $0.74 | 6.5 min | 117 | 73 | 33 | 60 | 0 + 44 | 2 | 2 / 8 (7) | 5 |
| c, critical | $2.92 | $2.10 | $0.83 | 6.9 min | 112 | 78 | 22 | 68 | 0 + 34 | 0 | 7 / 10 (4) | 3 |
| f, fork0 + critical | $3.25 | $2.03 + $0.28 | $0.94 | 7.4 min | 105 | 72 | 24 | 63 | 0 + 33 | 0 | 10 / 3 (6) | 1 |

Each arm has 36 key items over its three records.
Measured as R + P, a repeats 10, f 13 and c 17.
Counting R alone, a has 2, c 7 and f 10.

## How the critical fixer behaved

- **No disputes:** it disputed nothing in six runs.
  c and f fixed every finding, either as proposed or differently.
  Neither dispute ground was ever tested.
- **Lens precision:** no lens finding in any of the nine runs was wrong, so nothing called for a `premise false` dispute.
- **Noise was fixed too:** the fixer fixed noise where arm a rejected it.
  Arm a rejected 14 findings, and all 14 rejections held (sibling notes, items already named, cosmetic points).
- **Fixed differently:** 15 findings, and every one held.
  Examples: hc waits for the land lock before pausing, and hf holds every hub's land lock to the end.
- **Fixes that added a defect:** two fixes taken as proposed resolved their finding but added a key defect.
  - hc's goal-scope fix added a sleep deny on `loginctl` verbs that do not exist, leaving `systemctl sleep` open (KH3 R, KH12 R).
  - gc's scenarios fix dropped removed packages from the batch gate, which made its "skips nothing" bound false and dropped `./cmd/lyx` for a whole batch (KG1 R, KG2 R).
- **Arm a's two wrong dispositions:** both are in ga, and both are acceptances, not rejections.
  Two lines say the refusal-spec hedge was fixed, but Constraints still says the row lands "if" the mirrored error has one.

So at Discussion the strict judge turns into "fix everything."
That costs a little, and twice it made the record worse.
At Plan the lenses had wrong and noisy findings for it to dispute; here they did not.

## What fork0 lost or gained at Discussion

fork0 is a fork, so it inherits the master's exploration and its settled-question reply.
The question was whether that handoff, and the master's blind review afterwards, lose anything.

- **Handoff losses are small, and lenses repaired most of them:**
  - hf: fork0 carried all 17 settled points intact.
  - gf: fork0 dropped three small items.
    The master restored "card gates don't get it"; two test-list items reached the criteria only through the scenarios lens.
  - sf: fork0 dropped two settled items, the `discussion` gate on Darn-Burler-Fix and the gate-fix exception to the writer's never-edit rule.
    Lenses restored both.
- **The master's review restored nothing itself:** in all three f runs the master read fork0's files, validated them and spawned the lenses with no edit in between.
  Every restoration came through a lens.
- **One gain, in sf:** fork0 corrected the master's settled point 3 so that Go commits the discussion files, because `_lyx/discussion/` is on the records side.
  That avoids KS4, which sc, whose writer held the same reasoning, repeats.
- **f's key misses mostly trace to the master's own picks and premises, not to the handoff:**
  - hf: the master assumed an existing agent sleep deny (false at base, KH3 R).
    It lifted pauses through a plain `tenderengine.Resume`, which also lifts an operator's slug pauses.
    It cancels only when its own entry is lifted, so an operator resume of a pause that was already live lets the hibernate fire (KH2 R).
  - sf: the master picked a recipe-neutral `pr-gate` text that measures a darn run against the writer's unreviewed decisions (KS2 R, KS10 R).
    sa and sc both chose a told darn fragment instead.
    sf's final ledger also contradicts D2 and D5, because the support log was not brought in line with the lens fixes (KS12 R).
  - In all three cases no blind lens caught the problem.
- **Fewer key items avoided through a lens:** f got 1, against 5 for a and 3 for c.
  In Plan-Write fork0's blind lenses found more real findings; here f has the fewest raw findings (105) and its real M+ count (24) is level with c (22), below a (33).
- **Shared blind spot:** all three h records let a session whose state reads `unknown` pass confirmation (KH1 R in ha, hc and hf).

## Total cost

- Bench runs: $26.53 by `tokencount` (discussion $18.11 plus forks $8.42).
- Orch cost `tokencount` attributes to the run windows: $5.69 (this includes the bench agent's prime session traffic and the parent-review deliveries).
- Bench agent session plus the one grader agent (about 336k subagent tokens): not measured, estimated under $15, which would put the total at roughly $45, under the $60 cap.

## Verdict

1. Build nothing new for Discussion-Write: keep today's twill (arm a).
   It was cheapest ($2.66 against $2.92 and $3.25 a discussion) and its final records repeated the fewest key items (2 R + 8 P, against 7 + 10 for c and 10 + 3 for f).
   Its rejections all held.
2. The critical fixer does not carry over from Plan-Write.
   Discussion lenses produced no wrong findings, so the strict judge disputed nothing, fixed noise arm a rightly rejected, and twice brought in a key defect while fixing.
3. fork0 does not pay at Discussion.
   - The handoff itself loses little: a few small items, restored by lenses.
   - It gained one key item (sf, KS4).
   - But f was 22 % dearer than a, its lenses found fewer and less severe real problems than a's, and its records repeated the most key items (R 10).
   - The decisions live in the master, fork0 copies them faithfully (premise errors included), and the master's blind read restored nothing.
   - So at Discussion, fork0 removes the writer's own second look without adding the sharper lens catches it gave at Plan.
4. Caveats:
   - One run per arm per reference; within-arm spread in earlier Discussion benches was 1–4 R.
   - One grader, and real against noise and the key grades are judgement calls.
   - Each key comes from one real Discussion-Review and is graded by problem class across designs that differ.
   - The bench writers saw today's board, not the board at the reference's time.
   - The c and f gaps against a here (5 and 8 more R) are larger than the spread usually seen, but three references cannot settle that.

## Teardown

All nine pairs are torn down: `lyx reed down`, `lyx fabric remove` until it reported `finished: true`, and `lyx board remove` for every `twb-d0-*` note.
No pair directory or board entry remains.
`lyx fabric remove` kept the `twb-d0-*-weft` branches that exist only on origin; deleting them needs `--remote`, which was not passed.
Nothing was committed or pushed, and nothing outside the bench pairs and `.scratch/twill-discussion-fork0-bench/` was edited.
