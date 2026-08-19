All 4 declared cards match the 4 commits made this turn — 4 of 4 cards committed, matching the batch's declared `cards: 4`. Verify passed on the first attempt. Working tree is clean.

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/loomengine/config.go` — added `loomDirName`/`loomStatusFileName` constants, `LoomStatusRel`, `LoomDriverLog`, `LoomBootstrapLock`, refactored `LoomStatusFile`/`LoomStatusLock`/`LoomRunLock`.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/loomshed/seed.go` — added exported `ErrSeedExists` sentinel, wrapped into `Seed`'s refusal error.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/cmd/lyx/notransients_test.go` and `constructoranchoring_test.go` — added rows/assertions for the two new accessors.
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/loomengine/config_test.go` and `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/internal/loomshed/seed_test.go` — new unit coverage.

{"status":"success","commit_sha":"277cc0b8062f79f536bca101c3e150f8aa76abf6","session_id":"308646ba-99b7-4645-8f06-9ea70c31001b","cards_done":[11,12,13,14]}
