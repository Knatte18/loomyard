1 of 1 card committed — the batch's single card (Card 38) was completed with a matching commit subject.

{"status":"success","commit_sha":"81b3a70455a23277d609bfcc7344dbf9fc8ecaaf","session_id":"b8b7acdf-c032-4d66-919e-325e59eb7539","cards_done":[38]}

Summary: 1 of 1 card committed (Card 38: Audit and adopt across burler/fabric/shuttle). Files edited:
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/burlerengine/engine.go` — added `logger.Warn` on the `os.MkdirAll(burlerDir, ...)` and `os.MkdirTemp(burlerDir, "round-")` failure branches.
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/fabricengine/spawn.go` — added `logger.Warn` naming `exe`/`args` on `cmd.Start()` failure in `SpawnDetachedPush`, preserving the "not Wait()ed" detached-start behavior.
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/shuttleengine/run.go` — added `logger.Warn` naming `runDir`/`strandGUID` on the `saveRunState` failure branch, before its existing cleanup/return.

Verify command `go test ./internal/burlerengine/... ./internal/fabricengine/... ./internal/shuttleengine/...` passed (all green). Working tree is clean, commit pushed to `origin/trace-logging`.

{"status":"success","commit_sha":"81b3a70455a23277d609bfcc7344dbf9fc8ecaaf","session_id":"b8b7acdf-c032-4d66-919e-325e59eb7539","cards_done":[38]}
