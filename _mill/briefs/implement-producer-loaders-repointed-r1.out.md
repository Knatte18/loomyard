{"status":"success","commit_sha":"4278390eac5759ebbee2703d1c44f3bf1d895f44","session_id":"0115a36a-1b85-4d34-8a4b-b16a11779bf4","cards_done":[4,5,6,7]}

Task: implement batch `producer-loaders-repointed` (4 of 4 cards).

- Card 4 — `shuttleengine.LoadConfig` repointed to `LoadOrTemplate`, dropped `strings.Contains` "not initialized" rewrap, inverted `TestLoadConfig_NotInitialized`→`TestLoadConfig_UninitializedFallsBackToTemplate`, reworked module-arg test to positive discrimination. Commit `241b92d1`.
- Card 5 — same for `reedengine.LoadConfig`, GOOS-invariant assertions only (Width/CollapsedStripRows/Header.HeightRows), kept `runtime` import. Commit `5a3d3caf`.
- Card 6 — `perchengine.LoadConfigWithRegistry` repointed; fallback path resolves `judge_model` through `modelspec.LoadRegistry`'s own absent-file→`builtins()` degrade. Commit `4aa45070`.
- Card 7 — `websterengine.LoadConfig` repointed; no module-arg test existed for this package (per plan), so only the not-initialized inversion applied. Commit `4278390e`.

All 4 cards committed, 4 of 4 declared — confirmed against `git log f5aff794..HEAD --oneline`. Batch verify (`go test` across all four packages) passes. `goimports`/`golangci-lint` clean on every changed file — two pre-existing `errcheck` findings surfaced by whole-package lint (`internal/reedengine/lock.go:80`, `internal/websterengine/runlevel.go:323`) are untouched by this batch (`git diff --stat` empty on both) and out of scope. Working tree clean, all four commits pushed to `origin/config-template-fallback`.

{"status":"success","commit_sha":"4278390eac5759ebbee2703d1c44f3bf1d895f44","session_id":"0115a36a-1b85-4d34-8a4b-b16a11779bf4","cards_done":[4,5,6,7]}
