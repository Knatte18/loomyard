All findings processed and verified.

**Findings from `20260801-082441-code-review-r1.md`:**

1. **[NIT] Header bytes not counted toward the durable-sink size cap** (`internal/logger/sink.go:201` vs `249-258`) — VERIFY: accurate (confirmed `sinkBytesWritten` was only touched inside `writeDurable`, never after the header write). HARM CHECK: none. **FIX applied**: capture `headerLine()` once into `line`, write it, and set `sinkBytesWritten = int64(len(line))` after the write succeeds.

2. **[NIT] Opened file handle leaks if the header write fails** (`internal/logger/sink.go:195-204`) — VERIFY: accurate (the error path returned without closing `f`). HARM CHECK: none. **FIX applied**: added `_ = f.Close()` on the header-write error path before returning `sinkOK = false`.

3. **[NIT] Plan's Card 41 `Edits:` list omits two files** — the review itself states "Fix: None needed for this round" (plan-authoring inconsistency only, implementation already correct). VERIFY: accurate as described, but HARM CHECK / explicit reviewer instruction means **no code or plan change required** — treated as no-op per the review's own disposition.

Verified no data race is introduced: `durableWriter` (`internal/logger/logger.go:298-301`) always calls `ensureDurableSink()` before `writeDurable(p)`, and `sync.Once.Do` blocks all concurrent callers until the initializer completes, so the `sinkBytesWritten` write inside `sinkOnce.Do` happens-before any subsequent `writeDurable` call.

**Verification run** (all from `/home/knatte/Code/loomyard/wts/trace-logging`):
- `go build ./...`, `go vet ./...` — clean
- `goimports -w internal/logger/sink.go` — no diff (already formatted)
- `golangci-lint run ./internal/logger/...` — clean
- All 13 non-null batch `verify:` commands (hubgeometry, logger x4 incl. `-race`, cmd/lyx integration, reedengine, treadleengine, burler/fabric/shuttle, perchengine, scoutengine) — all passed. Batch 14's verify is `null`, skipped.

**Commit:** `83ccb8009a75567fcd42edfa03231a051fb3fe23` — `fix(logger): count header bytes in size cap; close fd on header-write failure`, pushed to `trace-logging`. Baseline HEAD was `72525a9f4cc0f8b6bd26e99a71dda36507e77867` (the holistic-fix housekeeping commit); HEAD now differs. `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes).

File touched: `/home/knatte/Code/loomyard/wts/trace-logging/internal/logger/sink.go`

{"status":"success","commit_sha":"83ccb8009a75567fcd42edfa03231a051fb3fe23","session_id":"4dc1a3dd-76d7-4a83-a637-7445d4555553"}
