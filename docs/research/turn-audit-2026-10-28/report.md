# Turn audit, 2026-10-28

Where the turns of an agent session go, which of them a merge would have removed, and which stencil or system-prompt wording is most likely to cut them.
Every figure derives from the scripts and TSV tables beside this file; the sample size `n` stands beside each number.
A recommendation is not a decision: it changes no stencil, system prompt, profile or config.
No change proposed here hands an agent pre-read or digested source; the agents keep reading the code themselves.

## Sample and method

- Source: the Claude Code transcripts under `~/.claude/projects/`, main sessions and their fork sub-agents (`<session>/subagents/*.jsonl`).
  No agent or model run beyond the darn writer itself was made: no replay, no bench, no grading agent.
- Runs: 144 `archive/*` tags in the records repository were created on or after 2026-10-20, and 23 of those slugs have a project directory on this machine.
  Only 10 slugs hold transcripts: one loom run (`dobby-and-adopt-hardening`) and nine darn runs, all listed in `slugs.txt`.
  The other landed runs' transcripts are not on this machine, so the scope's "landed since 2026-10-20" is met by what remains, not by every run.
  Their sessions started between 2026-10-27 and 2026-10-28.
  No OSL-1046 run is among them.
- Sample: 213 sessions (93 main, 120 forks), 2934 turns, 3407 tool calls.
  Only one loom run is in the sample, so every role that only a loom run has (Plan writer, Discussion writer, Webster Master, the Discussion and Plan reviewers and fixers) rests on `n = 1` to `n = 3` sessions.
  Those rows show a pattern; they do not show a typical value.
- A turn is one assistant API message: the records sharing one `message.id`.
  Its tool calls are the `tool_use` blocks across those records.
  A turn with no tool call ends a stretch of work (a final text).
- A role is the part after the last colon of the session's title; a fork counts under its parent's role with `+fork` appended.
  `plan` is the Plan writer and `plan+fork` its lens forks; `webster` is Merriam and `webster+fork` the implementer forks; `conflict` is the landing merge-conflict fixer and `fixer` the landing fixer.
- Cost is the `tools/tokencount` weight: input + 1.25 x cache write + 0.1 x cache read + 5 x output.
  It is a relative figure for ranking, not a price.
  `tools/tokencount` itself was not run: it counts cost per segment, not tool calls per turn, and `go run` is refused in this session.
- Call classes come from a keyword reading of the call, in `extract-turns.sh`:
  - `read`: the Read tool, and shell `sed -n`, `cat`, `head`, `tail`.
  - `search`: shell `grep`, `rg`, `find`, `ls`, `awk`.
  - `git-read`, `lym-read`, `quarry`, `skill`: a read-only git command, `lym board get|list` or `lym loom peers|show`, `lym quarry`, the Skill tool.
  - `edit`: the Edit and Write tools. `edit-script`: `sed -i`, `perl -i`, a heredoc or a redirect into a file outside `.scratch`.
  - `gate`, `test-lint`, `validate`: `lym gate`, a test, vet or lint run or `lym loom fmt`, and `lym loom validate-*`.
  - Anything else is `other-bash`, `other-write` or `scratch-write`.
  A command that chains statements takes the class of its strongest statement.
  The agents made no Grep or Glob call in the sample; they searched through the Bash tool (1939 Bash calls) and read files through the Read tool (372 calls) or shell `sed -n` and `cat`.
- A **mergeable run** is two or more consecutive single-call turns whose class is `read`, `search`, `git-read`, `lym-read`, `quarry` or `skill`.
  A gate, test, lint or validate call is never counted: a repeat follows an edit or a busy gate slot.
- Two counts of mergeable turns, both leaving out the run's first turn (the turn the merge keeps):
  - **strict**: a call that names a path-like token found in the previous turn's tool results and nowhere earlier in the session (prompt, attachments, older results, earlier call inputs) starts a new run, since it could not have been issued a turn earlier.
  - **class only**: the upper bound, with no dependency test.
  The strict count is an estimate, not a bound: it misses a dependency that is not a copied token (picking a line range after seeing the file length), and it counts a token the agent could have guessed.
- The saved weight of a run is the weight of its turns after the first.
  A merged turn is a little heavier in output and in the results it brings in at once; that is not subtracted.

