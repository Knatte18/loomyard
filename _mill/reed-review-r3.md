# reed — independent review, round 3 (`fable-high-r3`)

Reviewer: crucible round agent, clean-room (no prior-round `_mill/reed-review-*` material read before this findings list was complete).
Scope: `internal/reedengine`, `internal/reedcli`, `internal/hubgeom`, `cmd/lyx` integration, docs — per `_mill/reed-review-prompt.md` (round 3 instance).

## What was read

- `CONSTRAINTS.md` in full (Cwd Resolution Invariant, CLI/Cobra Invariant, Live-Substrate Spawn Observability, Sandbox Suite Coverage, Test Tier Purity).
- Every production file in `internal/reedengine` (geometry, server, lock, lifecycle, strand, spawn, apply, reconcile, state, overlay, parse, probe, version, header, headerpane, headertemplate, env, io, mouse, serverlog, proctree{,_linux,_windows}, config, template{,_posix}, name, doc.go).
- Every production file in `internal/reedcli` (cli, up, add, remove, status, resume, attach, header).
- `internal/hubgeom` (doc.go, hubgeom.go).
- Wave-1 commit `b98ee2ba` (stat + the `fabricengine.HubLogsDir` addition + reed-touching hunks).
- `docs/overview.md` reed bullet (~line 280) and execution-stack section; `tools/sandbox/SANDBOX-REED-SUITE.md` (scenario ideas only).

## Static-read observations (pre-live)

- **Told-direction one-way confirmed by import graph**: `go list -deps ./internal/reedengine` contains neither `internal/hubgeom` nor `internal/fabricengine`; `internal/hubgeom` imports `{fabricengine, lyxcwd, reedengine}`. The doc.go honesty note (lyxcwd still transitively present via logger) matches reality.
- **TmuxCmd discipline sweep**: raw `exec.Command` sites in reed production code are (a) `overlay.go` run/output — the chokepoint itself; (b) `lifecycle.go` spawnSession — carries `-L e.Socket()` explicitly in argv (needs Dir/Env/Detach, cannot route through TmuxCmd); (c) `reedcli/attach.go` — carries `-L socket` explicitly via `attachArgv` (terminal handover, cannot route through TmuxCmd); (d) `proctree_windows.go` — pwsh process-table probes, not tmux invocations. No socket-scoping bypass found.
- **Reap-root snapshot sweep**: exactly two descendant-closure snapshot sites exist — `alivePanePIDs` (RemoveStrand) and `sessionReapRoots`/`sessionReapRootsLocked` (Down) — both routed through the single `safeReapRoot` predicate. `serverPIDLocked`'s `#{pid}` snapshot is taken while the server demonstrably serves the query, so it is un-recycled at snapshot time, same discipline. No third pre-predicate site.
- `planReconcile`'s untracked-alive-pane kill has no child reap, but a straggler there is caught by Down's whole-session reap (sessionReapRoots covers every alive pane, tracked or not); noted, judged not a defect on the merge bar.

## What was tested

Hermetic gates (all green):
- `go build ./...` — pass.
- `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` — pass.
- `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` — pass (reedengine 0.79s, cmd/lyx 0.88s).

Direct tmux 3.6 grammar probes (isolated socket `r3probe1`, torn down after each):
- `new-session -d -s "a<TAB>b"` → exit 0, session created as the FOUR literal characters `a\tb` (vis-encoded), `has-session -t '=a<TAB>b'` MISSES. Same for newline (`\n` → literal `\n`), ESC (→ `\033`), DEL (→ `\177`), BEL (→ `\a`), and the invalid-UTF-8 byte 0xFF (→ literal `\377`). Exit 0 every time, exact-match target misses every time. **This is a live-confirmed R2-F1-shaped gap — see finding R3-F1.**
- Names that pass verbatim and exact-hit: a space-only name `" "`, unicode `svc-åäö-⚙`, `=`-leading, `-`-leading, `a#b%c`. So refusing only the vis-encode class is correct — no over-refusal needed.
- Socket-key side: `tmux -L "r3<TAB>sock" new-session` works (a socket key is a filename; control characters are legal there). The vis-encode gap is session-name-only.

