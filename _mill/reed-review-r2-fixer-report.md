# reed — fixer report, round 2 (`opus-high-r2`)

Companion to `_mill/reed-review-r2.md`.
Every finding recorded in that review is fixed; **nothing is deferred**.

## Summary

| finding | severity | status | fix commit |
| --- | --- | --- | --- |
| R2-F1 — a worktree name containing `.`/`:` makes `reed up` hang 20 s, fail opaquely, and strand an untearable session on the shared hub server | BLOCKING | CLOSED | `ae96397f` |
| R2-F2 — `Down` takes descendant-closure reap roots from dead panes, so a recycled pid gets waited on and SIGKILLed | MEDIUM | CLOSED | `530a1602` |
| R2-F6 — the capability probe runs on tmux's global DEFAULT socket, starting a server outside reed's hub and racing every concurrent invocation | MEDIUM | CLOSED | `28c3aa0f` |
| R2-F3 — a hub resolving to the filesystem root yields a socket key carrying a path separator, which tmux cannot open and does not report | LOW | CLOSED | `e9ad525f` |
| R2-F4 — `ConfigTemplate`'s doc names a `claude` key reed's template does not have | NIT | CLOSED | `3a40c8cb` |
| R2-F5 — `reedcli` names the resolved `*lyxcwd.Location` `layout`, reviving the vocabulary wave-1 retired | NIT | CLOSED | `eff5eda6` |

Doc-only follow-on commit `6796de47` carries the `doc.go` / CLI-`Long` updates for R2-F1/R2-F3 — see "Process deviations" below for why they landed behind their fixes rather than inside them.

## Changed files

Production:

- `internal/reedengine/server.go` — `validateToldTmuxIdentity` (new), `socketSafeBase` (new), `ServerName` now separator-safe.
- `internal/reedengine/lock.go` — `withOpLock` runs the told-geometry pre-flight before acquiring the lock.
- `internal/reedengine/geometry.go` — the `SessionName` and `SocketKey` field docs now state the constraints the caller owns and name their enforcement site.
- `internal/reedengine/strand.go` — `safeReapRoot` (new, the single reap-root predicate), `alivePanePIDs` refactored onto it, `sessionReapRoots` (new).
- `internal/reedengine/lifecycle.go` — `panePIDsLocked` → `sessionReapRootsLocked`, filtering dead panes; `Down`'s snapshot comment; `waitProcessExit`'s doc comment corrected.
- `internal/reedengine/probe.go` — `probeCapabilityLocked` routed through the socket-scoped `TmuxCmd`; `os/exec` import dropped.
- `internal/reedengine/doc.go` — two additions to the multiplexer-contract section (the silent session-name rewrite; the `-V` vs `list-commands` server-contact distinction).
- `internal/reedengine/template.go` — `ConfigTemplate`'s doc corrected.
- `internal/reedcli/cli.go` — `layout` → `location`; the parent command's `Long` gains the session-name constraint.

Tests:

- `internal/reedengine/server_test.go` — `TestValidateToldTmuxIdentity_SessionName`, `TestValidateToldTmuxIdentity_SocketKey`, `TestWithOpLock_RefusesARewrittenSessionNameBeforeTouchingTmux`, `TestServerName_SocketSafeForAHubAtTheFilesystemRoot`; `/` added to `socketUnsafeChars`.
- `internal/reedengine/strand_test.go` — `TestSessionReapRoots`.
- `internal/reedengine/probe_test.go` — `TestProbeCapabilityLocked_GoesThroughTheSocketScopedTmuxCmd`.
- `internal/reedengine/contract_integration_test.go` — `TestSessionNameRewriteIsSilentAndExactTargetsMissIt`.
- `internal/reedcli/smoke_lifecycle_test.go` — `TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite`; one stale `layout.Hub` cross-reference reworded.

No change to `CONSTRAINTS.md` (no new cross-cutting invariant — every rule added is reed-internal and documented in `geometry.go`/`doc.go`), to `docs/overview.md` (neither the module table nor the execution stack moved), or to `manifest/roadmap.md` (this is hardening, which the repo rules explicitly exclude from the roadmap).

## Sabotage proofs

Every new regression test was proved to fail against the reverted fix, then the fix restored and the test re-run green.

| test | sabotage | observed failure |
| --- | --- | --- |
| `TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite` | removed the `validateToldTmuxIdentity` call from `withOpLock` | FAIL after **20.3 s**: `socket lyx-warp-bare-HUB-73c535d7 carries session "rewritable_v2" after the refusal (sessions: [rewritable_v2 warp-bare])` — the exact stray, at the intended assertion |
| `TestWithOpLock_RefusesARewrittenSessionNameBeforeTouchingTmux` | same | FAIL: `withOpLock with a rewritten session name = nil; want a refusal` |
| `TestSessionReapRoots` | `sessionReapRoots` reverted to `p.PID > 0` | FAIL: `sessionReapRoots(...) = [100 200 300]; want [100 300]`, plus the all-dead case |
| `TestProbeCapabilityLocked_GoesThroughTheSocketScopedTmuxCmd` | restored the direct `exec.Command(e.cfg.Tmux, …)` | FAIL: `fork/exec …/does-not-exist-tmux.exe: no such file or directory` |

## Verification

All gates run against the **re-deployed** `.dev-bin/lyx` where live driving is involved.

Hermetic / tagged:

