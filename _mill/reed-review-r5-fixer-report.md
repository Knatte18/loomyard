# reed — round 5 fixer report (`opus-high-r5`)

Companion to `_mill/reed-review-r5.md`.
Every finding recorded in that review is fixed — 8 of 8, all severities including the NIT.
**Deferred: nothing.**

| Finding | Severity | Commit | Fix |
| --- | --- | --- | --- |
| R5-F3 | BLOCKING | `24c475a0` | An inconsistent binding table no longer destroys unrelated live panes |
| R5-F2 | MEDIUM | `80909e4c` | Pane bindings carry the session generation that minted them |
| R5-F5 | MEDIUM | `2c7e88ac` | A renamed worktree refuses instead of double-launching every strand |
| R5-F4 | MEDIUM | `ab36dcb6` | A persisted pane id is never a tmux target outside its own session |
| R5-F1 | MEDIUM | `c4ff17cb` | A corrupt `reed.json` fails actionably; `null` no longer loses the table silently |
| R5-F8 | NIT | `788dd562` | `requireSessionLocked` stops claiming "nothing persisted" for an unreadable file |
| R5-F6 | LOW | `c95de3db` | An op whose lock file vanished mid-run reports that it ran without exclusion |
| R5-F7 | LOW | `5e145a71` | R4-F3's own guard is no longer disarmed by the litter it exists to prevent |

Commit order follows severity and dependency, not the finding numbers: R5-F2's pane generation is the
mechanism R5-F5 and (in the common case) R5-F4 are built on, so it lands before them.

## Where the fixes live

**One root cause under four of the findings.** Reed trusted a persisted `PaneID` absolutely: nothing
on disk recorded which session incarnation minted it, nothing checked it belonged to this worktree's
session, and nothing checked the table was internally consistent. The fixes are three independent
layers rather than one, so no single one failing open re-opens the whole class.

- `internal/reedengine/generation.go` (new) — the pane-generation stamp: `PaneGeneration`'s probe
  (`display-message -p -t '=<session>:' '#{session_id}|#{pid}|#{session_created}'`), the
  adopt/clear/refuse decision, and the still-alive-foreign-session refusal.
- `internal/reedengine/state.go` — `PaneGeneration` type + `ReedState` field, `ReedState.UnmarshalJSON`
  (rejects a bare `null`), `unreadableStateError`.
- `internal/reedengine/reconcile.go` — `clearConflictingPaneBindings`.
- `internal/reedengine/render/policy.go` + `rules.go` — `removeDuplicatePaneCells`, wired into `Rules`.
- `internal/reedengine/spawn.go` — both repairs wired into `loadOrInitStateLocked`, the single load
  chokepoint every op passes.
- `internal/reedengine/lifecycle.go` — the pre-boot foreign-session refusal, `noSessionMessage`'s
  readable/unreadable split.
- `internal/reedengine/io.go` — `resolvePaneInThisSessionLocked`, shared by all three transport ops.
- `internal/reedengine/strand.go` — `paneIDsInSession`, filtering `RemoveStrand`'s kill list.
- `internal/reedengine/lock.go` — the post-op lock-identity check.

Docs updated in the same commits as the behaviour they describe: `internal/reedengine/doc.go` (two
new load-bearing-assumption bullets — pane ids are not stable across a server rebirth, and duplicate
pane cells destroy panes), `docs/overview.md` (the `.lyx/` paragraph now states the generation),
`tools/sandbox/SANDBOX-REED-SUITE.md` (new M23/M24 scenarios + log rows; `sandbox_coverage_test.go`
green). `CONSTRAINTS.md` is untouched: nothing here moves a cross-cutting invariant — every change
sits inside reed's own contract.
`manifest/roadmap.md` deliberately untouched (hardening, not a planned item).

## Tests added

Hermetic:

- `internal/reedengine/reconcile_test.go` — `TestClearConflictingPaneBindings` (6 rows).
- `internal/reedengine/render/rules_test.go` — `TestRules_NeverEmitsOnePaneNumberTwice` (4 rows,
  asserting over parsed layout cells, not string matching) and
  `TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide`.
- `internal/reedengine/generation_test.go` (new) — `TestParsePaneGeneration` (6 rows),
  `TestPaneGeneration_RecordedAndSameIncarnation` (5 rows), `TestAdoptPaneGenerationLocked` (7 rows
  covering adopt / keep / clear-same-session / clear-gone-session / refuse-live-foreign /
  namesake-is-not-the-orphan / probe-failure-fails-open), driven through `TmuxCmd`'s `execHook` seam.
- `internal/reedengine/strand_test.go` — `TestPaneIDsInSession` (5 rows),
  `TestResolvePaneInThisSessionLocked` (2 rows), and
  `TestRemoveStrand_NeverKillsAPaneOutsideThisSession`, which pins the CALL SITE rather than the
  helper.
