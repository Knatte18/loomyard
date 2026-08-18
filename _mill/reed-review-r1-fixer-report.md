# reed — round 1 fixer report (opus-medium-r1)

Companion to [`reed-review-r1.md`](reed-review-r1.md).
Every finding recorded in that review is fixed here — all ten, all severities including every NIT.
Nothing is deferred.

## Findings closed

| # | Severity | Fix | Commit |
|---|---|---|---|
| F1 | BLOCKING | `waitProcessExit` polls `proc.IsAlive` on a deadline instead of blocking on `os.Process.Wait`, so `reapPaneChildren`'s force-kill fallback is reachable at all | `d0bbbc82` |
| F2 | MEDIUM | strand split passes `-c e.geom.AnchorPath`; `geometry.go`'s AnchorPath contract corrected to state it | `a6bcb308` |
| F3 | MEDIUM | real-`claude` smoke test pins `--model haiku` on both launch and `--continue`, behind one named constant | `61f0407d` |
| F4 | LOW | dead `socketName` alias and its self-referential test deleted | `0373f7fd` |
| F8 | NIT | `server.go`'s package comment no longer claims a `Location` this package never sees | `0373f7fd` |
| F6 | NIT | `stateDir`'s dangling `HubLogsDir` cross-reference now names `Geometry.LogsDir`/`fabricengine.HubLogsDir` | `07883759` |
| F7 | NIT | `reedLockFileName` says `Geometry.AnchorPath`, not the retired `Layout` | `07883759` |
| F9 | NIT | `doc.go` qualifies the `lyxcwd` absence as DIRECT imports and names the transitive `logger` path | `07883759` |
| F10 | NIT | stale `layout` vocabulary in the mouse-boot integration test comment | `07883759` |
| F5 | LOW | `producers-standalone.md`'s three stale reed/tokenvocab rows record the shipped state; the section's currency claim is qualified | `ab6f8fdc` |

## The two fixes that changed behaviour, and how each was verified

### F1 — pane-child reaping was inert

`internal/reedengine/lifecycle.go`. The old `waitProcessExit` spawned a goroutine on `os.Process.Wait()` and treated ANY return as "the process exited", discarding the error.
`Wait` on a non-child returns `waitid: no child processes` immediately, and every pid reed waits on is a child of the **tmux server**, never of `lyx` — so the function answered "exited" unconditionally and `reapPaneChildren` never once reached its `p.Kill()`.

Replaced with a `proc.IsAlive` poll on the caller's deadline — the shape `waitServerProcessesGone` in the same file already used for socket processes, so this is now one liveness idiom in the package rather than two. `reapPaneChildren`'s force-kill also moved onto `proc.KillPID` (dropping a `findErr` branch that can never fire on Unix) and now logs a `Warn` when the force-kill fails or the child survives it, per the Live-Substrate Spawn Observability invariant's "teardown that did not confirm clean" clause. `reapSocketProcesses`'s raw `os.FindProcess`/`Kill` pair went the same way, which also removed a local `proc` variable shadowing the `internal/proc` package.

**Verified four ways.** (1) Live, pre-fix: `lyx reed down` returned `{"ok":true}` while leaking pid 1950052. (2) Live, post-fix, same fixture and payload: `down` took 15.1 s — the full graceful window, then the force-kill — and left nothing behind. (3) New smoke test `TestSmokeDownForceKillsSighupImmunePaneChildren`. (4) That test run against the OLD implementation, temporarily restored: `FAIL … pane subtree pid 1958750 still running immediately after down returned`. It is a real regression guard, not a test written to match the fix.

Why every pre-existing reap test passed against the broken code, stated plainly: their payloads (`sleep 300`, `pwsh -NoExit`) die to tmux's own SIGHUP cascade, so they could never distinguish "reed reaped it" from "tmux did". The new test's payload traps SIGHUP specifically to force that distinction. The test file's own helpers had *documented* the `Wait`/ECHILD hazard all along (`smoke_procalive_linux_test.go`'s header comment) — the knowledge existed on the test side and had never been applied to production.

### F2 — strand panes ignored the told anchor

`internal/reedengine/spawn.go`. `split-window` carried no `-c`, and tmux resolves a client-issued split's cwd from the **invoking client** — verified live on tmux 3.6, where a split targeting a pane sitting in `/tmp/lyxlive1` produced a pane in the calling shell's own directory instead.

