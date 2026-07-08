All four cards of the gate-loop batch are complete, committed, and pushed. The batch's `verify:` command (`go test ./internal/perchengine/`) passes cleanly.

Summary of what was implemented:
- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\engine.go` (new) — the `Burler` seam, `CommandRunner` type, `Options`, and `Engine`/`New` scaffold (card 10).
- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\gate.go` + `gate_test.go` (new) — `execGateCommand` (real exec.CommandContext runner, no shell), `writeGateOutput`, and `converged` (card 11).
- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\run.go` (new) — `Engine.Run`, the deterministic round loop: pause boundary, bounded burler retry (`runRound`), gate-command execution + feed-forward hydration (`collectPriorHydration`), convergence check, and the milestone/hard-cap/circling stuck ladder (card 12).
- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\run_test.go` (new) — the deterministic scenario suite with `fakeBurler`/`queuedShuttle`/`fakeCommandRunner` doubles covering loop-until-dry, hard cap, milestone gate (continue/uncertain/stop), per-round circling, judge fail-safe, all three gate modes, every non-done outcome branch, resume (continuation/terminal/hash-mismatch/stale-artifact), and pause (card 13).

Commits (all pushed to `internal-perch`):
- `9b41ed7` perch: add Burler seam, Engine scaffold, and Options
- `fd0379f` perch: add gate command runner, gate output file, and convergence evaluation
- `c782c70` perch: implement Engine.Run round loop with milestone ladder and stuck detection
- `859fafd` perch: add deterministic loop scenario suite (fake burler, fake judge)

No plan-file scope extensions were needed — every edit stayed within the batch's declared `Context:`/`Creates:` lists.

{"status":"success","commit_sha":"859fafde041e6e8d0668a33793dc89d449ffec4b","session_id":"c378e0a3-6282-48d4-8036-558409208f61"}