| gate | result |
| --- | --- |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | PASS |
| `go test ./...` (whole repo) | PASS |
| `go test -tags integration ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | PASS |
| `go test -tags smoke ./internal/reedcli/...` (18 tmux-only Smoke tests + the new one) | PASS, 1 documented SKIP (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, psmux-specific premise) |
| `go test -tags smoke -run TestSmokeClaudeResumeRecallsCodeword$` | PASS, 10.10 s, ONE real `claude` process, `--model haiku` per the campaign's cost discipline — run once, deliberately, by exact name, never folded into a sweep |

**3× concurrent smoke sweep** (tmux-only; `TestSmokeClaudeResumeRecallsCodeword` excluded, never run N-concurrent):

- Before the R2-F6 fix: **2 failures in 9 suite runs**, both `{"error":"run list-commands: exit status 1"}` aborting `up`, in two different tests.
- After: **9/9 clean** across three further sweeps.

Live re-driving after re-deploy (each scenario from the round's own fixture — a hub-wide deep three-segment anchor `apps/web/svc`, two hubs live at once, four worktrees including a prefix-colliding pair and a dot-named one):

| re-drive | before | after |
| --- | --- | --- |
| `up` in the dot-named worktree | 20.1 s hang → opaque timeout → stray `svc_v2` session on the shared server; `down` returned `ok:true` and killed nothing | **0.010 s**, `{"error":"tmux will not create session \"svc.v2\" verbatim: it contains \".\" … rename the worktree directory \"…/svc.v2\" …"}`; no session created; `status`/`down`/`resume` all report the same real reason instead of a false success |
| `up` on a fresh hub from the deep anchor | ok, `_board/.lyx/logs` created | unchanged |
| two worktrees + two hubs | one socket per hub, shared across worktrees | unchanged |
| strand pane cwd (payload's own `pwd`) | the told deep anchor | unchanged |
| `remove` in one worktree | sibling's child untouched | unchanged |
| prefix collision (`bet` vs `beta`) | `beta` fully intact | unchanged |
| crash/rebirth (`kill-server`, then `resume` in both worktrees) | `resumed:1` each, fresh header + strands | unchanged |
| header keepalive text | ` hub: <hub>` from the pane's own re-entrant CLI | unchanged |
| foreign pane | ignored by `status`, reaped by `up` | unchanged |
| `down` with a SIGHUP-immune descendant, both worktrees loaded | immune children of the downed worktree gone on return (15.1 s graceful window then force-kill); sibling's untouched | unchanged |
| 5 concurrent `add`s | all five distinct, layout consistent | unchanged |
| remove the sole strand, then `add` | header keeps the session; second add live | unchanged |
| `up` + `down`, watching `/tmp/tmux-<uid>/default` | default socket CREATED by every boot | **never created** |

**Teardown: confirmed zero stray state.** No tmux server processes, no stray `sleep` payloads, no stray `lyx` processes, both fixture sockets report `no server running`, and the operator's default tmux socket is absent.

## Process deviations, stated plainly

1. **R2-F6 was found during Job 2, not Job 1.** The 3× concurrent verification sweep surfaced it after the first five fixes had landed. It was written into `_mill/reed-review-r2.md` (commit `e6e284d6`) with its provenance labelled at the finding itself, then fixed like the rest. Strict A-before-B was kept for the five findings the clean-room pass produced; this one could not have been, and hiding it in the fixer report would have been worse than recording the deviation.
2. **The doc updates for R2-F1/R2-F3 landed one commit behind their fixes** (`6796de47`), not inside them. The commit-per-fix rule asks for the doc update in the same commit; that was missed and is recorded here rather than papered over by rewriting history mid-round.
3. **R2-F5's scope was held to `internal/reedcli`.** The same `layout`-for-`Location` identifier survives in `internal/burlercli`, `internal/shuttlecli`, `internal/perchcli` and `internal/webstercli`. `shuttlecli` is explicitly out of scope this round and the other three are not reed; renaming them is left as a follow-up rather than smuggled in.

## Deferred

**Nothing.** All six findings are fixed and verified.

## Not verified, and why

- **Windows / psmux.** Linux host. `proctree_windows.go`, the psmux version floor, and the psmux-only silent-split shape are compile-checked and unit-tested only; `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` self-skips. Named, not driven, per the round's brief. Note for a Windows pass: R2-F1's ban and R2-F3's substitution both treat `/` and `\` on every GOOS by design, but tmux's session-name rewrite set was measured on tmux 3.6 only — a psmux run should re-confirm it.
- **R2-F3 end to end.** Creating a git repo directly under `/` needs root. Both halves were confirmed independently (the derivation, and tmux's response to a separator-carrying `-L` key); the joined path was not driven.
- **R2-F2's pid-reuse trigger.** The mechanism was confirmed live (tmux reports a dead pane's `#{pane_pid}` for an exited process) and the exclusion is unit-pinned, but forcing the OS to recycle a specific pid was not attempted, so the harm is argued rather than reproduced.
- **`attach`'s interactive tail.** Driven only to the terminal handover; a real attached session needs a tty and an operator. `TestSmokeAttachRendersInsideHarnessPane` covers the rendered-inside-a-harness-pane half and passes.

## New sandbox-suite scenarios worth adding (not added this round)

`tools/sandbox/SANDBOX-REED-SUITE.md` already carries a `**Covers:** reed` tag, so the Sandbox Suite Coverage invariant is satisfied and no edit was required. Two scenarios this round exercised are not in it and would be worth adding when that file is next touched:

- **A worktree name tmux would rewrite** (`svc.v2`): `lyx reed up` must refuse immediately and name the directory, and the hub's socket must carry no rewritten session afterwards. This is a visible, operator-facing behaviour the black-box suite is the right home for.
- **`lyx reed up` must not create `/tmp/tmux-<uid>/default`** — a one-line check that reed never starts a server on the operator's own default socket.
