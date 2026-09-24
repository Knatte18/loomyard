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
| R1 | `opus-medium-r1` | Opus / medium (operator's pick) | DONE, independently verified — NOT a safety pass (1 BLOCKING, 3 MEDIUM, 3 LOW, 2 NIT, all fixed) |

## Baseline before R1

`go build ./...`, `go vet ./...`, `go test -count=1 ./...` green at `b9ebb1ba9` (92 `ok` packages).
Zero stray `lyx`/tmux/reed processes.

## CLOSED-AND-VERIFIED

**R1 (`opus-medium-r1`) independently verified 2026-09-24.** Self-reported READY — not trusted at face value.
- Gates reproduced green from cold at `c42984345`: `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `go test -count=5` over battenshed/battencli/battenrecipe/shedrun/cmd/lyx, `go test -tags integration -count=1 ./internal/battencli/...`.
- Sabotage-proven by the orchestrator (fix neutralised, test failed at the intended assertion, restored to an empty diff):
  - **F4** (BLOCKING, `1bd758f2f`, loomshed — Webster row never committed `_lyx/webster/{outcome.yaml,summary.md,reports/integration.yaml}`, so batten's success path always halted `blocked` at `Worktree-Teardown`): replaced `w.commit()` with a nil error → `TestWebsterProducer_CommitsTheRunRecordOnDoneOnly` fails (`commit calls = 0; want 1`). Neighbour check: the new required `CommitWebster` seam has one production `Env` builder that reaches the Webster row (`loomcli/wiring.go`); `battencli/wire.go`'s `Env` never builds it.
  - **F1** (MEDIUM, `ef703936c`, Run-Shed watched a driverless `running` child for 12h after a bootstrap failed post-seed): removed the running-and-unconfirmed arm → both new `TestInnerRun_*` tests fail (`spawn calls = 0; want 1`, `= 1; want 2`). The fix's load-bearing claim — re-spawn is idempotent against a live driver — checked in `loomcli/start.go:runDriverSpawnAndWait` (`mustSpawnDriver(runLockHeld, driverAction == driverStrandLive)`), so R1's L13 orphaned-but-alive driver is adopted, not doubled.
  - **F5** (MEDIUM, `993ed9439`, teardown re-entry after its own `Remove` blocked forever): reverse-applied → `TestWire_TeardownIsIdempotentAgainstAnAlreadyRemovedWorktree` fails.
  - **F3** (MEDIUM, `894f5df7c`, absent-worktree advice false and destructive): reverse-applied → `TestTaskWorktreeLocation_AbsentPairIsNamed` fails.
  - **F7** (LOW, `af081a157`, `lyx shed seed` wrote into `_board`/weft sibling): reverse-applied → `TestWriteSeed_RefusesFabricsOwnCheckouts` fails on both sub-tests.
  - **F2** (LOW, `05b2cca3e`, Driver Choice carve-out was file-granular): planted a second `seed.Driver` reader in `battencli/arm.go` → tripwire now fails naming it (R1 showed the same plant passing before the fix).
  - **F6** (NIT, `0be1f57b4`, stuck reason blamed the recipe for a driver/params disagreement): reverse-applied → `TestSeedChild_DisagreeingChildSeedIsStuck` fails.
- F8 (LOW, docs, `05b1668f3`) and F9 (NIT, docs, `c901e7257`) read and judged accurate; no changelog-style comments in any touched `.go` file.
- Live headline (from R1's report, not re-driven): after the fixes, two unattended `lyx batten run` drives (`t8`, and `t9` through F1's retried spawn) reached `Worktree-Teardown` → `done`. #018 held: a never-trusted fixture path had its trust dialog dismissed.
- Zero GitHub issues filed (newest still #265). Zero stray fixture processes (orchestrator `ps aux`). No harness instruction-shaped flag on the handback.

## OPEN — needs the operator

- **Fabric rollback leaves the warp branch behind** (R1, out of scope, not fixed): when a create fails and fabric rolls back, the destructive gate refuses to delete the warp branch, so the next create refuses until a manual `git branch -D`. Hit twice. Candidate mill-wiki task (Hard Rule 5) — needs the operator's go-ahead.
- **The llm-driven child has never reached a terminal state in either campaign.** R1's llm driver halted BLOCKED because the `ly` plugin (`ly-drive` skill) is not installed on this host. Environment gap, not a defect; closing it needs the operator to install `ly@loomyard` or accept the gap.
- **Two mill sessions run against this worktree:** `crucible-batten-followup:plan` (`/mill-plan`) and `crucible-batten-followup:go` (`/mill-go`), started 11:01 alongside the mill-start that the operator stopped. `status.md` is still `phase: discussing`, so they should be parked on their entry gates; the operator decides whether to kill them.

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

R1 did not converge. Get the operator's explicit model + effort pick for R2 (unused so far in either campaign: Opus/xhigh, Opus/max, Fable/xhigh, Sonnet/max), then re-seed `_mill/batten-review-prompt.md`'s "Round context" with R1's outcomes as CLOSED-AND-VERIFIED, and commit the re-seed before spawning.
Suggested R2 focus: R1's yield sat at batten's boundary with loom and fabric, not inside batten. Point R2 at shapes rather than instances: (1) enumerate every loom producer that writes under `_lyx` and whether Go commits it — F4 was one uncommitted directory, so the class is likely not closed; (2) enumerate every batten row's crash-window re-entry (F1, F5, and the predecessor's create-row fix are the same shape three times); (3) adversarial pressure on R1's own fixes, above all F1's spawn marker (machine-local `.lyx` scratch vs. a run resumed on another machine) and F4's commit on a resumed Done.
