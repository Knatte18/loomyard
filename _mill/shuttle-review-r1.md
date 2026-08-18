# shuttle — independent review, round 1 (`opus-medium-r1`)

Clean-room review of `internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`,
run against the worktree `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).
No prior `_mill/shuttle-review-*`, `_mill/reed-review-*`, or `_mill/reed-shuttle-HANDOFF.md` material was read before this findings list was complete.

Merge bar for this round: correctness in the NORMAL single-instance flow.
`LLM-DRIVING: YES` — every live scenario below spawned exactly one real `claude` process, every one pinned to `--model haiku`.
No N×-concurrent sweep was run.

## What was tested

### Hermetic (green throughout)

| Command | Result |
| --- | --- |
| `go build ./...` | ok |
| `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` | ok |
| `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` | ok (4/4 packages) |

### Substrate check (done FIRST, per the prompt)

`claude` at `/home/knatte/.local/bin/claude` (real, logged in), `tmux` at `/usr/bin/tmux` (3.6), `go` present.
`ps -eo comm \| grep -cx 'tmux: server'` = 0 at start.
So no skip below is an environment skip.

### Live driving — `./deploy-dev` snapshot, `lyx shuttle run|interrupt|send` driven directly, foreground

Substrate: this worktree itself (`lyx reed up`, socket `lyx-wts-c2a59680`).

| # | Scenario | Observed |
| --- | --- | --- |
| L0 | Pre-flight rejections, no claude spawned: bogus guid for `interrupt`/`send`, missing `--output-file`, `--prompt` + `--prompt-file` together, multiline `send` text, negative `--timeout`, pre-existing `--output-file`, `--effort ludicrous` | All eight rejected with self-describing JSON error envelopes; the effort case failed at `Prepare` and cleaned its run dir up |
| L1 | Happy path, `--model haiku`, write `DONE` to a file | `outcome:"done"` in 10s, file content `DONE`, strand gone from `reed status`, run dir removed |
| L2 | Autonomous run told to ask before writing | `outcome:"asking"`, question in `lastAssistantMessage`, strand still `live:true`, run dir kept |
| L3 | `lyx shuttle interrupt <guid>` then `lyx shuttle send <guid> <text>` against L2's surviving run, out of process | Both `ok:true`; pane capture showed the interrupt landed and the sent line was typed and answered; a second `send` drove the agent to write `REDIRECTED`. The out-of-process `FindRun` → `requireReadyAgentPane` → `sendVerified` path works end to end |
| L4 | `PreToolUse(Agent)` guardrail: prompt instructing an Agent-tool subagent dispatch | `outcome:"done"`, file written in-pane. Parent transcript contains one `"name":"Agent"` `tool_use` and one occurrence of `steerAgentDeny`'s text — the deny fired and the steer redirected the work, live |
| L5 | Interactive run (`--interactive`) + a follow-up `send` forcing the `AskUserQuestion` tool | `events.jsonl` gained a `"hook_event_name":"PreToolUse"` / `"tool_name":"AskUserQuestion"` line with the full `tool_input.questions[]` payload — the non-denying marker hook fires in real time, and `askQuestionText` has the shape it expects. `settings.json` for the run carried matchers `['Agent','AskUserQuestion']` with the AskUserQuestion hook set to the append command, not a deny |
| L5b | Autonomous run told to call `AskUserQuestion` | `outcome:"asking"`; parent transcript contains `steerAskUserQuestionDeny`'s text — the autonomous deny fired, and the agent restated the question as its final message exactly as the steer instructs. Both guardrail edges confirmed live |
| L6 | Pane killed mid-flight (`tmux kill-pane` on the strand's pane immediately after launch) | `outcome:"died"`, strand kept for diagnosis. A first attempt at 12s missed because haiku self-ended the counting turn first (classified `asking`, correctly) |
| L7 | Genuine wall-clock timeout (`--timeout 1s`) | `outcome:"timeout"` in 1.0s, strand left `live:true`, run dir kept |
| L8 | `lyx reed down` in a second process while a run's `Wait` was blocked | `{"error":"shuttle: reed status failed 2 times consecutively: reed status: no reed session; run \"lyx reed up\"","ok":false}` — **no `guid`, no `sessionId`, no `runDir`** → finding R1-F2 |
| L9 | Orphan sweep | L2's kept run dir, whose strand had since been removed, was gone after a later `Start` (age past `minAge` = 2 × `startup_timeout_s` = 180s). Freshly-created dirs were left alone throughout |
| L10 | `interrupt`/`send` for a guid whose run.json exists but reed no longer tracks the strand | `shuttle: strand "…" is not tracked by reed — its run has completed and been cleaned up` → wording finding R1-F6 |
| L11 | `interrupt` for a guid whose `run.json` was truncated mid-file while the agent was still live | `shuttle: "…" is not a shuttle strand: shuttle: no run found for strand "…"` → finding R1-F5 |
| L12 | **Composition with reed's foreign-session refusal.** `reed.json`'s `paneGeneration` repointed at a real, live, foreign-named session on the same socket (identity fields taken from a real `display-message` probe so reed's identity check actually matches) | Both `lyx shuttle interrupt` and `lyx shuttle run` surfaced reed's full refusal text verbatim — session name, socket, this worktree's own session name, both causes, and the exact `kill-session` remedy — through shuttle's `%w` wrapping (`shuttle: check strand liveness: …` and `shuttle: add strand: …`). **No finding: shuttle composes correctly with reed's hardened refusal and loses none of its actionability** |
| L13 | Fork-audit project-directory derivation | `claudeProjectDirFor(anchorPath)` resolves to `~/.claude/projects/-home-knatte-Code-loomyard-wts-reed-shuttle-crucible-hardening`, which is exactly where every live run's transcript landed. Confirms `anchorPath` (not `worktreeRoot`) is the semantically right workdir for `AuditForks`, because reed spawns every pane with `-c geom.AnchorPath` |

### Teardown

`lyx reed down`, foreign fixture session killed, `.lyx/shuttle` emptied, fabricated `reed.json` removed.
Final state: `ps -eo comm \| grep -cx 'tmux: server'` = **0**; `pgrep -a -x claude` lists only the operator's own long-lived sonnet/opus sessions — **zero stray haiku processes**, zero stray tmux.
`git status --porcelain` clean apart from this report.

### What could NOT be verified, and why

- **Windows behaviour of `posix.go`.** This host is Linux; `PosixPath` is only reached under `runtime.GOOS == "windows"` (`claudeengine.go:84`). Read for correctness instead — `C:\a b\c` → `/c/a b/c`, drive lowercased, UNC (`//srv/share`) and relative inputs rejected by the drive-root check, `C:/` accepted as the minimal valid input. The logic is right; **it carries a real, named Windows verification gap for this round**.
- **Anchor-vs-worktree divergence in live driving.** This worktree is anchored at its own root (`AnchorRel == "."`), so `anchorPath == worktreeRoot` here and no live scenario could distinguish them. That is precisely why R1-F3's fix lands as a construction-time check plus hermetic tests over a subpath-anchored fixture rather than as a live scenario.
- **A live resumed autonomous run** (R1-F7). `ResumeCmd` is replayed by `lyx reed resume`, a reed verb; the missing flags are confirmed by reading `buildResumeCmd` against `buildLaunchCmd`, the stall itself is not driven.

