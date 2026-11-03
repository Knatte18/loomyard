# Fixer report

All nine findings were checked against the code at c7cde044 (`internal/state/state.go:148` UpdateJSON, `internal/gatecli/cli_test.go:14,59-66`, `internal/shuttlecli/await.go:1-2,17-18`, `internal/loomshed/plancontext.go:141-147`, `contracts/specs/loom-plan-spec.md:320,594-596`, `internal/gatecli/cardverify.go:95-97`, `PATTERN.md:43`). Every premise held, and every finding is fixed.

## Changes

- **F1** (fixed as proposed). `artifact/10-gate-detached-runner.md`: the Run directory paragraph says that `writeLease` and `expireLease` update `lease.json` through `state.UpdateJSON(<lease path>, <lease path>.lock, mutate)`. `writeLease`'s mutate keeps the later deadline, and `expireLease`'s writes now less a second. The paragraph cites PATTERN-tolerant-records. `internal/state#UpdateJSON` is added under **Uses:**. `artifact/11-gate-capped-front.md`: the Front paragraph says the key directory names the current run id in a JSON record, `current.json`, written through `state.WriteJSON` (PATTERN-tolerant-records). `internal/state#WriteJSON` is added under **Uses:**.
- **F2** (fixed differently). `artifact/11-gate-capped-front.md`: the **Edit:** list gains `//internal/gatecli/cli_test.go`. The proposed member target `internal/gatecli#TestGateTest_RefusesBeforeResolvingTheHub` was not used. The change also removes the file's `internal/shuttleengine` import, and the file header ("the slot wait its help text names", cli_test.go:3) becomes false. Both of those are file-level text that a member target does not cover. Card 4 uses a file target for the same reason under F8. The Tests paragraph says that the subtest "help names the shared keep-cache-warm cap as the slot wait" is removed, along with the unused import and the header's slot-wait clause.
- **F3** (fixed as proposed). `artifact/10-gate-detached-runner.md`: `TestDetachStore` and `TestStartReading` are dropped from **Create:**, and so are their two Tests bullets. `artifact/11-gate-capped-front.md`: `TestJoinable`'s reused-pid row now drives `joinable` with a reader built from `startReading("windows", ...)` over injected creation times. The row pins three things: the Unix-nanosecond reading, the false answer when `CreationTime` fails, and two creation times telling a reused pid apart.
- **F4** (fixed as proposed). `artifact/14-agent-gate-calls.md`: `internal/loomshed#verifyLineFindings` is added under **Edit:** and `//internal/loomshed/doc.go` under **Uses:**. The Intent now rewrites three texts:
  - the spec's gate-keys sentence: a Go site runs `--wait`, and agent card, batch and `webster-fix` calls carry none;
  - the spec's `verify-gate-no-wait` reason;
  - the Detail of the plan check's finding, which now gives the reason that holds: Go re-runs the line outside an agent hook, and a call without `--wait` is a slot-held line under the card-verify runner's timeout.
- **F5** (fixed as proposed). `artifact/11-gate-capped-front.md` Choice (D2) gains a Bound sentence. An in-process slot wait has no cap and ends when a holder releases its slots. Each holder is bounded by its tier's `timeout_min`, and the caller's interrupt cancels the wait and exits 1.
- **F6** (fixed as proposed). `artifact/13-card-verify-capped.md`: in the runner paragraph, a matched card with no `**Verify:**` line makes the runner write the in-process ok envelope (`card <id> has no **Verify:** line; nothing to run`) to `stdout` and an unmarked `{Code: 0}`.
- **F7** (fixed as proposed). `artifact/11-gate-capped-front.md`: under `none`, `helperLaunch` writes `gate helper: the runner was never started` to `runner.log` before it returns the ended pid. The `none` row of `TestGateTest_DetachedRun` asserts that line on stderr.
- **F8** (fixed as proposed). `artifact/04-await-resolved-cap.md`: **Edit:** replaces `internal/shuttlecli#shuttleCLI.awaitCmd` with `//internal/shuttlecli/await.go`. The Intent rewrites the file header, which now says the verb reads the keep-warm config inside a worktree. It also rewrites `awaitVerb`'s comment, which now says resolution is still skipped because the verb resolves only its own config dirs in a worktree and needs no hub.
- **F9** (fixed as proposed). `artifact/08-runproc-gate-role.md`: the label over `//internal/runproc/doc.go` is changed from **Extends:** to **Edit:**. `artifact/09-run-records-call-id.md`: `//internal/gateslot/doc.go` moves to **Edit:**, and the empty Extends group is dropped.

## Disputed

None.

## Deferred

None.
