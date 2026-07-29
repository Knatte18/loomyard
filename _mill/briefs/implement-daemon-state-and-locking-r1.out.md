All 5 of 5 cards in the batch are committed, matching each card's declared `Commit:` message exactly (cards 12-16). The batch verify command (`go test -count=1 ./internal/codeintelengine/... ./internal/proc/... ./cmd/lyx/...`) passes, and `git status --porcelain --untracked-files=no` shows no uncommitted tracked changes.

Summary of work:

- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/proc/proc_linux.go` — added `IsAlive(pid int) bool` using `os.FindProcess` + `Signal(syscall.Signal(0))`.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/proc/proc_windows.go` — added `IsAlive(pid int) bool` using `os.FindProcess`'s own `OpenProcess`-backed existence check.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/CONSTRAINTS.md` and `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/leaf_enforcement_test.go` — allowlisted `internal/proc` for the Codeintelengine Leaf Invariant.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/daemonstate.go` (new) — `daemonState` struct, `supervisedProtocolVersion` const, `readDaemonState`, `writeDaemonState` (temp-file-then-rename), `daemonStale` (two-part check).
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/probe.go` (new) — `probe(ctx, client, timeout) error` via an empty `workspace/symbol` query.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/proc/isalive_test.go` (new), `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/daemonstate_test.go` (new) — round-trip, missing-file, staleness (dead-PID, mismatched-version, current), concurrent-partial-write, and probe success/timeout tests.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/cmd/lyx/tierpurity_test.go` — file-level `allowedSpawners` entry for `daemonstate_test.go`'s confirmed-dead-PID spawn fixture.

One implementation note found while writing `TestProbe_NoResponseReturnsErrServerTimeout`: an `io.Pipe`-backed `lspClient.writeMessage` blocks until something reads the write, so the "server never responds" test still needs a goroutine that reads (but never answers) the request — otherwise the client hangs in `writeMessage` before it ever reaches the ctx-bounded `select` loop. Fixed with a synchronized reader goroutine in the test itself; no production code changed by this.

{"status":"success","commit_sha":"976c6be5","session_id":"9809ec16-36a2-451d-aa36-4b3a2ea358a5","cards_done":[12,13,14,15,16]}
