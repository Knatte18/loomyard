1 of 1 card committed — batch fully complete.

Summary: I implemented the single Card 4 of batch `watch-loop-dormant-mode` (the third watch mode, `watchModeDormant`), all cards committed (1 of 1), verify passed, and the integration-tagged tests confirmed only the documented pre-existing failure (`TestWatchdogSelfHeal_HookProbeMatchesLiveTmux`). Files touched:

- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/watchdog.go` — added `watchdogDormantCycle` constant.
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/watchloop.go` — added `Dormant` timing field, `watchModeDormant`, `tickerPeriodFor` dormant case, `dormantFrom` loop state, dormant tick handling, and the sentinel/recovery branches in `handleWatchOutcome`.
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/watchloop_test.go` — renamed/extended the six-constant timing test, added a `tickerPeriodFor` cadence test, a mutex-guarded `safeLogBuffer`/`captureLog` helper, and four new driver tests (poll-mode dormancy, signal-mode dormancy, recovery-to-prior-mode, non-sentinel-failure-does-not-dormant).
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/doc.go` — extended the told-geometry-lifetime bullet describing the dormant cadence.
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/tools/sandbox/SANDBOX-REED-SUITE.md` — added one line each to M24 and M25's Watch sections.

Committed as `c9f184c53` and pushed to `reed-lock-stale-session-name`.

```json
{"status":"success","commit_sha":"c9f184c5344446f5a347fc27d53e4fb4b63faa0b","session_id":"47e60020-112a-4e88-8595-48bd5602022e","cards_done":[4]}
```
