# batten follow-up review — opus-medium-r1

Round 1 of the follow-up campaign. Reviewer tag `opus-medium-r1`.
Worktree `/home/knatte/Code/loomyard/wts/crucible-batten-followup`, branch `crucible-batten-followup`.

## Executive summary

Nine findings: 1 BLOCKING, 3 MEDIUM, 3 LOW, 2 NIT.
The outward-first focus paid off again: the BLOCKING and two of the MEDIUMs live at batten's boundary with loom and fabric, found by driving, not reading.

Top risks:
1. **F4 (BLOCKING)** — batten's normal single-slug SUCCESS path never finishes unattended: every successful loom child leaves webster's own contract files (`outcome.yaml`, `summary.md`, `reports/integration.yaml`) untracked in its weft, so `Worktree-Teardown` halts `blocked` on fabric's dirty-weft refusal. Root cause is in loom's Webster row, which commits nothing on Done.
2. **F1 (MEDIUM)** — a child bootstrap that fails after seeding its status (tmux, handshake, startup-gate/not-ready, or a Ctrl-C during the wait) is reported honestly once, then every resume watches a driverless child as "still running" for 12 hours, because `Run-Shed` treats "status file exists" as "spawn happened".
3. **F5 (MEDIUM)** — `Worktree-Teardown` has the crash-window idempotence gap `Worktree-Create` already closed: re-entered after its own `Remove` succeeded, it blocks forever.
4. **F3 (MEDIUM)** — the absent-task-worktree advice is false (deleting branches never rewinds the run to a fresh create) and destructive for no benefit.

Merge-readiness (pre-fix): NOT ready — F4 fails the merge bar outright.
All nine are scoped fixes; none needs its own task.

## Scope assessment

