{"status":"success","commit_sha":"bd97c58e4893c0014dccfb113a4be63c000d1d1f","session_id":"2f5fe224-bcec-4a0e-b60a-52ec02f7e2ca"}

Both findings fixed in docs/benchmarks/test-suite-timing.md:
1. BLOCKING (line 221): removed non-existent CopyBoardRepo from helper list, replaced with CopyHostHub/CopyWeft/CopyPaired plus parenthetical that CopyBoardRepo was evaluated and not needed.
2. NIT (235-242): updated "Reducing wall-clock" item 3 to reflect board/ide now offline; floor is now build/link overhead.
Both test tiers pass. Commit bd97c58.

{"status":"success","commit_sha":"bd97c58e4893c0014dccfb113a4be63c000d1d1f","session_id":"2f5fe224-bcec-4a0e-b60a-52ec02f7e2ca"}