| File | Content |
| --- | --- |
| `turns.tsv`, `calls.tsv` | per turn and per call rows (`extract-turns.sh`) |
| `sessions.tsv` | per session counts, mergeable runs, read-edit-test counts (`session-metrics.sh`) |
| `role-summary.tsv` | `sessions.tsv` summed per role (`role-summary.sh`) |
| `calls-histogram.tsv` | turns per role by tool-call count (`calls-histogram.sh`) |
| `merge-kinds.tsv`, `kinds-total.tsv` | the calls of the removable turns, per role and in total (`merge-kinds.sh`, `kinds-total.sh`) |

`run-all.sh` rebuilds every table; `slugs.txt` lists the runs.

## 1. Calls per turn

| Role | Sessions | Turns | Turns per session | Calls per turn | One call | Two calls | Three or more | Share of sample weight |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| darn | 14 | 761 | 54.36 | 1.08 | 86% | 5% | 2% | 24.1% |
| webster+fork | 4 | 323 | 80.75 | 1.37 | 76% | 15% | 8% | 15.3% |
| code-burler-fix | 7 | 200 | 28.57 | 1.47 | 55% | 11% | 17% | 4.8% |
| code-burler-review | 9 | 179 | 19.89 | 1.59 | 54% | 9% | 20% | 5.9% |
| plan-burler-review | 3 | 127 | 42.33 | 1.17 | 80% | 9% | 6% | 6.0% |
| code-burler-review+fork | 36 | 115 | 3.19 | 0.73 | 64% | 4% | 0% | 3.6% |
| discussion-burler-review | 3 | 115 | 38.33 | 1.15 | 72% | 4% | 8% | 4.2% |
| plan-burler-fix | 3 | 104 | 34.67 | 1.09 | 70% | 4% | 9% | 2.6% |
| plan | 1 | 101 | 101.00 | 1.40 | 79% | 7% | 6% | 5.6% |
| code-burler-fix+fork | 28 | 91 | 3.25 | 0.75 | 65% | 3% | 1% | 2.0% |
| discussion-burler-fix | 3 | 91 | 30.33 | 1.87 | 46% | 10% | 24% | 2.7% |
| plan+fork | 6 | 73 | 12.17 | 0.96 | 90% | 0% | 1% | 5.6% |
| describe | 9 | 66 | 7.33 | 0.73 | 73% | 0% | 0% | 1.1% |
| code-bouncer-seed | 9 | 64 | 7.11 | 0.97 | 58% | 3% | 11% | 1.2% |
| discussion-burler-review+fork | 12 | 58 | 4.83 | 0.84 | 74% | 5% | 0% | 2.5% |
| code-bouncer-judge | 9 | 54 | 6.00 | 1.17 | 33% | 17% | 17% | 1.1% |
| discussion | 1 | 51 | 51.00 | 1.06 | 76% | 8% | 2% | 1.5% |
| fixer | 1 | 50 | 50.00 | 1.02 | 86% | 8% | 0% | 1.1% |
| plan-burler-fix+fork | 12 | 49 | 4.08 | 0.84 | 69% | 4% | 2% | 1.3% |
| discussion-burler-fix+fork | 12 | 41 | 3.42 | 0.80 | 63% | 5% | 2% | 1.2% |
| tackler | 5 | 40 | 8.00 | 1.57 | 60% | 2% | 25% | 1.3% |
| friction | 4 | 33 | 8.25 | 0.76 | 76% | 0% | 0% | 0.4% |
| discussion+fork | 7 | 27 | 3.86 | 0.81 | 67% | 7% | 0% | 1.2% |
| webster | 1 | 24 | 24.00 | 1.00 | 54% | 12% | 4% | 0.5% |
| arbiter | 2 | 19 | 9.50 | 0.89 | 89% | 0% | 0% | 0.4% |
| discussion-bouncer-judge | 3 | 18 | 6.00 | 1.28 | 33% | 6% | 28% | 0.5% |
| plan-bouncer-judge | 3 | 18 | 6.00 | 1.28 | 33% | 6% | 28% | 0.5% |
| plan-burler-review+fork | 3 | 16 | 5.33 | 0.81 | 81% | 0% | 0% | 1.1% |
| conflict | 1 | 13 | 13.00 | 1.15 | 69% | 8% | 8% | 0.3% |
| discussion-bouncer-seed | 1 | 7 | 7.00 | 1.00 | 57% | 0% | 14% | 0.1% |
| plan-bouncer-seed | 1 | 6 | 6.00 | 0.67 | 67% | 0% | 0% | 0.1% |