Live CLI driving (dev binary `./deploy-dev` @ 84f3558b; real hub built with `lyx fabric clone` into the session scratchpad — `warp-HUB` with warp prime + two unicode worktrees):
- **S1 fresh-hub first boot**: `lyx reed up` in the warp prime of a hub whose `_board/.lyx/logs` did NOT yet exist → ok in 0.17s, logs dir created 0755, session `warp` up with header pane (%1 running the real `lyx reed header --blocking` keepalive) + initial bash pane. `fabricengine.HubLogsDir` ownership move holds on the very first boot.
- **S2 lifecycle + child reap**: added parent strand `bash -c "sleep 600 & sleep 600 & wait"` + child strand; `remove --recursive` removed both strands and the WHOLE process subtree (both `sleep 600` grandchildren confirmed gone immediately after return); session + header survived; status empty.
- **S3 crash/rebirth**: added strand with distinct `--resume-cmd 'sleep 401'`; `tmux kill-server` out from under; `lyx reed resume` → rebooted server, resumed:1, replayed the RESUME cmd (`sleep 401` live, not `sleep 400`), header pane relaunched with rendered hub token.
- **S4 unicode worktrees**: `lyx fabric add svc-åäö-⚙` and `svc-åäö-⚙-x` (a unicode PREFIX-colliding pair — a fixture neither round 1 nor round 2 used); `reed up` + `add` in both → sessions created verbatim, exact targets hit.
- **S5 unicode prefix-pair scope**: `reed down` in `svc-åäö-⚙` (the prefix) left `svc-åäö-⚙-x`'s session, strand, and `sleep 301` process untouched; a SECOND down (idempotence path, where a bare `-t` would prefix-match now that our session is gone) also left the sibling untouched; the downed worktree's `sleep 300` was reaped.
- **S6 vis-encode end-to-end repro (finding R3-F1)**: renamed a worktree to `svc<TAB>3` (`mv` + `git worktree repair` — the realistic operator path, since `fabric add` slugs are git-ref-safe and cannot carry control characters); `lyx reed up` burned a full 20s boot-attempt window, failed with the opaque `"tmux server is up but session \"svc\t3\" did not materialize within 20s"`, and STRANDED session `svc\t3` (the vis-encoded literal-backslash-t name) on the SHARED hub server beside `warp` and `svc-åäö-⚙-x`. A follow-up `lyx reed down` reported `ok:true` while leaving the stranded session in place (its kill-session targets the raw-TAB name, which can never match). Cleaned up manually via raw tmux; worktree renamed back.
- **S7 foreign-pane reconcile**: an operator-opened untracked pane (`sleep 999`) in reed's session was deterministically reaped by the next mutating verb (`add`), its process gone; tracked strands and header untouched.
- **S8 verbs/envelope**: `lyx reed header` returns rendered text on the envelope; `attach` pre-flights the friendly no-session error on the envelope and, with a session but no TTY, propagates tmux's own exit 1 after tmux's stderr reached the terminal; bare `lyx reed` exits 0 with the group listing, `lyx reed bogus` and outside-a-repo both emit `ok:false` JSON and exit 1.
- **Smoke suites**: `go test -tags smoke ./internal/reedcli/... -run Smoke` (excluding the claude test) — 18 PASS, 1 expected SKIP (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, psmux-only), 30.3s. `TestSmokeClaudeResumeRecallsCodeword` run deliberately by exact name — PASS 8.05s, exactly one real `claude` subprocess on the test's own `--model haiku` pin.
- **Integration gate**: `go test -tags integration ./internal/reedengine/... ./internal/reedcli/...` — all pass.
- **Substrate-behavior probes** backing static reasoning: (a) a Linux tmux server's `/proc/<pid>/cmdline` retains the full original argv including `-L <socket>` (only `comm` is rewritten to "tmux: server"), so `serverProcessesOnSocket`'s cmdline match finds the server on Linux — the zombie-reap backstop is real here; (b) `split-window -t <dead-pane>` succeeds on tmux 3.6 (exit 0, new pane), so `planPaneTarget`'s dead-corpse fallback targets are safe; (c) after the whole session, the operator's DEFAULT tmux socket had no server — the capability probe stayed on reed's own socket throughout (R2-F6 discipline holds live).

## Findings

### R3-F1 — MEDIUM — `validateToldTmuxIdentity` misses tmux's vis-encode class: control characters and invalid UTF-8 in a session name reproduce R2-F1's exact failure — CONFIRMED (live, end-to-end)

