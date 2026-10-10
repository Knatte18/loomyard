# Haiku 5.5 against Haiku 4.5, Sonnet 5.5 and Opus 5.5

Measured 2026-10-08.
One task, one run per configuration, so every number is a single sample.
The prompts and every run's output are in [haiku-5-5/](haiku-5-5/); the output directories were `.scratch/haikubench/out-<run>/` when the runs wrote them.

## Task

Every run got the same prompt ([haiku-5-5/prompt-a.md](haiku-5-5/prompt-a.md) … `prompt-f.md`; only the output directory differs).
The task: write `max_satisfying(versions, range_expr) -> str | None`, a semver range resolver in Python stdlib only.
It covers semver 2.0 ordering, caret, tilde, x-ranges, hyphen ranges, `||`, partial versions with operators, and the npm prerelease rule.
Each agent also wrote its own tests and a `report.md`, and clocked itself with `date +%s.%N` as its first and last tool call.

## Runs

Each run was an interactive session (not headless) that could not see the other runs' output.

| Run | Model | Effort | Prompt | Output dir |
|---|---|---|---|---|
| A | Haiku 4.5 | none (model has no effort levels) | `prompt-a.md` | `out-a` |
| B | Haiku 5.5 | medium (default) | `prompt-b.md` | `out-b` |
| C | Sonnet 5.5 | medium | `prompt-c.md` | `out-c` |
| D | Opus 5.5 | medium | `prompt-d.md` | `out-d` |
| E | Haiku 5.5 | high | `prompt-e.md` | `out-e` |
| F | Haiku 5.5 | xhigh | `prompt-f.md` | `out-f` |

Haiku 5.5 is the first Haiku with effort levels (low, medium, high, xhigh, max) and a 1M context window.
Run F loaded three code-style skills the operator wrote; none of the other runs did, so F is confounded.

## Time

"From prompt to stop" is wall time from submitting the prompt to the last tool call.

| Run | From prompt to stop | Note |
|---|---|---|
| A | 455 s | |
| B | 179 s | |
| C | 81 s | |
| D | 99 s | |
| E | 200 s | self-timed 86 s; about 115 s of thinking happened before the first tool call |
| F | 364 s | |

Approximate output speed: Haiku 5.5 ≈ 250 tok/s, Sonnet 5.5 ≈ 150, Opus 5.5 ≈ 120, Haiku 4.5 ≈ 107.
The Haiku 5.5 runs generated about four times as many tokens as Sonnet and Opus, so they finished later despite faster generation.
The claim that Haiku 5.5 is faster than Sonnet and Opus did not hold for this task.

## Tokens and cost

Counted from the transcripts, deduplicated by `message.id` keeping the largest `output_tokens`.
All cache writes were 1-hour.
Uncached input was negligible (tens of tokens; 426 for A).
Context per call is input + cache read + cache write; "largest" is the maximum over the run.

| Run | Calls | Cache write | Cache read | Output | Largest context | Cost |
|---|---|---|---|---|---|---|
| A | 53 | 90,385 | 3,239,668 | 48,472 | 90,393 | $0.75 |
| B | 8 | 74,094 | 354,926 | 44,569 | 74,096 | $0.04 |
| C | 5 | 32,925 | 123,542 | 12,107 | 41,157 | $0.27 |
| D | 8 | 31,987 | 208,637 | 11,793 | 37,751 | $0.53 |
| E | 12 | 82,783 | 677,088 | 48,647 | 82,785 | $0.05 |
| F | 11 | 123,396 | 799,136 | 94,897 | 132,331 | $0.19 |

Prices per MTok (input / 1h cache write / cache hit / output), from `platform.claude.com/docs/en/about-claude/pricing`:

| Model | Input | Cache write 1h | Cache hit | Output |
|---|---|---|---|---|
| Haiku 4.5 | 1 | 2 | 0.10 | 5 |
| Haiku 5.5, prompt up to 100k | 0.10 | 0.20 | 0.01 | 0.50 |
| Haiku 5.5, prompt over 100k | 0.50 | 1.00 | 0.05 | 2.50 |
| Sonnet 5.5 | 2 | 4 | 0.10 | 10 |
| Opus 5.5 | 4 | 8 | 0.20 | 20 |