- `internal/reedengine/state_test.go` — `TestLoadState_UnreadableFileIsActionable` (5 rows, including
  both `null` spellings).
- `internal/reedengine/lock_test.go` — `TestWithOpLock_ReportsALockFileRemovedMidOperation`,
  `TestWithOpLock_ReportsBothFailuresWhenTheOperationAlsoFailed`,
  `TestWithOpLock_QuietWhenTheLockFileSurvives`.
- `internal/reedengine/lifecycle_test.go` — a new `noSessionMessage` row for the unreadable case.
- `internal/reedengine/server_test.go` — R4-F3's guard made litter-proof and self-cleaning.

Live (`//go:build smoke`), `internal/reedcli/smoke_staterecovery_test.go` (new):

- `TestSmokeStaleStateFileIsNotMistakenForLiveStrands` (R5-F2) — asserts BOTH `live:false` and that
  `resume` actually rebuilds, since asserting only the first would pass for a fix that stopped
  trusting the binding without restoring the strand.
- `TestSmokeRenamedWorktreeRefusesRatherThanDoubleLaunching` (R5-F5) — asserts the refusal names the
  orphan and the remedy, that the refusal deposits no session of its own, and that following the
  named remedy actually clears it.
- `TestSmokeRemoveNeverKillsASiblingWorktreesPane` (R5-F4) — accepts either a refusal or a success
  from the remove, and fails only on the sibling's pane dying.

## Sabotage-proofs performed

Not exhaustive across all eight (the orchestrator's independent verification is the place for that),
but the three most load-bearing were proved and restored to an empty diff:

- Reverting `removeDuplicatePaneCells`'s call in `render/rules.go` → `TestRules_NeverEmitsOnePaneNumberTwice`
  fails on all four rows, printing the real duplicate layout strings
  (`"ada2,100x40,0,0[100x1,0,0,1,100x38,0,2,1]"` — pane 1 twice).
- Reverting `RemoveStrand`'s kill loop to the unfiltered `paneIDs` →
  `TestRemoveStrand_NeverKillsAPaneOutsideThisSession` fails with `killed=[%7]`.
- Reverting `validateToldAnchorPath`'s call site → R5-F7's hardened guard still goes red at the
  intended assertion (so hardening it did not weaken it), and it stays green with the litter
  deliberately pre-planted.

## Verification

Hermetic, all green after the last commit:

```
go build ./...                                                              OK
go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...  OK
go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...
    ok reedengine 0.795s | ok render 0.005s | ok reedcli 0.009s | ok hubgeom 0.003s | ok cmd/lyx 0.730s
go test ./...                        whole repo, no non-ok package
go test -tags integration ./...      whole repo, no non-ok package
```

Live smoke (`-tags smoke -run Smoke -skip ClaudeResume`): **22 top-level PASS + 4 subtests = 26 PASS
lines, 1 SKIP** (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, psmux-specific), 0 FAIL.
For comparability with round 4's "23 PASS / 1 SKIP": that count included subtests, and the
pre-existing suite had 19 top-level PASS + 4 subtests; this round adds 3 top-level tests.
`TestSmokeClaudeResumeRecallsCodeword` was not run at all — this round's scope never required a real
`claude` process, so none was spawned.

Live re-drive of every scenario, on BRAND-NEW fixtures (`sigmahub-HUB/{wt-one,wt-two}`, names not
used during the review), against the binary deployed from the final commit:

| Scenario | Before | After |
| --- | --- | --- |
| 1 truncated / `null` `reed.json` | opaque `unmarshal state` error; `null` silently reported `ok:true, strands:[]` | both name the file and both remedies; the strand's process survives, and deleting the file by hand keeps the session usable with the process still running |
| 2 stale file across a rebirth | `status` said `live:true` for a process running nowhere; `resume` said `resumed:0` | `live:false`; `resume` reports `resumed:1` and the process really runs |
| 2b inconsistent bindings (F3) | one `up` cut the session from 2 panes to 1, reported `ok:true`, then reported the strand live on the header | panes before=2, after=2; strand reported `live:false` |
| 2c copied `.lyx` (F4) | `remove` in `wt-one` killed `wt-two`'s pane and its process, `ok:true` | refused; `wt-two`'s process count stays 1 |
| 3 `.lyx` removed mid-op | silently granted a second lock (11024 ms vs 107 ms) | the op reports that its lock did not exclude other reed processes |
| 4 renamed worktree | `resume` reported `ok:true, resumed:1`; the strand ran TWICE; the orphan survived `down` forever | refused, naming the orphan and the `kill-session` remedy; process count stays 1; no second session deposited |

Teardown after every phase: `ps -eo comm | grep -cx 'tmux: server'` = **0**, and `tmux -L <socket> ls`
= `no server running` for all five sockets used (`lyx-kappahub-…`, `lyx-omegahub-…`,
`lyx-thetahub-…`, `lyx-sigmahub-…`, `r5probe`). Zero stray strand processes. `git status --porcelain`
empty; no `.lyx` left anywhere under `internal/` or `cmd/`.

## Prior rounds re-verified, not just cited

- **R3-F2 (diagnostic-only staleness, not a targeting bug)** — confirmed by driving the rename, not
  by reading the report: after `mv svc-orig svc-moved` the engine addressed `svc-moved` and re-stamped
  `reed.json`'s `socket`/`session`. Targeting is derived fresh from the current worktree path on every
  invocation, exactly as claimed.
- **R4-F4 generalizes** — the even-vertical header-split retry held through every scenario in this
  round, including repeated `up`s against a 1-row band with a corrupt table.
- **R4-F5's fix holds, but its SYMPTOM was reachable by another route** — adoption never misfired in
  any scenario, but "a strand reported live against a pane running something else" came back through
  the binding-trust path (R5-F2/R5-F3), which R4-F5 did not cover. Both routes are now closed.
- Rounds 1–3's regression guards all stay green in the runs above (the rewrite-refusal smoke test,
  `sessionReapRoots`, the socket-scoped capability probe).

