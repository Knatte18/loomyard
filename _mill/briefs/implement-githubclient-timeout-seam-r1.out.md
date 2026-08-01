1 of 1 card committed — matches the batch's single card exactly. Full batch complete: Card 1 committed and verify passes.

Files touched:
- `/home/knatte/Code/loomyard/wts/test-suite-overhaul/internal/githubclient/token.go` — `ghAuthTokenTimeout` changed from `const` to `var`
- `/home/knatte/Code/loomyard/wts/test-suite-overhaul/internal/githubclient/githubclient_test.go` — added save/override/restore seam for `ghAuthTokenTimeout` (10ms override) and shrank `slack` from 5s to 200ms in `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout`

`go test ./internal/githubclient/...` passes; the target test now runs in ~0.01s instead of ~5s.

{"status":"success","commit_sha":"120d112a7e61d1193509b0bbb425b3ca2e09aa56","session_id":"9c424b23-be29-4584-96c8-71faeda43604","cards_done":[1]}
