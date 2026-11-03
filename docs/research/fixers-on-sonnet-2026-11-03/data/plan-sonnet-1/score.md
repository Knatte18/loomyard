| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | Card 10 now has writeLease/expireLease going through `state.UpdateJSON(<lease path>, <lease path>.lock, mutate)` with the later-deadline and now-less-a-second mutates, and `internal/state#UpdateJSON` under Uses. Card 11's pointer is a JSON record written through `state.WriteJSON` with `internal/state#WriteJSON` under Uses. Both cite PATTERN-tolerant-records, and both signatures exist at state.go:30 and :148. |
| F2 | fixed | `internal/gatecli#TestGateTest_RefusesBeforeResolvingTheHub` is under card 11's Edit, and the Tests paragraph removes the subtest at cli_test.go:59-66 together with the `shuttleengine` import. That import's only use is line 62. |
| F3 | fixed | Card 10's Create entries and Tests bullets for `TestDetachStore` and `TestStartReading` are gone, and no reference to them is left. Card 11's TestJoinable reused-pid row now drives `startReading("windows", ...)` and covers the nanosecond reading, the false answer when CreationTime fails, and the reused-pid case. |
| F4 | fixed | Card 14 lists `internal/loomshed#verifyLineFindings` under Edit (plancontext.go:123/147) and `//internal/loomshed/doc.go` under Uses. Its Intent rewrites spec line 320's every-site-runs-`--wait` sentence, the reason at line 595 and the Detail of the verifyGateNoWaitCheck finding. |
| F5 | fixed | Card 11's Choice (D2) gains a Bound sentence: the uncapped in-process wait ends when a holder releases its slots, each holder is bounded by its tier's timeout_min, and the caller's interrupt exits 1. |
| F6 | fixed | Card 13's runner paragraph now says that when the matched card has no Verify line, the runner writes the ok envelope (`card <id> has no **Verify:** line; nothing to run`) to stdout with an unmarked `{Code: 0}`. |
| F7 | fixed | Card 11's `none` row now asserts the fixed line `gate helper: the runner was never started`, which `helperLaunch` under `none` writes to runner.log, in place of an empty log. |
| F8 | fixed | Card 4's Edit now names `//internal/shuttlecli/await.go` in place of the awaitCmd member, and its Intent rewrites the file header (await.go:1-2) and the awaitVerb comment (await.go:17-18) as the fix asked. |
| F9 | fixed | Card 8's label is now **Edit:** over runproc/doc.go. In card 9, the gateslot doc moved into the Edit list and the empty Extends group was dropped. |

## New defects

- LOW, `11-gate-capped-front.md`, Front paragraph (the key-directory pointer): "written through `state.WriteJSON` under the key directory's lock" does not say which lockPath WriteJSON gets. WriteJSON takes its own `lock.AcquireWriteLock(lockPath)`, so passing the key directory's `lock` file, which the front already holds through TryAcquireWriteLock, would make the front block on its own lock. The card never names a separate pointer lock path, as card 10 does with `<lease path>.lock`.
- NIT, `11-gate-capped-front.md`, Tests (`TestGateTest_RefusesBeforeResolvingTheHub`): the card removes the help subtest but leaves cli_test.go's file header, which still says the file covers "the slot wait its help text names" and becomes false once the subtest is gone. The header is not among the card's targets.

TALLY fixed=9 partial=0 open=0 new=2
