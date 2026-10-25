# Codex as a Burler reviewer: bench against Claude reviews

Bench of 2026-10-25, a precursor to note `codex-engine`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `plus`), Linux.
Nothing was built: the bench is shell scripts under `data/harness/`, Codex's own output and a hand grading.

## Conclusion

On this sample Codex does not reach the Claude reviewers' coverage,
and the bench stopped early on the plan's 5-hour limit.
Measured at medium effort, one run per cell, solo against Claude's lens fan:

- `gpt-6.1-sol` is the only setting worth a second look.
  It reproduced 22 of the 134 Claude findings (16 percent) and 7 of the 21 `BLOCKING` ones, returned the verdict Claude gave on 9 of 10 inputs, and added 11 findings Claude's round did not raise that held up on reading, 4 of them against the code.
  It took about 4 points of the 5-hour window per input, 10 on the largest plan.
- `gpt-6-luna` is fast (28 s a run) and cheap (about 1 point of the window for 10 runs) but reviews too shallowly: 1 of 21 `BLOCKING` findings, and an approval of 3 of the 4 plans Claude blocked.
- `gpt-6-astra` matched Sol's coverage on the five inputs it finished, at about 4 times Sol's plan cost,
  and the limit stopped it before the plans.
- 3 of the 68 Codex findings were graded false, all of them Luna's,
  and 1 of Luna's stays unchecked.
  Sol's 35 and Astra's 15 held up on reading.
  One grader read each finding,
  so these rates are not validated.

A Codex seat could earn a place as an extra, non-gating reviewer beside the Claude fan at `gpt-6.1-sol`, where it adds findings the fan misses.
As a replacement for the Claude fan the sample says no.
Effort above medium, repeat runs for noise and Astra on the plans stay unmeasured.
The table below says which cells ran.
The operator's `~/.codex/config.toml` stayed unchanged through the bench only as far as its file time shows, not as hashed; see `Codex runs`.

## Summary table

Medium effort, one run per cell.
Claude's reference over the same 10 inputs is 134 findings, 21 of them `BLOCKING`, from `opus` at medium effort with 3 or 4 lens forks.
"Found" counts distinct Claude findings that a Codex finding of the setting matched.
The reference figures, and every "found" and `BLOCKING` count, follow from `data/claude-findings.tsv` and `data/claude-usage.tsv`.

| Setting | Inputs run | Findings | Shared with Claude | Codex-only real | Codex-only unchecked | False | Claude findings found | `BLOCKING` found | Verdict as Claude's | Fresh input tokens | Wall time | 5-hour window |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-luna` medium | 10 of 10 | 18 | 12 | 2 | 1 | 3 | 12 of 134 | 1 of 21 | 6 of 10 | 400k | 284 s | about 1 point |
| `gpt-6.1-sol` medium | 10 of 10 | 35 | 24 | 11 | 0 | 0 | 22 of 134 | 7 of 21 | 9 of 10 | 862k | 1936 s | about 42 points |
| `gpt-6-astra` medium | 5 of 10 | 15 | 10 | 5 | 0 | 0 | 10 of 73 | 2 of 7 | 3 of 5 | 273k | 628 s | about 53 points |
| `gpt-6-luna` low | 1 of 10 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | no review written | 39k | 18 s | under 1 point |
| Claude `opus` medium, fan | 10 of 10 | 134 | | | | | | | | 236k to 489k, plus 21k to 92k in forks | 383 s to 735 s | |

The three medium settings on the five inputs Astra finished, `D1` to `D5`: Luna found 8 of 73 Claude findings, Sol 10 of 73 and Astra 10 of 73.
Sol's cost on those five inputs was about 14 points of the window, Astra's about 53.

