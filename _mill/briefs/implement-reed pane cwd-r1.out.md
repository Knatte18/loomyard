{"status":"success","commit_sha":"0c028c4901226137fd606b2b2c3dc3fb2efe125e","session_id":"c875bda6-4fd6-480a-962e-146d15370f64","cards_done":[1,2,3,4,5]}

All 5 of 5 cards committed this turn:

1. `internal/reedengine/geometry.go` — added `PaneCwd string` field to `Geometry`, updated the file-header "eight-field" wording, `AnchorPath`'s comment, and the new field's contract comment (no zero-value fallback). Commit `bf9c484d`.
2. `internal/reedengine/lifecycle.go` — both tmux spawn sites (`new-session` argv in `spawnSession`, and the header `split-window` in `ensureHeaderPaneLocked`) now read `-c` from `e.geom.PaneCwd`; `stateDir` still reads `AnchorPath`. Commit `172eafcb`.
3. `internal/hubgeom/hubgeom.go` / `hubgeom_test.go` — `ReedGeometry` sets `PaneCwd: l.AnchorPath()`; `TestReedGeometry` extended with an unanchored (`AnchorRel: "."`) row alongside the subpath-anchored one, asserting `PaneCwd == l.AnchorPath()` and (subpath row) `PaneCwd != WorktreeRoot`. Commit `564e8e64`.
4. `internal/reedengine/lock_test.go`, `contract_integration_test.go`, `mouse_boot_integration_test.go` — every `Geometry` literal that reaches a spawn site got an explicit `PaneCwd`; `newTestEngine` uses a distinct `filepath.Join(hub, "pane")` value, the rest mirror their existing `AnchorPath`. `header_test.go` left untouched per spec. Commit `846dcced`.
5. `internal/reedengine/lifecycle_test.go` — new hermetic `TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath` hooks `e.tmux.execHook`, captures the `split-window` argv, and asserts `-c` reads `PaneCwd` and differs from `AnchorPath`; doc comment states the `new-session` half is covered by the tagged suites. Commit `0c028c49`.

Verify: `go test ./internal/reedengine/... ./internal/hubgeom/...` and `go test -tags integration ./internal/reedengine/...` both passed clean. `gofmt -l` showed no drift. Working tree is clean and all commits pushed to `origin/webster-told-geometry`.