Against the recovered SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`):
- Delivered as designed: the four rows and their order; `Seed-Child` copying the recipe from the Board `type` (empty ⇒ `loom`, live L2) and the driver from batten's own seed (`params.child_driver`, live L12); `Run-Shed` as a self-routed 1440 × 30s watch that hard-errors on every non-running non-done child state and never routes to teardown (live, L15); `go`-only batten driver; `self` refused in prime; run directories durable under `_lyx/shed/<run-id>/`; hand-seed authority (`lyx shed seed <slug> --recipe batten`) honoured (L12).
- Promised, not delivered (known, deferred, re-confirmed): the SPEC's "batten's rows must self-heal machine-local resources ... recreate the child worktree from its branch before watching it". Still absent (L6); fabric's `Topology.Add` still refuses an existing branch. The shipped refusal text for it is F3.
- The SPEC's two residuals are documented in `battenshed/doc.go`; the "dead or parked" one misses the self-stopping driver (F8).
- No over-reach found: no relay-stepping, no batten bootstrap verb, no llm for batten's own rows.

## Code findings (provisional, severity ordering finalized at the end)

### F1 — Run-Shed's read-before-spawn treats a failed child bootstrap's seeded status as "spawn already happened" (provisional MEDIUM)

`internal/battenshed/innerrun.go` (Call, `!found` arm) and `internal/battencli/wire.go` (Spawn).
`lyx loom start` seeds the child's `status.json` (state `running`) and commits it in its step 1, BEFORE its driver spawn and readiness wait (steps 5-6, `internal/loomcli/start.go`).
So a `loom start` that fails after step 1 — `ensureStatusStrand` failing, the go arm's run-lock handshake refusing, or the llm arm's `StartDriver` returning a startup-gate/not-ready error — leaves the child with a `running` status file and no driver.
The first `Run-Shed` Call reports that honestly as a hard error (`spawn inner shed run: exit status 1: ...`), but the error names no remedy.
The operator's natural next move, resuming `lyx batten run <slug>`, finds the status file present, never re-spawns, and self-bounces "inner shed run still running" for the whole 1440-bounce (12h) window with no driver behind it.
Status: CONFIRMED live (see What was tested, scenario L3): with `LYX_REED_TMUX=/nonexistent/tmux` the child's `loom start` fails at step 4 after seeding `status.json` (`current_producer: Preflight`, `state: running`); the `Run-Shed` step reports `kind: producer` (so ly-drive retries it once); the very next `lyx batten step t2` without the bad env reports `outcome: stuck`, "inner shed run still running; sleeping 30s", with no `lyx loom run` process and no tmux session anywhere for the fixture.
Every further bounce repeats that for the 12h budget.
Suggested fix: record a spawn-confirmed marker in the row's own scratch directory only after `Spawn` returns success (cleared before each attempt), and spawn whenever the child has no status file OR (its status is `running` AND no confirmed spawn is recorded); `loom start --no-attach` is idempotent against a live driver, so the retry is safe.

### F2 — `driverFieldReadCarveOuts` is file-granular, so a second, illegitimate Driver read in `internal/battencli/arm.go` passes the tripwire silently (provisional LOW)

`internal/loomcli/bootstrap_test.go:694` keys the carve-out on the whole file.
Its justification names one site (`refuseAdoptedSeed`'s comparison), but any other `<seed>.Driver` read added anywhere in `arm.go` — including one that gates behaviour, which is exactly what the Driver Choice Single-Site Invariant forbids — is skipped by `if driverFieldReadCarveOuts[f.relPath] { continue }`.
Suggested fix: key the carve-out on file plus enclosing function (`internal/battencli/arm.go` + `refuseAdoptedSeed`) and have the scanner stamp each hit with its enclosing `FuncDecl` name.
Status: CONFIRMED — demonstrated on a `git archive HEAD` copy in the scratchpad (L5).

### F3 — the absent-task-worktree recovery advice is false and, followed verbatim, destructive for no benefit (provisional MEDIUM)

`internal/battencli/wire.go:57` (`taskWorktreeLocation`'s refusal text).
The advice says "resolve this by hand, deleting the pair's branches so a resumed run reaches a fresh create".
Deleting branches does not rewind batten's durable status: the resumed run resumes at its persisted `current_producer` (`Run-Shed` here) and hits the identical refusal again, so it never "reaches a fresh create".
What actually rewinds to a fresh create is deleting prime's batten run directory (`_lyx/shed/<slug>/`), which the message never names; and "the pair's branches" silently means four refs (both local branches AND both `origin` branches `Seed-Child`/`Worktree-Create` pushed), since a re-create with only the locals deleted is refused by fabric's own push ("non-fast-forward" on `<slug>-weft`).
Meanwhile deleting the branches throws away the task's unmerged work — the destructive half of the advice buys nothing.
Also the refusal is surfaced as a hard error with `kind: producer`, which ly-drive retries once — harmless (it refuses again) but noted.
Status: CONFIRMED live (L6).
Suggested fix: reword to the true recovery: either restore the pair by hand from its branches (non-destructive, work preserved), or abandon this run by deleting prime's run directory (name its path) AND the pair's local and remote branches, after which "lyx batten run <slug>" re-seeds and starts from a fresh create — stating that the second path discards the task's unmerged work.

### F4 — every successful loom child leaves webster's contract files untracked in its weft, so batten's SUCCESS path always halts `blocked` at `Worktree-Teardown` (provisional BLOCKING)

`internal/loomshed/webster.go` / `internal/shedadapters/webster.go` (Webster row), `internal/loomcli/wiring.go` (Env wiring).
Webster's Master writes `_lyx/webster/outcome.yaml`, `_lyx/webster/summary.md` and `_lyx/webster/reports/integration.yaml`; webster's own CLI commits only `state.json` and the per-batch reports (`webster: begin-batch` / `record-batch`), and the Webster row commits nothing on Done — unlike Discussion-Write and Plan-Write, whose artifacts `CommitDiscussion`/`CommitPlan` commit (commit `22e5b6314`).
Per the Fabric Git Invariant ("Agents write into `_lyx` via the junction; Go reads and commits") those files are Go's to commit, and nothing does, through Publish and Finalize to `state: done`.
Batten's teardown correctly never forces, so `Topology.Remove`'s `refuseDirtyWeftWorktree` refuses: `worktree removal failed (session shutdown already succeeded): weft worktree has uncommitted changes; run "lyx fabric sync" or use --force`.
`docs/overview.md` documents teardown blocking on "anything an AGENT left uncommitted or untracked (a build artefact, a note)" as an operator clean-up; this is not that — it is lyx's own artefact, on every successful run, with a perfectly clean agent.
So the merge bar's normal single-instance flow never reaches the SUCCESS terminal state unattended.
Status: CONFIRMED live (L8): `t3` child reached `Finalize`/`done`, merged `Add // three comment above main` into `main`; batten halted `blocked` at `Worktree-Teardown`; after removing the (agent-built, fixture-has-no-.gitignore) `fx` binary from the warp side, the weft refusal remained with only the three webster files untracked; `lyx fabric sync` from `t3` then `lyx batten step t3` → `done`.
Suggested fix: give the Webster row a commit seam mirroring `CommitPlan` — an `Env.CommitWebster` closure committing webster's durable directory (`websterengine.DirRel()`), required by `websterEntry` and invoked by `loomshed`'s Webster wrapper on `Done` before returning.

