No tracked modifications outstanding; verify passes; all four cards committed and pushed.

Summary of work:
- `internal/shuttleengine/mux.go` (new) — `MuxOps` seam interface + compile-time assertion against `*muxengine.Engine`.
- `internal/shuttleengine/fakes_test.go` (new) — `fakeMux`/`fakeEngine` hermetic test doubles.
- `internal/shuttleengine/run.go` (new) — `Runner`, `Result`, `Run`, `NewRunner`, `Start`, `Run` (Start+Wait), opportunistic orphan sweep, and (added in the final commit) `Interrupt`/`Send`/`playInputs`.
- `internal/shuttleengine/run_test.go` (new) — Start wiring/validation/cleanup/sweep tests plus Interrupt/Send tests.
- `internal/shuttleengine/wait.go` (new) — `Run.Wait` poll loop, clock seam, events-offset reading, liveness/startup probe, `finalize` cleanup.
- `internal/shuttleengine/wait_test.go` (new) — all four outcome classifications, KeepPane, trust-dismiss/startup-timeout, timeout, multi-Stop offset tracking, partial-line resilience.

{"status":"success","commit_sha":"0ccd12136d51ddae5a46148aa5138b4f0df7b70d","session_id":"68dae419-4b1b-4354-b1e1-e7abd6147a41"}
