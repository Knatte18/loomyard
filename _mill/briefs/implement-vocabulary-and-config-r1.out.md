8 of 8 cards committed (cards 7 through 14), plus one plan-extension commit (`5f050ac00`) that added `internal/reedengine/lifecycle_test.go` and `internal/reedengine/apply_test.go` to card 13's Edits list before editing them, since both referenced the removed `Config.Header` field and weren't listed in any card's scope. Batch verify (`go test ./internal/tokenvocab/ ./internal/reedengine/ ./internal/hubgeom/ ./internal/standalonegeom/ ./internal/configsync/`) passes. Working tree is clean.

Relevant files:
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/tokenvocab/tokenvocab.go`, `doc.go`, `render.go`, `tokenvocab_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/geometry.go`, `config.go`, `config_test.go`, `template.go`, `template_posix.yaml`, `template_windows.yaml`, `statusline.go` (was `header.go`), `statuslinetemplate.go` (was `headertemplate.go`), `status-line.md` (was `console-header.md`), `statusline_test.go` (was `header_test.go`), `lifecycle.go`, `lifecycle_test.go`, `apply.go`, `apply_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/hubgeom/hubgeom.go`, `hubgeom_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/standalonegeom/reedgeom.go`, `standalonegeom_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/configsync/configsync_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/.gitattributes`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/_mill/plan/02-vocabulary-and-config.md` (plan extension)

Note for the next batch: `go build ./...` still fails on `internal/reedcli/header.go` (`c.eng.HeaderText undefined`) — that file is out of this batch's scope and is expected to be renamed in a later batch (status-line-pins per the plan's file union).

{"status":"success","commit_sha":"378adfd33ec5a34692f1e6c8b475ecbe1d56b7bc","session_id":"b10b8c5f-73a0-4eb4-8fa4-ca98ae6f09e6","cards_done":[7,8,9,10,11,12,13,14]}
