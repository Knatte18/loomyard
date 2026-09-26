# happy-path crucible — orchestrator handoff

Campaign question: does a normal, real task get from start to landed with the deployed `lyx`, under the `go` and `llm` drivers?
Each round is one review+fix round plus the orchestrator's own verification; a further round runs only on the operator's decision.

## State

- Round 1 `opus-medium-r1` (Opus, medium): complete and verified; fixes `242d46983`..`ff6e654ba`.
- Round 2 `fable-high-r2` (Fable, high): complete and verified; fixes `36eabede9`, `6c3a2eb5c`, `4f38f510a`, `12c7a2981`.
- Nothing pushed. The full result, per round, is `.scratch/result-crucible-happy-path.md` (gitignored, this session's report).
- No round is running. No fixture process is alive.

## CLOSED-AND-VERIFIED

- Round 1: F1 `242d46983`, F3 `5550dd00d`, F6 `f51cb430f` sabotage-proven and live; F5 `c4f0b1a39`/`3d59b14c6` live; F4 partial `efc1f7053`/`d4f6a83d8`; F8 `655bcb6be` (superseded by round 2's F3); F9 `761dc64a5`; F10 `ff6e654ba` (never exercised live: no run halted).
- Round 2: F1 `36eabede9` (multi-line `## verify:` chained) sabotage-proven and live; F2 `6c3a2eb5c` live; F3 `4f38f510a` and F4 `12c7a2981` live in the llm driver's transcript.
- Round 2 regression review judged all nine round-1 fix commits sound.
- Both drivers landed a three-package, multi-batch task on the orchestrator's `verify3` hub; the llm run bounced once (Webster Burler review `BLOCKING` on a committed build binary, fixed, then approved).

## Residual

- `ly` plugin not installed on this machine, and `lyx` cannot supply `ly-drive` itself (Stencil Ownership Invariant decision needed to bundle it).
  Merge this branch before `/plugin install ly@loomyard`, or the marketplace copy carries `main`'s stale skill and re-hits round 1's F5.
- F7 (LOW, NOT-FIXED): a Stuck row with no `on_stuck` reports only `stuck with no OnStuck target`; the cause is in the trace alone.
- `ff6e654ba` (ly-drive resumes a blocked baseline) has never run live.
- Inert scratch trees `$HOME/crucible-happy-path/{verify2,verify3,fable-high-r2}` remain; `rm -rf` was refused by the permission system, so the operator removes them.

## Issues filed

#275, #276, #277, #278. Already filed before the campaign and ignored: #269, #270, #271, #274.

## Method notes

- The orchestrator misread a round agent's interim hand-back (sent when the operator interrupted it) as its final report and tore down the round's live re-drive hub.
  A hand-back whose own text says the re-drive is incomplete is not a finished round: check `git log` and the fixer report's re-drive section before touching any of the round's substrate.
- Grep for live fixture processes by hub path misses `lyx loom run` (its args carry no path) and orphans whose cwd was deleted; check `/proc/<pid>/cwd` and every `lyx-warp-LYXHUB-*` tmux socket instead.

## Next action

None: report to the operator and `lyx:orch`, and wait. Push/merge is the operator's call.
