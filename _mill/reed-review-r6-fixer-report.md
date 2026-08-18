# `reed` — fixer report, round 6 (`opus-high-r6`)

Every finding recorded in `_mill/reed-review-r6.md` is CLOSED. Nothing deferred.

| id | severity | fix commit | regression test | sabotage-proved |
| --- | --- | --- | --- | --- |
| R6-F2 | MEDIUM | `d67ad559` | `TestAdoptPaneGenerationLocked/a recorded session that is listed but does not answer the probe…` (hermetic) | yes |
| R6-F1 | LOW | `210ccb70` | `TestSmokeDiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume` (smoke) | yes |
| R6-F3 | MEDIUM | `fc2d3391` | `TestSmokeDownReportsTheSessionItAbandons` (smoke) | yes |
| R6-F4 | LOW | `106eacc3` | `TestDisplayMessageDoesNotErrorForAnAbsentSession` (integration) + a `parsePaneGeneration` case carrying tmux's real absent-session answer | pinned by construction, see below |
| R6-B1 | Part B gap | `f1da29cc` | `TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims` (hermetic) | yes |
| R6-B2 | Part B gap | `65ca1520` | `TestSmokeNoSessionMessageDistinguishesAnUnreadableStateFromAnEmptyOne` (smoke) | yes, both directions |
| — | sandbox suite | `b80a9676` | M25 added, M24 extended | n/a |

## What each fix changed

### R6-F2 — the foreign-session refusal no longer reads a failed probe as "gone"

`internal/reedengine/generation.go`.
Existence is now answered by `list-sessions`, which exits 1 only for an unreachable socket, instead of by `paneGenerationLocked`'s error — which, per the P1 measurements, does not mean "absent" at all.
A recorded session that IS listed but does not answer the identity probe now yields an actionable refusal instead of `nil`.
The three-way decision is `classifyRecordedSessionLocked` (added in R6-F3's commit, which is where the second consumer arrived).

`adoptPaneGenerationLocked`'s own fail-open on ITS probe is deliberately untouched — it is a genuinely two-sided trade, and its doc comment now says why that reasoning does not transfer to this call site.

**Verified:** the deterministic race-window reproduction from P4, re-run live against the deployed binary, now refuses BEFORE booting and leaves `list-sessions` showing only the original session (was: a bare `wt-moved` session deposited on the shared socket).
The ordinary refusal still fires, a fresh sibling worktree still boots normally on the shared socket, and the refusal is still escapable by the remedy it names (`resume` after `kill-session` → `ok:true, resumed:1`, strand live again).

### R6-F1 — the diagnostic verbs name the orphan

`internal/reedengine/lifecycle.go`, `requireSessionLocked`.
It now consults `refuseLiveForeignSessionLocked` on the state it had already loaded, before falling through to `noSessionMessage`.
No tmux round trip is added on the healthy path — the check returns immediately when the recorded session name is empty or is this worktree's own, which is every state file reed itself writes.

**Verified live:** `status` and `attach` in a renamed worktree now return the refusal naming the orphan, the socket and the `kill-session` remedy, instead of `no reed session (1 strands persisted); run "lyx reed resume"…`.

### R6-F3 — `down` names the live session it abandons

`internal/reedengine/lifecycle.go` (`DownResult.AbandonedSession`, detection before the state delete), `internal/reedcli/up.go` (envelope key + `Long` help).
It reports and warns; it does NOT kill.
The recorded name is this worktree's own former session after a rename but a SIBLING's live session after a hand-copied `.lyx` (R5-F4), and reed cannot tell them apart — killing would re-open R5-F4 under a different verb.
The state file is still deleted, so `down` stays the idempotent escape a tmux-less operator needs.

**Verified live:** `{"abandonedSession":"wt-south","ok":true,"session":"wt-east"}`, with the abandoned session still listed and its strand process still running afterwards; an ordinary `down` emits the unchanged `{"ok":true,"session":…}`.

### R6-F4 — the substrate contract is documented as measured, and pinned

`internal/reedengine/generation.go` (`paneGenerationLocked`, `parsePaneGeneration`), `internal/reedengine/lifecycle.go` (`serverPIDLocked`), `internal/reedengine/doc.go` (a new bullet in the multiplexer-contract surface).

The test is what makes this more than a comment edit: `TestDisplayMessageDoesNotErrorForAnAbsentSession` asserts the raw wire answer AND what `parsePaneGeneration` makes of it in one test, so the coupling cannot be broken from either end.
That is also why it is not "sabotage-proved" in the same sense as the others — there is no call site to remove; the test fails if tmux changes, if the parse rule is relaxed, or if the empty-field answer starts resolving to a live session.
The hermetic table additionally carries tmux's literal absent-session answer (`|2912080|`) as a rejection case.

### R6-B1 — `clearConflictingPaneBindings` pinned at its call site

`internal/reedengine/spawn_test.go`.
`Status` is the observable that isolates the reconcile-side layer: it reads the loaded table against live panes and never touches the render path, so `removeDuplicatePaneCells` cannot mask the assertion.
Both of R5-F3's live-observed conflict shapes are covered, plus a no-conflict control row so the test cannot pass by clearing indiscriminately.

**Sabotage:** with the call replaced by an empty slice, the hermetic suite FAILS (it was green before this test existed).

### R6-B2 — `noSessionMessage`'s split pinned at its call site

`internal/reedcli/smoke_staterecovery_test.go`.
It is a smoke test on purpose, and the reason is recorded in the test's own doc comment: the branch needs `hasSession` to answer `(false, nil)`, which needs a real `*exec.ExitError` with code 1, and `os.ProcessState` has no public constructor — so `execHook` cannot reach it. A real tmux supplies it for free (`has-session` against a socket with no server exits 1), and the test boots no session.

**Sabotage:** hard-coding the argument `true` fails the unreadable branch; hard-coding `false` fails the absent-file branch. Both directions proved.

## Verification at HEAD (`b80a9676`)

| gate | result |
| --- | --- |
| `go build ./...` | clean |
| `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | clean |
| `gofmt -l` over the three trees | no output |
| `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all `ok` |
| `go test ./...` (whole repo) | all `ok` |
| `go test -tags integration ./...` (whole repo) | all `ok` |
| `go test -tags smoke ./internal/reedcli/... -run Smoke -skip ClaudeResume` | `ok`, 25 top-level PASS / 1 SKIP / 0 FAIL |

The one SKIP is pre-existing and unrelated: `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` is the psmux-only last-pane-corpse shape, which tmux cannot produce.
Round 5's baseline was 22 top-level PASS; the three added smoke tests account for the difference.

Live driving was re-run against the DEPLOYED binary after every source change (`./deploy-dev` before each live scenario), never against a stale snapshot.

**Teardown:** `ps -eo comm | grep -cx 'tmux: server'` = **0**; `tmux -L lyx-lambdahub-HUB-1460bad0 ls` = "no server running"; `tmux ls` (the operator's default socket) = "no server running"; no leftover strand processes. Every session this round created was torn down.

## Docs updated in the same commits as the fix they document

- `internal/reedengine/doc.go` — a new multiplexer-contract bullet for display-message's exit-0 absent-session behaviour (R6-F2's commit).
- `internal/reedcli/up.go`'s `down` `Long` — the `abandonedSession` report (R6-F3's commit; CLI/Cobra Invariant help-accuracy obligation).
- `tools/sandbox/SANDBOX-REED-SUITE.md` — M25 new, M24 extended (`b80a9676`).