## Scope assessment (post wave-1)

Shipped scope matches `docs/overview.md:282` and `internal/shuttleengine/doc.go`: one agent per `Start`, four outcomes, file contract as the only return channel, provider behind the `Engine` seam, `Model`/`Effort` engine-validated rather than `Spec.validate`-policed, `Version`/`ForkSubagents` programmatic-only with no CLI flag (documented as deliberate). Nothing shipped beyond scope; nothing plan-promised is missing.

Wave-1 (`b98ee2ba`) dropped no observable behaviour. Every consumer of the two told strings was traced and each is semantically the right one:

| Consumer | Field | Why it is correct |
| --- | --- | --- |
| `runDirRoot` / `FindRun` (`rundir.go:48`) | `anchorPath` | `.lyx` is the ephemeral sibling of `_lyx` **at the anchor** (Durable-vs-Ephemeral State Invariant). Verified live: run dirs landed at `<anchor>/.lyx/shuttle/<runID>` |
| `sweepOrphansOpportunistic` (`run.go:191`) | `anchorPath` | reed's own `stateDir()` is `filepath.Join(geom.AnchorPath, DotLyxDirName)` (`reedengine/lifecycle.go:34`) — the two must agree or the sweep reads the wrong `reed.json` |
| `AuditForks` workdir (`wait.go:257`) | `anchorPath` | reed spawns every pane with `-c geom.AnchorPath` (`reedengine/spawn.go:155`, `lifecycle.go:311`, `:614`), so the anchor IS the pane's process cwd the Engine doc requires. Confirmed live (L13) |
| `spec.validate` (`run.go:94`) | `worktreeRoot` | `run`'s own help promises a relative `--output-file` "resolves against the WORKTREE ROOT, not the shell's cwd" |

