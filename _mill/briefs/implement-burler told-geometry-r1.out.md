{"status":"success","commit_sha":"209ec5790a9df5cfb3c71873f867a88d6ff30859","session_id":"3e36368d-324a-4b3b-8947-c9acb2378adf","cards_done":[6,7,8,9,10,11]}

All 6 cards in batch `02-burler-told-geometry.md` are committed (6 of 6 committed — full completion, verified honestly via the git log range check above):

- Card 6: `internal/burlerengine/engine.go` — `Engine.layout *lyxcwd.Location` → `Engine.geom Geometry`, `New`'s signature and body updated, `Run` reads `e.geom.WorktreeRoot`/`e.geom.AnchorPath`, `lyxcwd` import dropped, doc comments updated. Commit `e00e1013`.
- Card 7: `internal/burlerengine/prompt.go` — file header retargeted from `*lyxcwd.Location` to the engine's `Geometry`. Commit `630e9e88`.
- Card 8: `internal/burlerengine/engine_test.go` — all 8 `New(...)` call sites converted from `&lyxcwd.Location{...}` to `Geometry{...}` literals; `TestEngine_Run_MaterializesInstructionFiles` rewritten with distinct `worktreeRoot`/`anchorPath` plus two new swap-guard assertions; `lyxcwd` import dropped; file header updated. Commit `9ef56c75`.
- Card 9: `internal/burlerengine/smoke_round_test.go` and `smoke_cluster_test.go` — `burlerengine.New` construction repointed through `hubgeom.BurlerGeometry(h.Location)`. Commit `b9c7f981`.
- Card 10: `internal/burlercli/cli.go` — `burlerengine.New` construction repointed through `hubgeom.BurlerGeometry(layout)`. Commit `bfaee972`.
- Card 11: `internal/perchcli/cli.go` — same swap for perch's burler construction site only. Commit `209ec579`.

Verify command passed in full: `go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/burlercli/... ./internal/perchcli/...` all ok, and `go vet -tags smoke ./internal/burlerengine/...` clean. Also ran `go build ./...` (module-wide) with no failures, and the manual `grep -rn lyxcwd internal/burlerengine/ --include=*.go` check — the one hit is a prose comment in `geometry.go` (pre-existing from batch 1, explicitly called out by the plan's Shared Decision as an expected false-positive a naive grep flags, not a production import). Working tree is clean (`git status --porcelain --untracked-files=no` empty).

{"status":"success","commit_sha":"209ec5790a9df5cfb3c71873f867a88d6ff30859","session_id":"3e36368d-324a-4b3b-8947-c9acb2378adf","cards_done":[6,7,8,9,10,11]}
