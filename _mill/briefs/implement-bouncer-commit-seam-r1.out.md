5 commits match the 5 declared cards (Cards 4, 5, 6, 7, 8) exactly, one commit each. All 5 of 5 cards committed. No `Commit: none` cards in this batch.

{"status":"success","commit_sha":"99a0355ab003bbdd5ae182542bd71faa1b06eb24","session_id":"c3883707-dc78-40fa-898f-e7ccb5f9d9e5","cards_done":[4,5,6,7,8]}

Summary of work done in this batch (bouncer-commit-seam):

- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/shedadapters/bouncer.go` — added optional `Commit func() error` field to `BouncerConfig`; `settle`'s approved branch now calls it before returning `Done`, returning any non-nil error as `settle`'s own error rather than routing through `degrade`.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/shedadapters/bouncer_seed_test.go` — extracted `testBouncerConfig(t)` helper from `newTestBouncer` (pure refactor, no behavior change).
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/shedadapters/bouncer_commit_test.go` (new) — five tests pinning the Commit seam's behavior across approval, blocking, nil, error, and cancellation.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/shedrecipe/entries_bouncer.go` — added `commit_seam` config key resolution (`plan`/`discussion`/absent) to `bouncerEntry`, guarded by `requireSeam` on the named `Env` field; added `commit_seam` to `configRejectUnknown`'s recognised set.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/shedrecipe/entries_bouncer_test.go` — added `TestBouncerEntry_CommitSeam` with six subtests covering resolution, the presence guard, and the unknown-key rejection surface.

All 5 cards committed individually (5 commits, one per card), `verify: go test ./internal/shedadapters/... ./internal/shedrecipe/...` passes, and the working tree is clean.