Invariants re-checked and holding: **Shuttle Provider-Seam** (`shuttleengine` imports no `claudeengine`; `TestProviderSeamImportRule` green; no Claude vocabulary found outside `claudeengine`), **Durable-vs-Ephemeral State** (`runDirRoot` builds `.lyx` from `lyxdirs.DotLyxDirName`, never a literal, and never `_lyx`), **Shell Mechanics Seam** (`command.go` emits no raw shell syntax, only `shell.Shell` calls), **CLI/Cobra** (`Short` on all four commands, JSON envelopes everywhere, help tree green).

## Findings

Severities: **3 MEDIUM, 5 LOW, 2 NIT.** Every one is fixed in Job 2.

### R1-F1 — smoke suite pins no model (MEDIUM, CONFIRMED)

`internal/shuttlecli/smoke_run_test.go:260`, `smoke_guardrail_test.go:62` and `:119`, `smoke_interrupt_test.go:123`.

All four `//go:build smoke` tests launch a real `claude` on whatever the account default model is — none passes `--model` or sets `Spec.Model`.
`internal/reedcli/smoke_test.go:182` already establishes the pattern (`const smokeClaudeModel = "haiku"`, "the model every real `claude` process this package spawns must run on") for exactly this reason;
shuttle's suite never adopted it.

Failure scenario: every `-tags smoke` invocation of `internal/shuttlecli` burns default-model tokens on four tests whose assertions are entirely mechanical (a file's content, an envelope field, a strand's presence) and gain nothing from a stronger model.
It is also a standing operator instruction for this campaign.

Fix: add `smokeClaudeModel` to the suite's shared-helper file (`smoke_run_test.go`, which already owns `claudeBinaryPath`/`deferHubRelease`) and thread it into all four tests — `--model` for the three `RunCLI`-driven ones, `Spec.Model` for the `Runner.Start`-driven one.

### R1-F2 — `Wait`'s mechanism-failure exits discard the run's identity (MEDIUM, CONFIRMED live)

`internal/shuttleengine/wait.go:73`, `:87`, `:259` — three `return Result{}, err` sites.

Reproduced live (L8): with a run in flight, `lyx reed down` from another process made `lyx shuttle run` exit
`{"error":"shuttle: reed status failed 2 times consecutively: reed status: no reed session; run \"lyx reed up\"","ok":false}`
with no `guid`, no `sessionId`, no `runDir`.
`finalize` never ran, so the run directory is still on disk and the strand may still be registered — the caller is handed the one situation where it most needs those handles, with none of them.
The `AuditForks` site (`:259`) is worse still: it throws away a run that already classified `OutcomeDone`, before its strand and run dir are cleaned up.

Note what the fix must NOT be: reclassifying a `Status` failure as `OutcomeDied` is wrong, because reed returns the same untyped error for its foreign-session refusal (L12), where the run may well be alive under another session — and reed exposes no sentinel to tell the two apart (`noSessionMessage` builds a bare `errors.New`).
Adding one is a reed change and out of this campaign.

Fix: return the identity-bearing `Result{SessionID, StrandGUID, RunDir}` — `Outcome` empty — alongside the error at all three sites, and say so in `Wait`'s doc comment.

### R1-F3 — the told anchor/worktree pair is validated nowhere (MEDIUM, CONFIRMED)

`internal/shuttleengine/run.go:40`.

