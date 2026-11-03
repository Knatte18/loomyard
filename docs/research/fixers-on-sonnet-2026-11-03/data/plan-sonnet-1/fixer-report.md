# Fixer report

All nine findings fixed as proposed.

## Changes
- F1: card 10 routes writeLease/expireLease through `state.UpdateJSON` (Uses added); card 11 writes the key directory's current-run pointer through `state.WriteJSON` (Uses added); both cite PATTERN-tolerant-records.
- F2: card 11 adds `TestGateTest_RefusesBeforeResolvingTheHub` to Edit and states removal of the help subtest and the unused `shuttleengine` import.
- F3: card 10 drops `TestDetachStore` and `TestStartReading` (Create entries and Tests bullets); card 11's TestJoinable reused-pid row takes the Windows `startReading` cases.
- F4: card 14 adds `internal/loomshed#verifyLineFindings` (Edit) and `//internal/loomshed/doc.go` (Uses), and rewrites the gate-keys sentence, the `verify-gate-no-wait` reason and the finding Detail.
- F5: card 11 Choice (D2) gains the Bound sentence for the uncapped in-process wait.
- F6: card 13 runner paragraph specifies the no-Verify-line answer (ok envelope, `{Code: 0}`).
- F7: card 11's `none` row asserts a fixed line that `helperLaunch` writes to `runner.log`.
- F8: card 4 targets `//internal/shuttlecli/await.go` in place of `awaitCmd`, with the header and `awaitVerb` comment changes in the Intent.
- F9: card 8 Extends relabelled Edit; card 9 gateslot doc moved to Edit, empty Extends group dropped.

## Disputed
None.

## Deferred
None.
