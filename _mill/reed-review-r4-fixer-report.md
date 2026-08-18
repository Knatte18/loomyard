# `reed` — round 4 fixer report (`opus-medium-r4`)

Fixes for every finding recorded in `_mill/reed-review-r4.md`. Commit-per-fix on `reed-shuttle-crucible-hardening`; nothing pushed.

## Findings closed

| Finding | Severity | Fix commit | One-line what |
|---|---|---|---|
| R4-F1 | MEDIUM | `621165ce`, `b3c74d54` (message follow-up) | `validateToldTmuxIdentity` now refuses a told session name carrying `\`, tmux's third silent rewrite class (vis(3) doubles the backslash) — the R2-F1/R3-F1 shape, third character class |
| R4-F4 | MEDIUM | `9ec33880` | the header rebuild retries once behind a `select-layout even-vertical` re-tile, so an untracked one-row header band at the top can no longer wedge `up`/`resume` permanently |
| R4-F3 | LOW | `cb935402` | `validateToldAnchorPath` backstops the third told field whose bad value fails silently: a non-absolute `AnchorPath` is refused at `withOpLock` instead of succeeding against the wrong tree |
| R4-F5 | LOW | `be746b85` | strand-pane adoption is narrowed to the sole-alive-non-header-pane case, so `add` never guesses which of several untracked panes is idle and silently types a command into a busy one |
| R4-F2 | NIT | `b3f58a2b` | `ServerName`'s readable half is bounded at 48 bytes (rune-safe), so a long hub directory name still yields a socket key tmux can open |
| — | — | `bd20bbec` | `tools/sandbox/SANDBOX-REED-SUITE.md` gains M22, the scrubbed-`reed.json` recovery scenario R4-F4/R4-F5 surfaced |

**Deferred: nothing.** Every recorded finding is fixed, all severities.

## Per-finding detail

### R4-F1 — backslash session names (MEDIUM)

`internal/reedengine/server.go`. New `doubledSessionNameChars` constant and its branch in `validateToldTmuxIdentity`. The branch runs **last**, after the vis-encode check, for a reason recorded in the constant's doc: its message prints both names with `%s` rather than `%q`, so an operator sees the directory they must actually rename instead of a rendering that doubles the very backslash at issue — and printing unescaped is only safe once control characters, DEL, and invalid UTF-8 have already been refused. (The first cut used `%q` and produced `rename the worktree directory "…/a\\b"` for a directory named `a\b`; caught by the new smoke row and fixed in `b3c74d54`.)

Docs updated in the same commits: `geometry.go`'s `SessionName` contract, `doc.go`'s "Silent session-name rewriting" bullet (now stating all three classes and that the printable-ASCII sweep proves the set complete), `reedcli/cli.go`'s `reed` group `Long` help (CLI/Cobra Invariant's help-accuracy obligation), and `SANDBOX-REED-SUITE.md`'s M20.

**Tests.** Three rows added to `TestValidateToldTmuxIdentity_SessionName` (backslash mid-name, trailing, and beside an already-banned dot) plus a passing row proving quote/dollar/backtick are still left alone. `TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite` is now table-driven, one row per rewrite class, with the backslash row skipped on Windows (no directory can carry a path separator there). Its message assertion now decodes the JSON envelope instead of substring-matching raw JSON, since the backslash is escaped on the wire.

**Verification.** Sabotage-proved: emptying `doubledSessionNameChars` fails both dedicated unit rows. Live, on a real hub with a worktree directory named `bs\slash` — before: `reed up` hung 20.05 s, failed with `tmux server is up but session "bs\\slash" did not materialize within 20s`, and left `bs\\slash` squatting on the shared server that `reed down` reported `ok:true` while failing to kill. After: every verb refuses in **0.009 s** with `tmux will not create session "bs\slash" verbatim: it contains a backslash, which tmux silently doubles to "\\" — rename the worktree directory "…/bs\slash" so its name carries no backslash`, and the shared server's session list is untouched.

### R4-F4 — wedged header rebuild (MEDIUM)

`internal/reedengine/lifecycle.go`. The split moved out of `ensureHeaderPaneLocked` into two new helpers: `splitPaneAboveLocked` (the `-b` split plus the `validateSplitCreatedNewPane` guard, carrying the load-bearing why-`-b` comment verbatim) and `splitHeaderPaneAtTopLocked` (the retry). `topmostPaneID` replaces the inline min-by-`pane_top` loop. On a failed first split the retry issues `select-layout -t <session>: even-vertical`, re-enumerates, recomputes the topmost target, and splits once more; on a failed retry the **first** error is returned, because it describes the state the operator actually has.

No capability-contract change: `select-layout` and `split-window` are both already in `requiredSubcommands`. No reed layout string is computed on this path, so `anyPlacedStrand`'s empty-layout hazard is not in play.

**Tests.** `TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit` drives the exact substrate shape through the `execHook` seam (a one-row pane at `pane_top` 0, a tall pane below), asserting the re-tile is actually issued, that exactly two split attempts happen, and that the retried split's pane becomes the header. `TestTopmostPaneID` pins target selection by `pane_top` rather than list order. `TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp` is the end-to-end guard: up, add, delete `reed.json`, then `up` must exit 0, the rebuilt header must sit at `pane_top` 0, and a further `add` must still work.

**Verification.** Sabotage-proved: reverting the retry fails the hermetic test on its `want nil` assertion and the smoke test with the real `{"error":"split header pane: exit status 1: no space for new pane"}`. Live, on a hub whose `.lyx/reed.json` was deleted with the session up and a one-row header laid out — before: `up` and `resume` failed permanently. After: `up` returns `ok:true` (with a `Warn` trace naming the retry), the new header lands at `pane_top=0`, and the following `add` re-establishes reed's real layout (header `h=1` at top, strand `h=48`).

### R4-F3 — told `AnchorPath` backstop (LOW)

`internal/reedengine/server.go` gains `validateToldAnchorPath`; `lock.go`'s `withOpLock` calls it immediately after `validateToldTmuxIdentity`, before `os.MkdirAll` — which is the point, since creating `.lyx` is the very act that would otherwise litter the caller's working directory. Kept as a second, separately named function rather than widened into `validateToldTmuxIdentity`, so each name stays true to what it checks. `geometry.go`'s `AnchorPath` contract now states the absoluteness requirement and names its backstop.

**Tests.** `TestValidateToldAnchorPath` (absolute forms pass; empty, bare-relative, dot-relative, parent-relative refused) and `TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState`, which mirrors the existing session-name ordering test by asserting the op body never runs and no lock file is created.

**Verification.** Hermetic only, deliberately: hub mode cannot reach this (`hubgeom.ReedGeometry` always passes the absolute `Location.AnchorPath()`), exactly as with the socket-separator branch beside it. Stated as a limit, not claimed as live-verified.

### R4-F5 — adoption of one of several untracked panes (LOW)

`internal/reedengine/spawn.go`. `planPaneTarget`'s adopt branch now calls the new `soleAliveNonHeaderPane`, which reports false both when there is no candidate and when there are several. With more than one candidate the planner splits instead — a new pane's shell is idle by construction — and the reconcile tail reaps the leftovers once a strand is bound.

**Tests.** Two rows on `TestPlanPaneTarget`: several untracked alive panes must split the tallest, and the narrowing must NOT reach the case adoption exists for (header + one alive pane + a dead corpse still adopts).

**Verification.** Sabotage-proved on the "several" row. Live, on the post-scrub state: before, `add` adopted the previous header pane still running `lyx reed header --blocking`, `capture-pane` showed `sleep 6666` typed onto its screen unexecuted, `status` said `"live":true`, and no such process existed. After, `add` splits a fresh pane, `pgrep -a -x sleep` shows `sleep 6666` genuinely running, and the layout is correct (header `h=1` at `top=0`, strand `h=48`).

### R4-F2 — unbounded socket key (NIT)

`internal/reedengine/server.go`. `ServerName` now truncates the readable half through `truncateAtRuneBoundary` at `maxSocketSafeBaseBytes` (48). Bounding in **bytes** rather than runes is deliberate — the kernel's `sun_path` limit is a byte limit, so a byte cap makes the resulting ≤61-byte key a proof rather than an ASCII-only approximation — and the cut lands on a rune boundary so a multi-byte hub name is never left half-written. This is the same substitute-at-the-derivation posture `socketSafeBase` already takes, and equally collision-free: the hash half is untouched.

**Tests.** `TestServerName_BoundedForALongHubBasename` (a 200-character basename must stay within the bound, and two hubs sharing it must not collide) and `TestTruncateAtRuneBoundary` (including the straddling-rune and exactly-at-limit cases, with a `utf8.ValidString` assertion on every row).

**Verification.** Live: a hub with an 85-character basename went from unbootable (`run list-commands: exit status 1: error connecting to … (File name too long)`) to `up`/`add`/`status`/`down` all succeeding in 0.15 s, with the identity hash `85094ad1` unchanged.

## Gates, all green after the last fix

| Gate | Result |
|---|---|
| `go build ./...` | clean |
| `go vet` on `reedengine`/`reedcli`/`hubgeom`, plus `-tags smoke` and `-tags integration` | clean |
| `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all `ok` |
| `go test -tags integration -count=1` on `reedengine`/`reedcli` | all `ok` |
| `go test -tags smoke -run Smoke -skip ClaudeResume -count=1` | 23 PASS, 1 SKIP (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, psmux-only), 31.5 s |
| 3× CONCURRENT copies of that smoke suite | all three `rc=0` |
| `TestSmokeClaudeResumeRecallsCodeword`, run deliberately by exact name | PASS, 10.1 s, on `--model haiku` via the pinned `smokeClaudeModel` constant — one `claude` process, as budgeted |