Fresh input tokens are `input_tokens` less `cached_input_tokens` from each run's `turn.completed` event.
Cached reads are 73 to 95 percent of input in every run and are in `data/usage.txt`.
The window percentages are the difference of the integer `used_percent` the rollout files report, so a single run is known to about one point.
`data/usage.txt` carries each run's reading, the last `token_count` event of its rollout,
and the per-run points in `Billing and rate limits` are the differences between consecutive readings.
Verdict agreement counts a Codex `APPROVED` or `BLOCKING` against Claude's round-1 verdict.
Claude approved only `D3` (15 findings, none `BLOCKING`),
and every Codex setting blocked it.

## Method

### Inputs

Ten finished loom runs from the history of `loomyard-weft`, each with the round-1 review of Claude's Discussion-Review or Plan-Review perch.
The artifact reviewed is the decision record and support log, or the plan, from the weft tree at the commit that wrote the round-1 review.

| Id | Kind | Run | Weft commit | Code commit | Claude verdict | Claude findings |
|---|---|---|---|---|---|---|
| `D1` | discussion | `plan-card-rules` | `c3afefc21` | `e634efb90` | `BLOCKING` | 17 |
| `D2` | discussion | `gate-runs-once` | `868ea6dbb` | `7e4a041b9` | `BLOCKING` | 14 |
| `D3` | discussion | `dotnet-scoped-verify` | `988770777` | `fed963ec5` | `APPROVED` | 15 |
| `D4` | discussion | `batten-cancel-verb` | `27038d064` | `e634efb90` | `BLOCKING` | 15 |
| `D5` | discussion | `finalize-dirty-names-paths` | `5216fd218` | `0d953f140` | `BLOCKING` | 12 |
| `D6` | discussion | `run-process-ownership` | `13d617b64` | `0d953f140` | `BLOCKING` | 16 |
| `P1` | plan | `gate-runs-once` | `c36025a89` | `ab038b4e8` | `BLOCKING` | 12 |
| `P2` | plan | `quarry-csharp` | `394715f7f` | `750903aef` | `BLOCKING` | 13 |
| `P3` | plan | `orch-strand-cycle` | `cd1719863` | `c8785677a` | `BLOCKING` | 9 |
| `P4` | plan | `machine-handover` | `045707759` | `750903aef` | `BLOCKING` | 11 |

The runs are the recent ones with a recoverable tree, picked to span verdicts and finding counts.
`data/inputs.txt` carries the full commits and each round's recorded start time.
The code commit is the last commit of `main` before that start time, the nearest recoverable stand-in for what the Claude reviewers read.
Each snapshot is a `git archive` extracted under `.scratch/`, with no `.git`.

### Prompt

The prompt is the burler reviewer chain over that snapshot: `burler-template-reviewer.md`, its two instruction files with the explore step, the review-file format, the Discussion or Plan rubric of the same commit with amendment `none`, the round's focus directive where the Claude round had one, and the solo cluster rule.
The target and fasit text are the ones `contracts/recipes/loom-recipe.yaml` gives the two segments.
The hub-style `specs` and `stencils` directories the rubrics point at are copied from the snapshot's own `contracts/`.
Left out: the inlined PATTERN overview, the parent and edit directives, and prior rounds (round 1 has none).
Codex's work directory held the snapshot, the artifact and none of Claude's review files.
The sandbox does not confine reads, as `Codex runs` says.

### Codex runs

