# `reed` — independent review, round 4 (`opus-medium-r4`)

Clean-room round-4 review of `internal/reedengine` + `internal/reedcli` + `internal/hubgeom`, per `_mill/reed-review-prompt.md`.
Findings below were formed before any prior-round material was opened (one accidental exposure is disclosed in "Clean-room hygiene" at the end).

**Result: 2 MEDIUM, 2 LOW, 1 NIT — all CONFIRMED, none BLOCKING.**
Both MEDIUMs are live-reproduced defects reachable from ordinary operator actions, and none of the five is a wave-1 regression.
R4-F5 surfaced during Job 2's own verification of R4-F4's fix and could not have arrived during Job 1; its provenance is stated plainly in its own section, per the sequencing rule's round-2 precedent.
Convergence recommendation is at the bottom.

## What was tested

### Hermetic gates (baseline, before any edit)

| # | Command | Result |
|---|---|---|
| H1 | `go build ./...` | clean |
| H2 | `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | clean |
| H3 | `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all `ok` |
| H4 | `go test -tags integration -count=1 ./internal/reedengine/... ./internal/reedcli/...` | all `ok` (the real-tmux wire-contract tier) |
| H5 | `go test -tags smoke ./internal/reedcli/... -run Smoke -skip ClaudeResume -v -count=1` | 18 PASS, 1 SKIP (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, psmux-only), 30.7 s |
| H6 | 3× CONCURRENT copies of H5, launched simultaneously | all three `rc=0`; `pgrep -x tmux` = 0 afterwards |

`TestSmokeClaudeResumeRecallsCodeword` was deliberately excluded from H5/H6 per the campaign's cost declaration; it is not implicated by any finding below, and no ad-hoc `claude` process was spawned this round.

### Substrate probes — tmux 3.6 wire behaviour (own scratch sockets, all torn down)

