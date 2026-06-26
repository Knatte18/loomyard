All cards have been successfully implemented and tested. The batch consolidates the drift and status read-path tests by folding redundant standalone tests into siblings as pre-condition checks and additional phases, reducing fixture builds while preserving all coverage assertions.

Commit history:
- Card 7: Fold PairInSync_InSync into BrokenJunction pre-check
- Card 8: Fold PairInSync_JunctionPointsElsewhere into BrokenJunction
- Card 9: Fold Status_PairedViewFields into InSyncVsDrifted pre-check
- Card 10: Fold Status_CodeguidePollutionReportOnly into LyxPollutionDetected

Verify command output confirms all TestPairInSync and TestStatus tests pass.

{"status":"success","commit_sha":"450a4abaffc7505b89128ca6f4cab1b0ec0ab4c5","session_id":"8ca05ef7-e989-4a34-be41-af68b550c89f"}