- All sessions (n = 213): 2934 turns, of which 14% have no call, 72% one call, 7% two and 7% three or more.
  Main sessions (n = 93) average 23.0 turns and 1.21 calls per turn; forks (n = 120) average 6.6 turns and 1.04.
- The writers that spend the most turns also have the highest single-call share: the darn writer 86% (n = 14), the Plan writer's lens forks 90% (n = 6), the Plan writer 79% (n = 1), the Plan reviewer 80% (n = 3).
- The code reviewers and fixers batch more (54% to 55% single-call, n = 9 and n = 7), and a Discussion fixer more again (46%, n = 3).
- A review lens fork lives about 3 to 5 turns (n = 51 across code, Discussion and Plan review lens forks); it has little left to merge.
- The darn writer's 54 turns per session is near the 58 turns the 2026-10-22 token audit found; the Webster implementer forks average 81 turns here (n = 4) against 62 there.

## 2. Mergeable turns

| Role | Sessions | Turns | Removable turns per session (strict) | Share of session weight (strict) | Removable turns per session (class only) | Share of session weight (class only) | Removable turns as share of turns (strict) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| darn | 14 | 761 | 11.9 | 20% | 15.6 | 25% | 22% |
| webster+fork | 4 | 323 | 12.2 | 15% | 15.8 | 18% | 15% |
| code-burler-fix | 7 | 200 | 4.1 | 13% | 4.7 | 15% | 14% |
| code-burler-review | 9 | 179 | 3.9 | 19% | 5.2 | 21% | 20% |
| plan-burler-review | 3 | 127 | 17.7 | 46% | 23.7 | 56% | 42% |
| code-burler-review+fork | 36 | 115 | 0.1 | 3% | 0.3 | 9% | 3% |
| discussion-burler-review | 3 | 115 | 15.7 | 35% | 19.0 | 39% | 41% |
| plan-burler-fix | 3 | 104 | 9.7 | 25% | 12.3 | 29% | 28% |
| plan | 1 | 101 | 33.0 | 23% | 49.0 | 33% | 33% |
| code-burler-fix+fork | 28 | 91 | 0.3 | 8% | 0.5 | 13% | 10% |
| discussion-burler-fix | 3 | 91 | 6.3 | 18% | 8.7 | 23% | 21% |
| plan+fork | 6 | 73 | 3.8 | 28% | 4.5 | 32% | 32% |
| describe | 9 | 66 | 0.3 | 3% | 2.0 | 20% | 5% |
| code-bouncer-seed | 9 | 64 | 0.4 | 5% | 0.4 | 5% | 6% |
| discussion-burler-review+fork | 12 | 58 | 1.3 | 26% | 1.7 | 31% | 28% |
| code-bouncer-judge | 9 | 54 | 0.0 | 0% | 0.0 | 0% | 0% |
| discussion | 1 | 51 | 23.0 | 42% | 25.0 | 44% | 45% |
| fixer | 1 | 50 | 12.0 | 30% | 21.0 | 43% | 24% |
| plan-burler-fix+fork | 12 | 49 | 0.6 | 14% | 0.9 | 22% | 14% |
| discussion-burler-fix+fork | 12 | 41 | 0.3 | 8% | 0.5 | 12% | 10% |
| tackler | 5 | 40 | 0.6 | 4% | 1.4 | 11% | 8% |
| friction | 4 | 33 | 0.2 | 2% | 0.8 | 6% | 3% |
| discussion+fork | 7 | 27 | 0.9 | 18% | 1.0 | 21% | 22% |
| webster | 1 | 24 | 0.0 | 0% | 0.0 | 0% | 0% |
| arbiter | 2 | 19 | 1.5 | 13% | 4.0 | 36% | 16% |
| discussion-bouncer-judge | 3 | 18 | 0.0 | 0% | 0.0 | 0% | 0% |
| plan-bouncer-judge | 3 | 18 | 0.0 | 0% | 0.0 | 0% | 0% |
| plan-burler-review+fork | 3 | 16 | 0.7 | 11% | 1.7 | 28% | 12% |
| conflict | 1 | 13 | 1.0 | 6% | 2.0 | 12% | 8% |
| discussion-bouncer-seed | 1 | 7 | 1.0 | 10% | 1.0 | 10% | 14% |
| plan-bouncer-seed | 1 | 6 | 0.0 | 0% | 1.0 | 23% | 0% |