```text
CODEX_HOME=<scratch>/codex-home codex exec -m <model> -c model_reasoning_effort="<effort>" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

Each run got a fresh copy of the work directory.
The review file is read from the copy afterwards.
`CODEX_HOME` sits under the worktree's `.scratch/` and holds a symlink to `~/.codex/auth.json`, made by hand before the bench; `data/harness/setup-codex-home.sh` reproduces it.
The link is writable through to the operator's file.
The target's mtime, 2026-10-24 11:16, predates the link, so no token refresh wrote through it.
Codex wrote one trust entry into that scratch `config.toml`,
and the bench's sessions are under the scratch directory only.

The acceptance criterion that the operator's `~/.codex/config.toml` is byte-identical before and after is met only as far as the file time shows, not as hashed.
No hash was taken before the bench.
A read after the bench gave sha256 `3854c50a…` and an mtime of 2026-10-24 16:52, before the bench started.
A second read at 12:48 on 2026-10-25 found the file rewritten, 675 bytes with a new hash, 14 minutes after the last bench run.
An interactive operator `codex` session, started 2026-10-24 11:49 and still running, was writing into `~/.codex` at that minute,
and no bench process was alive.
The directory `~/.codex` showed an mtime of 12:15 on 2026-10-25 at the first read, inside the bench window.
No entry of the directory carries that time, so the change stays unexplained.
The live operator session is the likely writer,
but nothing recorded ties it to 12:15.
`data/operator-codex-home.txt` holds both hashes, the entry names with their mtimes and the `auth.json` facts.

`-s workspace-write` confines writes to the work directory.
Reads are not confined.
Input blinding rests on the copied work directory holding none of Claude's review files, not on the sandbox, since `build.sh` reads the artifacts from the weft checkout and a run could have opened that checkout.
Whether any run did was checked on the scratch `events.jsonl` of all 31 runs, which hold every command and its output:

```text
grep -l 'loomyard-weft\|bench/inputs' .scratch/bench/runs/*/*/events.jsonl
```

The command matches no file.
One run read outside its work directory beyond the Go module cache: Astra on `D1` ran `rg` for `EncloseAt` over `/home/knatte/Code` with `-g enclose.go`, a search that spans the weft checkout but returns only files named `enclose.go`,
and its output lists only `quarry` paths.
`.scratch/` is not committed, so a reader of the repository cannot rerun the check.

### Grading

One Claude session graded all findings, by reading the Codex finding against the Claude findings of the same round and, for a Codex finding with no match, against the cited lines of the artifact and sometimes the snapshot's code.
Labels:

- `shared`: the Codex finding names the same defect as a Claude finding, or the same decision's same flaw from another side.
  A `shared` finding may be sharper or weaker than its Claude match.
  Luna's `D4` F1 reads a lock-name ambiguity Claude filed as a `LOW` consistency finding as a deadlock.
- `codex-only real`: the claim holds against the artifact text, or against code where it cites code, and no Claude finding of the round covers it.
- `codex-only unchecked`: not gradable.
  The row's `note` says why.
- `false`: the claim does not hold.

Every label is in `data/match.tsv`, one row per Codex finding, with the Claude finding it matched, that finding's severity and a note.
Each `codex-only` row was read against the cited artifact lines and, where the claim concerns code, the snapshot's code.
The fixer report and bouncer verdict of the rounds holding `codex-only` rows (`D1`, `D2`, `D3`, `D4`, `D6`, `P1`, `P3`, `P4`) were searched by keyword for each finding's topic, not read line by line.
None touches a `codex-only` finding except that `D2`'s fixer report fixed Luna's retention gap as a side effect of a Claude finding,
so that evidence confirms or refutes none of the labels.
The grading did not test whether the Claude findings themselves are valid.
"Claude findings found" is therefore a recall against Claude's list, not against the truth: Claude's `LOW` and `NIT` findings are in the 134 and most of the ones no Codex run reproduced.

## What Codex found that Claude's round did not

The `codex-only real` findings of Sol, with the Astra and Luna duplicates, are:

- `D1`: a whole-second write time cannot order an amendment after a settle in the same second (Sol F2).
- `D1`: a wrongly labelled `Extends` can merge cleanly and break the code silently, against the record's "never silently" bound (Sol F3, Astra F2).
- `D2`: exact args matching makes a repeated `-- -count=1` request carry the earlier green instead of rerunning (Sol F2, Astra F3).
- `D3`: a PATTERN background line is narrowed while the record says no PATTERN entry changes (Sol F3); arguable and minor.
- `D4`: `lyx tender reap` cannot reach the older-binary processes the open risk leaves to it, checked against `internal/tenderengine/processes.go` (Sol F4, Astra F3).
- `D4`: the landed-work refusal is read once, before the cancel waits for the write locks, and is not rechecked before the end (Sol F1).
- `P1`: card 20 removes the `Command` switch key while `internal/shedbuild/build_engines_test.go` still sets it,
  and no card migrates the fixture, checked against the snapshot (Sol F4).
- `P1`: card 24's `awk` Verify line tests only lines that start with a command key, so a tier with no such line passes (Sol F2).
- `P1`: the retag leaves `cmd/lyx/treesitterlink_integration_test.go` listing under `integration,tmux,llm`, checked against the snapshot,
  and no card mentions the guard (Sol F3).
- `P3`: card 6 exempts a dead holder with children, which the decision record settles nowhere.
  The card states the exemption, so Sol's `BLOCKING` overstates it (Sol F1).
- `P3`: no card edits `internal/loomcli/doc.go` or `internal/shuttleengine/claudeengine/doc.go`, though cards 4 and 3 change behavior in those packages, checked against the snapshot's two files (Sol F3).

Astra's `codex-only real` findings beyond the duplicates above are two `D1` claims:
D6 tells the plan writer to write a card's docs under `Extends` while the rubric flags an `Extends` on a ref the card rewrites (F3),
and D9 places a path that exists at the root before it tries the package directory, so a bare name at the root wins (F4).

Luna's two `codex-only real` findings are a support-log inconsistency in `D4`, where Turn 3 picks refusal on an unreachable origin while D2 proceeds, and the undefined "recent" in `D2`'s green-record pruning, which the round's fixer report then fixed as a side effect of a Claude finding.

The three findings graded `false` are Luna's: `D2` F2 (`--window` already bounded to `batten.open_window`), `D6` F3 (the SIGTERM fact is stated and holds at the code commit) and `P4` F1 (the plan already confines `Takeover` to `lyx adopt`).
Luna's `D4` F3 stays unchecked: it bundles four premises and asks for research rather than naming a defect.

## What Codex missed

Of the 21 Claude `BLOCKING` findings Sol found 7, Luna 1 and Astra 2 of the 7 on `D1` to `D5`.
Sol missed all four `BLOCKING` findings of `D4` and all three of `P4`: code-reading defects such as `RemovePair` taking a snapshot only when two flags are both set, `runproc.End` killing a step mid-merge before the run-lock wait, and an owner guard that reads origin's Bolt.
Codex's findings are mostly about the artifact's own text and a few cited functions.
Claude's lens forks read more code and raise more claims about what existing code does.
The three Luna approvals of `P1`, `P2` and `P3` read the plan and its decision record and ended with `APPROVED` and no findings in 28 to 39 s.

## `codex exec` itself

- A headless run with the ChatGPT login works: `codex exec` read the prompt from stdin, ran its shell commands in the `workspace-write` sandbox and wrote the review file.
  `exec` takes no approval flag.
  None was needed.
- `--json` prints one JSON event per line.
  The closing `turn.completed` event carries `input_tokens`, `cached_input_tokens`, `output_tokens` and `reasoning_output_tokens`.
  `-o` writes only the final message.
- Each run also writes a rollout under `CODEX_HOME/sessions/`.
  Its `token_count` events carry `rate_limits` with the `used_percent` and window length of both windows.
- A scratch `CODEX_HOME` with only a linked `auth.json` was enough.
  No trust prompt blocked `exec`.
- Wall time per run: Luna 18 to 39 s, Sol 103 to 491 s, Astra 93 to 156 s on the runs that finished.
  The largest plan (`P1`, 33 cards) took Sol 491 s and 2.9M input tokens.
- `gpt-6-luna` at `low` read the instruction files and the artifact, then ended its turn with an empty final message and no review file.
  The bench has no retry for that and counts it as no review.
- The model cache lists efforts `low` to `ultra` for the Sol and Astra slugs and `low` to `max` for Luna.
  Only `low` and `medium` were used.

## Billing and rate limits

The plan reports two windows in every rollout: 300 minutes and 10080 minutes.
Across the bench the 5-hour window went from 0 to 96 percent in about 50 minutes of runs and the weekly window from 0 to 15 percent.

- Luna: about 1 point of the 5-hour window for 10 runs.
- Sol: 1 to 43 points over 10 runs, 2 to 5 points each except `P1` (10) and `P4` (7).
- Astra: 43 to 96 points over 5 runs, 5 to 17 points each, about 4 times Sol's cost for the same discussion inputs.

The sixth Astra run (`D6`) was cut off by the limit after 18 s,
and the four Astra plan runs were refused within 2 s with "You've hit your usage limit ... try again at 4:45 PM".
Those five cells are unmeasured, as is everything at effort `high`, `xhigh`, `max` and `ultra`, and every repeat.

The decision record set a stop at 70 percent used on either window.
The grid script did not enforce it during the bench: the window read 68 percent after `D3` on Astra and 79 after `D4`,
and the bench stopped only when runs started failing.
A check between runs would have stopped the grid after Astra's `D4`, before the 17 points `D5` cost.
The committed `data/harness/grid.sh` now reads both windows after each run, writes them into the cell's `result.txt` and exits once either shows 70 percent used.
It also skips only cells that finished with `rc=0`,
so the refused Astra cells run again on a rerun.
The stop is checked after a run, never before the first, and a refused run writes no reading, so a rerun started while a window is still at 70 or more spends its first cell before the grid can stop, and refused cells loop on without stopping it.
The script was syntax-checked and its window reader run on a bench rollout, but not run as a whole, since a run spends plan limits.

Under the ChatGPT login `codex exec` bills against plan rate limits and shows no per-run price.
A token count and a window percent are the only cost signals.
The 5-hour window of a Plus plan did not hold the full medium-effort grid of three models over ten inputs.
Astra was the part that did not fit.

## Limits of this comparison

- One run per cell: no variance is measured,
  so Sol and Astra, which tie on the five shared inputs, cannot be ranked.
- Solo against a fan: Claude's round used 3 or 4 lens forks on top of a handler, which Codex's solo reviewer does not have.
  A Codex fan would cost several times the window figures above.
- Recall against Claude's list, not against ground truth.
  1 Codex finding is unchecked, and Claude's own false findings were not looked for.
- The code commit is the main branch at the round's start, not the run's fork point.
- A single grader, the Claude session that drove the bench.
  `data/match.tsv` is the audit trail.
- The prompt omits the PATTERN overview a production reviewer is given, which may lower Codex's findings on PATTERN rules.
- Ten inputs from one repository over two weeks.
  The plan inputs are 4.

## Data

`data/` holds the following:

- `inputs.txt`: the inputs with weft commit, round start time and code commit.
- `usage.txt`: one line per run with return code, wall seconds, tokens, verdict, finding count and the 5-hour and weekly `used_percent` after the run, read from the run's rollout.
  The lines of runs the plan refused carry no percentages.
- `match.tsv`: the grading of every Codex finding, with the matched Claude finding, its severity and a note.
- `claude-findings.tsv`: every Claude round-1 finding of the ten inputs with its severity and the Codex settings that matched it.
- `claude-usage.tsv`: the Claude round-1 review's wall seconds and fresh tokens per input, and its lens forks' fresh tokens, from the weft round usage files.
- `operator-codex-home.txt`: the state of the operator's `~/.codex` read after the bench.
- `reviews/<input>/<setting>.md`: every review file Codex wrote.
- `harness/`: the scripts that staged the inputs (`build.sh`), ran the grid (`grid.sh`, `run.sh`, `windows.sh`), listed findings (`dump.sh`) and produced `usage.txt` (`usage.sh`) and the Claude reference files (`claude-reference.sh`), plus `setup-codex-home.sh`, which creates the scratch `CODEX_HOME` and links `auth.json`.
  The original `usage.txt` was built by hand from the same files, and `usage.sh`, `claude-reference.sh` and `setup-codex-home.sh` were written afterwards and reproduce the committed old columns exactly.
  The scripts work on `.scratch/bench/` and `.scratch/codex-home/`, which are not committed.
  `build.sh` and `run.sh` take their paths from the worktree this bench ran in.
