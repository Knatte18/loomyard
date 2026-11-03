| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | Card 10 now updates `lease.json` through `state.UpdateJSON(<lease path>, <lease path>.lock, mutate)` with both mutates as specified and `internal/state#UpdateJSON` under Uses. Card 11 adds a `current.json` pointer through `state.WriteJSON` with `internal/state#WriteJSON` under Uses. Both cite PATTERN-tolerant-records, and the signatures match state.go:30 and :148. |
| F2 | fixed | `internal/gatecli#TestGateTest_RefusesBeforeResolvingTheHub` is now in card 11's Edit list. The Tests paragraph removes the subtest "help names the shared keep-cache-warm cap as the slot wait" (cli_test.go:58-62) and the then-unused `internal/shuttleengine` import (cli_test.go:14). |
| F3 | fixed | Card 10 drops both Create glyphs and both Tests bullets, and no reference to either test is left anywhere. Card 11's TestJoinable reused-pid row now uses `startReading("windows", ...)` and covers the nanosecond reading, a failed `CreationTime` and a reused pid. Card 11 already lists `plan:internal/gatecli#startReading` under Uses. |
| F4 | fixed | Card 14 adds `internal/loomshed#verifyLineFindings` under Edit (it encloses plancontext.go:147) and `//internal/loomshed/doc.go` under Uses. Its Intent rewrites the spec's gate-keys sentence, the `verify-gate-no-wait` reason and the finding's Detail. |
| F5 | fixed | Card 11's Choice (D2) paragraph gains a Bound sentence: the wait ends on a holder's release, which is bounded by its tier's `timeout_min` (gateslot/config.go:67), or on the caller's interrupt, which exits 1. |
| F6 | fixed | Card 13's runner paragraph now says a matched card with no `**Verify:**` line makes the runner write the in-process ok envelope, whose text matches cardverify.go:94, and an unmarked `{Code: 0}`. |
| F7 | fixed | Under `none`, `helperLaunch` now writes `gate helper: the runner was never started` to `runner.log`, and the `none` row asserts that line on stderr. |
| F8 | fixed | Card 4's Edit target `internal/shuttlecli#shuttleCLI.awaitCmd` becomes `//internal/shuttlecli/await.go`. Its Intent now rewrites the file header (await.go:1-2) and `awaitVerb`'s comment (await.go:17-18) as proposed. |
| F9 | fixed | Card 8's label is now **Edit:**. Card 9 moves `//internal/gateslot/doc.go` under **Edit:** and drops the empty Extends group. |

## New defects

- LOW, `14-agent-gate-calls.md` ImpactSummary: it still names only the four webster render sites and says the card Verify rule stays. It leaves out the card's new edit of `loomshed#verifyLineFindings`'s finding Detail and the spec's gate-keys sentence, so the blast-radius line no longer matches the card's targets.
- NIT, `04-await-resolved-cap.md` ImpactSummary: it still reads "one verb's default cap and help text, and one constant's doc comment". The card's target is now the whole `await.go`, whose file header and `awaitVerb` comment it also rewrites.

TALLY fixed=9 partial=0 open=0 new=2
