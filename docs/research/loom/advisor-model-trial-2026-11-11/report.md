# Tuning the darn writer's prompt for an Opus advisor, started 2026-11-11

This study should tune a prompt addition that tells a Sonnet darn writer when to consult an Opus advisor and what to ask.
It stopped before the first iteration: lym has no route that sets the advisor for one run.
No iteration ran, so the prompt is not ready to bench (`advisor-model-bench`).

## Earlier result: seats

`docs/research/twill/discussion-bench.md` tried the reverse arrangement on Discussion-Write: a chair with one advisor in a seat beside it.
With one sample per configuration it showed no better draft from an advisor, and an advisor at `high` effort or on Opus roughly doubled the cost.
The chair's own model and effort moved the result more than the advisor did.
A seat runs as a second full session beside the chair; the advisor tool charges the strong model per consult only.
The seats result therefore does not predict the advisor's cost or quality.

## Finding: the advisor's settings key

Claude Code 2.1.322 (`claude --version`) reads an `advisorModel` setting and a `--advisor` flag.
Its binary names both in the message "change or unset the advisorModel setting (or the --advisor flag)".
It also holds the switches `CLAUDE_CODE_ENABLE_EXPERIMENTAL_ADVISOR_TOOL` and `CLAUDE_CODE_DISABLE_ADVISOR_TOOL`.
Not yet confirmed: the setting's value form, that it takes effect in a lym tmux session, and how advisor calls show in the usage records.

## Blocker: no route sets the advisor for one run

- `claudeengine` writes a `--settings` document for every launch (`internal/shuttleengine/claudeengine/settings.go`),
  but it carries hooks and scope only, and `lym shuttle run` has no flag that adds a key to it.
  The task forbids a new lym config key.
- `lym shuttle run` has no `--advisor` flag, and its `--model` spec names an engine, a model and an effort only.
- `.claude/settings.local.json` is rendered by heddle's `claude-settings` target, so it is never hand-edited.
  A heddle entry renders into every live worktree of the hub, and a wrapper set as claude.yaml's `binary` reaches every session of the hub,
  so neither isolates one run.

## Rerun setup, as far as it went

- The recorded run is `archive/reed-tab-and-attach-fixes/d707f865fb9b` in the weft repository:
  recipe `darn`, profile `light`, 170 lines added and 33 deleted over 10 files, one review round, verdict `APPROVED`.
  Its two findings were LOW test gaps: a missing row for the failed-without-`FailedTier` mark, and no test that `attach` and `up` accept a legitimate worktree and that `down` still runs in a refused checkout.
  The Opus bouncer ruled `CONVERGED`.
- A detached checkout of the base commit `0091f39c` (`aeea549d^`) held the rerun's code, outside every fabric pair.
- `lym shuttle run` in that checkout fails with "this hub records no shortname", since the checkout is outside the hub.
  The way forward the orch chose: run the shuttle from the task worktree and point the writer at the checkout by absolute path.
  The limit stays: the writer's cwd, hooks and settings are the task worktree's.
- Each rerun's diff is reviewed by one read-only Opus reviewer through shuttle against the Code-Review rubric.
  That stands in for Code-Review, which needs a pair, and is compared with the recorded run's diff and review round.

## Iterations

None ran.

## Is the prompt ready to bench

No.
It does not exist yet, and the advisor cannot be set for a single run.

## What the operator decides

A route that sets `advisorModel` or `--advisor` for one shuttle run only, such as an engine-level per-run settings key on `lym shuttle run`.
That is a code change in lym, which the task excludes as a config key but may accept as a run option.
