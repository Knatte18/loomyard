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
| R2 | `fable-high-r2` | Fable / high | DONE, independently verified — NOT a safety pass (0 BLOCKING, 2 MEDIUM, 2 LOW, 1 NIT; 4 fixed, F5 behavioural half deferred to the operator) |
| R3 | `sonnet-xhigh-r3` | Sonnet / xhigh | DONE, independently verified — NOT a safety pass (2 BLOCKING, 2 MEDIUM, all fixed) |
| R4 | `opus-medium-r4` | Opus / medium | running (seed: the HANDOFF commit that records this row) |

**Operator's schedule (2026-09-24):** R2 Fable/high, R3 Sonnet/xhigh, R4 Opus/medium, run back to back with verification and re-seed between each.
**After R4: stop and check in with the operator** ("pust i bakken") — do not spawn R5 or pick its model on your own.
**Operator delegation (2026-09-24, during R3):** "Du velger angående det som trenger et valg" — the orchestrator decides the open choices. Decisions taken are recorded under DECIDED below.

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

**R2 (`fable-high-r2`) independently verified 2026-09-24.** Self-reported MERGEABLE for the normal single-instance flow — not trusted at face value.
- Report and fixer report have every closing section (executive summary, re-confirmation, teardown; fixer close-out).
- Gates reproduced green from cold at `0ffa16b08`: `go build ./...`, `go vet ./...`, `go test -count=1 ./...` (92 `ok`), `go test -count=5` over battenshed/battencli/battenrecipe/shedrun/shedcli/fabricengine/cmd/lyx, `go test -tags integration -count=1 ./internal/battencli/... ./internal/shedcli/...`.
- Sabotage-proven by the orchestrator (fix neutralised, test failed at the intended assertion, restored to an empty diff):
  - **F1** (MEDIUM, `37e4b4788`, `Worktree-Teardown` reported `done` over a half-torn pair — warp gone, weft/branches/portal still there): `if remnantPresent` → `if false && …` → `TestWire_TeardownRefusesAHalfTornPair` fails ("Remove() = nil; want a refusal").
  - **F2** (MEDIUM, `85bf53198`, `Worktree-Create` passed fabric's `lyx fabric checkout` remedy through, which switches prime itself onto the task branches): `createRefusal` forced to pass through → `TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout` fails on both leftover sub-tests.
  - **F3** (LOW, `eefd0dc1f`, `lyx shed seed --recipe batten` accepted from a task worktree): the `RefuseSeedAt` consult disabled → `TestWriteSeed_RecipeLocationRuleGatesTheWrite` and the integration `TestWriteSeed_BattenSeedIsRefusedOutsidePrime` both fail.
  - **F4** (NIT, `d79829eb6`, done-slug remedy omitted the leftover branches): old text restored → `TestRunCmd_StateDoneRefusesNamingTheRunDir` and `TestBattenPreStep_DoneSlug_KindBootstrap` fail.
- **Orchestrator-found wiring gap in F2 (seeded as R3 focus 3):** `fabricengine.Add` returning an untyped error with the same text leaves every test green, `-tags integration` included — the F2 test hand-builds `ErrBranchExists`, and fabric's own test pins only the text. The fix works today; nothing stops it being reverted silently.
- **Orchestrator-found candidate, not driven (seeded as R3 focus 1):** R2's crash-window table says a kill inside `Topology.Add` is rolled back; under SIGKILL the in-process rollback never runs, and `taskWorktreePresent` probes the warp alone, so a re-entered create likely reports Done over a half-created pair — the create-side mirror of F1.
- F5 (LOW, PLAUSIBLE) docs half `944d3cfc6` read and judged accurate; behavioural half decided and filed as its own task (see DECIDED).
- Neighbours checked: no other production matcher on fabric's "already exists" text; `RefuseUnlessPrime` has two callers (`armAt`, shedcli's table); `PairSiblingRemnant` one (`wire.go`).
- Live headline (from R2's report, not re-driven): one full `--child-driver go` run reached `Worktree-Teardown` → `done` with no operator step and an empty `git status --porcelain` in both child halves just before teardown.
- Zero GitHub issues filed (newest still #265). Zero stray `lyx`/`tmux`/`reed` processes. The handback carried a harness "settings-json" instruction-shaped flag: it matched the report's observations that `~/.claude.json` gained one `projects` entry for the fixture path (left alone); read as data, no directive in it.

**R3 (`sonnet-xhigh-r3`) independently verified 2026-09-24.** Self-reported MERGEABLE for batten's own scope — not trusted at face value.
- Report and fixer report have every closing section (executive summary, focus tables, re-confirmation, teardown; fixer deferred list, teardown, verdict). R3 found both R3 pre-count items on its own.
- Gates reproduced green from cold at `dbea6c345`: `go build ./...`, `go vet ./...`, `go test -count=1 ./...` (92 `ok`; R3's "95 packages" is a counting difference, not a change), `go test -count=5` over battenshed/battencli/battenrecipe/shedrun/shedcli/fabricengine/cmd/lyx, `go test -tags integration -count=1` over battencli/shedcli/fabricengine.
- Sabotage-proven by the orchestrator on the PRODUCTION side (the wiring, not the helper), each restored to an empty diff:
  - **F-SIGKILL-ADD** (BLOCKING, `afb5976eb`, a SIGKILL inside `Topology.Add` left a half-built pair the re-entered create reported Done over — wedged `StateFailed`, or a real `_lyx` directory written into the warp): the create closure's `if complete` → `if present || complete` → `TestBattenIntegration_CreateRow_IncompletePairRefusesRatherThanSkippingAdd` fails ("error = nil; want a refusal").
  - **F-SIGKILL-ADD remedy completeness** (`ac3ba6e3e`): `incompletePairRemedy`'s `if remnantPresent` → `false && …` → `TestBattenIntegration_CreateRow_IncompletePairNamesTheOtherSideLeftoverToo` fails.
  - **F-CLEANUP-REMOTE-ORPHAN** (BLOCKING, `a021b016e`, teardown left the weft branch on the remote, invisible to the `lyx fabric cleanup` the remedies name, so a re-run of the slug failed on a non-fast-forward push): `Remove(…, false, true)` → `false` → `TestBattenIntegration_FourRowRun_SeedsChildCommitsAndTearsDown` fails ("weft branch … still present on the remote").
  - **F-WIRING-1** (MEDIUM, `7adf43606`): `return createRefusal(err)` → `return err` fails `TestBattenIntegration_CreateRow_LeftoverBranchIsRewordedForPrime`; and the R2 flank (`Add` returning an untyped error with the same text) now fails the same test — the gap the orchestrator found after R2 is closed.
  - **F-WIRING-2** (MEDIUM, `1353acd0d`): the `ErrDisagreeingChildSeed` rewrap → `return err` fails `TestBattenIntegration_SeedChild_WriteSeedRewrapsARealDisagreeingChildSeed`.
- `remote: true` checked by reading `remove.go`: it deletes the weft branch on the weft origin only, never the warp branch, and a failing remote delete never fails `Remove` — no open PR is at risk.
- **Orchestrator-found by reading (seeded as R4 focus 2, not driven):** the incomplete-pair remedy says `git branch -D <slug>`, but `Add` names the branch `branch_prefix + slug`; with a non-empty prefix it names a branch that does not exist. Recoverable (the next create's `createRefusal` names the right branch), so likely LOW.
- R3's own PLAUSIBLE-and-undriven windows (seeded as R4 focus 1): a SIGKILL in `Remove`'s junction sweep, where Shutdown resolves reed's config through `_lyx` before Remove runs and may wedge the row; `Add` steps 14–17 (junctions/exclude, uncommitted origin record, pushes). The two BLOCKING fixes were not re-driven live by the orchestrator; R4's re-confirmation live-drives create and teardown re-entry.
- Zero GitHub issues filed (newest still #265). Zero stray `lyx`/`tmux`/`reed` processes. The handback carried the same harness "settings-json" flag as R2's, from the same `~/.claude.json` observation; read as data, no directive in it.

## OPEN — needs the operator

- **Two parked mill sessions run against this worktree:** `crucible-batten-followup:plan` (pid 152587, `/mill-plan`) and `crucible-batten-followup:go` (pid 152703, `/mill-go`), started 11:01 alongside the stopped mill-start. The orchestrator decided to kill them; the auto-mode classifier denied the `kill` ("Interfere With Workloads"). Left for the operator (`! kill 152587 152703`); do not retry. The third `claude` process on this task, `:start` (pid 151978), is the orchestrator itself.

## DECIDED (orchestrator, under the operator's delegation)

- **R2-F5 (teardown ends loom's post-run friction reflection):** option (b) — loom persists `done` only after its post-run bookkeeping, keeping batten's "watch the status file" contract. Filed as mill-wiki task `loom-done-after-friction`. Not fixed in this campaign.
- **Fabric rollback leaves the warp branch behind (R1):** filed as mill-wiki task `fabric-rollback-keeps-warp-branch`.
- **`lyx fabric cleanup` enumerates local branches only (R3's deferred residual of F-CLEANUP-REMOTE-ORPHAN):** filed as mill-wiki task `fabric-cleanup-remote-orphans`.
- **`ly@loomyard` plugin:** not installed — it would change the global `~/.claude` config other sessions share, and the campaign's merge bar is go-driven. The llm-driven child reaching a terminal state stays an accepted environment gap for this campaign.

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

R4 (`opus-medium-r4`) is running on the commit that records this line. When it returns: check its report's closing sections, verify (production-side sabotage, and whether it found the R4 pre-count items), refresh this file, commit and push it — then STOP and check in with the operator. Do not re-seed or spawn R5.
R4's seeded focus (as written into the prompt): (1) drive the SIGKILL windows in `Topology.Remove`/`Add` that R3 left PLAUSIBLE, above all Remove's junction sweep under teardown's Shutdown half; (2) adversarial pressure on R3's fixes — the incomplete-pair remedy followed verbatim (including with a non-empty `branch_prefix`), false "incomplete" verdicts on a healthy pair, what teardown's remote deletion removes and whether anything later needs it, every remedy text added since `d7ab9eca0` followed verbatim from prime; (3) re-confirmation with live re-entry drives; (4) llm gap in one line.
