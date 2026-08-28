Working tree clean, all 5 commits match the 5 declared cards' `Commit:` messages exactly. Verify passed (`go test ./internal/reedengine/...` → ok on both packages).

5 of 5 cards committed — all complete.

{"status":"success","commit_sha":"cb87930d179cef9d49ea9b3e700d590565a06380","session_id":"71cfa9f2-280e-4403-9917-dda896e2e06b","cards_done":[3,4,5,6,7]}

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/apply.go` — added `renderInputs`/`toRenderInputs`/`fixedHeightPins`, rewired `planLayout`, installed the hook in `applyLayoutLocked` after `select-layout`
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/windowsize.go` — added `resizePinHookArgvs` (pure argv builder) and `installResizePinsLocked` (non-fatal installer)
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/attach.go` — hoisted the box local, installed the hook in `AttachArgv`'s pre-flight, extended doc comment
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/windowsize_test.go` — table-driven `TestResizePinHookArgvs`
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/apply_test.go` — new `applyHookRecorder` fixture and tests for `applyLayoutLocked`'s install position/zero-pin/guard-skip/non-fatal behavior
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/attach_test.go` — extended `attachRecorder`/`newAttachHook` with `set-hook` recording, new tests for `AttachArgv`'s install position, degraded-path silence, and failure non-fatality; narrowed the never-mutates test's doc comment

{"status":"success","commit_sha":"cb87930d179cef9d49ea9b3e700d590565a06380","session_id":"71cfa9f2-280e-4403-9917-dda896e2e06b","cards_done":[3,4,5,6,7]}