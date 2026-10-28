---
round: 1
focus:
  - "internal/shedverbs/loopchild.go: does the retry loop start the child at most five times, double the wait from 100 ms through LoopSpec.Sleep, retry only ETXTBSY, and log each retry at Warn through internal/logger?"
  - "internal/shedverbs/loopchild.go and loop.go: does the start-failed cause read `start-failed after <n> tries: <error>` with the ETXTBSY meaning added, and does interruptedTraceFile fall back to the loop's own trace file only when the child left no trace and no non-empty stderr? Check how the trace path reaches it from internal/loomcli/looparm.go and internal/battencli/arm.go, including the empty-path case."
  - "internal/proc/spawnbusy.go: is IsSpawnBusy correct for wrapped errors (fork/exec PathError, syscall.Errno), and does it build on every platform the package targets?"
  - "tools/deploy/main.go: is the temp file created in the install directory, removed on build or rename failure, and renamed atomically over the target? Check file mode, the Windows path, and a leftover temp file after success."
  - "Tests: do the new loop and deploy tests cover each acceptance criterion in the decision record without duplicating existing tests, and do they sit out no real waits?"
  - "Docs: do the proc and shedverbs doc.go changes cover the new behavior, and does any PATTERN invariant need an entry?"
---

Focus covers each decision, D1 to D4, and the repository's docs-in-same-commit rule.
