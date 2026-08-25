All 5 cards (4, 5, 6, 7, 8) are committed and match their `Commit:` messages exactly. Working tree is clean. Verify passes.

{"status":"success","commit_sha":"4ca6ae672aa1791b6903f937d49d1110ebe85244","session_id":"5d684d4f-df51-44d2-9678-8eb63a3a387c","cards_done":[4,5,6,7,8]}

Summary: 5 of 5 cards committed (cards 4, 5, 6, 7, 8 — all real commits, none had `Commit: none`). Verify (`go test ./internal/shedrecipe/...`) passes.

Files touched:
- `internal/shedrecipe/recipe.go` — added `ReviewModel`, `ReviewEffort`, `ReviewVersion`, `ReviewTimeout` to `Env`.
- `internal/shedrecipe/entries_bouncer.go` — fallback to `env.Review*` for model/effort/version.
- `internal/shedrecipe/entries_burler.go` — added `profile.rubric_stencil` key (mutually exclusive with `rubric`), `burlerRoundProfile` now takes `stencilsDir`, and `env.Review*` fallback for model/effort/timeout.
- `internal/shedrecipe/entries_bouncer_test.go` — new `TestBouncerEntry_EnvReviewFallback`.
- `internal/shedrecipe/entries_burler_test.go` — new `TestBurlerRoundEntry_RubricStencil` and `TestBurlerRoundEntry_EnvReviewFallback`.

{"status":"success","commit_sha":"4ca6ae672aa1791b6903f937d49d1110ebe85244","session_id":"5d684d4f-df51-44d2-9678-8eb63a3a387c","cards_done":[4,5,6,7,8]}
