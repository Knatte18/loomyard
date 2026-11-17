# Code Mode on a darn writer doing a repeated edit: one sample per arm

2026-11-17, board task `code-mode-darn-bench`.
Two darn-writer runs on one landed task, one control and one Code Mode, plus one guard-check session; a direction from one sample per arm, not an effect size.

## Question

The Plan-writer bench found no Code Mode gain, and the web study found Code Mode pays on mechanical, read-heavy work.
The bench asks whether a darn writer making one edit across many packages gets cheaper, faster or leaner in Code Mode.
It also asks whether lym's shell guard still refuses a guarded command run inside a Code Mode script.

## Input

The landed task `test-binary-fork-guard`, commit `888969e7`, base commit `112bb821`.
Its record asks for one shared helper that resolves "this lym binary" and never returns a test binary, used at every pin and spawn of a lym command.
The landed change touches 68 files in 19 Go directories, +997 / -290 lines.
The change is the same small edit repeated at many pins, around a new helper, a `tmuxkit.Main` guard and a process cap.
The recorded Opus writer cost $6.71 and ran 23 minutes.

## Setup

- Each arm ran in its own local clone of this repository at the base commit, outside the hub, through `lym shuttle run` from this task's worktree.
  The method is the one of [advisor-model-bench-2026-11-12](../../loom/advisor-model-bench-2026-11-12/report.md).
- Both writers ran on `claude-opus-5-5` at `medium`, the darn writer's default, one after the other, control first.
- The prompt is the darn writer's template rewritten for a clone: the landed task's record, the edit directive, the subagent rule and the change-description step.
  It sends the writer to the clone's `PATTERN.md` instead of inlining it.
  Both prompts are identical bar the clone's name; they are in `data/`.
- The Code Mode arm differs only in the launch: this worktree's `claude.yaml` named `data/claude-codemode.sh` as the `binary`, which execs `claude` under `CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=1`.
  The file was removed after the arm and before the control, so no other session saw it.
  The first-use dialog had been answered during the earlier bench.
- No gate, cold fixer or test ran: a clone has no hub.
  Writers could compile and vet; each prompt said to run no test.
- Cost is the list price of every message in the session transcript and its subagents, from `internal/shuttleengine/claudeengine/pricing.go`, computed by `data/metrics.jq`.
  A Code Mode transcript also holds one row per inner call; these carry no usage and are counted apart from turns.

## Guard check

One Code Mode session ran `sed -i` against a scratch file through six routes; the full table is `data/guard-report.md`.

| Route | `sed -i` | Redirect into the file |
|---|---|---|
| Direct Bash call | refused | not tried |
| Script's `tools.Bash` | refused | ran |
| Script's `$` template and `$(string)` | refused | ran |
| Script's `write()` | not applicable | ran, returned 6 bytes |

The guard held for every `sed -i` inside the script, with the refusal text of the direct call.
The redirects ran because the probe file is not a source file, which is the guard's own wording.
The probe's `write()` is a whole-file write like the Write tool.
So a Code Mode writer cannot reach `sed -i` through a script, and it keeps the sanctioned whole-file write.
The check did not test a redirect into a source file inside a script.

## Writer figures

| Figure | Control | Code Mode |
|---|---|---|
| Cost, writer | $6.71 | $5.89 |
| Cost, with lookup subagents | $7.17 | $5.89 |
| Wall time | 18m46s | 24m12s |
| Assistant turns | 60 | 43 |
| Direct tool calls | 214 | 80 (41 `REPL`, 39 `Poll`) |
| Inner calls recorded in `REPL` | 0 | 569 |
| Lookup subagents | 4 (Sonnet, 23 turns) | 0 |
| Cache-read tokens, writer | 12.51M | 8.04M |
| Cache-read tokens, with subagents | 12.93M | 8.04M |
| Output tokens, writer | 118k | 78k |
| Peak context, tokens | 374,947 | 307,638 |
| Tool results marked as errors | 0 | 1 |
| `REPL` scripts that ended in an error | 0 | 0 |

- The control's direct calls are 80 Edit, 48 Read, 46 Bash, 36 Write and 4 Agent.
  The Code Mode inner calls are 241 Bash, 150 Read, 148 Edit and 30 Write; shell run through the `$` template is not recorded as a call, so these are a lower bound.
- Every `REPL` eval settled.
  The one error is an inner `go build` that failed on an unused import in `internal/loomcli/cli.go`; the control's tool results show none.
- Code Mode made 12% less writer cost and 36% fewer cache-read tokens, with 17 fewer turns, and took 29% longer.
  Two evals held the session long: one 293 seconds and another 117 seconds.
  The bench did not trace what those two waited on, so the cause of the extra wall time is not known.
- The control spent 4 Sonnet lookup subagents, $0.47; the Code Mode writer spawned none.
- The turn count fell by 28%, a larger drop than the Plan writer's three turns, but the direct calls fell by far more because each `REPL` call carries its batch.

## Output

Both runs made three commits in their clone, and both clones compile and vet on every Go directory they changed.
No test ran, so neither diff's tests are known to pass.

| Figure | Landed | Control | Code Mode |
|---|---|---|---|
| Files changed | 68 | 85 | 83 |
| Lines added / deleted | 997 / 290 | 1278 / 146 | 1181 / 163 |
| Go directories changed | 19 | 29 | 24 |
| Test-file lines added / deleted | not measured | 685 / 50 | 608 / 51 |

Neither diff was reviewed, so the report says nothing of their quality against the landed one.
Both added more than the landed diff did, the control by more.

## Verdict

- The guard holds in Code Mode: `sed -i` is refused in every route a script has, so the shell guard needs no change for Code Mode on this evidence.
- On this repeated-edit task Code Mode cost 12% less for the writer and 18% less with the control's subagents, and it took 29% longer.
  One sample per arm cannot tell the cost gap from run-to-run spread; the control's writer cost lands on the landed run's $6.71 to the cent, which says the control is typical, not that the gap is real.
- The batching worked as the web study predicts: 569 inner calls ran in 41 scripts, and turns fell from 60 to 43.
  The gain is in cached context carried per turn, not in time.
- The wall time went the other way from the cost, and this sample does not show why.
- Quality is unmeasured, so this bench does not support adopting Code Mode for the darn writer.
  A deciding bench needs the verify gate in the loop, a review of each diff, and a second sample of each arm.
- Adopting it would still need the launch line to carry the variable and its tooling to read the `REPL` rows, as the Plan-writer report said.

## Deviations and limits

- One sample per arm; the control ran first and the Code Mode arm after it, so the two share no wall clock but the second ran later in the account's day.
- No gate or test ran, so cost and time leave out the verify loop the recorded run had, and the diffs' correctness is unknown.
- The prompt points to `PATTERN.md` rather than inlining it, and omits the hub's board, loom and drift steps; both arms are alike in this.
- The change description each writer wrote is in `data/`; the spec's validator did not run.
- The bench's spend by the transcripts' list price is $13.25: control $7.17, Code Mode $5.89, guard check $0.19, against the $25 cap.
- Raw material is in `data/`: both prompts, the wrapper script, the `jq` program, the guard report and both change descriptions.
  The session transcripts live in the Claude project directory of this worktree.
