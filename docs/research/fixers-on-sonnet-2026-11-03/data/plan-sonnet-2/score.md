| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | Card 10's Run directory paragraph now routes writeLease/expireLease through `state.UpdateJSON(path, lockPath, mutate)` (matches state.go:148) with `internal/state#UpdateJSON` under Uses; card 11 names the run-id pointer a `state.WriteJSON` record (state.go:30) with `internal/state#WriteJSON` under Uses; both cite PATTERN-tolerant-records. |
| F2 | fixed | Card 11 Edit lists `internal/gatecli#TestGateTest_RefusesBeforeResolvingTheHub`, and Tests removes the subtest at cli_test.go:58 and the `shuttleengine` import, whose only other use is that subtest's line 62. |
| F3 | fixed | TestDetachStore/TestStartReading are gone from card 10's Create and Tests, which now map each helper to a covering surface row; card 11's TestJoinable reused-pid row drives `startReading("windows", ...)` over injected creation times. |
| F4 | fixed | Card 14 adds `internal/loomshed#verifyLineFindings` (plancontext.go:123) under Edit and `//internal/loomshed/doc.go` under Uses, and its Intent rewrites spec line 320 (the every-site sentence), line 595 (the 75 rationale) and the Detail at plancontext.go:147; no loomshed test asserts that Detail text. |
| F5 | fixed | Card 11 Choice (D2) gains the Bound sentence: the wait ends when a holder releases its slots, each run being bounded by its tier's `timeout_min`, or on the caller's interrupt, which exits 1. |
| F6 | fixed | Card 13 adds the no-Verify-line branch, which writes the ok envelope to `stdout` with the exact text from cardverify.go:94, plus an unmarked `{Code: 0}`. |
| F7 | fixed | Card 11 helperLaunch `none` writes `gate helper: the runner was never started` to `runner.log`, and the `none` row asserts that line on stderr. |
| F8 | fixed | Card 4 Edit swaps `awaitCmd` for `//internal/shuttlecli/await.go`, and its Intent rewrites the header (await.go:1-2) and the awaitVerb comment (await.go:17-18) as prescribed; Create `plan:` awaitCap resolves not_found, so redundant-file-target skips it. |
| F9 | fixed | Card 8's label is now `**Edit:**`; card 9 moves `//internal/gateslot/doc.go` into Edit and drops the empty Extends group. |

## New defects

none

TALLY fixed=9 partial=0 open=0 new=0
