| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | Card 10's Run directory paragraph routes writeLease/expireLease through `state.UpdateJSON(<lease path>, <lease path>.lock, mutate)` citing PATTERN-tolerant-records, with `internal/state#UpdateJSON` under Uses; card 11 names `current.json` written through `state.WriteJSON` and lists `internal/state#WriteJSON` (both exist at state.go:30 and :148). |
| F2 | fixed | Card 11 adds `//internal/gatecli/cli_test.go` under Edit (a test file, no other member of that file on the card) and its Tests paragraph removes the subtest, the then-unused `shuttleengine` import (used only at cli_test.go:62) and the header's slot-wait clause. |
| F3 | fixed | Card 10's Create list and Tests drop TestDetachStore and TestStartReading; card 11's TestJoinable reused-pid row now drives `startReading("windows", ...)` covering the Unix-nanosecond reading, the false answer and two creation times. |
| F4 | fixed | Card 14 adds `internal/loomshed#verifyLineFindings` (the function holding the Detail at plancontext.go:147) under Edit and `//internal/loomshed/doc.go` under Uses, and its Intent rewrites the spec's gate-keys sentence and the `verify-gate-no-wait` reason and Detail to the reason that holds. |
| F5 | fixed | Card 11's Choice (D2) gains the Bound sentence naming the holders' release, each tier's `timeout_min` (gateslot config.go:67) and the caller's interrupt. |
| F6 | fixed | Card 13's runner paragraph adds the no-Verify-line case writing the in-process ok message (matches cardverify.go:95-97) to `stdout` with an unmarked `{Code: 0}`. |
| F7 | fixed | Card 11's helperLaunch `none` now writes `gate helper: the runner was never started` to `runner.log`, and the `none` row asserts that line on stderr. |
| F8 | fixed | Card 4's Edit replaces the `awaitCmd` member with `//internal/shuttlecli/await.go`, and its Intent rewrites the file header (await.go:1-2) and the `awaitVerb` comment (await.go:17-18). |
| F9 | fixed | Card 8 relabels Extends to Edit over the runproc doc; card 9 moves the gateslot doc into Edit and drops the empty Extends group. |

## New defects

- NIT — `10-gate-detached-runner.md`, Run directory paragraph: `detachRun` "names its files" but the list leaves out the new `lease.json.lock` that the `state.UpdateJSON` call adds to the run directory.

TALLY fixed=9 partial=0 open=0 new=1