`NewRunner(reed, engine, anchorPath, worktreeRoot string, cfg)` takes two adjacent parameters of the same type whose four consumers are semantically distinct (see the scope table above).
A caller that swaps them compiles cleanly and, in a subpath-anchored worktree, silently relocates the run-dir root, reed's state lookup, and the fork-audit transcript directory into the wrong tree.
An empty or relative value fails the same way — it succeeds against whatever the process cwd happens to be.

reed hardened the identical seam during its own campaign: `validateToldAnchorPath` (`reedengine/server.go:226`) refuses an empty or relative told anchor at every op boundary, on exactly this reasoning ("does not fail — it succeeds against the WRONG tree").
Shuttle, whose told pair is strictly more confusable than reed's named struct fields, has no equivalent at all.

Fix: validate once at construction and surface the result from every public entry point (`Start`, `Interrupt`, `Send`, `Inject`): both non-empty, both absolute, and `anchorPath` equal to or inside `worktreeRoot`.
That last clause is the swap detector — hub geometry always satisfies `AnchorPath == WorktreeRoot/AnchorRel` (`hubgeom.ReedGeometry`), so it holds for all five production call sites and is violated exactly by a swap.

### R1-F4 — a test fixture defeats the swap guarantee its own helper promises (LOW, CONFIRMED)

`internal/shuttleengine/run_test.go:220`: `NewRunner(reed, engine, worktree, worktree, cfg)`.

`newTestRunner`'s doc comment (`run_test.go:23-26`) states the fixture rule this file lives by — "`anchorPath` and `worktreeRoot` are distinct values (never the same temp dir twice) so a swapped `NewRunner` argument pair fails a test rather than passing".
`TestRunner_Start_SweepSkipsEntirelyOnReedStateReadError` passes the same value for both, which is precisely the masking case the rule exists to prevent.

Fix: give it a distinct subpath anchor, as every other fixture in the file does.

### R1-F5 — an unreadable `run.json` is reported as "not a shuttle strand" (LOW, CONFIRMED live)

`internal/shuttleengine/rundir.go:130-134` (skip) and `:141` (message).

Reproduced live (L11): truncating a live run's `run.json` mid-file made `lyx shuttle interrupt <guid>` answer
`shuttle: "…" is not a shuttle strand: shuttle: no run found for strand "…"` — while that agent was still running in its pane.
The skip itself is right (one damaged dir must not abort the scan for every other run), but the resulting message tells the operator the guid is not shuttle's at all, when the truth is that shuttle's own record of it is damaged and the remedy is completely different.
A partially-written `run.json` is exactly the shape a crash or a full disk leaves behind.

Fix: count the dirs skipped as unreadable and name that count in the not-found error.

### R1-F6 — untracked-strand message asserts one cause among several (LOW, CONFIRMED live)

`internal/shuttleengine/run.go:359`: `strand %q is not tracked by reed — its run has completed and been cleaned up`.

Reproduced live (L10) after a `lyx reed down`/`up` cycle: the run had not completed and nothing had cleaned it up;
reed's state was simply reset. The same message appears after a `lyx reed remove`, after a state-file loss, and after a server rebirth.

Fix: state the observable fact and name the alternatives, rather than asserting the one cause.

### R1-F7 — `buildResumeCmd` drops every launch flag except `--settings` (LOW, PLAUSIBLE)

`internal/shuttleengine/claudeengine/command.go:99-105`, against `buildLaunchCmd` at `:78-94`.

The launch line carries `--model`, `--effort`, and (for an autonomous run) `--dangerously-skip-permissions`;
the resume line carries none of them.
reed replays `ResumeCmd` verbatim when rebuilding a session (`reedengine/lifecycle.go:739-743`, falling back to `Cmd` only when `ResumeCmd` is empty).
So a `lyx reed resume` of an autonomous shuttle run brings the agent back permission-gated and on the account-default model: it will stall at the first tool-permission dialog with no operator present, and shuttle will classify the stall as `timeout` rather than surfacing the cause.
Marked PLAUSIBLE because the resulting stall was not driven live — the flag asymmetry itself is confirmed by reading.

Fix: thread `interactive`, `model` and `effort` into `buildResumeCmd` so a resumed run comes back in the same mode it launched in.

