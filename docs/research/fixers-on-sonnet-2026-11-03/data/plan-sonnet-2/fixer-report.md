# Fixer report

All nine findings fixed as proposed in the artifact. Nothing disputed, nothing deferred.

## Changes

- F1: card 10 Run directory paragraph now routes `writeLease` and `expireLease` through `state.UpdateJSON` with the stated mutates, and `internal/state#UpdateJSON` is under Uses. Card 11's key-directory pointer is a JSON record written through `state.WriteJSON`, which is under Uses. Both cite PATTERN-tolerant-records.
- F2: card 11 lists `TestGateTest_RefusesBeforeResolvingTheHub` under Edit, and its Tests paragraph removes the help subtest and the unused `internal/shuttleengine` import.
- F3: card 10 drops `TestDetachStore` and `TestStartReading` from Create and Tests, with a sentence naming the surface rows that cover them. Card 11's `TestJoinable` reused-pid row now drives `startReading("windows", ...)`.
- F4: card 14 adds `internal/loomshed#verifyLineFindings` under Edit and `//internal/loomshed/doc.go` under Uses. Its Intent rewrites the spec's gate-keys sentence, the `verify-gate-no-wait` reason and the finding's Detail.
- F5: card 11 Choice (D2) gains the Bound sentence for the uncapped in-process wait.
- F6: card 13's runner paragraph specifies the ok envelope and `{Code: 0}` for a matched card with no Verify line.
- F7: card 11's `helperLaunch` under `none` writes a fixed line to `runner.log`, and the none row asserts that line on stderr.
- F8: card 4 targets `//internal/shuttlecli/await.go` instead of `awaitCmd`, and its Intent rewrites the header and the `awaitVerb` comment.
- F9: card 8's label is now Edit, and card 9's gateslot doc moved to Edit with the empty Extends group dropped.

## Disputed

None.

## Deferred

None.