- Over the sample (n = 213 sessions), the strict count finds 582 removable turns: 463 in main sessions (5.0 per session, 22% of main-session weight, n = 93) and 119 in forks (1.0 per session, 16% of fork weight, n = 120).
  The class-only count finds 791.
- A removable turn weighs about what an average turn of its role weighs: the removable share of weight is within a few points of the removable share of turns for the darn writer (20% against 22%) and the Webster forks (15% against 15%).
  The sample does not show the removable turns costing more than the rest of a session.
- The saved weight is the weight of the whole turns after each run's first; the real saving is smaller by the merged turn's extra output and results.
- Roles where the strict share is at least a quarter of the session weight: the Plan reviewer (46%, n = 3), the Discussion writer (42%, n = 1), the Discussion reviewer (35%, n = 3), the landing fixer (30%, n = 1), the Plan writer's lens forks (28%, n = 6), the Plan fixer (25%, n = 3).
- Roles with under 1.5 removable turns per session: the bouncer judge and seed, the arbiter, the tackler, the friction writer, the describe writer, and the code review and fix lens forks (n = 36 and n = 28).
  The Discussion review lens forks carry 26% of their weight in 1.3 turns per session (n = 12); their sessions are about 5 turns long.

## 3. Read, then edit, then test

Per-edit-session columns divide by the sessions with at least one edit call (the Edit tool, Write tool or an `edit-script` call).
"Reads after the first edit" counts `read`, `search`, `git-read`, `lym-read` and `quarry` calls in turns after the first edit turn.
"Switches" counts changes between a read call and an edit call in call order.
"Re-edits" counts Edit-tool calls on a path the session edited before.

| Role | Edit sessions | Edit calls | Edit turns | Edit calls per edit turn | Reads after first edit | Switches | Re-edits | Gate calls per session | Test and lint calls per session | Validate calls per session | Weight after first edit |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| darn | 12 | 173 | 144 | 1.20 | 12.5 | 11.8 | 2.4 | 2.71 | 2.29 | 2.71 | 69% |
| webster+fork | 4 | 172 | 108 | 1.59 | 33.8 | 25.0 | 22.8 | 11.50 | 5.00 | 0.00 | 94% |
| code-burler-fix | 7 | 76 | 37 | 2.05 | 6.3 | 6.4 | 3.1 | 3.57 | 1.00 | 0.00 | 77% |
| code-burler-review | 9 | 41 | 18 | 2.28 | 0.4 | 1.6 | 0.0 | 0.00 | 0.33 | 0.00 | 29% |
| plan-burler-review | 3 | 10 | 7 | 1.43 | 1.3 | 3.3 | 1.0 | 0.00 | 0.00 | 1.00 | 18% |
| discussion-burler-review | 3 | 20 | 9 | 2.22 | 2.3 | 3.3 | 1.3 | 0.00 | 0.00 | 0.00 | 31% |
| plan-burler-fix | 3 | 34 | 24 | 1.42 | 6.7 | 8.3 | 3.7 | 0.67 | 0.00 | 1.00 | 67% |
| plan | 1 | 29 | 19 | 1.53 | 8.0 | 11.0 | 8.0 | 3.00 | 0.00 | 3.00 | 41% |
| discussion-burler-fix | 3 | 107 | 30 | 3.57 | 4.7 | 5.7 | 29.7 | 0.00 | 0.00 | 0.00 | 73% |
| discussion | 1 | 5 | 3 | 1.67 | 1.0 | 2.0 | 2.0 | 0.00 | 0.00 | 1.00 | 35% |
| fixer | 1 | 6 | 5 | 1.20 | 21.0 | 6.0 | 2.0 | 4.00 | 2.00 | 0.00 | 49% |
| conflict | 1 | 5 | 5 | 1.00 | 4.0 | 5.0 | 0.0 | 1.00 | 0.00 | 0.00 | 49% |