Under the CLI this was accidentally correct, because `lyxcwd.Resolve` gates process cwd to equal `AnchorPath`. Under `RunCLIIn(cwd, …)` — a seam pinned by the CLI/Cobra Invariant — it is not, and every strand command would run against the wrong tree.

**Verified three ways.** (1) New smoke test `TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd`, the first test in the package to drive the injected-cwd seam (every other smoke test uses `t.Chdir`, which makes the two directories identical and the bug invisible). (2) That test against the un-`-c`'d split: `FAIL … pane %2 current path = ".../003"; want the told anchor ".../warp-bare-HUB/warp-bare"`. (3) Live: a strand whose command `cd`s to `/tmp`, then a second strand split off it — the second pane now comes up at the anchor rather than inheriting `/tmp`.

To add that test without duplicating helpers, `addStrand`/`socketAndSession`/`paneIDForStrand` each became a thin delegation to a new `…In(cwd, …)` sibling, with the empty-cwd default preserving every existing call site unchanged.

## Verification — full gate

| Gate | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` (whole repo) | clean |
| `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all ok |
| `go test -count=1 ./...` (whole repo) | all ok, zero failures |
| `go test -tags integration ./internal/reedengine/...` | ok |
| `go test -tags smoke ./internal/reedcli/... ./internal/reedengine/... -run Smoke -count=1` | 18 PASS, 1 SKIP (psmux-specific, correct on tmux), 0 FAIL |
| Markdown/vocabulary guards (`TestEnforcement_MarkdownLinks`, `TestEnforcement_FabricVocabulary`) | PASS |
| Live re-drive of all 14 review scenarios against the redeployed binary | all reproduced green |
| Stray state: `pgrep -af tmux` | **zero tmux**; zero stray strand children; the `/tmp` live fixtures deleted |

`./deploy-dev` was re-run before every live re-drive, so nothing below was validated against a stale snapshot.

## Scope taken beyond the recorded findings — disclosed, not hidden

Three small in-class items were fixed alongside F6–F10's vocabulary sweep rather than left as known-wrong text: `lifecycle.go`'s `cfg+layout` → `cfg+geometry`, the mouse test's file-header `SAME layout` → `SAME geometry`, and `reedcli/cli.go`'s resolution-chain comment, which listed `cwd -> layout -> config -> Engine` and omitted the geometry step this whole campaign is about.
None changes behaviour. They are named here so round 2 can see exactly what moved and judge it, rather than discovering unrecorded edits.

The `layout` **identifier** in `reedcli/cli.go` (the local `*lyxcwd.Location`) was deliberately NOT renamed: it is a real variable of a real type, renaming it is a judgement call rather than a correction, and no finding recorded it.

## Deferred

**Nothing.** Every recorded finding is fixed and verified.

## Named gaps this round could not close

These are honest coverage limits, not unfixed findings:

- **Windows/psmux.** Linux host. The new `TestSmokeDownForceKillsSighupImmunePaneChildren` self-skips on Windows (its payload needs a POSIX shell SIGHUP trap). The F1 fix itself is cross-platform — `proc.IsAlive` has a Windows implementation — but the Windows behaviour of that implementation (`os.FindProcess` succeeding for a dead pid is a real weakness there) is unexercised by this round and is a genuine open question for a Windows-hosted round.
- **`attach`'s interactive tail** is driven only to the terminal handover; a real attached session needs a tty and an operator.
- **A crash landing strictly between two steps inside one engine op.** Probed only at command boundaries (server death, header death, drained pane set — all recovered cleanly).
- **Cost note.** `TestSmokeClaudeResumeRecallsCodeword` ran twice this round: once in the pre-fix baseline sweep on the operator's default model (the careless-broad-sweep the brief warned against — disclosed in the review's "What was tested"), and once post-F3 on pinned `haiku`. It is now structurally cheap for everyone downstream.

## Merge-readiness

**Ready to merge.** The gate for this round is correctness in the normal single-instance flow, and every scenario in that flow passes live, hermetically, and under the smoke suite. The one BLOCKING finding (F1) is closed with a regression test proven to fail against the old code, and the geometry-integrity question this campaign was convened to answer resolves cleanly: the seven-field `Geometry` cannot be constructed inconsistently through the only production populator, because `lyxcwd.Resolve`'s cwd gate sits above it — the real defect was not a field mix-up but a told field (`AnchorPath`) the spawn path was not consuming, which F2 closes.
</content>