### R1-F8 — the package doc's closing paragraph is stale (NIT, CONFIRMED)

`internal/shuttleengine/doc.go:28-33`.

It still describes the package as "the foundation batch" providing "the pure, hermetic building blocks the rest of shuttleengine is built from", closing with "Nothing here calls tmux or claude, or knows about either."
The package now contains `run.go` and `wait.go`, which drive reed's pane transport (`SendText`/`SendKey`/`CapturePane`), poll a live pane's capture through the engine's `Startup` classifier, play key choreography into a live tmux pane, and read reed's own state file.
A reader who takes that sentence at face value will look for the run loop somewhere else.

Fix: rewrite the paragraph to describe the package as it now is.

### R1-F9 — the fork-mode Agent hook does not do what its comments claim (NIT, CONFIRMED)

`internal/shuttleengine/claudeengine/settings.go:5-6`, `:24-25`, `:91`.

Both comments say the fork-mode `PreToolUse(Agent)` hook allows "only unnamed fork calls".
The hook is `grep -q '"subagent_type":"fork"' || echo '<deny>'` — it allows **any** Agent call naming `subagent_type: fork`, named or not.
Whether a fork was named is a fact the AUDIT records (`ForkAudit.NamedSpawns`, "a defect signal since named forks lose inherited context"), for the caller's policy to interpret — deliberately not something this hook refuses.
The comments describe a guard that does not exist, which is the kind of claim a later reader "restores" by tightening the grep.

Fix: correct both comments to describe the fork-vs-non-fork discrimination the grep actually performs.

### R1-F10 — teardown is not logged through `internal/logger` (LOW, CONFIRMED)

`internal/shuttleengine/wait.go:247-274` (`finalize`), against `run.go:156`.

`CONSTRAINTS.md`'s **Live-Substrate Spawn Observability** requires that a code path starting a real OS process on behalf of a run "logs the spawn **and its teardown** via `internal/logger` — `logger.Info` for normal spawn/teardown events, `logger.Warn` for a retry or a teardown that did not confirm clean", and names `internal/shuttleengine/run.go` a known instrumented call site.
Shuttle logs the spawn (`logger.Info("shuttle: run started", …)`) but nothing at all at teardown: `finalize` removes the strand and deletes the run directory with no `logger` call, and its two cleanup failures go to the bare `log` package (`wait.go:266`, `:269`), which the durable Info+ trace sink never captures.
The result is a trace file that shows every shuttle run starting and none ending.

Fix: emit `logger.Info` with the terminal outcome at `finalize`, and promote the two cleanup failures to `logger.Warn` — "a teardown that did not confirm clean" is the invariant's own wording for exactly these two.

## Observations — deliberately NOT findings

- **The orphan sweep trusts a `reed.json` that reed itself refuses to act on.** During L12, with `reed.json` repointed at a foreign session, a `Start` attempt's opportunistic sweep deleted two kept diagnosis dirs before reed refused the `AddStrand`. This is inherent to the sweep's contract, not a defect in it: its live-guid set is exactly what that `reed.json` says, and `template.yaml`'s `run_dir` comment already documents the matching worktree-local requirement. Closing it would need shuttle to detect reed's foreign-session verdict, which reed exposes no API for — a reed change, out of this campaign.
- **`Wait` cannot distinguish "reed is gone" from "reed refuses".** Named here so a later round does not read R1-F2's fix as having resolved it. It is blocked on reed growing a typed sentinel; see R1-F2 for why guessing is worse than the current behaviour.
- **No `--fork-subagents` / `--version` CLI flag.** `Spec.ForkSubagents` and `Spec.Version` are reachable only programmatically. `docs/overview.md:282` documents this as deliberate ("no CLI flag — consumers drive it via the model-spec notation's `version=` param"), and `ForkSubagents: false` genuinely never reaches `AuditForks` (`wait.go:256` gates on it), so no transcript scan happens for a caller that does not ask for one.

## Out-of-campaign

None. No reed defect surfaced while reviewing shuttle — reed's hardened `LoadState` error shapes, its foreign-session refusal, and its `Status` contract all composed correctly with shuttle in live driving (L8, L10, L12).
