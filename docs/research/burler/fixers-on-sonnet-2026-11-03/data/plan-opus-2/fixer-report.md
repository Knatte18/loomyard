# Fixer report — plan-opus-2

## Changes

- F1 (fixed as proposed): `10-gate-detached-runner.md`'s Run directory paragraph now has `writeLease` and `expireLease` update `lease.json` through `state.UpdateJSON(<lease path>, <lease path>.lock, mutate)`. `writeLease`'s mutate keeps the later deadline and `expireLease`'s writes now less a second. The paragraph cites PATTERN-tolerant-records, and `internal/state#UpdateJSON` is added under **Uses:**. `11-gate-capped-front.md`'s Front paragraph now says the key directory names the current run id in `current.json`, written through `state.WriteJSON` with its own lock file `current.json.lock`, which is not the key directory's held `lock`, since `UpdateJSON`'s doc warns that nesting on a held lock hangs. It cites PATTERN-tolerant-records, and `internal/state#WriteJSON` is added under **Uses:**.
- F2 (fixed as proposed): `11-gate-capped-front.md` adds `internal/gatecli#TestGateTest_RefusesBeforeResolvingTheHub` to **Edit:**. Its Tests paragraph removes the subtest "help names the shared keep-cache-warm cap as the slot wait" and the then unused `internal/shuttleengine` import, which line 62 of `cli_test.go` at c7cde04 shows is the import's only use.
- F3 (fixed as proposed): `10-gate-detached-runner.md` drops `TestDetachStore` and `TestStartReading` from **Create:** and drops their Tests bullets. In `11-gate-capped-front.md`, `TestJoinable`'s reused-pid row now drives `joinable` with a reader built from `startReading("windows", ...)` over injected creation times. That one row pins the Unix-nanosecond reading, the false answer when `CreationTime` fails, and two creation times telling a reused pid apart.
- F4 (fixed as proposed): `14-agent-gate-calls.md` adds `internal/loomshed#verifyLineFindings` under **Edit:** and `//internal/loomshed/doc.go` under **Uses:**. Its Intent now rewrites three texts: loom-plan-spec.md's gate-keys sentence (a Go site runs `--wait`; agent card, batch and `webster-fix` calls do not), the `verify-gate-no-wait` reason, and the `verifyGateNoWaitCheck` Detail. The last two now give the reason that still holds.
- F5 (fixed as proposed): `11-gate-capped-front.md`'s Choice (D2) paragraph gains a Bound sentence. An uncapped in-process slot wait ends only when a holder releases its slots, each holder's run bounded by its tier's `timeout_min`, or when the caller interrupts.
- F6 (fixed as proposed): `13-card-verify-capped.md`'s runner paragraph now says that a matched card with no `**Verify:**` line in the runner's read gets the in-process ok envelope on `stdout` and an unmarked exit `{Code: 0}`.
- F7 (fixed as proposed): `11-gate-capped-front.md`'s helper description now has `helperLaunch` under `none` write `gate helper: the runner was never started` to `runner.log`, and the `none` row asserts that line on the front's stderr.
- F8 (fixed as proposed): `04-await-resolved-cap.md` replaces `internal/shuttlecli#shuttleCLI.awaitCmd` under **Edit:** with `//internal/shuttlecli/await.go`. Its Intent now rewrites the file header and `awaitVerb`'s comment.
- F9 (fixed as proposed): in `08-runproc-gate-role.md` the label over `//internal/runproc/doc.go` changes from **Extends:** to **Edit:**. In `09-run-records-call-id.md`, `//internal/gateslot/doc.go` moves into **Edit:** and the now empty **Extends:** group is removed.

## Disputed

None.

## Deferred

None.
