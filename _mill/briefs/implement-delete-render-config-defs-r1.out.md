All 4 of 4 cards for this batch are committed, matching each card's `Commit:` message exactly. Verify (`go build ./...` and `go test ./internal/muxengine/...`) passes green. Working tree is clean.

{"status":"success","commit_sha":"3b5b03b9","session_id":"18715d31-9d20-4c75-b154-9e1835db1746"}

Summary of work (files, all paths absolute):

- `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/internal/muxengine/render/types.go` — removed `AnchorTop` const, `Display.TopBandRows`, `Params.TopBandRows`, updated doc comments (Card 9, commit `4a84b037`).
- `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/internal/muxengine/render/policy.go` and `policy_test.go` — `partitionByAnchor` now returns only the below-parent stack; migrated `TestPartitionByAnchor` to the one-value signature (Card 10, commit `69a548f3`).
- `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/internal/muxengine/render/rules.go` — `Rules` rewritten to the below-parent-only flow, top-band reservation loop and the `focus == "" && len(top) > 0` fallback deleted (Card 11, commit `a63497c3`).
- `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/internal/muxengine/config.go`, `template.go`, `template_posix.yaml`, `template_windows.yaml` — removed `Config.TopBandRows` and the `top_band_rows` template lines/comment (Card 12, commit `3b5b03b9`).

All 4 cards committed and pushed to `mux-anchor-top-redesign`. `go build ./...` and `go test ./internal/muxengine/...` both pass. `grep -rn "TopBandRows|top_band_rows|AnchorTop|anchor:top"` over `internal/` returns nothing — no residual top-band references anywhere in the code.
