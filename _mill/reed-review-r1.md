# reed — independent review, round 1 (opus-medium-r1)

Round tag: `opus-medium-r1`.
Subject: `internal/reedengine`, `internal/reedcli`, `internal/hubgeom`, and reed's `cmd/lyx` integration, at branch `reed-shuttle-crucible-hardening` (post wave-1 commit `b98ee2ba`).
Merge bar for this round: **correctness in the NORMAL single-instance flow is the gate.**
An N×-concurrent stress suite is a diagnostic amplifier, not a merge blocker on its own.

## What was tested

Log appended as each command/scenario returned.

### Hermetic gates (baseline, before any edit)

| Command | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | clean |
| `go test -count=1 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all ok |

### Static / import-graph checks

| Check | Command | Observation |
|---|---|---|
| hubgeom one-way told direction | `go list -deps ./internal/reedengine \| grep hubgeom` | no match — `reedengine` does not reach `hubgeom`. Also structurally impossible: `hubgeom` imports `reedengine`, and Go forbids import cycles, so the direction is self-enforcing rather than merely unviolated today. |
| reedengine direct imports of `lyxcwd`/`fabricengine` | `grep -rn 'lyxcwd\|fabricengine' internal/reedengine/*.go` (non-test) | only comment/doc mentions; no production import. But `internal/lyxcwd` IS still in `go list -deps ./internal/reedengine`, reached transitively through `internal/logger` — see F9. |
| `hubgeom` importers | `grep -rn internal/hubgeom --include=*.go .` | `reedcli`, `shuttlecli`, `burlercli`, `perchcli`, `webstercli` + smoke tests. No engine imports it. |
| dead code after the constructor change | `grep -rn 'socketName' --include=*.go .` | `socketName` survives with no production caller — see F4. |

### Live driving (real tmux 3.6, `/usr/bin/tmux`, deployed dev binary via `./deploy-dev`)

Fixture: a hand-built hub `/tmp/lyxlive1/myrepo-HUB` with a **subpath anchor** (`_board/.lyx-anchor` = `sub/dir`) and **three** worktrees live at once — `wt-alpha`, `wt-beta`, and `wt` (deliberately a strict name PREFIX of the other two).
A second, separate hub `/tmp/lyxlive2/fresh-HUB` with **no `_board` at all** covers the fresh-boot case.
The subpath anchor is what makes an `AnchorPath`-vs-`WorktreeRoot` field mix-up observable at all.

| # | Scenario | Command(s) | Observed |
|---|---|---|---|
| L1 | Told-geometry, fresh boot on a subpath-anchored worktree | `lyx reed up` in `wt-alpha/sub/dir` | `{"ok":true,"session":"wt-alpha","socket":"lyx-myrepo-HUB-f703d3ec"}`. Socket keyed on the HUB, session on the WORKTREE, state at `wt-alpha/sub/dir/.lyx/reed.json` (the ANCHOR, not the worktree root), panes' `#{pane_current_path}` = the anchor. All four coordinates agreed. |
| L2 | `HubLogsDir` ownership move, first boot on a hub with no `_board/.lyx` | `lyx reed up` in `fresh-HUB/wtx` (hub had no `_board` directory at all) | `<hub>/_board/.lyx/logs` created from nothing; `up` returned ok. The moved `fabricengine.HubLogsDir` works on the real first-boot path, not only in the moved unit fixture. |
| L3 | Cwd gate above hubgeom | `lyx reed status` from `wt-beta` and from `wt-beta/sub` | Both refused with `ErrCwdOutsideAnchor` naming the recorded anchor. `hubgeom.ReedGeometry` is only ever reached with a `Location` that already passed the exact-equality cwd gate, so all seven Geometry fields derive from one gated resolution — a field mix-up is not reachable through the only production populator. |
| L4 | Cross-worktree scope boundary, shared per-hub server | `up`+`add` in `wt-alpha`, `up`+`add` in `wt-beta` (both on socket `lyx-myrepo-HUB-f703d3ec`), then `down` in `wt-alpha` | alpha's session and its `sleep 9000` child gone; beta's session, panes, and `sleep 9100` child untouched. |
| L5 | Prefix-name sibling isolation, incl. the idempotent second `down` | `up`+`add`+`down`+`down` in worktree `wt` while `wt-beta` live | Both `down`s returned ok; `wt-beta`'s panes and child survived. The `=<name>` exact-target grammar holds on the ignored-error path. |
| L6 | Crash / rebirth — tmux SERVER killed out from under a live hub | `tmux -L … kill-server`, then `status`, then `resume` in `wt-beta` | `status` gave the actionable `no reed session (1 strands persisted); run "lyx reed resume"…`; `resume` returned `resumed:1`, rebuilt session + header + strand with fresh pids, and `status` reported the strand live. No error, no action on dead state. |
| L7 | Header corpse / missing-header healing | `kill-pane` the header, then `lyx reed up` in `wt-beta` | Header re-split at `#{pane_top}=0` (physically topmost), cwd = anchor, `up` ok. |
| L8 | Header text resolves the right hub through the pane re-exec | `lyx reed header`; `capture-pane -p` on the header pane | Both rendered ` hub: /tmp/lyxlive1/myrepo-HUB`. The header pane's own `lyx reed header --blocking` re-exec resolved the SAME hub from its pane cwd, so `Geometry.HubPath` and the pane cwd agree. |
| L9 | `attach` under the new construction | `lyx reed attach </dev/null` in `wt-beta` and in `wt` (no session) | With a session: pre-flight passed, handover reached real tmux (`open terminal failed: not a terminal`), exit 1 propagated — right socket/session. Without a session: JSON envelope `no reed session; run "lyx reed up"`, exit 1. |
| L10 | Remove every strand, then add again | 3× `lyx reed remove <guid>`, then `add` | Session survived on the header pane alone; the later `add` split the header and ran. An operator-created untracked pane (`%6`, split by hand) was reaped by the next mutating verb, as `planReconcile` documents. |
| L11 | **Strand-pane cwd** — where does a split pane actually land? | `tmux -L … split-window -t %4 -P -F '#{pane_id} #{pane_current_path}'` issued from a shell standing in the lyx repo, where `%4`'s own cwd was `/tmp/lyxlive1` | The new pane came up in **the invoking client's cwd** (the lyx repo root) — neither `%4`'s cwd nor the anchor. This is the mechanism behind **F2**. |
| L12 | **`down` child reaping** — a SIGHUP-immune pane descendant | strand payload `/tmp/lyxlive1/stubborn.sh` (`sh -c 'trap "" HUP; exec sleep 9600' &` plus a foreground `sleep 9700`), 3 s settle, then `lyx reed down` | `down` returned `{"ok":true,"session":"wt-beta"}` **while `sleep 9600` (pid 1950052) stayed alive**. The pid was inside the snapshotted descendant closure and was contractually due a force-kill. This is **F1**, reproduced end to end. |
| L13 | `os.Process.Wait()` semantics on a NON-child pid (F1's root cause) | standalone Go probe against a live `sleep 300` started by the shell | `Wait RETURNED after 20.378µs, err=waitid: no child processes`, and `Signal(0)` confirmed the process still alive both before and after. Go 1.26, Linux. |
| L14 | Live smoke suite | `go test -tags smoke ./internal/reedcli/... -run Smoke -count=1 -v` | 16 PASS, 1 SKIP (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, psmux-specific premise, correctly skipped on native tmux). 23.9 s. |

**Cost-discipline disclosure, stated plainly:** the L14 sweep matched `TestSmokeClaudeResumeRecallsCodeword` and therefore launched one real `claude` — exactly the careless-broad-sweep the brief told me to avoid. It ran **once** (9.47 s, one process, no fan-out). Inspecting it afterwards produced **F3**: that test pins no `--model` at all, so it burns the operator's default model on every run.

### What I could NOT verify, and why

- **Windows/psmux path.** This is a Linux host. Every psmux-specific assumption in `doc.go`'s multiplexer-contract section (silent split failure, last-pane corpse-instead-of-destroy) is compile-checked only here, and `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` self-skips. Named, not driven, per the brief.
- **`attach`'s interactive tail.** Driven only to the point of the terminal handover; a real attached session needs a tty and an operator.
- **A genuine mid-operation crash between two engine steps.** I could not time a kill inside the op-lock window. I probed the reachable equivalents instead (L6 server death between commands, L7 header death between commands, L10 pane set drained between commands) and all recovered cleanly. The un-probed residual is a kill landing strictly between `kill-pane` and `SaveState` inside one op; `RemoveStrand`'s emptied-session swallow is the only path that persists after a failed apply, and it is unit-covered.

## Findings

Severity legend: BLOCKING / MEDIUM / LOW / NIT. Status: CONFIRMED (reproduced or traced end to end) vs PLAUSIBLE.
**Every finding below is fixed in Job 2, all severities including NIT.**

### F1 — `waitProcessExit` reports every non-child process as already exited, making pane-child reaping inert — BLOCKING, CONFIRMED

`internal/reedengine/lifecycle.go:910` (`waitProcessExit`), consumed at `lifecycle.go:867` (`reapPaneChildren`).

```go
go func() { _, _ = proc.Wait(); close(done) }()
select {
case <-done:
	return nil            // <- "the process exited"
case <-time.After(timeout):
	return fmt.Errorf(...)
}
```

`os.Process.Wait()` on a pid that is **not a child of the calling process** returns immediately with `waitid: no child processes` — measured at 20 µs against a demonstrably alive process (L13).
The error is discarded, so `waitProcessExit` returns `nil` for such a pid unconditionally.

Every pid it is ever handed is a non-child: pane shells and the tmux server are children of the **tmux server**, never of the `lyx` process (the server is even `proc.Detach`ed into its own session at `lifecycle.go:302`).
Consequently `reapPaneChildren`'s `err == nil { continue }` branch always fires and the force-kill fallback beneath it (`p.Kill()` + grace wait, `lifecycle.go:870-875`) is unreachable dead code.

**Failure scenario, reproduced (L12):** a strand whose payload has a SIGHUP-immune descendant. `lyx reed down` returns `{"ok":true}`, tmux's own SIGHUP cascade cannot reap the descendant, reed's force-kill never runs, and the process survives the teardown reed reports as clean. The same helper backs `RemoveStrand`'s reap, so `remove` leaks identically. This is the "`down` must reap every pane **child** process" invariant from the original hardening campaign — it does not currently hold.

The normal-flow blast radius is limited (an ordinary pane payload dies to tmux's cascade anyway), but the guarantee reed advertises — and the code written to provide it — is entirely inert, and `down` reports success while leaking. Rated BLOCKING because a named invariant is silently unenforced, not because the happy path visibly breaks.

**Suggested fix.** Poll liveness instead of waiting on a parent-child relationship that does not exist: `proc.IsAlive(pid)` on a deadline, the same shape `waitServerProcessesGone` (`lifecycle.go:881`) already uses for socket processes. `internal/proc` is already imported by this file.

### F2 — a strand pane is spawned in the invoking client's cwd, not `Geometry.AnchorPath` — MEDIUM, CONFIRMED

`internal/reedengine/spawn.go:115`:

```go
out, err := e.tmux.output("split-window", "-t", splitTargetID, "-P", "-F", "#{pane_id}")
```

No `-c`. `new-session` (`lifecycle.go:294`) and the header split (`lifecycle.go:489`) both pass `-c e.geom.AnchorPath`; this one does not.

Verified live on tmux 3.6 (L11): `split-window` issued from an external client with **no `-c`** places the new pane in the **client's** cwd — not the target pane's cwd, and not the session's.
So a strand pane's working directory is "wherever the `lyx` process happened to be", which is `AnchorPath` today only because `lyxcwd.Resolve`'s exact-equality gate forces process cwd == `AnchorPath` on the CLI path.

**Failure scenario.** Through `reedcli.RunCLIIn(cwd, out, args)` — a seam pinned by the CLI/Cobra Invariant and compile-checked by `cmd/lyx/seamsignature_test.go` — the injected cwd feeds `lyxcwd.WithCwd`, so `Geometry` describes worktree A while the **process** cwd is still worktree B (or the caller's own directory entirely). Every strand pane then launches its command in B. For reed's principal consumer that means launching a `claude` agent against the wrong tree while reed reports success.

This also makes `geometry.go:20-22`'s stated contract false as written: *"AnchorPath is the base stateDir joins onto for reed.json/reed.lock, **and the cwd every pane is spawned with**."* It is the cwd every pane *except a strand pane* is spawned with.

**Suggested fix.** Pass `-c e.geom.AnchorPath` on the strand split, exactly as the other two spawn sites do, plus a smoke test that drives `RunCLIIn` with an injected cwd differing from the process cwd and asserts `#{pane_current_path}`.

### F3 — the real-`claude` smoke test pins no model, burning the operator's default — MEDIUM, CONFIRMED

`internal/reedcli/smoke_resume_test.go:148-149`:

```go
launch := smokeInvokeLine(claudePath, prompt)
resume := smokeInvokeLine(claudePath, "--continue")
```

`smokeInvokeLine` (`smoke_test.go:248`) quotes exactly what it is given; neither line carries `--model`. Every run of `TestSmokeClaudeResumeRecallsCodeword` therefore spawns a real `claude` on the operator's configured default (Opus-class), and the test is reachable from a bare `-run Smoke` sweep (observed at L14).

The campaign brief makes `--model haiku` an explicit operator instruction for every real `claude` this round touches. The test contradicts it structurally, so no amount of reviewer discipline fixes it — the next person to run `-run Smoke` pays full price again. Nothing in what the test asserts (a codeword surviving a server kill and a `--continue` resume) is model-specific.

**Suggested fix.** Pin `--model haiku` on both the launch and the `--continue` resume line.

### F4 — `socketName` is dead production code after the constructor change — LOW, CONFIRMED

`internal/reedengine/server.go:33`.

Before wave-1, `New` and `Socket()` both called `socketName(layout.HubPath)`. Both now read `geom.SocketKey` (`lock.go:44`, `lock.go:50`), and `hubgeom.ReedGeometry` derives that key by calling the exported `ServerName` directly. `socketName` survives with **zero production callers** — its only remaining reference is `server_test.go:67`, a test asserting the dead wrapper equals the live function.

A private alias that exists only to be asserted equal to its own target is a maintenance trap: it reads as a second derivation site for the socket key when there is exactly one.

**Suggested fix.** Delete `socketName` and the test case that pins it; `ServerName` is the single derivation, and `hubgeom_test.go` already pins that `Geometry.SocketKey` equals it.

### F5 — `manifest/designs/producers-standalone.md`'s "verified against the current tree" table is stale for reed — LOW, CONFIRMED

`manifest/designs/producers-standalone.md:64-65` and `:99`.

The section head at line 57 states *"Every row below was verified against the current tree, not inherited from the discovery task"* — a currency claim. Three rows falsify it for reed, all broken by `b98ee2ba` itself:

- Line 64 lists `internal/reedengine` under **"engine constructors take `*lyxcwd.Location`"**, citing `Engine.layout` and `socketName`. `reedengine` takes a `Geometry` and holds no `Location`; `Engine.layout` no longer exists.
- Line 65 cites `tokenvocab.go` (`Ctx.Layout`). The same commit replaced `Ctx.Layout` with `Ctx.RepoName`/`Ctx.HubPath` and dropped `lyxcwd` from tokenvocab's allowlist in `CONSTRAINTS.md`.
- Line 99 marks `reedengine.LoadConfig` as hard-failing on an absent file and **"Blocks standalone: yes"**. It calls `configengine.LoadOrTemplate` and degrades to the embedded template — `CONSTRAINTS.md`'s Config Strictness Invariant pins `reedengine` on the degrading side.

**Suggested fix.** Correct the three reed/tokenvocab rows to record the shipped state, and date the section head's currency claim so an untouched row is not read as re-verified. Rows owned by other modules stay untouched — reed's round does not get to re-verify shuttle's or perch's.

### F6 — `stateDir`'s comment cross-references a `HubLogsDir` that no longer exists in this package — NIT, CONFIRMED

`internal/reedengine/lifecycle.go:29-31`: *"distinct from HubLogsDir's hub anchor **above**"*. Wave-1 deleted `reedengine.HubLogsDir` (it moved to `fabricengine`), so "above" now points at nothing in this file or package. A reader chasing the contrast finds no such symbol.

**Suggested fix.** Name `fabricengine.HubLogsDir`, and phrase it as the value reed is now TOLD (`Geometry.LogsDir`) rather than one it derives.

### F7 — `reedLockFileName`'s comment still says "a Layout's ephemeral .lyx directory" — NIT, CONFIRMED

`internal/reedengine/lock.go:19-20`. `Layout` is a retired type name; the directory is `filepath.Join(Geometry.AnchorPath, lyxdirs.DotLyxDirName)`. The same file's own type doc was updated to say "the Geometry it was told" in wave-1, so this line is an internally inconsistent leftover.

### F8 — `server.go`'s package comment claims the server name is computed from a `Location` — NIT, CONFIRMED

`internal/reedengine/server.go:3-5`: *"a tmux-specific derivation (not a filesystem path) computed from a `Location.HubPath` value lyxcwd already resolves."* `reedengine` no longer sees a `Location` at all; `ServerName` takes a plain `hubPath string` that `hubgeom` supplies. The justification for the file's existence survives the rewording intact — only the vocabulary is stale.

### F9 — `doc.go` overstates the absence of `lyxcwd` from reedengine's imports — NIT, CONFIRMED

`internal/reedengine/doc.go:21-22`: *"internal/lyxcwd and internal/fabricengine are consequently absent from this package's production imports."*

True for **direct** imports only. `go list -deps ./internal/reedengine` still lists `internal/lyxcwd`, reached through `internal/logger`. `CONSTRAINTS.md` holds the rest of the repo to an explicit honesty standard on exactly this distinction — the Treadle Runner-Seam Invariant says outright that `lyxcwd` "is reachable through both `logger` and `shuttleengine`, so excluding it buys no isolation", and the Shed Producer-Seam Invariant separates "policed on direct imports" from what is transitively true. reed's own doc should not read stronger than the invariant that governs it.

**Suggested fix.** Say "direct production imports", and name the transitive `logger` path, so the claim is exactly as strong as it is true.

### F10 — stale `layout` vocabulary in a wave-1-touched test comment — NIT, CONFIRMED

`internal/reedengine/mouse_boot_integration_test.go:128-129`: *"reusing e1's **layout** and cfg"*. Wave-1 changed the very next line to `New(cfg2, e1.geom)` but left the comment naming the removed field.

## Scope assessment (plan-promised vs shipped)

- **Nothing shipped beyond scope.** `hubgeom` holds exactly one exported function, `ReedGeometry`, as `producers-standalone.md` T3 specifies. No `BurlerGeometry`/`PerchGeometry`/`WebsterGeometry` was smuggled in early.
- **Nothing plan-promised was dropped.** All seven `Geometry` fields map 1:1 onto values the pre-wave-1 `Engine` derived from its `Location`, and I traced each to its consumer: `SocketKey`→`Socket()`, `SessionName`→`SessionName()`, `AnchorPath`→`stateDir`/`new-session -c`/header split `-c`, `WorktreeRoot`→`Strand.Worktree` + `<WORKTREE>`, `LogsDir`→the boot-time mkdir/prune, `RepoName`+`HubPath`→`tokenvocab.Ctx`.
- **One observable behaviour did change silently**, and it is F2: `AnchorPath` is now a told field that the strand-spawn path does not consume, so what used to be "the pane cwd reed derives" is now "the pane cwd reed happens to inherit". The refactor did not introduce the missing `-c` — it predates wave-1 — but it did convert an implicit derivation into an explicit, documented contract that the code does not honour, which is what makes it a finding now rather than before.
- **`New` validating nothing is a deliberate, documented design decision, not a finding.** `geometry.go` states it, and L3 shows the only production populator is downstream of a gate that makes an inconsistent `Geometry` unconstructible. Adding a validator would contradict the stated contract and buy nothing reachable.
- **The one-way told direction is stronger than a review obligation**, contrary to what `hubgeom/doc.go` implies by merely asserting it: `hubgeom` → `reedengine` makes the reverse a compile error. No guard test is needed or worth adding.
</content>