The over-100k tier applies to the whole prompt of the call that crosses it.
F's cost is tier-aware; it would be about $0.08 had F stayed under 100k.
No time-limited promotion was found on the pricing page.

Why A was the most expensive Haiku: 53 calls, each re-reading a growing cached context (3.2M cache-read tokens in total) and rewriting large parts of the file.
Why the Haiku 5.5 runs have larger contexts than Sonnet and Opus: they wrote the whole solution in few, large turns, and those turns (thinking plus file content) stay in the context.
Thinking content is empty in the transcripts, so what was thought is inferred from context-size jumps and timing.

## Accuracy

Two checks, both hidden from the agents:

1. A grader with 66 cases (`python3 -I grader.py <dir>`).
2. A 30,000-case differential fuzz across all six modules, scoring deviations from the majority answer.

| Run | Grader | Fuzz deviations |
|---|---|---|
| A | 61/66 | 3,244 |
| B | 66/66 | 0 |
| C | 66/66 | 0 |
| D | 66/66 | 0 |
| E | 66/66 | 0 |
| F | 66/66 | 2 |

A had three bugs.
F's two deviations: `*` in a set that also contains a prerelease comparator, and a `0.0.0` prerelease; F treats `*` like `>=0.0.0`.

## Code volume

| Run | Lines | Code lines | Docstring lines | Chars | Longest function | Own tests | Report chars |
|---|---|---|---|---|---|---|---|
| A | 466 | 375 | 9 | 17,423 | 244 lines | 46 | 5.4k |
| B | 226 | 169 | 12 | 7,331 | | 32 | 6.2k |
| C | 185 | 142 | 8 | 5,763 | | 25 | 2.3k |
| D | 222 | 164 | 11 | 6,717 | | 46 | 2.4k |
| E | 204 | 153 | 3 | 7,073 | | 45 | 8.3k |
| F | 345 | 205 | 67 | 11,238 | | 55 | 10.6k |

C was the most compact; A was the least, with one 244-line function.
F's extra volume is mostly docstrings (67 lines), consistent with its loaded skills.

## Why A iterated so much

A ran 53 calls with repeated test-fix cycles; B, E and F wrote almost everything in one turn after long thinking.
A's pattern is write, run, fail, patch, which is cheap per call but expensive in total because every call re-reads the whole cached context.

## Caveats

- One easy, fully specified task; it does not measure code review, long loops or ambiguous requirements.
- One run per configuration; no variance estimate.
- F is confounded by the skills it loaded.
- Whether thinking blocks persist across turns in the context is inferred, not observed.
- E's self-timed duration hides the pre-start thinking; use "from prompt to stop".
- Low effort was not run, and max was not run.

## Scripts

`grader.py`, `fuzz.py`, `usage.py` and `trace.py` were kept in the measuring session's scratchpad, out of the agents' sight, and are lost, so the grader and fuzz figures cannot be rerun.

## Loomyard roles

The measuring session's reading of which Loomyard roles suit Haiku 5.5, hypotheses from the config and not from this test, which shows only that Haiku 5.5 implements a precise spec well:

- Candidates: the loom `driver` (mechanical, low output, many calls, where the cache price dominates) and the Bouncer `judge`, the latter only after its verdicts are compared against Sonnet's on the same rounds.
- Not candidates: review and fix, Webster implementation (forks inherit the Master's model, and the Master runs above 100k), and the long design sessions (orch, Discussion, Plan).
- Write `haiku[medium]`, not bare `haiku`, since the alias in `models.yaml` has no effort default; medium scored as well as high and xhigh here, at the lowest cost.

The board note `haiku-roles` carries the trial.

## Possible next round

Give Haiku 5.5, Sonnet 5.5 and Opus 5.5 run A's code to review.
The ground truth is known (three bugs), so it measures review ability, which this round did not.
