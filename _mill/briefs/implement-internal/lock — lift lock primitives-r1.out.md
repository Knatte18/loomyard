All tasks completed successfully:

1. Card 1: Created `internal/lock/lock.go` with verbatim lift of `FileLock`, `AcquireWriteLock`, `AcquireReadLock`, and `Release` from `internal/board/lock.go`
2. Card 2: Created `internal/lock/lock_test.go` by porting the tests from `internal/board/lock_test.go` with updated package declaration and imports
3. Verify: `go test ./internal/lock/...` passed successfully

Files created:
- `C:\Code\mhgo\wts\extract-shared-infra\internal\lock\lock.go`
- `C:\Code\mhgo\wts\extract-shared-infra\internal\lock\lock_test.go`

{"status":"success","commit_sha":"db061781d8078364ecd19cab1790aef9a693b4ad","session_id":"40613790-1c94-4b85-bfce-3c3c6d8b4932"}