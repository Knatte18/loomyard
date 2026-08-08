All 3 cards (25, 26, 27) declared in the batch file are committed — 3 of 3 cards committed. Verify passed. Working tree is clean.

{"status":"success","commit_sha":"a977f297851a1398a36a9d6f38cf23ed31009b87","session_id":"57212ecd-f685-47b8-88f5-60deea6453ec","cards_done":[25,26,27]}

Summary of work (all 3 of 3 cards committed):

- `internal/fabricengine/status.go` (card 25, commit `c15909cb`) — deleted `PollutionEntry.ReportOnly` and both its writers (the synthetic scan-error entry and the `_raddle` classification branch), narrowed the `git ls-files` pathspec and classification switch to `_lyx` alone, and rewrote the doc comments that enumerated the old `{_lyx, _pattern, _raddle}` scanned set.
- `internal/fabricengine/junction_pattern_integration_test.go` (card 26, commits `eecda51a`, `a977f297`) — renamed `TestDetectHostPollution_PatternTrackedAsRestorable` to `TestDetectHostPollution_LyxTrackedAsRestorable`, dropped the deleted-field assertion, and added `TestDetectHostPollution_ScanErrorIsNonFatal` and `TestDetectHostPollution_RaddleNoLongerReported`.
- `internal/fabriccli/cli_test.go` (card 27, commit `a977f297`) — confirmed no pre-existing `report_only`/`ReportOnly` JSON assertion existed, and added `TestRunCLI_PairsReportsPollutionEntryWithRemedy` pinning the CLI-boundary pollution JSON shape.

Verified: `go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/` passes; `grep -rn 'ReportOnly\|report_only' internal/ cmd/` returns nothing (batch's stated exit condition); working tree is clean.
