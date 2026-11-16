# Code Mode and shell pipelines on the Plan writer: one sample per arm and input

2026-11-16, board task `claude-code-mode`.
Six Plan-Write sessions and six Plan-Burler review rounds; a direction from one sample per arm and input, not an effect size.

## Question

Claude Code's experimental Code Mode lets a session write one JavaScript program that makes several tool calls in one step.
Earlier audits found the Plan writer spends many turns on serial reads and searches.
The bench asks whether Code Mode makes a Plan-Write cheaper, faster or better, and whether a gain comes from Code Mode or only from letting the writer batch in the shell.

## Inputs

No landed tapestry run has a twill-free Plan-Write: every archive tag with a plan carries lens files.
Both inputs are twill runs, replayed with `plan_lenses` empty, as the 2026-11-12 bench replayed `machine-name-verb`.

| Input | Run and tag | Base commit | Landed | Pairs |
|---|---|---|---|---|
| Small | `machine-name-verb`, `archive/machine-name-verb/fdb791c78e4b` | `0604f2ea` | 4 cards, 23 files | `cm-sa`, `cm-sb`, `cm-sc` |
| Large | `run-fsm-tracked-records`, `archive/run-fsm-tracked-records/6cbd899d38d2` | `055ef994` | 17 cards | `cm-la`, `cm-lb`, `cm-lc` |

Each pair was a worktree pair whose branch carries the base commit's tree as a forward commit, the tag's decision record, a hand-seeded run at Plan-Write with Motif skipped, and `plan_lenses` empty.
The run paused before Plan-Burler-Fix, so one Plan-Bouncer and one Plan-Burler review round ran on each plan and no fix did.

## Arms

| Arm | Plan-Write | How the setting reached the writer's launch |
|---|---|---|
| A, control | today's Plan-Write, `plan: opus[medium]` | none |
| B, Code Mode | A with `CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=1` | the pair's `claude.yaml` `binary` named `data/claude-codemode.sh`, which execs `claude` under that variable |
| C, pipelines | A with the read rule lifted | the pair's `claude.yaml` `binary` named `data/claude-pipelines.sh`, which execs `claude` with `--append-system-prompt` stating that read-only shell pipelines may read and search |

The binary route needs no production code and sets nothing in user settings; the wrapper runs inside the pane, past the reed server's scrub of `CLAUDE_CODE_*`.
Arm C lifts the rule by an appended instruction that overrides the shared edit directive, not by changing the shared stencil, which other runs were reading.
For B and C the pair's `claude.yaml` was deleted once Plan-Write ended, so the review round ran on the default binary.
Plan-Bouncer had already started in `cm-sb` and `cm-lb` when the file was deleted; their Bouncer sessions show REPL rows.
The reviewer sessions show none.
The first-use dialog "Yes, turn on Code Mode" was answered once, in the `cm-sb` writer's pane.
It wrote `codeModeCaveatsAcceptedV1` and `codeModeAutoModeCaveatAcceptedV1` to `~/.claude.json` and changed no setting.

## Writer figures

Cost is the writer plus its lookup subagents, from `tokencount` and the usage records.
Wall time is the usage record's start to end; the transcript's first to last turn is in `data/metrics.md`.
Calls inside REPL are the inner calls the transcript records; shell run through the `$` template inside REPL code is not recorded, so B's inner counts are a lower bound.

| Figure | A small | B small | C small | A large | B large | C large |
|---|---|---|---|---|---|---|
| Cost | $1.49 | $1.56 | $2.02 | $6.33 | $5.56 | $6.61 |
| Wall time | 4m51s | 5m31s | 5m25s | 17m01s | 17m21s | 19m50s |
| Assistant turns | 24 | 21 | 36 | 49 | 46 | 50 |
| Tool calls, direct | 24 | 33 (18 REPL, 15 Poll) | 40 | 91 | 82 (41 REPL, 41 Poll) | 74 |
| Tool calls, with those inside REPL | 24 | 69 | 40 | 91 | 171 | 74 |
| Lookup subagents | 0 | 0 | 0 | 4 | 5 | 3 |
| Peak context, tokens | 131,491 | 138,491 | 152,022 | 346,140 | 269,573 | 361,042 |
| Read and Grep calls | 1 | 19 inner | 5 | 18 | 33 inner | 6 |
| Shell read pipelines | 15 | 29 inner | 27 | 34 | 63 inner | 47 |

- No writer made a Grep call.
  The control writers already read mostly by shell pipeline, so arm C's lifted rule changed little of what they did: A made 15 of 16 and 34 of 36 Bash calls read-only pipelines.
- B's `REPL` call returns "(dispatched #N)" and the output comes back in a later `Poll` call, so each REPL batch costs two direct calls.
- B's five large-input lookup subagents all show REPL rows, so a subagent spawned through the Agent tool inherits Code Mode with its session.
- B's turns fell by three on both inputs, and its direct calls fell by nine on the large input.
  Its calls with the inner ones counted rose from 24 to 69 and from 91 to 171, so Code Mode did batch, but the turn count barely moved.
