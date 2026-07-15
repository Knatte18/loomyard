No tracked in-scope modifications are dirty (the only untracked file is the brief itself, which is out of scope). All 6 of 6 cards are committed, matching all `## Cards` `Commit:` messages in the batch file exactly.

Summary of work completed for batch `01-mouse-default.md` (round 1): 6 of 6 cards committed.

- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/mouse.go` — new `mouseOption` validator (commit `e4f7d062`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/mouse_test.go` — table-driven test for it (commit `e4f7d062`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/config.go` — new `Mouse` field (commit `a8fbb68f`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/template_posix.yaml`, `template_windows.yaml` — new `mouse:` key (commit `a8fbb68f`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/lifecycle.go` — validates `Mouse` up front and pins `set-option -g mouse` at boot (commit `677d1188`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/config_test.go` — asserts default `"off"` (commit `666934cd`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/mouse_boot_integration_test.go` — new integration test, verified passing locally against real tmux 3.6 (commit `14ace15c`)
- `/home/knatte/Code/loomyard/wts/mux-mouse-default/internal/muxengine/doc.go` — godoc update (commit `dcddf9aa`)

`verify: go test ./internal/muxengine/` passes. The integration-tagged test was also run manually (`go test -tags integration ./internal/muxengine/`) and passes alongside the pre-existing contract test. Working tree is clean of in-scope changes.

{"status":"success","commit_sha":"dcddf9aa","session_id":"86cb89df-14ea-4dea-bf60-63cfa49871ba"}