Every live scenario from the review (L1–L11) was re-driven end to end against the redeployed binary after the last fix: fresh-hub first boot with `<hub>/_board/.lyx/logs` created, add/status, grandchild reap on `remove` with a sibling strand untouched, crash + `resume`, cross-worktree scope, the prefix-sibling double-`down`, `header`, and `attach`.

**Teardown.** Zero tmux servers afterwards: `ps -eo comm | grep -cx 'tmux: server'` = 0, and `tmux -L <socket> ls` reports `no server running` for every socket used. See the review report's "Correction to the prompt's teardown probe" — `pgrep -x tmux` reports 0 even while a server IS running, because the server's `comm` is `tmux: server`; it must not be used alone as evidence of a clean teardown. No stray strand processes remain. The `/tmp/r4crucible` fixture tree itself is left in place (its removal was not permitted from this session); it holds no processes and no tmux state.

## Files changed

- `internal/reedengine/server.go` — backslash refusal class, `validateToldAnchorPath`, `ServerName` bound + `truncateAtRuneBoundary`.
- `internal/reedengine/lifecycle.go` — `splitHeaderPaneAtTopLocked`, `splitPaneAboveLocked`, `topmostPaneID`.
- `internal/reedengine/spawn.go` — `soleAliveNonHeaderPane`, narrowed adoption.
- `internal/reedengine/lock.go` — anchor pre-flight at `withOpLock`.
- `internal/reedengine/geometry.go`, `internal/reedengine/doc.go` — contract/doc updates in the same commits as their fixes.
- `internal/reedcli/cli.go` — `reed` group `Long` help.
- Tests: `internal/reedengine/server_test.go`, `lifecycle_test.go`, `spawn_test.go`, `internal/reedcli/smoke_lifecycle_test.go`.
- `tools/sandbox/SANDBOX-REED-SUITE.md` — M20 extended, M22 added (`sandbox_coverage_test.go` green).
- `_mill/reed-review-r4.md`, `_mill/reed-review-r4-fixer-report.md`.

