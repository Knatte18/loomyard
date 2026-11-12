# A Sonnet darn writer with an Opus advisor, on three landed darn tasks, 2026-11-12

This bench reran three landed darn tasks with a Sonnet writer that consults an Opus advisor, and compared each run with the task's recorded Opus run.
It is one sample per task, so it shows a direction and settles nothing.

## The advisor prompt

The text below is the prompt addition, used unchanged for all three runs.
It closes each run's prompt, after the setup, the task and the repository rules.
No iteration of it ran.

```markdown
## Your advisor

You have an `advisor` tool.
It sends your whole conversation so far to a stronger model and returns its guidance.
A consult costs far more than one of your own turns, so spend it where a wrong call is expensive to undo, and nowhere else.

Consult in these four places:

1. Once after you have read the decision record, the PATTERN entries it touches and the code it changes, and before your first edit.
   Ask whether your plan (files, order, public surface, tests) delivers the record and obeys the PATTERN entries, and what you are likely to miss.
2. Before an edit that adds an exported symbol, moves a package boundary or touches a PATTERN-bound area, when you weigh two real options.
3. When you have failed the same step twice.
4. Once after your last commit and before you write the change description.
   Ask what a strict reviewer would flag: a record step left out or done beyond its scope, a stale doc, a redundant or missing test.

Do not consult for syntax, a lookup, formatting, or anything the record already settles.

Put each consult in a few sentences: the decision, the options you see, your lean, and one question.
Follow the advice unless you can name a fact in the repository that contradicts it; in that case say so in your next consult.
Make at most five consults for the whole task.
```

## Setup

- Each task ran from its base commit in a local clone of this repository, outside every fabric pair: `0091f39c` (small), `efda1928` (middle) and `db1c9e2c` (large).
- The writer ran through `lym shuttle run --model sonnet --advisor opus`, from this task's worktree, with the clone named by absolute path.
  Sonnet at its default `medium` effort matches the recorded writers, which ran Opus at `medium`.
- The prompt is the darn writer's template rewritten for the clone: the task's decision record from the run's archive (the large record without its review carry-over block), the repository's `PATTERN.md`, the change-description format and the advisor prompt above.
- The three writers ran at the same time.
  No run slept or paused, so the wall times need no sleep removed.
- The clones are outside the hub, so `lym gate test`, the verify gate, the cold fixer and the Code-Review perch did not run.
  Writers could compile and vet but not run tests; no test of any rerun was run.
  The recorded runs ran their verify gate with a cold fixer.
- Each rerun's diff was reviewed by one read-only Opus reviewer through shuttle against the Code-Review rubric, standing in for the perch, which needs a pair.
  The same reviewer, with the same prompt, also reviewed the landed Opus diff of each task as a control.
  The landed diffs include their review fixes and the reruns' diffs do not, so the control favours the Opus side.
- Cost is the list price of every message in the session transcript, from `internal/shuttleengine/claudeengine/pricing.go`.
  An advisor consult is priced from the `advisor_message` entry in the transcript's usage iterations, at Opus 5.5 rates, with no cache.
  The recorded runs' cost is the usage record's `cost_usd`.

## Results

### Writer

| Task | Writer | Cost | Of which advisor | Consults | Time | Lines added / deleted | Files |
| --- | --- | --- | --- | --- | --- | --- | --- |
| small: `reed-tab-and-attach-fixes` | Opus, recorded | $3.24 | | | 12.6 min | 170 / 33 | 10 |
| small | Sonnet + Opus advisor | $2.10 | $1.09 | 2 | 8.5 min | 85 / 20 | 10 |
| middle: `adopt-takeover-fixes` | Opus, recorded | $4.23 | | | 16.4 min | 535 / 49 | 17 |
| middle | Sonnet + Opus advisor | $3.41 | $1.43 | 2 | 12.3 min | 257 / 34 | 10 |
| large: `decision-record-replace-verb` | Opus, recorded | $7.71 | | | 27.7 min | 2815 / 151 | 33 |
| large | Sonnet + Opus advisor | $6.96 | $2.34 | 2 | 23.7 min | 2261 / 109 | 26 |

- The recorded large run's $7.71 holds $1.14 of Sonnet subagent work beside its Opus work; the reruns spawned no subagent.
- The Sonnet writer alone cost $1.01, $1.99 and $4.62.
  The advisor was 52%, 42% and 34% of each rerun's writer cost.
- Each consult sent 51k to 272k uncached input tokens to Opus, and returned 5k to 32k.
  The advisor's answers are redacted in the transcript, so what it said is not known; the consults' sizes are the only evidence of where they came in the run.