### F5 — `Worktree-Teardown` is not idempotent against its own post-condition, so a kill between `Remove` succeeding and the transition persisting strands the run forever (provisional MEDIUM)

`internal/battencli/wire.go` (`Teardown.Shutdown` / `Teardown.Remove` closures), `internal/battenshed/teardown.go`.
`Worktree-Create` got an already-present probe for exactly this crash window (shedengine persists the transition only after the producer returns), but its mirror image was not closed: re-entering teardown after the pair is already gone calls `Shutdown`, whose `taskWorktreeLocation` refuses "not present", so the row halts `blocked` with `session shutdown failed: ... deleting the pair's branches so a resumed run reaches a fresh create ...` — advice that is meaningless at teardown, where the pair being gone IS the goal.
No documented action unblocks it (re-stepping repeats it; `Remove` itself refuses a missing worktree with `worktree ... not found`).
Status: CONFIRMED live (L9) by reproducing the post-crash state: `t5` stepped to `Worktree-Teardown` (child status hand-set to `done`), `lyx fabric remove t5` (the state a kill right after teardown's own `Remove` leaves), then `lyx batten step t5` → `blocked` with the stuck reason above.
Suggested fix: make both teardown halves treat an absent task worktree as their post-condition already satisfied (log it, `Shutdown` returns no abandoned session, `Remove` returns nil), probed with the same `taskWorktreePresent` the create row uses.

### F6 — `Seed-Child`'s disagreeing-seed stuck reason says the seed "disagrees with recipe" even when the recipe agrees (provisional NIT)

`internal/battenshed/seamchild.go:130`.
The `ErrDisagreeingChildSeed` arm renders "the task worktree's own seed already disagrees with recipe %q", but a pre-existing seed disagreeing only in `params` or `driver` agrees on the recipe: live, `t7`'s reason read `the task worktree's own seed already disagrees with recipe "loom": ... already seeded with {Recipe:loom Driver:go Params:map[parent:some-other-branch]}; refusing to overwrite with {Recipe:loom Driver:go Params:map[parent:main]}` (L10).
The routing is right; the lead sentence points the operator at the wrong field.
Suggested fix: "the task worktree already has its own seed, and it disagrees with the one Seed-Child would write: ...".
Status: CONFIRMED live.

### F7 — `lyx shed seed` writes a seed into fabric's own checkouts (`_board`, a weft sibling) with no refusal (provisional LOW)

`internal/shedcli/seed.go` (`newSeedCommand` → `writeSeed`).
Every other `lyx shed` verb for a batten seed reaches `battencli.ArmAt` → `fabricengine.RequireDrivableWorktree` and refuses fabric's own checkouts, but `seed` resolves cwd and writes with no such gate: from `_board`, `lyx shed seed zz --recipe loom` returned ok and left `?? _lyx/shed/` untracked in the Board checkout of `weft:main` (L11), where the next Board write's `Bolt` commit — which stages everything in that repo — would sweep it onto `weft:main`.
Worse, the "no seed found" refusal `lyx shed status <slug>` prints from a weft sibling tells the operator to run exactly that ("run \"lyx shed seed <slug> --recipe <name>\" first").
Suggested fix: `seed` calls `fabricengine.RequireDrivableWorktree(location)` before `writeSeed`, the same refusal batten's own verbs apply.
Status: CONFIRMED live.

### F8 — `battenshed`'s documented residual omits the commonest undetected-driver case: an llm driver that stops ITSELF with the run non-terminal (provisional LOW, docs)

`internal/battenshed/doc.go:15-19`.
The residual names a driver strand that "dies mid-run" or is "alive but parked". Live (L12), the child's ly-drive session started, found no `ly-drive` skill installed on this host, wrote a `BLOCKED` report to `.lyx/shed/self/drive-report-*.md`, and ended its turn — neither dead nor parked — leaving the child `running` at `Preflight`, which `Run-Shed` reports as "still running" for the whole 12h budget.
The same shape follows from every ly-drive stop that leaves the run non-terminal: a hand-back on a non-`producer` refusal kind, or the 120-step autonomous cap.
Suggested fix: extend the residual text to name a driver that stops of its own accord without the run reaching a terminal state (and where its report lands), so an operator reading a long-quiet `Run-Shed` knows to look there.
Detection itself stays out of scope (design residual).
Status: CONFIRMED live.

### F9 — `waitOrCancel`/`InnerRunDeps.Sleep` promise that "an operator's stop is not held for the whole poll interval", but no production path ever cancels the context (provisional NIT)

`internal/battenshed/innerrun.go:15-17`, `internal/battenshed/deps.go` (`Sleep` field doc).
`cmd/lyx/main.go` runs the root on a never-cancelled context and lyx installs no signal handler anywhere; an operator's Ctrl-C terminates the process outright (L13), and `lyx batten pause` sets `pause_requested`, which shedengine reads between rows — it does not interrupt the sleep.
The cancellation arm is correct and harmless, but the stated rationale describes a production behaviour that does not exist.
Suggested fix: state the true reason — a caller that embeds the producer with a cancellable context (the tests, any in-process driver) gets a prompt return; a CLI process is simply killed.
Status: CONFIRMED by reading + L13.

## Focus-1 enumeration table

Enumeration shell (repo root):
`git show --format= 36900c6f5 -- internal/shedrun/seed.go internal/fabricengine/fabric.go internal/loomcli/bootstrap_test.go internal/shell/posix.go internal/boardcli/cli.go internal/shedrecipe/entries_batten.go`;
`grep -rnE '\bWriteSeed\b|ErrDisagreeingSeed|RequireWarpWorktree|RequireDrivableWorktree' --include=*.go .`;
function-value/closure reach: `grep -rnE 'shedrun\.WriteSeed[^(]|RequireDrivableWorktree[^(]|RequireWarpWorktree[^(]' --include=*.go .` (none) and `grep -rn 'ArmAt' internal/shedcli` (`table.go:56` holds `battencli.ArmAt` as the batten row's `Arm` func value);
text consumers: `grep -rn -E 'disagreeing seed|already seeded with|refusing to overwrite' --exclude-dir=.git --exclude-dir=_mill .`.

| Changed item | Site | How reached | Behaviour checked | Verdict |
|---|---|---|---|---|
| `shedrun.ErrDisagreeingSeed` + reworded message | `shedrun.WriteSeed` | producer | wraps with `%w`; test `seed_test.go:175` | correct |
| same | `battencli/wire.go:339` SeedChild `WriteSeed` closure (a func value stored in `SeedChildDeps`, called by `seamchild.go:87`) | closure | `errors.Is` → `ErrDisagreeingChildSeed` → Stuck; live L10 (params-only disagreement) | correct routing; wording F6 |
| same | `battencli/arm.go:222` `armSeed` auto-seed | direct | guarded by a prior `ReadSeed` found-check, so only a concurrent-seed race reaches it; the envelope then carries the new "shedrun: disagreeing seed: ..." text | correct |
| same | `shedcli/seed.go:76` `writeSeed` (`lyx shed seed`) | direct | error envelope, operator-facing; reword is still self-describing | correct (see focus 4) |
| same | `loomcli/sharedbootstrap.go:104` `seedAndCommitBootstrap` (`loom start`, `loom step`) | direct; reached from batten via the `Run-Shed` Spawn exec | `start`: error envelope, folded into batten's spawn error by `childSpawnError`; `step`: `bootstrapStageSeed` → `KindUnseeded` (stage-based label, documented in `step.go`; every non-`producer` kind hands back without retry in ly-drive) | correct |
| same (text) | `battencli/wire_test.go:236` child-output fixture | test | uses a pre-reword spelling ("shedrun: refusing to overwrite with disagreeing seed"); assertion is substring "disagreeing seed", still true of the new text | correct (fixture is arbitrary sample output) |
| same (text) | `plugins/ly/skills/ly-drive/SKILL.md`, stencils | prose | no string match anywhere | n/a |
| `fabricengine.RequireDrivableWorktree` | `battencli/arm.go:286` `armAt` | direct, and via `shedcli` table func value `battencli.ArmAt` | live: `lyx batten ...` and `lyx shed status t6` from `warp-weft`/`_board` refuse (L1, L11) | correct |
| `RequireWarpWorktree` (wrapped) | `fabriccli/fabric.go:471,478` | direct | owner-set directory, allowed | correct |
| same | `fabricengine/fabric_test.go` | test | owner | correct |
| same | Fabric Vocabulary scan roots | — | `walkEnforcementRoots(..., []string{"internal","cmd"})`; no caller in `tools/` or elsewhere outside the roots; no non-owner caller hidden by a skipped directory | correct |
| `driverFieldReadCarveOuts` | `loomcli/bootstrap_test.go:694` | test | file-granular: planted second reader in `arm.go` passes (L5) | F2 |
| `shell/posix.go` | `posixShell.Quote` doc comment only | 19 `.Quote(` callers | code unchanged | correct |
| `boardcli/cli.go` | upsert help text (`type`, `short_name`) | help | both fields exist in `boardengine.Task`; `type` round-trips live (t1 upsert echoes `"type":"loom"`) | correct |
| `shedrecipe/entries_batten.go` | `innerRunEntry` doc comment only | — | matches code (no `Now` field any more) | correct |
| `CONSTRAINTS.md`, `docs/overview.md`, `SANDBOX-FABRIC-SUITE.md` | docs | — | overview's batten entry re-read against live behaviour: teardown paragraph (F4), "spawns only while the task worktree has no status file" (F1) | see F1/F4 |

## Focus-2 enumeration table

| Question | Answer | Evidence |
|---|---|---|
| How long does Spawn block, `go` child | 0.48s (loom start's step 1 + run-lock handshake) | L4 |
| How long does Spawn block, `llm` child | 10.5s on a never-trusted path (includes shuttle dismissing the workspace-trust dialog); ~3s on the now-trusted path | L12, L13 |
| Startup-gate failure / timeout inside `loom start` | `loom start` exits 1 with its envelope; `childSpawnError` folds it into a named hard error at `Run-Shed` (kind `producer`) — honest the FIRST time; every resume thereafter is a false "still running" (F1) | L3 |
| Cancelled `lyx batten run` during the wait (SIGINT to the process group, 1s into an llm Spawn) | batten and `loom start` both die at once (no signal handling anywhere); the child's status is already seeded `running`; the driver's claude strand survives in the child's own tmux session (it belongs to the child, so not a batten-owned orphan) and ran on; a resume would not re-spawn (F1 path) — if the kill lands before shuttle adds the strand there is no driver at all | L13 |
| SIGTERM to the batten pid alone | not driven; by reading, `exec.Command` (not `CommandContext`) leaves `loom start` running to completion as an orphan — bounded by its own 30s handshake / `startup_timeout_s` | reading |
| #018 regression? | no: the fresh-path llm driver was not parked on the trust dialog; `~/.claude.json` gained exactly one entry, `.../fx2/hubs/lwarp-LYXHUB/lwarp` with `hasTrustDialogAccepted: true` (Claude keys a linked worktree by its main worktree path) | L12 |

## Docs & operability findings

- F3, F6, F8, F9 are text/doc findings (listed above with the code findings).
- `docs/overview.md`'s batten entry claims "`Run-Shed` spawns only while the task worktree has no status file" (true, and the root of F1) and "Teardown ... blocks ... [on] anything an agent left uncommitted" (true, but F4 shows lyx itself leaves such dirt on every success). Both move with F1/F4's fixes.
- Operability: every `Run-Shed`/`Seed-Child` hard error surfaces with `kind: producer`, the one kind ly-drive retries once. For the absent-worktree refusal and a failed spawn that retry is harmless (it re-refuses; after F1's fix a retried spawn is exactly the right move).
- Focus 4 (`ErrDisagreeingSeed` across `WriteSeed` callers): no change needed. `shedcli`'s `lyx shed seed` is an operator verb whose error envelope already names both seeds, and `loomcli`'s bootstrap has no Shed row to route a Stuck to — its refusal reaches the operator on `start`'s envelope and on `step`'s `kind: unseeded` (a stage label, documented at `loomcli/step.go`). Batten is the one caller with a row whose verdict can express "human judgment needed", and it does. Inventing a Stuck route elsewhere would be over-reach.
- Deferred re-evaluation: (1) recreate-from-branch — gap still holds (L6), accepted. (2) `step`-mode pacing — each `lyx batten step` of a running child still takes the full 30s (L3: 30.045s); "cancellable mid-wait" is true only for an in-process ctx (F9); accepted shape. (3) residual texts — see F8. (4) GitHub #263 — not touched.

## What was tested

- Read: recovered SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), `internal/battenshed/{doc,deps,innerrun,seamchild}.go`, `internal/battencli/{wire,arm}.go`, `internal/shedrun/seed.go`, `internal/loomcli/{start,driverlaunch,sharedbootstrap}.go`, `internal/shedcli/seed.go`.
- `git show --format= 36900c6f5 -- <the six non-batten files>`: the whole predecessor hardening landed in one squash commit; its non-batten code hunks are `shedrun.ErrDisagreeingSeed` (+ message reword), `fabricengine.RequireDrivableWorktree`, the `driverFieldReadCarveOuts` allowlist, a comment fix in `shell/posix.go`, two help lines in `boardcli/cli.go`, and a doc-comment edit in `shedrecipe/entries_batten.go`.
- `grep -rn 'signal.Notify\|NotifyContext\|os/signal'` repo-wide: no signal handling anywhere in lyx; `cmd/lyx/main.go` runs the root on a never-cancelled context.
- Hermetic at start of Job 1: `go build ./... && go vet ./... && go test -count=1 ./...` — all green (EXIT 0).
- Third filing path sweep: `ls -d internal/*selfreport* internal/*friction*` (selfreportcli, selfreportengine, friction, frictionengine) and `grep -rln 'CreateIssue\|issue create\|/issues'` over `internal cmd`: only `selfreportengine.CreateIssue` (reached from `loomcli/arm.go` Tier 1 and `selfreportcli`); `landingshed/publish.go` opens PRs, not issues. The only prose path is `ly-drive`'s operator-gated `lyx selfreport create`, which the autonomous section keeps gated (it becomes a report line). No third automatic path. As belt-and-braces every live command ran with `GH_TOKEN=GITHUB_TOKEN=invalid-crucible-guard`.
- L0 fixture: bare `warp.git` (go.mod + main.go) and an EMPTY bare `weft.git` (a weft with a non-empty initial commit is refused by clone: "its history carries neither .lyx-anchor nor an empty tree") under the session scratchpad `.../scratchpad/fx/`; `lyx fabric clone --into .../fx/hubs weft.git warp.git` built `warp-LYXHUB/{warp,warp-weft,_board}`. Committed onto prime's weft (`09352c2`) BEFORE any Board task: `selfreport: false`, `friction: ""` (loom.yaml), `require_pr_to_base: []` (landing.yaml — the key lives there, not in loom.yaml), plus `sonnet[effort=low]` for discussion/plan/review/driver/conflict; verified with grep. Board tasks `t1` (`type: loom`), `t2` and `t3` (type empty).
- L1 bookend floor: `lyx batten status t2` from `_board` and `lyx batten run t2` from `warp-weft` both refuse naming the checkout ("is the hub's _board checkout" / "is the weft sibling of a pair"); from prime, `status t2` before any seed refuses with the missing-seed message; `lyx batten run` (no slug) and `lyx batten run self` refuse with their two distinct texts.
- L2 `t2` (type empty) stepped: `lyx batten step t2 --child-driver go` → Worktree-Create done (0.10s); `lyx batten step t2` → Seed-Child done; child seed `{"recipe":"loom","driver":"go","params":{"parent":"main"}}` committed as `batten: seed child t2` on `t2-weft`; prime seed `{"recipe":"batten","driver":"go","params":{"child_driver":"go"}}`. Empty Board type defaulted to `loom` as designed.
- L3 (F1): `LYX_REED_TMUX=/nonexistent/tmux lyx batten step t2` → `{"error":"battenshed: Run-Shed: spawn inner shed run: exit status 1: {\"error\":\"run -V: fork/exec /nonexistent/tmux: no such file or directory\"...}","kind":"producer"}`; child `status.json` left at `Preflight`/`running`. Then plain `lyx batten step t2` → 30.0s, `outcome: stuck`, "inner shed run still running", no driver process, no tmux session for the fixture. F1 confirmed.
- L4 focus-2 timing, go child: `lyx -v batten step t3` (3rd step, Run-Shed first Call) logged `spawning loom session` 11:27:49.163 → `loom session wait complete` 11:27:49.639: Spawn blocked 0.48s for a `go` child (the go arm's run-lock handshake). Then `lyx batten run t3` backgrounded to completion (output to scratch `t3-run.out`).
- L5 (F2 demo): `git archive HEAD | tar -x -C scratch/f2copy`; appended `func illegitimateDriverGate(seed shedrun.Seed) bool { return seed.Driver == shedrun.DriverLLM }` to the copy's `internal/battencli/arm.go`; `go test -run TestDriverChoiceSingleSiteInvariant ./internal/loomcli/` → `ok` (planted gate passes silently). Control: the same function in a new `internal/boguspkg/b.go` → FAIL. The working tree was never touched.
- L6 (F3): moved `t2` and `t2-weft` out of the hub (simulating "on another machine"); `lyx batten step t2` → hard error (kind `producer`) with the recorded advice. Followed it verbatim: `git worktree prune` (needed first — `git branch -D t2` refuses while git still records the moved worktree), `git -C warp branch -D t2`, `git -C warp-weft branch -D t2-weft`; `lyx batten step t2` → the IDENTICAL refusal at `Run-Shed`; `lyx batten status t2` still `current_producer: Run-Shed`. Only after also deleting prime's `_lyx/shed/t2/` did `lyx batten step t2 --child-driver go` reach `Worktree-Create`, which then blocked: first on my simulation's leftover portal link (artefact of moving dirs, not a defect), then on `push weft branch "t2-weft" failed ... non-fast-forward` because the origin branches `Seed-Child`/create pushed still exist. `t2` is left blocked at create and is not reused (deleting the bare repos' branches was refused by this session's permission gate).
- L8 (F4, focus 5 landing path): `lyx batten run t3` (child `go`) ran to exit 0 in ~4.5 min wall: child loom walked Discussion → Plan → Batchifier → Webster → Webster-Review → Publish → Finalize to `done` (`require_pr_to_base: []`), `main` gained `232efff Add // three comment above main`; batten's envelope `{"halted_producer":"Worktree-Teardown","outcome":"blocked"}`, stuck reason "worktree has uncommitted changes; use --force; this pair's portal junction and launcher scripts were already torn down before the refusal". `git status` in `t3`: `?? fx` (agent-built binary); in `t3-weft`: `?? _lyx/webster/outcome.yaml`, `?? _lyx/webster/reports/integration.yaml`, `?? _lyx/webster/summary.md`. `git log -- _lyx/webster` shows only `webster: begin-batch`/`record-batch` commits (state.json + batch report). Removed `fx`; `lyx batten step t3` → blocked again, now "weft worktree has uncommitted changes; run \"lyx fabric sync\"". `lyx fabric sync` in `t3`, `lyx batten step t3` → `done` in 0.1s; hub has no `t3`/`t3-weft`; `ps aux` shows no fixture tmux/lyx/claude process.
- L9 (F5): Board tasks `t4`..`t7` (type empty). `lyx batten step t5 --child-driver go`, `lyx batten step t5` (create, seed); hand-wrote the child `status.json` as `state: done` so `Run-Shed` reads done without a real child run; `lyx batten step t5` → Run-Shed done; first `lyx fabric remove t5` refused (my hand-written status untracked) → `lyx fabric sync` in `t5`, `lyx fabric remove t5` ok; `lyx batten step t5` → `outcome: stuck`, `state: blocked` at `Worktree-Teardown`, `stuck_reason: "session shutdown failed: battencli: the task worktree for \"t5\" is not present ... deleting the pair's branches so a resumed run reaches a fresh create ..."`.
- L10 re-entry + disagreeing seed (focus 3a, floor): `t6` create + seed, then hand-set prime's `status.json` back to `Seed-Child`/`running` (the kill-between-commit-and-persist state) → `lyx batten step t6` done, `t6-weft` log unchanged (no double seed commit); set back to `Worktree-Create` → `-v` logs "create worktree skipped, the task worktree is already present", done, no second create. `t7`: after create, hand-wrote and committed a child seed disagreeing ONLY in `params.parent` (`some-other-branch`) → `lyx batten step t7` → `blocked` at `Seed-Child` with the named stuck reason (F6 wording), and the hand seed was left untouched. The only production `WriteSeed` closure calls `shedrun.WriteSeed` directly and `errors.Is`-matches its sentinel, so no wrapped/reworded variant can reach `SeedChild` today; a seed with an unknown key or bad JSON is a hard error ("decode existing seed"), not an adoption.
- L11 (F7): from `_board`, `lyx shed seed zz --recipe loom` → ok, `?? _lyx/shed/` in the Board checkout (removed by hand afterwards). From `warp-weft`: `lyx shed status t6` → batten's `RequireDrivableWorktree` refusal (ArmAt reached through shedcli's table func value). From `t6-weft`/`t6`: `lyx shed status t6` → "no seed found for run \"t6\"; seeded runs: [self t2 t3 t5]. run \"lyx shed seed t6 --recipe <name>\" first" (the frozen copies the overview documents); from `t6`, `lyx batten step t6` → the Bookend refusal naming both worktrees.
- L12 (focus 6, F8): second fixture `fx2` (fresh bare `lwarp.git`/`lweft.git`, `.gitignore` for the module binary, same config override committed and pushed before any task). `grep -c fx2 ~/.claude.json` = 0 before. Board `l1` (`type: loom`); hand-seed authority: `lyx shed seed l1 --recipe batten --param child_driver=llm` → prime seed `{"recipe":"batten","driver":"go","params":{"child_driver":"llm"}}`; `lyx -v batten run l1` (no flags) adopted it, and Seed-Child wrote the child seed with `driver: llm`. Spawn 11:35:31.378 → 11:35:41.892 (10.5s). `~/.claude.json` gained `.../lwarp-LYXHUB/lwarp` (`hasTrustDialogAccepted: true`) — trust dismissed, not parked (#018 holds). The driver pane then reported: "The `ly-drive` skill is not in the available skills list ... Status: BLOCKED — nothing was done", wrote `.lyx/shed/self/drive-report-20260924-113531-980d.md`, and idled. Environment gap: `~/.claude/plugins/installed_plugins.json` has no `ly@loomyard` (docs/skills.md's installed set is `ly + craft + golang + raddle`); installing a plugin into the operator's global Claude config is not mine to do, so a genuine llm-driven terminal state could not be reached. Child stayed `running`/`Preflight`; batten kept bouncing "still running". Stopped batten with SIGINT; `lyx reed down` in `l1`.
- L13 (focus 2 cancellation): `l3` (`--child-driver llm`), `setsid lyx -v batten step l3` (third step = Run-Shed); 1s after "spawning loom session", `kill -INT -- -<pgid>`: both `lyx batten step l3` and its `lyx loom start --no-attach` child gone; child status `Preflight`/`running`; the child session kept panes `%9 bash`, `%10 lyx` (status strand), `%11 claude` (driver, alive, idle at prompt). (A first attempt on `l2` missed its window — Spawn finished in ~3s on the now-trusted path — and sent no signal.) `lyx reed down` in `l2`, `l3`.
- L14 PrimeRunLock: held `.../warp/.lyx/shed/run.lock` with `flock -x ... sleep 8`; `lyx batten step t4 --child-driver go` → `blocked` at Worktree-Create, `stuck_reason: "prime lock \"...\" is already held ..."`; after release `lyx batten step t4` → done, `t4`/`t4-weft` created.
- L15 Run-Shed never routes to teardown on a halted child (floor): on `t6`, hand-set the child's status to `paused`, `failed`, then `blocked` in turn; each `lyx batten step t6` → hard error `inner shed run reached state "<state>": error=... current_producer="Plan-Bouncer"; the task worktree's own run must be resumed from inside that worktree ...`; batten `current_producer: Run-Shed`, `state: failed`, `t6` pair still on disk.
- L16 (focus 5) `lyx loom step` driven by hand inside `t4` (after batten's create + seed): first attempt used SIGINT on a `&`-backgrounded step, which a non-interactive shell starts with SIGINT ignored, so it was not a valid interruption (the step finished; the next step reported `kind: busy` while it did). Redone with SIGTERM to the step's process group 8s into `Discussion-Bouncer`'s review agent: the killed step printed nothing; the next `lyx loom step` resumed the same row and the loop walked Discussion-Burler → Plan-Write → Plan review → Batchifier → Webster → Webster review → Publish → Finalize to `continue: false`, `state: done` in 15 steps (11:40:59 → 11:44:18), `main` gained `24ced0b Add t4 comment`. No regression of the closed loom fixes observed. The step-driven child reproduces F4 exactly: `t4-weft` left with the same three untracked webster files (plus the agent's `fx` binary in `t4`).
- Hermetic, end of Job 1: `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...` ok; `go test -tags integration -count=1 ./internal/battencli/...` ok; full `go build/vet/test ./...` re-run below before the first fix.
- Post-findings regression check against `_mill/batten-end-to-end/batten-review-HANDOFF.md` (read only after F1-F9 were committed): create-row idempotence (L10), the absent-worktree refusal naming no `lyx fabric checkout` (L6), `childSpawnError` carrying the child envelope (L3), the durable-status/absent-lock-dir read (every `lyx batten status` here), and the posix quote comment — all still hold. No regression found. The HANDOFF records an earlier round's success-path teardown firing for real; F4 is therefore either newer than that round or was cleaned by hand there — either way it reproduces on today's `main`, twice (run-driven `t3`, step-driven `t4`).
- L7 PrimeRunLock/dirty-status interplay: while `t3` was watching (prime weft carrying the documented uncommitted `M _lyx/shed/t3/status.json`), `t2`'s Worktree-Create proceeded past fabric's clean-tree checks all the way to the push — a watching slug's uncommitted status does not block another slug's create.

## Teardown

(pending)