Not touched, deliberately: `CONSTRAINTS.md` (no new cross-cutting invariant — every change is inside reed's own existing scope), `docs/overview.md` (its reed bullet and its `.lyx/reed.json` pane-generation paragraph both remain accurate), and `manifest/roadmap.md` (hardening, not a planned item).

## Deferred

Nothing.

## Not fixed because they are not defects — recorded so the next round does not re-derive them

- **`adoptPaneGenerationLocked` still fails open on its own probe.** Verified live (P2) and left alone: clearing a healthy worktree's whole binding table over a tmux hiccup is strictly worse than the staleness it guards. What changed is that the code no longer implies the same trade applies to the refusal.
- **`serverPIDLocked` returning the shared server's pid for an absent session.** Traced through `Down` and it is harmless — the value is only spent after `list-sessions` came back empty. Only the comment was wrong, and it is now right.
- **Windows.** Nothing Windows-specific was driven; this host is Linux. Every substrate claim in the review and in the new comments is labelled tmux 3.6 on Linux. `TestDisplayMessageDoesNotErrorForAnAbsentSession` runs against whatever binary `LoadConfig` resolves, so psmux's answer to the same question would surface there rather than being assumed.

## Convergence recommendation

**Recommend convergence on reed after this round**, with one honest qualifier below.

The evidence: Part A found real defects, but every one of them is a *second-order* consequence of round 5's mechanism rather than a new class — the refusal's error handling, the message that routes an operator away from it, the report of what `down` walks away from, and a comment that described the substrate backwards.
Nothing this round destroys data, crosses a worktree boundary, or produces a false-healthy strand report; the two MEDIUMs are a residue and a silent abandonment, both now visible.
Six rounds have now hardened the same module and the findings have moved steadily from "reed does the wrong thing" (R1-F1, R2-F1, R5-F3) to "reed does the right thing but says the wrong thing about it".

The qualifier, stated rather than buried: the two Part B gaps were found by *sabotaging call sites*, not by reading code, and I found one more of that shape only because the prompt pointed me at it. A systematic wiring-level audit of all six rounds' fixes is explicitly out of scope here and remains the one un-swept surface I can name. If a seventh round is ever spent on reed, that audit — not another behavioural pass — is what I would spend it on, and it is mechanical enough to be worth a cheaper model than this one.