| # | Scenario | Observation |
|---|---|---|
| P1 | `new-session -d -s 'a\b'` then `has-session -t '=a\b'` | **new-session exits 0**; `list-sessions` reports `a\\b`; the exact target `=a\b` fails, exit 1, `can't find session: a\b`. tmux vis-encodes the backslash (`vis(3)` doubles `\` unless `VIS_NOSLASH`, which tmux's `session_check_name` does not set). Same silent-rewrite shape as `.`/`:` and the control-char class. |
| P2 | **Exhaustive sweep** of every printable ASCII byte `0x20`–`0x7E` as an `x<c>y` session name, each round-tripped through `has-session -t '=x<c>y'` | Exactly **three** mismatches: `x.y`, `x:y`, `x\y`. The first two are already refused by `validateToldTmuxIdentity`; `\` is not. Everything else in the printable range passes verbatim (space, `"`, `#`, `$`, `'`, `` ` ``, `*`, `%`, `-`…). |
| P3 | 400-character session name; `svc-åäö-⚙` | Both created and exact-matched verbatim — no session-name length limit, valid multi-byte UTF-8 untouched. Confirms the vis check refuses only the encode class, never unicode. |
| P4 | `-L <key>` at key lengths 88/90/92/93/94/100/250 | 92 characters is the last key that works; **93+ fails `(File name too long)`**. `/tmp/tmux-1000/` is 15 bytes and `sockaddr_un.sun_path` is 108, so the ceiling is `107 − len(socketdir)`. Unlike the other malformed-identity cases this one exits **1**, not 0. |
| P5 | `split-window -b -t <1-row pane>`; then `select-layout even-vertical` + retry | Naive split: `no space for new pane`, exit 1. After `even-vertical` the same split succeeds and the new pane lands at `pane_top=0`. Basis for R4-F4's fix. |

### Live driving — the deployed dev binary against real hubs (no launcher, `sandbox-reed-suite.cmd` never invoked)

Fixture: `/tmp/r4crucible/r4main-HUB/{wt-alpha,wt-beta,wt,bs\slash}`, each a real git worktree directly under the hub, plus a second hub with an 85-character basename. Deliberately different from every prior round's fixture choice.

| # | Scenario | Observation |
|---|---|---|
| L1 | `lyx reed up` on a hub with no `_board/.lyx` at all | `ok:true` in 0.16 s; `<hub>/_board/.lyx/logs` created; session `wt-alpha`, socket `lyx-r4main-HUB-b8cc8b6a`; header pane `%1` running the real `lyx` keepalive at `top=0`, initial pane `%0`; both panes' `pane_current_path` == the told anchor. `fabricengine.HubLogsDir` ownership move holds on a genuine first boot. |
| L2 | Two `add`s, one of them a `bash -c "sleep 4000 & sleep 5000"` grandchild tree | `pstree` confirms `pane_pid → bash → {sleep,sleep}`; both strands `live:true`. |
| L3 | `remove` the grandchild-owning strand | Returned in 0.029 s; **both grandchildren gone**, the sibling strand's `sleep 4321` untouched. Round-1 F1's pane-child reap holds through the told-geometry path. |
| L4 | Crash/rebirth — `SIGKILL` the tmux server, then `resume` | `{"ok":true,"resumed":1}` in 0.17 s; header rebuilt fresh, pane ids reset (`%1` header, `%0` strand), pane cwd still the told anchor. |
| L5 | Cross-worktree scope — `wt-beta` up with a strand, then `down` in `wt-alpha` | Only `wt-alpha` died; `wt-beta` session and its `sleep 9876` survived; server survived. |
| L6 | Prefix boundary — session `wt` beside session `wt-beta`; `down` in `wt`, twice | `wt` gone, `wt-beta` and its strand alive after BOTH downs (the idempotent second `down` is the load-bearing case `exactSessionTarget` exists for). |
| L7 | `lyx reed up` in the worktree directory literally named `bs\slash` | **20.05 s hang**, then `{"ok":false,"error":"tmux server is up but session \"bs\\\\slash\" did not materialize within 20s"}`. `list-sessions` shows a session `bs\\slash` **squatting on the shared per-hub server**. → R4-F1 |
| L8 | `lyx reed down` in that same worktree | `{"ok":true,"session":"bs\\slash"}` — reports success while the stranded session survives untouched; `status` says "no reed session". No reed verb can reach it. → R4-F1 |
| L9 | `lyx reed up` under an 85-character hub basename | Fails in 0.105 s with `run list-commands: exit status 1: error connecting to /tmp/tmux-1000/lyx-hhh…-HUB-85094ad1 (File name too long)`. **Fails fast and names the path** — R2-F6's TmuxCmd routing is what buys that. → R4-F2, downgraded to NIT on this evidence |
| L10 | Session up with a strand, then `rm .lyx/reed.json` (what `git clean -xdf` in the worktree does — `.lyx` is never-tracked machine-local scratch by invariant) | `up` AND `resume` both hard-fail, permanently: `split header pane: exit status 1: no space for new pane`. `status` still reports the session healthy. Only escape is `down` + `up`, which the message never names. → R4-F4 |
| L11 | `lyx reed header`, `lyx reed header --blocking` (live in the pane), `lyx reed attach` before/after `up` | `header` renders `hub: /tmp/r4crucible/r4main-HUB` from told `Geometry.HubPath`; the live header pane's `capture-pane` shows the same text; `attach` pre-flight before `up` returns the friendly no-session envelope; `attach` after `up` reaches the right server (tmux answers `open terminal failed: not a terminal`, i.e. socket+session resolved, only the tty handover failed). |
| L12 | Teardown discipline | After every scenario: `pgrep -x tmux` = 0, no stray `sleep` children. `pgrep -a -x tmux` used throughout rather than `ps aux | grep tmux`. |

### Re-verification of prior campaign invariants (all still hold)

- Pane-**child** reap on `remove` — L3. On `down` — H5's `TestSmokeDownReapsPaneChildProcesses` + `TestSmokeDownForceKillsSighupImmunePaneChildren`.
- Crash/rebirth — L4.
- Cross-instance/cross-worktree scope — L5, L6.
- `attach` / `header --blocking` socket+session resolution — L11.
- **`TmuxCmd` discipline, own independent sweep**: `grep` for `exec.` across `internal/reedengine/*.go` and `internal/reedcli/*.go` (non-test) yields exactly three non-`TmuxCmd` process spawns, and all three are correct by construction: `lifecycle.go:300` (the server spawn, which must set `cmd.Dir`/`cmd.Env`/`proc.Detach` and passes `-L e.Socket()` explicitly), `reedcli/attach.go:53` (the stdio-handover child, passes `-L c.eng.Socket()`), and `proctree_windows.go` (a PowerShell process-table query, not tmux at all). No indirect call through a function value either — the only injected `run func(...)` is `probeCapability`'s, bound to `e.tmux.output`. Confirms rounds 2 and 3.
- **Reap-root purity**: `safeReapRoot` still has exactly two call sites (`alivePanePIDs`, `sessionReapRoots`), and both snapshots are taken under `withOpLock` before any kill. The residual TOCTOU on a *descendant* pid recycled between snapshot and reap is named honestly in `waitProcessExit`'s own doc comment and is not closable without pidfd; not recorded as a finding.
- **`reed.json` persisted-field audit (the R3-F2 shape, elsewhere)**: `Socket`/`Session` re-stamp on every load (R3-F2's fix, verified in L1's on-disk state); `StrippedEnv` is written only on a boot that actually spawned a server, which is exactly what it diagnoses; `HeaderPaneID` and every `Strand` field are read back on live paths. No second stale-diagnostic field.

## Findings

### R4-F1 — `validateToldTmuxIdentity` misses tmux's THIRD session-name rewrite class: the backslash — MEDIUM, CONFIRMED

`internal/reedengine/server.go:121-145` (character-class constant at `server.go:70`, vis-class helper at `server.go:88`).

`validateToldTmuxIdentity` refuses two rewrite classes: the `.`/`:` substitution (`rewrittenSessionNameChars`, R2-F1) and the vis-encode class of ASCII control characters / DEL / invalid UTF-8 (`firstVisEncodedSessionNameByte`, R3-F1).
It misses a third. `vis(3)` **doubles the backslash itself** (`\` → `\\`) unless `VIS_NOSLASH` is set, and tmux's `session_check_name` does not set it.
`\` is `0x5C` — above `0x20`, not DEL, and valid UTF-8 — so `firstVisEncodedSessionNameByte` passes it, and it is not in `rewrittenSessionNameChars`.

**Failure scenario (inputs/state → wrong behaviour).**
A worktree directory named `bs\slash` — `\` is a legal POSIX filename byte, meaningless to the kernel and to git's worktree machinery — yields `Geometry.SessionName == "bs\slash"`. The pre-flight passes it.
`ensureServerAndSessionLocked` runs `new-session -d -s 'bs\slash'`, which **exits 0 and creates a session named `bs\\slash`** (P1, L7).
Every session target this package issues is the exact-match `=<name>` form, so `hasSession` returns exit 1 forever: `up` hangs the full 20-second attempt window and fails with `tmux server is up but session "bs\\slash" did not materialize within 20s`, naming neither cause nor remedy (L7).
The rewritten session is then left **squatting on the SHARED per-hub server**: `down` issues `kill-session -t '=bs\slash'`, which misses it, and reports `ok:true` anyway (L8). No reed verb in any worktree can address or tear it down; it survives every teardown until an operator reaches for raw tmux.

This is R2-F1's failure shape and blast radius exactly, with a different character.

**Severity.** MEDIUM, calibrated against R3-F1 rather than R2-F1: identical consequences, but lower reachability — `fabric add` mints git-ref-safe slugs, so reaching it takes a hand-created or hand-renamed worktree directory.

**Suggested fix.** Fold `\` into the same `withOpLock` pre-flight. It belongs with `rewrittenSessionNameChars` in mechanism (a fixed, enumerable set tmux rewrites) but not in message (doubled, not substituted with `_`), so a separately named constant with its own error line reads truer than overloading either existing branch. Refuse, never sanitize, for R2-F1's reason: substituting would map two sibling worktrees onto one session.
P2's exhaustive printable-ASCII sweep is what lets the fix claim completeness rather than "one more character": `.`, `:`, `\`, plus the control/DEL/invalid-UTF-8 class, is the entire rewrite surface of tmux 3.6's `session_check_name`. The doc comments and the `lyx reed` help text should say so.

### R4-F2 — `ServerName` bounds nothing, so a long hub directory name yields an unusable tmux socket key — NIT, CONFIRMED

`internal/reedengine/server.go:36-42`.

`ServerName` builds `"lyx-" + filepath.Base(hubPath) + "-" + <8 hex>` with no length bound.
A `-L` key is a filename inside the per-user socket directory and the whole path must fit `sockaddr_un.sun_path` (108 bytes); P4 measured this host's live ceiling at 92 characters. `ServerName`'s key is `len(base) + 13`, so a hub directory whose basename is ≥ 80 characters is unusable.

**Failure scenario.** `lyx reed up` under such a hub fails (L9). **Recorded honestly against my own initial hypothesis:** I expected a 90-second cause-free spiral through the boot loop. It is not — the capability probe runs first and, because R2-F6 routed it through `TmuxCmd`, it fails in **0.105 s** carrying tmux's own stderr, which names the exact offending socket path. That is a genuinely actionable failure, which is why this is a NIT and not the LOW I first wrote down.

What remains is that the configuration is unusable at all, for a reason nothing prevents at its derivation, and that the message frames it as a `list-commands` capability problem rather than as "your hub directory name is too long".

**Suggested fix.** Bound the readable half at its derivation, exactly as `socketSafeBase` already substitutes separators there rather than refusing. `ServerName`'s own doc already states the basename half "is purely for human readability — uniqueness rests entirely on the hash", so truncation cannot collide and needs no refusal path or error plumbing. A 48-rune cap keeps every key ≤ 61 characters — comfortable headroom even for a long `TMUX_TMPDIR` — and leaves every ordinary hub name byte-identical.

### R4-F3 — the identity pre-flight backstops 2 of 7 told fields, and skips the one whose failure is silent — LOW, CONFIRMED by trace

`internal/reedengine/server.go:121-145`, `internal/reedengine/lifecycle.go:33-35` (`stateDir`), `internal/reedengine/geometry.go:32-36`.

`Geometry`'s contract is "New validates nothing; the caller owns every field", with `validateToldTmuxIdentity` as the documented **backstop** — its own doc says the socket branch "binds the standalone tellers that will populate a Geometry themselves".
That backstop covers `SocketKey` and `SessionName`. `AnchorPath` is the third field with a failure a caller cannot observe: `stateDir()` is `filepath.Join(geom.AnchorPath, ".lyx")`, so an empty or relative `AnchorPath` silently resolves `reed.json`/`reed.lock` **against the lyx process's own working directory** instead of the worktree — and the same value is passed as `-c` to all three tmux spawn sites, where an empty `-c` makes tmux fall back to the invoking client's cwd. That is precisely the failure `launchStrandLocked`'s own comment says the explicit `-c` exists to prevent ("every strand command would run against the wrong tree while reed reported success"). No error is raised at any point.

Hub mode cannot reach it (`hubgeom.ReedGeometry` always passes the absolute `l.AnchorPath()`), so this is a contract-backstop gap rather than a live defect — the same standing the socket-separator branch already has. Worth closing now rather than when the first standalone teller lands, since that teller is the exact caller the backstop was written for.

**Suggested fix.** Extend the same pre-flight: `AnchorPath` non-empty and `filepath.IsAbs`. Same chokepoint, same refuse-don't-repair posture, no new validation seam.

### R4-F4 — a header rebuild against a 1-row top pane is impossible, wedging `up` and `resume` permanently — MEDIUM, CONFIRMED

`internal/reedengine/lifecycle.go:461-505` (`ensureHeaderPaneLocked`'s topmost-target selection and its `-b` split).

`ensureHeaderPaneLocked` must place the new header pane physically topmost, so it splits `-b` off the pane with the smallest `pane_top`. Once reed's own layout has been applied, that pane IS the header band — and the **shipped default** `header.height_rows: 1` (`template_posix.yaml:12`) makes it exactly one row tall. `split-window -b` against a 1-row pane fails: `no space for new pane`, exit 1 (P5).

Normally this never bites, because the recorded `st.HeaderPaneID` names that pane and the function early-returns (present + alive) or kills the corpse first (present + dead, `len(live) > 1`). Both of those branches depend on `st.HeaderPaneID` still naming the top pane. When it does not, the top pane is an *untracked* 1-row band and the split is unsatisfiable.

**Failure scenario (natural, no hand-editing of state).**
Session up with a strand and a laid-out 1-row header. The operator scrubs machine-local scratch — `git clean -xdf` in the worktree, or any `.lyx` cleanup, which by the Durable-vs-Ephemeral State Invariant is a *never-tracked, deliberately disposable* tree — removing `.lyx/reed.json` while tmux keeps running.
Then (L10): `lyx reed up` → `{"ok":false,"error":"split header pane: exit status 1: no space for new pane"}`. `lyx reed resume` → identical. Every subsequent invocation → identical. `lyx reed status` meanwhile reports the session as perfectly healthy.
The worktree is wedged: no reed verb can boot it, and the error names neither the cause nor the one escape (`lyx reed down`, then `up`).

The same state is reachable from a second direction: `ensureHeaderPaneLocked` splits the header pane and only then `SaveState`s its id, so a process death inside that window leaves the identical untracked-header-band state.

**Severity.** MEDIUM. It is recoverable (`down` + `up`), which keeps it below BLOCKING; but it is reachable from an ordinary, sanctioned operator action on a tree the repo's own invariants declare disposable, it is permanent until the operator guesses the remedy, and `status` actively reassures while it holds.

**Suggested fix.** Make the header split resilient rather than special-casing the state that produced it: if the `-b` split against the topmost target fails, issue `select-layout -t <session>: even-vertical` (already in `requiredSubcommands`, so no capability-contract change), re-enumerate, recompute the topmost target, and retry the split exactly once. P5 verifies this recovers the wedged shape and still lands the new pane at `pane_top=0`; the op's normal `reconcileApplyPersistLocked` tail then restores reed's real layout and reaps the untracked band. If the retry also fails, surface the original error unchanged.

### R4-F5 — `add` adopts one of several untracked panes and silently swallows the strand's command — LOW, CONFIRMED

`internal/reedengine/spawn.go:22-74` (`planPaneTarget`'s adopt branch).

**Provenance, stated plainly:** this did not arrive during Job 1. It surfaced during Job 2, while live-verifying R4-F4's fix on the very state that fix makes reachable — the same class of exception round 2 recorded, and recorded here rather than buried in the fixer report.

`planPaneTarget` adopts the first alive non-header pane whenever no strand holds a binding. The branch exists for one case: the pane `new-session` leaves behind on a fresh boot. But it fires whenever `st.Strands` holds no binding at all, which after total state loss is true with *several* untracked panes present — and then it is a guess about which of them is an idle shell.

**Failure scenario.** Continuing L10 with R4-F4's fix in place: `up` recovers, then `lyx reed add --cmd 'sleep 6666'` adopts the previous header pane, which is still running `lyx reed header --blocking`. `send-keys -l` into a busy pane exits 0 and types the text onto its screen, where it never executes. Verified live: `capture-pane` shows `sleep 6666` sitting on the blocked header pane's screen, `lyx reed status` reports `"live":true`, and `pgrep -a -x sleep` shows no such process anywhere on the box. Reed reports a running strand that is not running.

**Severity.** LOW. It is pre-existing designed behaviour rather than anything R4-F4 introduced (the same adoption rule already reaches an operator-split foreign pane), and it needs total state loss to bite — but its symptom is the worst kind, a false `live:true`.

**Suggested fix.** Narrow adoption to the sole-alive-non-header-pane case it was written for. With more than one candidate there is no signal distinguishing idle from busy, and splitting a fresh pane is unconditionally correct: the new pane's shell is idle by construction, and the leftover untracked panes are reaped by the reconcile tail as soon as a strand is bound.

## Correction to the prompt's teardown probe (worth carrying forward)

The round-4 prompt prescribes `pgrep -x tmux` or `ps -eo comm | grep -x tmux` in preference to `ps aux | grep tmux`.
Measured on this host: a running tmux server's `comm` is **`tmux: server`**, not `tmux`, so **both prescribed probes report zero while a server is very much alive** — a false negative in the opposite direction from the `ps aux` false positive they were chosen to avoid. I hit it mid-round and re-verified every teardown claim.
The probes that actually decide it: `ps -eo comm | grep -x 'tmux: server'`, `pgrep -a -f 'tmux .*-L lyx-'` (still argv-self-match-prone, so read the matches), or authoritatively `tmux -L <socket> ls` reporting `no server running`.
All teardown claims in this report use those.

## Scope assessment (plan-promised vs shipped)

Reed's scope post-wave-1 is right, and the told-geometry refactor dropped no observable behaviour.
`Geometry`'s seven fields were each exercised live this round: `SocketKey` (L1/L5 — one shared server per hub), `SessionName` (L6 — exact targeting, no prefix bleed), `AnchorPath` (L1/L4 — state dir and all three spawn sites' pane cwd, read back off `pane_current_path`), `WorktreeRoot` (strand `Worktree` stamp), `LogsDir` (L1 — created on a genuinely fresh hub's first boot, closing the `HubLogsDir` ownership move), `RepoName`/`HubPath` (L11 — the live header pane's rendered tokens).
`hubgeom.ReedGeometry` remains the only production populator, builds all seven from one already-resolved `*lyxcwd.Location`, and holds the Cwd Resolution Invariant: it resolves nothing itself. Nothing deferred-that-should-be-v1; nothing shipped beyond scope.

Attribution: none of R4-F1..F5 traces to a wave-1 commit. All five are pre-existing (`ServerName`, `validateToldTmuxIdentity`'s ancestry, `ensureHeaderPaneLocked`, and `planPaneTarget` all predate `b98ee2ba`'s seam swap).

## Convergence recommendation

Stated in full in `_mill/reed-review-r4-fixer-report.md`, after the fixes landed, so it accounts for what fixing them actually surfaced (R4-F5 in particular).
The short form: round 3 found 1 MEDIUM + 1 NIT; round 4 found 2 MEDIUM + 2 LOW + 1 NIT, and R4-F4/R4-F5 are a *new bug class* rather than further instances of an already-fixed one — the first such in two rounds.

## Clean-room hygiene — one accidental exposure, disclosed

While sweeping for `TmuxCmd` bypasses I ran a repo-wide `grep -rn "socketName\|HubLogsDir" --include=*.go --include=*.md .` without excluding `_mill/`.
The result set included matched lines from `_mill/reed-review-r1.md`, `-r1-fixer-report.md`, `-r2.md`, `-r3.md`, and `-shuttle-HANDOFF.md`.
Stated plainly: this happened after R4-F1 and R4-F2 were already formed and recorded, both from my own tmux wire probes (P1–P4), and neither appears in the exposed lines. R4-F3 and R4-F4 were formed later, from `geometry.go`/`server.go` tracing and from live scenario L10 respectively — neither has any relationship to the exposed text, which named round 1's F4/F6 (dead `socketName`, a stale `HubLogsDir` cross-reference) and round 2/3 scenario summaries, all already-closed items I am not re-litigating.
Every subsequent grep excluded `_mill/`.