No change to `docs/overview.md`, `CONSTRAINTS.md`, or `manifest/roadmap.md`: no invariant moved, no scope moved, and this is a hardening pass, which the roadmap deliberately does not record.

## Convergence recommendation — I would spend one more round

**Merge-ready: yes.** Every gate is green, every finding is fixed with a sabotage-proved regression test, and correctness in the normal single-instance flow — the stated merge bar — was re-driven live end to end.

**Converged: not quite, and I would not declare it here.** The rotation was planned to end at round 4, and the honest reading of what round 4 produced argues against taking that exit:

- The round found **more** than round 3, not less: 2 MEDIUM + 2 LOW + 1 NIT against round 3's 1 MEDIUM + 1 NIT. The trend line the campaign was converging along did not continue.
- More importantly, R4-F4/R4-F5 are a **new defect class**, the first in two rounds. Rounds 2 and 3 both found instances of the told-identity family; R4-F1 is a third instance of that same family, and its exhaustive printable-ASCII sweep now closes it as a family. But R4-F4 came from somewhere else entirely — *state-loss recovery*, a surface no round has systematically probed — and R4-F5 came out of verifying R4-F4's fix. That is the classic signature of an under-explored area, not of a converged one.
- Concretely, the state-loss surface has more in it than I drove. I probed `reed.json` deleted with the session up. Not probed: `reed.json` truncated or corrupted mid-write; `reed.json` restored from an older copy while panes moved on; `.lyx` deleted wholesale including the lock files while an op is in flight; a `reed.json` written by one worktree read after that worktree was renamed. R4-F5 in particular shows a false `live:true` is reachable in this area, which is the one symptom class an orchestrating caller (shuttle, next campaign) cannot defend against.

**Recommendation.** Merge this branch — it is strictly better than what it started from. Then spend **one more round scoped narrowly to state-loss and state-corruption recovery**, rather than a fifth general pass; a general pass would likely re-walk the surfaces four rounds have now covered thoroughly. If that round comes back clean, reed is done with real evidence behind the claim rather than a rotation counter running out.
