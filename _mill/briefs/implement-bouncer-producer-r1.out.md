No dirty tracked files. All 4 cards (5, 6, 7, 8) are committed, matching all commit messages declared in the batch file. Verify passed with `go test ./internal/shedadapters/...` — all green.

4 of 4 cards committed (cards 5, 6, 7, 8), verify passed.

- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/bouncer.go` — new `BouncerConfig`, `NewBouncer`, and `Bouncer.Call` with its four modes (seed, re-bounce, judge, replay) and helpers (`judged`, `degrade`, `ensureFocus`, `settle`, `seedCall`, `runSeedSpawn`, `judgeCall`).
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/bouncer_config_test.go` — `NewBouncer` validation table plus the shared stencils-fixture helper (fixed to stamp via `stencilstore.ApplyStamp` so a body with its own leading comment, like the shipped templates, doesn't get double-wrapped).
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/bouncer_seed_test.go` — seed call, seed-vs-re-bounce discriminator, seed-spawn-failure fallbacks, seed-side harvest, re-bounce, marker-completeness/stamp-leak regressions, and Spec identity/passthrough tests.
- `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/internal/shedadapters/doc.go` — package doc amended in the five required places to record the Bouncer.

`go test ./internal/shedadapters/...` passes; `gofmt -l` and `go vet` are clean; no tracked files remain dirty.

{"status":"success","commit_sha":"f634e64d344bad45a260ee2adf4088775e3b003f","session_id":"d5fcc1a3-620c-470e-b474-5f21d7d7d150","cards_done":[5,6,7,8]}
