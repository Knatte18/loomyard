All 3 cards in this batch (Card 2, Card 3, Card 4) are committed, verify passes, and the working tree is clean.

Summary of work:
- `internal/websterengine/implementer-body.md` (new) — shared implementer-job body with markers `{{.card_pointers}}`, `{{.worktree_root}}`, `{{.prev_digest}}`, `{{.self_fix_cap}}`, `{{.report_path}}`.
- `internal/websterengine/fork-prefix.md` (new) — thin in-session fork prefix.
- `internal/websterengine/recovery-prefix.md` (new) — full cold-start recovery prefix with optional `{{.pattern_directive}}`.
- `internal/websterengine/render.go` — replaced the single fork-template embed with the three-asset composition (`joinTemplateAssets`, `composeForkTemplate`, `composeRecoveryTemplate`), thinned `RenderForkPrompt` (dropped `*planparser.Plan`), added `RenderRecoveryPrompt`/`RecoveryTemplate`/`ImplementerBodyTemplate`, added `renderCardPointers`, dropped `shared_decisions` from `RenderIntegrationPrompt`, deleted dead card-rendering functions and `noSharedDecisions`.
- `internal/websterengine/fork-template.md` deleted.
- `internal/websterengine/integration-template.md` — removed the `## Shared Decisions` section/marker, updated the banner.
- `internal/websterengine/recoverbatch.go`, `internal/websterengine/beginbatch.go`, `internal/websterengine/doc.go` — switched call sites to the new signatures and swept stale `RenderForkPrompt`/"SAME fork prompt"/plan-level-context comments.
- `internal/websterengine/template_test.go` — rewritten for the new composed-template marker sets, added omits-Shared-Decisions/omits-Rename-mechanic and card-pointer-relative assertions, `RenderRecoveryPrompt` coverage (PATTERN active/inactive), and the reuse-guarantee test.
- `internal/websterengine/master-template.md` — Master's orientation now reads only `00-overview.md`, not every card file.

Card-count self-check: 3 of 3 declared cards committed (Card 2, Card 3, Card 4 — this batch has no Card 1). Batch `verify:` (`go build ./... && go test ./internal/websterengine/... ./internal/hubgeometry/...`) passes.

{"status":"success","commit_sha":"dbacedffa9538ef549d9a62a8b8b755c9d4e56b0","session_id":"d3b69a4c-83f5-43ce-85d6-5e3bfb7ec3d7","cards_done":[2,3,4]}
