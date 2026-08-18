# reed — fixer report, round 3 (`fable-high-r3`)

Companion to `_mill/reed-review-r3.md`.
Both findings recorded there are CLOSED; nothing deferred.

## Findings closed

| Finding | Severity | Fix commit | One-line what |
|---|---|---|---|
| R3-F1 | MEDIUM | `12ca3ea5` | `validateToldTmuxIdentity` now refuses session names tmux would vis-encode (ASCII control chars, DEL, invalid UTF-8) — the second half of tmux's silent session-name rewrite, alongside R2-F1's `.`/`:` half |
| R3-F2 | NIT | `7af91088` | `loadOrInitStateLocked` re-stamps `reed.json`'s `socket`/`session` identity diagnostic from told geometry on every load, so a renamed worktree's state file stops recording an identity reed no longer drives |

Plus one invited follow-up (not a finding): commit `cd4d655a` adds the two sandbox-suite scenarios round 2's fixer report named but did not add — M20 (rewrite-refusal, now covering BOTH rewrite classes) and M21 (default-socket purity) — `TestSandboxCoverage_AllModulesCoveredOrExcluded` stays green.

## Changed files

- `internal/reedengine/server.go` — new `firstVisEncodedSessionNameByte` helper (documents the vis-encode class with the live tmux 3.6 probe results) + the refusal branch in `validateToldTmuxIdentity`; `rewrittenSessionNameChars` doc corrected (it no longer claims "no other character is touched").
- `internal/reedengine/doc.go` — the "Silent session-name rewriting" contract bullet now states both rewrite halves honestly.
- `internal/reedengine/geometry.go` — `SessionName` field doc extended to the full banned class.
- `internal/reedcli/cli.go` — the `reed` group `Long` help extended (CLI/Cobra Invariant help-accuracy obligation).
- `internal/reedengine/server_test.go` — 10 new table rows: five refusal rows (TAB, NL, ESC, DEL, BEL, invalid 0xFF) and four accept rows (space-only, unicode, literal U+FFFD, `a#b%c=d-e`) so an over-broad fix fails too.
- `internal/reedengine/contract_integration_test.go` — `TestSessionNameRewriteIsSilentAndExactTargetsMissIt` gains a `tab` case pinning the wire fact (raw TAB → the two literal characters `\t`, exit 0, exact target misses).
- `internal/reedengine/spawn.go` + `spawn_test.go` — R3-F2 re-stamp; `TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIdentity` replaces the old verbatim-identity assertion (strand data still asserted verbatim).
- `tools/sandbox/SANDBOX-REED-SUITE.md` — M20/M21 scenarios, ref-range and session-log updated.

## How each fix was verified

**R3-F1**
- Unit: `go test ./internal/reedengine/` — the new refusal rows fail without the fix (they call `validateToldTmuxIdentity` directly) and pass with it; the accept rows prove no over-refusal.
- Integration: `go test -tags integration -run TestSessionNameRewrite` — dot/colon/tab all pass against real tmux 3.6.
- Live, end-to-end, deployed binary (re-deployed after the change): the same `svc<TAB>3` worktree that pre-fix burned a 20s attempt window, errored opaquely, stranded an unaddressable session, and then let `reed down` report `ok:true` while leaving it — now refuses in **0.008s** with `tmux will not create session "svc\t3" verbatim: it contains '\t', ... rename the worktree directory ...`, `down` refuses equally loudly (matching R2-F1's posture), and the shared server carries zero new sessions. The unicode worktree `svc-åäö-⚙` still `up`s/`down`s cleanly (no over-refusal live).

**R3-F2**
- Unit: the rewritten spawn_test asserts re-stamped identity + verbatim strand data.
- Live: renamed `svc-åäö-⚙-x` → `svc-åäö-⚙-y`; next `reed up` produced `.lyx/reed.json` with `session: svc-åäö-⚙-y` (previously it would have kept the stale `-x`), strand table intact.

## Full verification sweep (after all fixes, final deployed binary @ `cd4d655a`)

- `go build ./...`, `go vet` (reed pkgs), `go test ./...` whole-repo, `go test -count=5` on reedengine/reedcli/hubgeom/cmd-lyx — all green.
- `go test -tags integration` on reedengine + reedcli — green.
- Full tmux-only smoke suite — 18 PASS, 1 expected SKIP; **3× concurrent** sweeps (claude test excluded) — all three `ok` (~30s each).
- `TestSmokeClaudeResumeRecallsCodeword` run once, deliberately, by exact name — PASS (8.05s), one real `claude` on the test's own `--model haiku` pin (run during Job 1; no fix touched its code path).
- Teardown: all fixture sessions `reed down`'d through the CLI; `pgrep -x tmux` → zero tmux processes; the operator's default socket reports "no server running" (it never hosted one all session).

## Observations, stated plainly

- `golangci-lint run` (no repo lint config, tool defaults) reports one pre-existing errcheck hit: `internal/reedengine/lock.go:93` `defer l.Release()`. It predates this campaign, is the repo's deliberate best-effort defer-release pattern, and was not a finding of this round's review — left untouched rather than churned post-review.
- The live fixture hub under the session scratchpad could not be deleted (permission gate on the removal command); it holds no processes and the scratchpad is session-isolated and auto-cleaned.
- Windows verification gap (as the prompt anticipates): psmux's session-name vis-encode behavior is unprobed on this Linux host. The fix is conservative for both backends — it refuses rather than sanitizes, so a psmux that happens to accept a control character verbatim would refuse a name it could technically spend; that trade matches the R2-F1 precedent.

## Deferred

Nothing.