Roles with a review fork's three turns, or whose edits are a single report file (the bouncer seed and judge, the tackler, the friction writer), are left out; they have no read-edit-test pattern to improve.

- Reads do not stop at the first edit.
  The darn writer reads 12.5 times after its first edit (n = 12 edit sessions) and switches between reading and editing 11.8 times; the Webster implementer forks read 33.8 times and switch 25.0 times (n = 4).
  The darn writer spends 69% of its weight after its first edit, the Webster forks 94%, so the late reads are paid at the full context.
- Edits are nearly one per turn.
  The darn writer makes 1.20 edit calls per edit turn (n = 12), the Webster forks 1.59 (n = 4), the Plan writer 1.53 (n = 1); only the review fixers batch (2.05 and 3.57 per edit turn, n = 7 and n = 3), and a code reviewer writing its review file (2.28, n = 9).
- Edit turns that directly follow a single-call edit turn, an upper bound on edits that could share a turn: 3.1 per darn session (7% of weight, n = 14), 4.3 per Webster fork (6%, n = 4), 6.0 for the Plan writer (7%, n = 1).
- The Webster forks edit the same file again 22.8 times per session (n = 4) and the Discussion fixers 29.7 times (n = 3), the latter on the decision record they correct round after round.
- Gates, tests and lints: the darn writer runs 2.71 gate, 2.29 test or lint and 2.71 validate calls per session (n = 14); the Webster forks 11.50 gate and 5.00 test or lint calls (n = 4).
  Consecutive single-call turns that both run a check: 2.07 per darn session, 5.00 per Webster fork, 2.00 for the Plan writer.
  They are not counted as mergeable: a repeat follows an edit, or the gate rule's "no slot free yet" rerun that must be its own turn.

## 4. Kinds of serial calls

The calls of the 582 removable turns (strict), by class and by command.
The command is the tool name, or for Bash the first word of the first statement after any leading `cd` or variable assignment, with the second word for `lym`, `git` and `go`.

| Class | Removable turns | Share of turns | Share of weight |
| --- | --- | --- | --- |
| search | 388 | 67% | 70% |
| read | 124 | 21% | 20% |
| git-read | 38 | 7% | 5% |
| lym-read | 21 | 4% | 3% |
| quarry | 9 | 2% | 3% |
| skill | 2 | 0% | 0% |

| Command | Removable turns | Share |
| --- | --- | --- |
| `grep` | 265 | 46% |
| `sed` (`sed -n`) | 149 | 26% |
| `cat` | 65 | 11% |
| `lym` (`board`, `loom`, `quarry`) | 26 | 4% |
| `ls` | 22 | 4% |
| `git` | 13 | 2% |
| `echo` | 11 | 2% |
| Read tool | 9 | 2% |

- Serial calls are shell `grep` and `sed -n` almost throughout: 71% of the removable turns start with one of them, and the Read tool is 2%.
  A turn often already chains several statements with `;` (a `grep` then a `sed -n` in one call), so the agents merge inside a call but not across turns.
- `lym quarry` makes 9 removable turns (class `quarry`); `lym loom validate-*` makes none, since validate is outside the mergeable set; `lym board get|list` and `lym loom peers|show` make 21 (class `lym-read`).
- The roles with the most `grep` turns: the darn writer 70 (n = 14), the Webster forks 27 (n = 4), the Plan reviewer 27 (n = 3), the Discussion reviewer 21 (n = 3), the code fixer 14 (n = 7).
  Rows per role: `merge-kinds.tsv`.

## 5. Recommendations

These are recommendations, not decisions.
The transcripts show how many turns were serial, not whether a stencil line removes them; no run measured that effect and none could here.
Each estimate is the strict removable turns per session from section 2 plus, where it applies, the single-call edit pairs from section 3, as a ceiling; the "if half" column halves it, on the assumption that agents merge about half of the eligible runs.
The weight is the share of the role's session weight.

