# Retro of 10 landed runs up to 2026-11-16T11:54:11Z

## The runs and the window

The runs landed after 2026-11-10T06:46:08Z and up to 2026-11-16T11:54:11Z:

- `adopt-worker-record` (tapestry), landed 2026-11-11T07:22:50Z
- `gate-scope-by-coverage-map` (raddle), landed 2026-11-11T08:37:42Z
- `decision-record-replace` (raddle), landed 2026-11-11T15:24:45Z
- `testmap-upkeep-design` (raddle), landed 2026-11-11T16:26:53Z
- `creel-notice-hygiene` (darn), landed 2026-11-11T17:02:52Z
- `plan-writer-context` (darn), landed 2026-11-12T18:08:51Z
- `adopt-redesign` (raddle), landed 2026-11-15T14:24:11Z
- `test-only-functions-in-prod` (darn), landed 2026-11-15T15:40:52Z
- `weaver` (raddle), landed 2026-11-16T09:02:14Z
- `codex-same-as-claude` (raddle), landed 2026-11-16T11:54:11Z

The six raddle runs record no change description at their tag, so they have no landed commit and no cost per changed line.
The figures files sit beside this report:
`figures.md` whole, `tokencount.md`, `forkaudit.md`, and the tables behind `figures.md` (`sessions.jsonl`, `rows.jsonl`, `refusals.jsonl`, `usage.jsonl`, `facts.jsonl`).
All three scripts ran without error.
`tokencount.md` counts no Codex session and attributes $0.00 to the orch.

Turn numbers below are the ordinal of the assistant line in the session's transcript, read with `jq`,
except the refusal turns, which are the ones in `figures.md`.
A session is named by its first eight characters.

## Figures that matter

Cost per run, from `tokencount.md`:

| run | recipe | total | share of the three darn and tapestry runs |
|---|---|---|---|
| adopt-worker-record | tapestry | $40.90 | Plan $20.72, Webster $13.47 |
| test-only-functions-in-prod | darn | $33.04 | Darn $17.87, Code-Review $13.89 |
| creel-notice-hygiene | darn | $9.68 | Darn $6.52, Code-Review $2.85 |

The two largest runs are $73.94 of the $100.34 total.
The six raddle runs cost $1.28 to $3.53 each; their time is 74.8% to 92.5% idle, waiting on the operator.

Producers against their median, from `figures.md`:

| row | median turns | median cost | outlier |
|---|---|---|---|
| Darn-Write | 59 | $6.52 | test-only-functions-in-prod 50336444 (99 turns, $17.87) |
| Code-Burler-Review | 25.5 | $1.63 | test-only-functions-in-prod 6b616569 (36 turns, $5.40) |
| Code-Burler-Fix | 45 | $2.08 | test-only-functions-in-prod 32cc24c5 (62 turns, $4.77) |

Verify gate: `test-only-functions-in-prod` ran it 10 times with 7 reworks, `creel-notice-hygiene` 4 times with 2, `adopt-worker-record` 3 times with 1 and 8 gate-fixer sessions.
Review findings, summed over rounds: code 40 (design 20, consistency 19, decision 1, scope 0), plan 19 (consistency 9, design 8, decision 1, scope 1).
No ledger key was open in more than one round, and no record was missing.

Refused commands: 5 in all, in `decision-record-replace` (3), `test-only-functions-in-prod` (1) and `codex-same-as-claude` (1).

Webster forks: one run, `adopt-worker-record`, four forks, all in the `none` arm.
The fork covering one card (06-adopt-step-order) weighed 2.0M against 0.6M for a fork of three cards.

## Proposals, most severe first

### 1. Stage the reviewer's diff in pieces that fit one Read (medium)

- Route: a check, in how the review is staged.
- Files: `internal/burlerengine/prompt.go` and `contracts/stencils/burler/burler-step-1-explore.md`.
- Moment: `test-only-functions-in-prod`, 6b616569, turns 18 to 35.

The reviewer wrote the branch's production diff to `.scratch/prod.diff` itself (turn 18), about 3,600 lines.
It then read that file in five spans (turns 19, 23, 25, 27, 28), and its own Read of the last span was refused for size,
so turn 28 repeats turn 27 with `allow_large`.
The session cost $5.40 against a row median of $1.63, and the reviewer's own context held the whole diff.
The Bash `git diff --stat` calls of turns 11 to 15 had already named the size.
Burler can write the round's diff as one file per package, each under the Read cap, and name them in the explore step,
so the reviewer reads what it needs and never retries a refused Read.
One session shows this; a second oversized review would confirm it.

