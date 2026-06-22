## Summary of Fixes Applied

Applied all four findings from the code review:

### Finding 1: TestRenderLayerBuckets unaccounted in fold and name-map
- Added TestRenderLayerBuckets case to TestRenderProposalAndShapesHomepage (asserts Layer A and Layer B bucket headers). Commit 04fced9.

### Finding 2: Equivalence guardrail name-map incomplete
- Completed the board name-map in docs/benchmarks/test-suite-timing.md (11 folded names + 2 dropped names with justifications). Commit 9eb022f.

### Finding 3: Count headers contradict lists
- weft "5 dropped" -> "6 folded"; muxpoc "5 dropped" -> "8 folded". Commit 9eb022f.

### Finding 4: os.Getwd in TestRunCLIErrors
- Replaced os.Getwd()/defer os.Chdir with t.Chdir() in TestRunCLISpawnDispatch and TestRunCLIErrors. Commit 931a106.

All verify commands pass (board, worktree, weft, ide, muxpoc).

{"status":"success","commit_sha":"edf85b6f9c78b1f42aa7a652bbbcf231d18c558c","session_id":"f966e41f-9041-4d76-869d-f9f189a9e383"}