| Role | n | Wording and place | Turns per session, ceiling and if half | Weight, ceiling and if half |
| --- | --- | --- | --- | --- |
| darn | 14 | `darn-template-write.md`, Step 4: "Read every file the change touches, with the greps that find them, in one turn before the first edit; make independent edits in one turn; run `lym gate test` once, after the last edit." | 11.9 + 3.1 of 54.4; 7.5 | 20% + 7%; 13.5% |
| webster+fork | 4 | `webster-body-implementer.md`: "Before your first edit, read every file your cards name in one turn. Put all edits that do not depend on each other in one turn. Run the gate once after the last edit." | 12.2 + 4.3 of 80.8; 8.3 | 15% + 6%; 10.5% |
| plan | 1 | `loom-template-plan.md`: "Issue every search and read you can name in one message; wait for a result only to choose the next call." | 33.0 + 6.0 of 101; 19.5 | 23% + 7%; 15% |
| plan+fork | 6 | The twill lens directive (`twill-directive.md`): the same sentence. | 3.8 of 12.2; 1.9 | 28%; 14% |
| plan-burler-review, discussion-burler-review | 3 + 3 | `burler-step-1-explore.md` (step 1 reads the focus): "Read the files of your focus in one turn, then check them." | 17.7 of 42.3 and 15.7 of 38.3; 8.9 and 7.9 | 46% and 35%; 23% and 17.5% |
| plan-burler-fix, discussion-burler-fix | 3 + 3 | `burler-template-fixer.md`: "Read the finding's files together; apply the corrections to one file in one turn." | 9.7 + 1.7 of 34.7 and 6.3 of 30.3; 5.7 and 3.2 | 25% + 6% and 18%; 15.5% and 9% |
| discussion | 1 | `loom-template-discussion.md`: the same sentence as the Plan writer's. | 23.0 of 51; 11.5 | 42%; 21% |
| code-burler-review, code-burler-fix | 9 + 7 | The same sentences as the other burlers. | 3.9 of 19.9 and 4.1 + 0.3 of 28.6; 2.0 and 2.2 | 19% and 13% + 1%; 9.5% and 7% |
| landing fixer (`fixer`) | 1 | `landing-template-fixer.md`: the Plan writer's sentence. | 12.0 of 50; 6.0 | 30%; 15% |
| all roles | 213 | One sentence in the system prompt shuttle appends with `--append-system-prompt`: "Request every call whose arguments you already know in one message, and read what you need before you edit; wait for a result only to choose the next call." | see the rows above; forks of review lenses and the bouncer, arbiter, tackler, friction and describe sessions have under 1.5 | the same |

- The largest single cut is in the roles that explore before writing: the Plan and Discussion perches' reviewers, the Plan and Discussion writers, the darn writer and the Webster forks.
  The sentence for the Plan and Discussion roles rests on `n = 1` to `n = 3` sessions each and should be read as a hypothesis for the next loom runs.
- Weighted over the sample (n = 213 sessions, weight 53.8 million), the class-only upper bound of the removable turns is 20% of the weight in forks and 27% in main sessions; the strict ceiling is 16% and 22%.
  Halving the strict figure gives 8% for forks and 11% for main sessions.
  The weight in the sample is dominated by the darn writer (24.1%), the Webster forks (15.3%), the Plan reviewer (6.0%) and the code reviewers (5.9%).
- Two cuts are not mergeable gathers: the Webster forks and the Discussion fixers re-edit one file 22.8 and 29.7 times per session.
  The Edit tool applies several edits to a file when it receives them in one message; the one-line wording above ("independent edits in one turn") covers it.
- No recommendation for the code review and fix lens forks, the bouncer, the arbiter, the tackler, the friction writer or the describe writer: their sessions are 3 to 8 turns and the strict figure is under 1.5 turns.
  The Discussion and Plan review lens forks are 1.3 and 0.7 removable turns per session (n = 12 and n = 3), too few to word a line for.

## Limits

- The sample is the transcripts present on this machine: 10 runs of which one is a loom run, started within two days; it is not the population of landed runs.
- Role rows with `n` under 5 are indicative; the loom-only roles have `n` of 1 to 3.
- The darn runs in the sample include research and bench-data runs, which read more than a code change would.
- The dependency test is a text match; its error runs both ways (section 1's method note).
- Weight is relative: it does not price the models, and the fork rows start from the inherited context of the parent.
- The mergeable figures assume the same work could be done with the same results; a merged turn delivers its results together, which can lengthen a later turn's context slightly.
