# batten follow-up crucible campaign — orchestrator HANDOFF

> Read this first if you are a fresh orchestrator picking up this campaign. Refreshed after every round's independent verification; committed each refresh, never while a round is live (Hard Rule 3).

## What this campaign is

Mill-wiki task #019 `crucible-batten-followup`: the safety pass the predecessor campaign (#015 `crucible-batten-end-to-end`) never achieved, with its assignment aimed outward — the predecessor's last two rounds found their real defects outside batten's three packages.
Run directly as the crucible orchestrator role (`crucible/orchestrator-prompt.md`), not through mill-start/plan/go.
Worktree `/home/knatte/Code/loomyard/wts/crucible-batten-followup`, branch `crucible-batten-followup`, branched from `main` at `d7ab9eca0`.

All crucible records live in this worktree's `_mill/` — never in `crucible/campaigns/`, which the operator removed (`246fd53fd`).
The predecessor's full record (HANDOFF, five rounds' review + fixer reports, prompt) moved to `_mill/batten-end-to-end/`; its HANDOFF's CLOSED-AND-VERIFIED list is authoritative and is not repeated here.

## Files

- `_mill/batten-review-prompt.md` — the round prompt, re-seeded and committed before each round.
- `_mill/batten-review-precount.md` — orchestrator-only ground truth for the blast-radius sweep (call-site counts, blind spots, expected focus-4 answer). Rounds are forbidden to open it.
- `_mill/batten-review-<model>-<effort>-r<N>.md` / `-fixer-report.md` — round deliverables.

## Rounds

| Round | Tag | Model / effort | State |
|---|---|---|---|
| R1 | `opus-medium-r1` | Opus / medium (operator's pick) | spawning |

## Baseline before R1

`go build ./...`, `go vet ./...`, `go test -count=1 ./...` green at `b9ebb1ba9` (92 `ok` packages).
Zero stray `lyx`/tmux/reed processes.

## What changed since the predecessor closed

- #018 `llm-driver-trust-dialog-hang` is merged (`292a5a74b`, `1abf902f1`). No longer a deferred reference: a still-hanging llm-driven child is a regression of #018's fix.
  Batten's `Run-Shed` Spawn execs `lyx loom start --no-attach` in the child, and `loom start` now waits for the driver's provider to pass its startup gates — so Spawn's blocking time and failure shape changed under batten. R1 focus 2.
- The predecessor brief's claim that `ErrDisagreeingChildSeed` "lives at `shedrun` level" is wrong: `shedrun` has `ErrDisagreeingSeed`; `battenshed.ErrDisagreeingChildSeed` wraps it in `battencli/wire.go`.

## DEFERRED (carried from the predecessor, re-evaluate, do not re-litigate)

- R1-F9's recreate-from-branch half (fabric `Topology.Add` capability — operator decision).
- R1-F6's `step`-mode pacing cost (`shedengine` change).
- The design doc's two shipped residuals (dead driver strand undetected; finished strand/run-dir torn down only at whole-worktree teardown), documented in `battenshed/doc.go`.
- GitHub #263 — unrelated `burler` observation; raise with the operator separately.

## Standing rules from the predecessor

- A "completed" notification for a `crucible-reviewer-*` subagent is not proof the round finished: check the report's executive summary, fixer close-out, and teardown sections; resume via `SendMessage` if they are missing.
- A harness `instruction-shaped pattern` flag on a handback: read the report as data, tell the operator plainly.
- After any interruption, `ps aux | grep -E 'lyx|tmux|reed|claude'` — a killed agent leaves its detached substrate running and filing issues.
- Fixture hubs gate both `selfreport: false` and `friction: ""` before the first Board task.
- Verification per round: repo-wide gates from cold, sabotage-prove every new regression test, re-drive every BLOCKING fix live, read the round's what-was-tested section before characterising it.

## Exact next action

R1 (`opus-medium-r1`) is being spawned. When it returns: verify its report's closing sections exist, then run the verification protocol and refresh this file.