- **Where**: `internal/reedengine/server.go:59-108` (`rewrittenSessionNameChars` / `validateToldTmuxIdentity`); false documentation at `internal/reedengine/doc.go` ("no other character is touched", stated in the silent-session-name-rewriting bullet), `internal/reedengine/server.go:60-62` (same claim), `internal/reedengine/geometry.go:23-29` (SessionName field doc names only `.`/`:`).
- **Failure scenario**: tmux's `session_check_name` does two rewrites, not one: `.`/`:` → `_` (covered by R2-F1's fix) AND `utf8_stravis(VIS_OCTAL|VIS_CSTYLE|VIS_TAB|VIS_NL)` over the whole name — which silently rewrites EVERY ASCII control character (0x00–0x1F), DEL (0x7F), and every byte that is not part of a valid UTF-8 sequence into a multi-character escape (TAB → literal `\t`, NL → `\n`, ESC → `\033`, BEL → `\a`, DEL → `\177`, 0xFF → `\377`), creates the session under the rewritten name, and exits 0. All six probed live on tmux 3.6; exact-match `=` targets miss the raw name every time. End-to-end through the deployed CLI (S6): 20s hang, opaque "did not materialize" error naming neither cause nor remedy, a stranded unaddressable session squatting the SHARED per-hub server, and `reed down` then reporting `ok:true` while leaving it there.
- **Reachability**: lower than R2-F1's dot-name case — `lyx fabric add` slugs are git-ref-safe (git refuses control bytes in refnames), so the trigger needs a hand-renamed (`mv` + `git worktree repair`) or externally-created worktree directory, which Linux permits for any byte except `/` and NUL. Hence MEDIUM, not BLOCKING; the mechanism and blast radius are identical to R2-F1.
- **Also confirmed safe (no over-refusal)**: space-only names, unicode (`svc-åäö-⚙`), `=`-leading, `-`-leading, and `#`/`%` names are created verbatim and exact-match fine — the fix must refuse ONLY the vis-encode class.
- **Suggested fix**: extend `validateToldTmuxIdentity` to refuse a `SessionName` carrying any byte tmux would vis-encode — any rune < 0x20, the rune 0x7F, or any invalid-UTF-8 byte — with the same refuse-never-sanitize posture and an actionable message naming the worktree directory. Correct the three "no other character is touched" / `.`-`:`-only doc claims in the same change. Add unit tests over the class and extend `contract_integration_test.go`'s rewrite-pinning test with a control-character row so the wire behavior stays pinned against version drift.

### R3-F2 — NIT — persisted `ReedState.Socket`/`Session` are stamped once at first init and never refreshed, so `reed.json`'s identity diagnostic goes silently stale after a geometry change — CONFIRMED (code inspection)

- **Where**: `internal/reedengine/spawn.go:151-163` (`loadOrInitStateLocked`).
- **Scenario**: the two fields are written only when `reed.json` is first created. Nothing in production reads them back (verified by grep — every consumer uses `e.geom` via `Socket()`/`SessionName()`), so they exist purely as an on-disk forensic diagnostic — and after a worktree rename (S6's exact fixture: the `.lyx` junction moves with the worktree, `reed.json` survives) the file permanently records a socket/session pair that is no longer the one reed drives, actively misleading in exactly the debugging situation the fields exist for.
- **Suggested fix**: re-stamp both fields from the engine's told geometry in `loadOrInitStateLocked` when they differ; one unit test.

## Scope assessment

- Wave-1 (`b98ee2ba`) reed hunks reviewed in full: a mechanical seam swap (`layout.X()` reads → told `geom.X` fields, `socketName` → told `SocketKey`, `HubLogsDir(location)` deleted in favor of told `LogsDir` built by `fabricengine.HubLogsDir`). No observable behavior dropped or changed — S1–S8 re-verify the composed behaviors live, including the fresh-hub first-boot logs-dir creation the move relocated.
- `hubgeom` is correctly one-way (import graph verified) and its single test pins all seven fields against a deliberately-distinct fixture (anchor ≠ worktree ≠ hub, RepoName ≠ every basename).
- Told-geometry field integrity beyond `hubgeom`: `reedengine.New` still validates nothing (documented contract), `withOpLock` pre-flights identity; the seven fields cannot disagree in hub mode since `ReedGeometry` reads one resolved Location. R3-F1 is the one gap found in the pre-flight itself.
- Out-of-scope respected: shuttle untouched; `layout`→`location` rename follow-up in the four other CLIs left standing; Windows verification gap named (psmux vis-encode behavior unprobed — this host is Linux; the new validation is conservative for both backends since it refuses rather than sanitizes).

## Non-findings (assessed and cleared)

- **No third reap-root snapshot site**: only `alivePanePIDs` and `sessionReapRoots` take descendant-closure roots, both through the single `safeReapRoot` predicate; `serverPIDLocked`'s `#{pid}` is snapshotted while the server provably serves the query.
- **No TmuxCmd/socket-scoping bypass**: the only raw exec sites are the TmuxCmd chokepoint itself, `spawnSession` (needs Dir/Env/Detach; carries `-L` explicitly), `attach` (terminal handover; carries `-L` explicitly), and the Windows pwsh process-table probes (not tmux). Live-confirmed the default socket stayed serverless all session.
- **`Down` vs. concurrent sibling boot race** (down's empty `list-sessions` window reaping a sibling's in-flight first boot): real but self-healing — the sibling's boot loop retries up to 8 attempts/90s and the window is sub-second; worst case is added boot latency, no correctness loss. Not a defect on the merge bar.
- **`planReconcile`'s untracked-pane kill has no child reap**: dead panes have no live process; untracked alive panes are foreign (reed never launched them), tmux's SIGHUP cascade handles the common case (S7: foreign `sleep 999` gone), and Down's whole-session reap remains the backstop.
- **Render**: checksum algorithm matches tmux's rotate-right-1 accumulator; zero-strand header band emits a single-cell body, never a zero-pane layout (`anyPlacedStrand` refusal separately guards the empty-layout destruction); stacked-adds and layout-survival smoke tests pass.