### 2. A reviewer rubric line on facts a decision rests on (medium)

- Route: a reviewer rubric line.
- File: `contracts/stencils/loom/loom-rubric-discussion-review.md`.
- Moment: `decision-record-replace`, 91782c57 turn 20, e44fb294 turn 14, 2c77f780 turn 21.

Three separate sessions of one run each built a scratch git repository to learn what `git commit --allow-empty -- <path>` commits when another path is staged.
Two were reviewers and one a fixer, in different rounds.
Each rebuilt the experiment because the record states the behaviour its decision relies on without the observed result.
Proposed line: a decision that rests on how git or another tool behaves states the observed result and where it is pinned,
so the next round reads it and does not repeat the experiment.

### 3. A refusal tells the agent which part of a compound command ran (low)

- Route: a check, in the shell guard's refusal text.
- File: `internal/shellguard` (the refusal message).
- Moment: `test-only-functions-in-prod`, agent-ae723f78 (a fork of 50336444) turn 7; `codex-same-as-claude`, agent-acbe71dc (a fork of a23c382e) turn 3; `decision-record-replace`, 2c77f780 turn 21.

Each refused command opened with a probe or a scratch step (`sed -i.bak` on `/dev/null`, `python3 -V`, `rm -rf $d`) followed by useful read-only work:
the greps of turn 7 and turn 3, a git experiment in turn 21.
The guard refuses the whole command, and the agent's next turn re-runs the parts it still needed.
A refusal that says nothing ran, and names the refused segment, saves the agent working out which part failed.
Whether the current text already says so was not checked; this is a proposal to confirm and, if absent, add.

### 4. The `cat`, `head` and `sed -n` clause has no effect (low)

- Route: a deletion, or a guard row if the operator wants the rule kept.
- Files: `contracts/stencils/edit/edit-directive.md` and `contracts/stencils/engine/claude/edit-directive.md`, line 9 and line 8.
- Moment: `creel-notice-hygiene` c2df8481 turns 10 and 13; `test-only-functions-in-prod` 50336444 turns 10 and 12 and 6b616569 turns 11 and 13; `adopt-worker-record` f6b42ead turns 6 and 12.

Every sampled session of the darn writer, the code reviewer and the plan writer read its first files with `cat`,
and each call joined several reads, such as `cat a; echo ----; cat b`, with other lookups in one Bash call.
None was refused, and none caused a visible fault.
Those calls are the batching the same prompt asks for, so the clause is broken in every sample and costs nothing.
The operator chooses between deleting the clause and a shell guard row that refuses `cat <path>` with a pointer to Read;
the row would turn one batched call into several parallel Reads with the same turn count.

## Questions the runs answered

- Producers that stand out: Darn-Write and the Code-Review pair of `test-only-functions-in-prod`.
  The writer ran 99 turns and spent turns 18 to 30 building and trying `deadcode` for three operating systems (50336444),
  and the reviewer's cost is proposal 1.
- Serial lookups: `figures.md` ranks 50336444 first, 19 serial of 39 lookup turns.
  The serial turns of 18 to 21 are dependent probes (build, retry with other flags, retry per OS), so batching would not have removed them.
  The first reads in c2df8481 and 6b616569 are batched into one Bash call each.
  The cold gate-fixer a3c9eacf read 14 files in one turn (turns 11 to 24).
  No instruction failed to batch in the spans read.
- Refusals: 3 redirects or `sed -i` into a file with a source extension, 1 `rm` of a computed path, 1 `python3` probe.
  All five were agent probes of an experiment or a version; proposals 2 and 3 cover them.
- Verify gate failures: `test-only-functions-in-prod` went to a cold fixer twice (a3c9eacf, 4fecb479), and a3c9eacf read 14 file spans across packages in one turn,
  consistent with a tree-wide guard finding that no writer-side check runs before the gate.
  The gate report itself was not read, so the fault's kind is not shown.
- Recurring review findings: by kind only, design and consistency in equal parts, in code and in plan rounds, with scope at 0 for code.
  No key recurs across rounds.

## Questions the runs gave no answer to

- Which verify gate fault a check could have caught earlier: the gate reports were not read.
- Which prompt sentence the sessions never needed: no span showed a sentence idle other than the clause of proposal 4.
- Where an agent searched for a path, command or format it should have been told: no span showed it.
- How fork cost moves with the pack: all four Webster forks are in the `none` arm, so `forkaudit.md` has nothing to compare.
- Whether the codex sessions behave as the Claude ones: `tokencount.md` counts no Codex session.
