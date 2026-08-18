# `reed` — independent review, round 4 (`opus-medium-r4`)

Clean-room round-4 review of `internal/reedengine` + `internal/reedcli` + `internal/hubgeom`, per `_mill/reed-review-prompt.md`.
Findings below were formed before any prior-round material was opened (one accidental exposure is disclosed in "Clean-room hygiene" at the end).

## What was tested

### Hermetic gates (baseline, before any edit)

| # | Command | Result |
|---|---|---|
| H1 | `go build ./...` | clean |
| H2 | `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | clean |
| H3 | `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all `ok` |

### Substrate probes — tmux 3.6 wire behaviour (run directly, own sockets, all torn down)

| # | Scenario | Observation |
|---|---|---|
| P1 | `new-session -d -s 'a\b'` then `has-session -t '=a\b'` on a scratch socket | **new-session exits 0**; `list-sessions` reports `a\\b`; the exact target `=a\b` fails with `can't find session: a\b`, exit 1. tmux vis-encodes the backslash (`vis(3)` doubles `\` unless `VIS_NOSLASH`, which tmux does not set). Same silent-rewrite shape as `.`/`:` and the control-char class. |
| P2 | Exhaustive sweep of every printable ASCII byte `0x20`–`0x7E` as `x<c>y` session names, each round-tripped through `has-session -t '=x<c>y'` | Exactly **three** mismatches: `x.y`, `x:y`, `x\y`. The first two are already refused by `validateToldTmuxIdentity`; `\` is not. Nothing else in the printable range is rewritten (space, `"`, `#`, `$`, `'`, `` ` ``, `*`, `%` all pass verbatim). |
| P3 | 400-character session name; `svc-åäö-⚙` | Both created and exact-matched verbatim — no session-name length limit, valid multi-byte UTF-8 untouched. Confirms the existing vis check refuses only the encode class, never unicode. |
| P4 | `-L <key>` with key lengths 88/90/92/93/94/100/250 | 92 chars is the last key that works; **93+ fails with `error creating/connecting … (File name too long)`**. `/tmp/tmux-1000/` is 15 bytes and `sockaddr_un.sun_path` is 108, so the ceiling is `107 − len(socketdir)`. Unlike the other malformed-identity cases this one exits **1**, not 0 — so it does not strand a session, but it does burn the whole boot budget. |

*(more rows appended as the live-driving pass runs — see "Live driving" below)*

## Findings

### R4-F1 — `validateToldTmuxIdentity` misses tmux's THIRD session-name rewrite class: the backslash — MEDIUM, CONFIRMED

`internal/reedengine/server.go:121-145` (and the character-class constant at `server.go:70`).

`validateToldTmuxIdentity` refuses two rewrite classes: the `.`/`:` substitution (`rewrittenSessionNameChars`, R2-F1) and the vis-encode class of ASCII control characters / DEL / invalid UTF-8 (`firstVisEncodedSessionNameByte`, R3-F1).
It misses a third: `vis(3)` **doubles the backslash itself** (`\` → `\\`) unless `VIS_NOSLASH` is set, and tmux's `session_check_name` does not set it.
`\` is `0x5C` — above `0x20`, not DEL, and perfectly valid UTF-8 — so `firstVisEncodedSessionNameByte` passes it, and it is not in `rewrittenSessionNameChars`.

**Failure scenario (inputs → wrong behaviour).**
A worktree directory named `a\b` (a legal POSIX filename; `\` has no meaning to the Linux kernel or to git's worktree machinery) yields `Geometry.SessionName == "a\b"`.
`validateToldTmuxIdentity` passes it. `ensureServerAndSessionLocked` spawns `tmux -L … new-session -d -s 'a\b' …`, which **exits 0 and creates a session named `a\\b`**.
Every target this package issues is the exact-match `=<name>` form, so `hasSession("a\b")` returns exit 1 forever: the boot loop burns its full budget (8 attempts / 90 s), each iteration force-reaping the socket and respawning, and finally reports `tmux session did not start after 8 attempts`, naming neither the cause nor the remedy.
Worse, the same shape R2-F1 documented: the rewritten session is left squatting on the SHARED per-hub server, addressable by no reed verb — `down` issues `kill-session -t '=a\b'`, which misses it too, so it survives every teardown a sibling worktree can perform.

Verified at the tmux wire level in P1/P2 above; the reed-level reproduction is driven in the live pass below.

**Severity.** MEDIUM, matching R3-F1's calibration exactly: identical failure shape and blast radius to the already-fixed R2-F1 (BLOCKING), but lower reachability — `fabric add` mints git-ref-safe slugs, so it takes a hand-created or hand-renamed worktree directory to reach.

**Suggested fix.** Fold `\` into the same pre-flight. It belongs with `rewrittenSessionNameChars` in mechanism (a fixed, enumerable character set tmux rewrites) but not in message (it is doubled, not substituted with `_`), so a separate named constant with its own error line is clearer than overloading either existing branch. Refuse, never sanitize, for exactly R2-F1's reason: substituting would map two sibling worktrees onto one session.
P2's exhaustive printable-ASCII sweep is what lets the fix claim completeness rather than "one more character": `.`, `:`, `\` plus the control/DEL/invalid-UTF-8 class is the whole rewrite surface of tmux 3.6's `session_check_name`.

### R4-F2 — an over-long told `SocketKey` yields a 90-second, cause-free boot failure — LOW, CONFIRMED

`internal/reedengine/server.go:36-42` (`ServerName`), with the failure surfacing at `internal/reedengine/lifecycle.go:328-377`.

`ServerName` builds `"lyx-" + filepath.Base(hubPath) + "-" + <8 hex>` and bounds nothing.
A tmux `-L` key is a filename inside the per-user socket directory, and that path must fit `sockaddr_un.sun_path` (108 bytes) — P4 measured the live ceiling at 92 characters for this host's `/tmp/tmux-1000/`.
`ServerName`'s key is `len(base) + 13`, so a hub directory whose basename is ≥ 80 characters produces an unusable socket key.

**Failure scenario.** `lyx reed up` under such a hub: every `tmux -L <key> …` call fails with `error connecting to … (File name too long)`, exit 1. `hasSession` maps exit 1 to `(false, nil)`, so the boot loop treats it as "not up yet", polls a full 20-second attempt window, finds `list-sessions` errored (so it takes the zombie branch), force-reaps nothing, and retries — until `bootOverallTimeout`/`maxBootAttempts` end it with `tmux session did not start after 8 attempts (fast-failure spiral guard…)`. That message names the spiral guard, not the socket key, so an operator has nothing to act on.
Nothing is stranded (unlike R4-F1 this path exits 1 rather than 0), which is what holds this at LOW alongside R2-F3's filesystem-root case.

**Suggested fix.** Bound the readable half at its derivation, exactly as `socketSafeBase` already substitutes separators there rather than refusing: `ServerName`'s doc already states that "the basename half is purely for human readability — uniqueness rests entirely on the hash", so truncating it cannot collide and needs no refusal path. A cap of 40 runes keeps every key ≤ 53 characters, comfortably under the tightest plausible `sun_path` budget (a long `TMUX_TMPDIR` included), and leaves every ordinary hub name untouched.

### R4-F3 — the identity pre-flight validates 2 of 7 told fields, and the one with a silent-wrong-place failure mode is not among them — LOW, CONFIRMED (by trace)

`internal/reedengine/server.go:121-145`, `internal/reedengine/lifecycle.go:33-35` (`stateDir`), `internal/reedengine/geometry.go:32-36`.

`Geometry`'s contract is "New validates nothing; the caller owns every field", with `validateToldTmuxIdentity` as the documented **backstop** for a teller that populates the struct some other way — and its own doc comment says the socket check "binds the standalone tellers that will populate a Geometry themselves".
That backstop covers `SocketKey` and `SessionName` only. `AnchorPath` is the third field with a failure mode a caller cannot observe: `stateDir()` is `filepath.Join(e.geom.AnchorPath, ".lyx")`, so an empty or relative `AnchorPath` silently resolves `reed.json`/`reed.lock` **against the lyx process's own working directory** instead of the worktree — and `-c e.geom.AnchorPath` is then passed to all three tmux spawn sites, where tmux answers an empty `-c` by falling back to the invoking client's cwd (the precise failure `launchStrandLocked`'s own comment says the explicit `-c` exists to prevent).
No error is raised at any point; the op succeeds against the wrong tree.

Hub mode cannot reach it (`hubgeom.ReedGeometry` always passes `l.AnchorPath()`, an absolute path), so this is a contract-backstop gap rather than a live defect — the same standing as the socket-separator branch that is already there. It is worth closing now rather than when the first standalone teller lands, since that teller is exactly the caller the backstop was written for.

**Suggested fix.** Extend the same pre-flight with an `AnchorPath` check: non-empty and `filepath.IsAbs`. Same chokepoint, same refuse-don't-repair posture, no new validation seam.

### R4-F4 — `docs/overview.md`'s reed bullet still describes the pre-wave-1 construction — NIT

*(pending — confirmed during the docs pass, see below)*

## Clean-room hygiene — one accidental exposure, disclosed

While sweeping for `TmuxCmd` bypasses I ran a repo-wide `grep -rn "socketName\|HubLogsDir" --include=*.go --include=*.md .` without excluding `_mill/`.
The result set included matched lines from `_mill/reed-review-r1.md`, `-r1-fixer-report.md`, `-r2.md`, `-r3.md`, and `-shuttle-HANDOFF.md`.
Stated plainly: this happened after R4-F1 and R4-F2 were already formed and recorded, both of which came from my own tmux wire probes (P1–P4), and neither appears in the exposed lines.
The exposed lines named round 1's F4/F6 (dead `socketName`, a stale `HubLogsDir` cross-reference) and round 2/3 scenario summaries — all already-closed items I am not re-litigating. No finding below was derived from them. Subsequent greps excluded `_mill/`.