- Every writer used 2 of its 5 consults.
- Every rerun's diff is smaller than the recorded one: about half the added lines on the small and middle tasks, and 80% on the large one.
  The smaller diffs carry less work in the cost, so cost per added line favours the Opus run on the small and middle tasks.

### Review

The recorded column is the run's own Code-Review round 1, by Sonnet lenses and a Bouncer, before any fix.
The rerun column is one Opus reviewer on the unfixed rerun diff.
The control column is the same Opus reviewer on the landed Opus diff, after its fix rounds.
The reviewers differ between the recorded column and the other two, so compare the rerun with the control first.

| Task | Recorded round 1 | Recorded rounds and bounces | Rerun | Control (landed diff) |
| --- | --- | --- | --- | --- |
| small | 2 LOW; APPROVED | 1 round, 0 bounces | 1 BLOCKING, 2 LOW; CHANGES ($0.64) | 1 MEDIUM, 1 LOW; CHANGES ($0.59) |
| middle | 2 MEDIUM, 4 LOW | 2 rounds, 1 bounce, CONVERGED | 1 MEDIUM, 4 LOW; CHANGES ($1.24) | 2 LOW; APPROVED ($0.89) |
| large | 1 BLOCKING, 3 MEDIUM, 4 LOW | 2 rounds, 2 BLOCKING open at the end, circling accepted | 3 MEDIUM, 3 LOW; CHANGES ($1.39) | 1 LOW test gap and 1 LOW wording; APPROVED ($1.25) |

The rerun findings that matter:

- Small: the BLOCKING finding is a real rule break.
  The writer's new comments, help texts and tests spell the bare tokens `warp` and `weft` in `internal/reedcli`, which `PATTERN-fabric-boundary`'s vocabulary rule forbids outside its owner set, and a vocabulary test would fail the gate.
  The writer had read `PATTERN.md`, and the advisor's consults did not prevent it.
- Middle: the MEDIUM finding is a narrowed behaviour: the drop-local clear of untracked lym files matches `_lym/` at the weft root only, so a hub with a subpath anchor still meets the failure the note reported.
  The writer's change description admits this limit.
  The other four are LOW: a push-lock wait that also covers the prime's weft, a doc claim that is not true, two untested branches of the new wait, and a `--here` pull the pause still holds.
- Large: the three MEDIUM findings are a way forward that names `lym fabric commit` where the record forbids it, two spec and directive wording criteria carried by only some of the texts, and a filing-detection function with no test of its own.

## Reading the result

- Cost: at list price the Sonnet writer with the advisor cost 35%, 19% and 10% less than the recorded Opus writer, the saving shrinking as the task grows.
  The saving comes from Sonnet alone; the advisor spent back half the small run's writer cost.
  The diffs differ in size, so the comparison is of cost per task as each model chose to do it, not of cost per line.
- Time: each rerun was shorter by 4 to 5 minutes, with no verify gate in the loop and a smaller diff; the time is not a clean win.
- Quality: the reruns drew a BLOCKING or MEDIUM finding each, where the recorded round 1 drew a MEDIUM or worse on two of three tasks.
  Next to the control the reruns are worse on every task, and next to the recorded round 1 they are about even on the middle and large tasks and worse on the small one.
  The recorded round 1 and the rerun used different reviewers, and the control is post-fix, so neither comparison is clean.
- The advisor did not stop the small run's PATTERN violation, though the prompt told the writer to ask the advisor about the PATTERN entries before the first edit.
- Untested: the reruns never ran a test, so a failing test of any rerun would not show here, and the recorded runs' verify gate and cold fixer had no counterpart.

On this sample a Sonnet writer with an Opus advisor does not match the Opus writer's first-pass quality, and it costs less by an amount that falls with task size.
Three single runs cannot separate the model effect from the run-to-run spread in either direction.

## Finding: the advisor route

- `lym shuttle run --advisor opus` reached Claude Code's advisor tool in every run: the transcript holds a `server_tool_use` named `advisor`, and its usage iterations hold an `advisor_message` entry for `claude-opus-5-5`.
- The advisor's tokens are not in the session's own token totals, only in those iterations, so a cost read from the totals alone misses them.
- The advisor reads the whole conversation each consult, uncached, so a consult late in a long run costs more.

## Not decided

Whether a lym config key for the advisor, beside each role's model and effort, follows is the operator's call.
This bench shows no gain in quality and a cost saving that shrinks with size, so it gives no reason to add one yet.
A bench that settles it needs the verify gate in the loop, more than one sample per task, and the same reviewer for the recorded and the rerun side.
