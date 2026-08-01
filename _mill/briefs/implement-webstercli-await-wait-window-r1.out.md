Clean tree. Both card commits present, matching declared `Commit:` messages exactly. 2 of 2 cards committed — batch complete.

{"status":"success","commit_sha":"2e779a9021af3d6892a6f49e41717bc3cf023841","session_id":"5941ba38-12f3-4b50-aa87-cf5917157b1b","cards_done":[2,3]}

Summary: Both cards in batch `webstercli-await-wait-window` are committed (2 of 2 cards). Card 2 (commit `31f92310`) adds `--wait 1ns` to the `NoReport_WindowElapses` subtest of `TestAwaitBatchCmd_ReportPresenceEnvelope` in `/home/knatte/Code/loomyard/wts/test-suite-overhaul/internal/webstercli/verbs_test.go` and fixes the stale docstring. Card 3 (commit `2e779a90`, same file) folds `TestRecordBatchCmd_DigestEnvelope` and `TestRecordBatchCmd_NoReportEnvelope` into one table-driven `TestRecordBatchCmd_Envelope`, preserving every original assertion.

Verify (`go test -tags integration ./internal/webstercli/...`) passes in 0.232s (down from ~30s). Manual timing confirmations also pass: `TestAwaitBatchCmd_ReportPresenceEnvelope` completes in 0.011s and `TestRecordBatchCmd_Envelope` shows both table rows (`DigestEnvelope`, `NoReportEnvelope`) passing. Working tree is clean; final HEAD is `2e779a9021af3d6892a6f49e41717bc3cf023841`.

{"status":"success","commit_sha":"2e779a9021af3d6892a6f49e41717bc3cf023841","session_id":"5941ba38-12f3-4b50-aa87-cf5917157b1b","cards_done":[2,3]}
