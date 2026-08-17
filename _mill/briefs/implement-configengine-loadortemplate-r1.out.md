{"status":"success","commit_sha":"345f58c41f96e9e8684a151463e7390b7e9629c1","session_id":"9427fa5e-58a1-4b5e-be72-4d98acdb4ea5","cards_done":[1,2,3]}

Task: implement batch `configengine-loadortemplate` (3 of 3 cards).

- Card 1 — `ErrNotInitialized` sentinel wrapped by `FindBaseDir`, message byte-identical. Commit `28b4fab3`.
- Card 2 — extracted shared `load(baseDir, module, template, fallbackOnAbsent)`; `Load`/`LoadOrTemplate` delegate to it; one shared `fallbackToTemplate` tail; rewrote file-header comment. Commit `8cb14b8e`.
- Card 3 — added `LoadOrTemplate` test coverage (both fallback triggers, both-present matches `Load`, strict-when-present in three forms, env override, absent baseDir+`.env`, fallback-tail error wrap, `ErrNotInitialized` wrap/non-match, chmod-based absence-only discrimination) plus reworded `TestLyxDirNameConstant` comment. Commit `345f58c4`.

All 3 cards committed, 3 of 3 declared — honest count confirmed against `git log f029c4c0..HEAD --oneline`. `go test ./internal/configengine/...` passes (all pre-existing `TestLoad_*`/`TestFindBaseDir_*` unmodified and green). `gofmt`/`goimports`/`golangci-lint` clean. Working tree clean. Pushed to `origin/config-template-fallback`.

{"status":"success","commit_sha":"345f58c41f96e9e8684a151463e7390b7e9629c1","session_id":"9427fa5e-58a1-4b5e-be72-4d98acdb4ea5","cards_done":[1,2,3]}