- C was slower and dearer on the small input (36 turns against 24) and 4% dearer on the large one.

## Review round and total cost

Reviewer is the Plan-Burler reviewer plus its forks.
Run total is every Plan session, Plan-Bouncer included, plus the gate fixer.

| Figure | A small | B small | C small | A large | B large | C large |
|---|---|---|---|---|---|---|
| Reviewer cost | $3.48 | $3.28 | $3.10 | $7.27 | $7.05 | $7.91 |
| Review findings: BLOCKING, MEDIUM, LOW, NIT | 1, 4, 6, 1 | 1, 5, 8, 0 | 1, 5, 5, 2 | 2, 11, 9, 1 | 2, 11, 6, 0 | 2, 10, 7, 0 |
| Plan cards | 4 | 4 | 4 | 14 | 13 | 14 |
| Run total | $5.29 | $5.02 | $5.28 | $13.91 | $12.93 | $14.82 |

Every plan got the verdict BLOCKING from its first round.
The bench's spend by `tokencount` is about $57 against the $80 cap.

## Quality against the keys

Keys are in `keys.md`, fixed before any plan was read: L1 to L9 for the small input, F1 to F17 for the large one.
One grader subagent per input read the plans without knowing the arms; every grade carries its quoted line in `data/grade-small.md` and `data/grade-large.md`.
The keys are findings of the landed run's own review, so they favour a plan that follows the landed run's route; the large keys include judgment calls on thin plan text.

| Input | A: R / P | B: R / P | C: R / P |
|---|---|---|---|
| Small, L1 to L9 | 2 / 2 | 1 / 2 | 2 / 1 |
| Large, F1 to F17 | 4 / 3 | 3 / 3 | 5 / 6 |

- B repeated or partly repeated the fewest keys on both inputs, by one key on the small input and one on the large.
  C is no better than A on either input and worst on the large one, with F6, F8 and F10 repeated.
- All three large plans repeat F16 (batten step and loom start do not ask the act rule) and partly repeat F17 (stale docs); F15 is n/a for all three.
- The small plans repeat L4 in A and C and L7 in A; B avoids L4 and partly repeats L7.
- Each pair's first review round carried one BLOCKING finding on the small input and two on the large one, in every arm.
  The small A and C plans share one: mounting the `machine` module fails the sandbox coverage guard because no card lists an `excludedModules` entry.
  B's differs: its `show` reads the developer's real machine file, which contradicts decision D8.
- Files named against files landed, at package level for the large input: every arm named every directory its plan touches that the landed run also changed, and every arm missed `internal/boardengine`.
  The landed commit of the large run changes 149 files, with orch-session work and docs bundled in, so the unnamed count is an upper bound.

## Verdict

- Code Mode shows no gain this sample can tell from noise.
  B's cost is 5% above A on the small input and 12% below on the large one; its wall time is 14% above and 2% above; its turns are three fewer on each.
  Its plans repeat one key fewer on each input, which is inside what a second sample of the control could differ by.
- C does not explain a Code Mode gain, because there is none to explain; pipelines alone cost more on both inputs and made the worst large plan on the keys.
- A Code Mode writer makes twice the tool calls of the control on the large input, but the batches replace few turns, because each REPL call needs a Poll call and the control already batches in the shell.
  The `Poll` part is corrected in [code-mode-web-research-2026-11-16](../code-mode-web-research-2026-11-16/report.md): the writers sent most `Poll` calls in the same turn as their `REPL` call.
- A lookup subagent runs in Code Mode with its session.
- The Discussion writer bench is not warranted by this result; the gate the board entry set, a gain on the Plan writer, was not met.
- Adopting Code Mode per role would need the launch line to carry the variable, as `forkSubagentEnvKey` rides it in `internal/shuttleengine/claudeengine/command.go`, and a resume to carry it too.
  Its tooling would need to read the `REPL` rows.
  None of that is justified by this bench.

## Deviations and limits

- One sample per arm and input, one grader per input; a second control run is the missing comparison for every difference above.
- The inputs are twill runs, replayed without their lenses, so the keys come from a plan reviewed under twill.
- All three small arms asked the orch how to read the hostname through `internal/machinename`; the orch recorded the same answer in each, a hand-fed input none of the large arms needed.
- The six runs ran at once in two waves, so they shared the account's rate and each other's wall-clock; the A runs started a few minutes before B and C.
- The `cm-sb` and `cm-lb` Plan-Bouncer sessions ran in Code Mode, which the arm's definition did not intend; the reviewer sessions did not.
- The `cm-sb` writer waited at the first-use dialog until it was answered by hand, and its wall time includes that wait.
- The control's edit directive already allowed shell reads in practice, so C's lifted rule was a small change.
- The large input's landed change count (149 files) is not the 116 files of the tag's usage record, since the landing commit bundles other work.
- `tokencount` prices `claude-opus-5-5` from its price table; no Fable session ran.
- Raw figures and grades are in `data/`: `metrics.md`, `grade-small.md`, `grade-large.md`, the two `tokencount` reports and the two wrapper scripts.