## Deviations and residues, stated plainly

- **No Job 1 → Job 2 sequencing exception.** All eight findings were formed and the review committed
  (`379e2094`) before any production or test file was touched. R5-F7 surfaced while establishing the
  round's BASELINE hermetic gate — that is Job 1 work, and its provenance is stated in the review file
  itself, not only here.
- **The three-layer defence is deliberate overlap, not redundancy I failed to collapse.** The
  generation stamp fails open on a probe error and on an unstamped file; the consistency repair and
  the session-membership checks are unconditional. Collapsing them would make one probe failure
  re-open a destructive path.
- **An unstamped `reed.json` with live bindings is adopted, not cleared.** That state is only
  reachable by upgrading the binary while a session runs, where the bindings are in fact valid;
  clearing them would tear down and relaunch every running strand on the first post-upgrade op. Stated
  in `adoptPaneGenerationLocked`'s doc comment as a deliberate trade.
- **R5-F6 detects, it does not prevent.** An unlinked inode is unreachable by name, so no name-based
  lock can make the second op wait. The code says so rather than implying otherwise.
- **Windows not driven** — this host is Linux. R5-F6's *repro* is POSIX-shaped (flock inode
  semantics); its detection fix is portable (`os.SameFile`). Named, per the prompt.
- **Genuine two-process concurrency for R5-F6** used an external `flock` holder rather than two real
  `lyx` processes for the timing measurement; the detection half WAS driven with a real `lyx` op raced
  by `rm -rf .lyx`.
- **Two observations were recorded as explicitly NOT findings** (see the review's own section):
  `fsx.AtomicWriteBytes` not fsyncing, and `.tmp-*` litter after a `kill -9`. Both fixes belong in
  `internal/fsx`, shared by `state`/`websterengine`/`shedengine`, and are a repo-wide durability
  decision rather than reed's to make unilaterally in a scoped round. They are named so a later round
  can pick them up, not deferred findings.

## Convergence recommendation

**Do not converge on reed yet — but the remaining risk is now narrow and nameable, not open-ended.**

This round was spent to decide whether the campaign ends. It found a real, coherent defect class that
four prior rounds had not probed, including the campaign's second BLOCKING finding and two shapes that
silently destroyed a *sibling worktree's* running work while reporting `ok:true`. That is not the
profile of a converged module.

What has changed, though, is that the class now has a named mechanism and three layers of defence
rather than a growing list of symptoms. My own judgement on what a round 6 should and should not be:

- **Worth one more round, scoped to what THIS round changed.** Five of eight fixes touch the single
  load chokepoint every op passes, and one adds a tmux round trip to `loadOrInitStateLocked` — a hot
  path shuttle drives. A fresh reviewer should probe the new code the way this round probed the old:
  what happens when the generation probe fails intermittently, when two worktrees race the same
  socket through the new pre-boot refusal, and whether the refusal can wedge an operator who cannot
  reach tmux directly.
- **NOT worth another general safety pass.** Rounds 1–4 exhausted the told-identity family and the
  reap/TmuxCmd families; this round exhausted the four named loss modes. A fifth general sweep would
  re-walk closed ground.

**Merge-readiness verdict: MERGE-READY.** Every gate is green (`build`/`vet`, `-count=5` on all reed
packages, whole-repo `go test ./...`, `-tags integration ./...`, the full tmux-only smoke suite at
22+4 PASS / 1 expected SKIP), every fix is verified live on fresh fixtures, teardown is clean, and
nothing is deferred. The branch is in better shape than it was handed to me in a literal sense too:
it arrived with a RED hermetic gate (R5-F7) and leaves with none.
