All 4 of 4 cards committed (17, 18, 19, 20), verify passed in full, working tree clean.

Summary of changes:
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/stencils/stencils.go` — added `BurlerTemplateRoundOrchestrator`, `BurlerStep1Explore`, `BurlerStep2Review`, `BurlerStep3Fix` embed vars and registry rows
- Moved (via `git mv`) `internal/burlerengine/{round-orchestrator-template.md,instruction-1-explore-template.md,instruction-2-review-template.md,instruction-3-fix-template.md}` to `stencils/burler/{burler-template-round-orchestrator.md,burler-step-1-explore.md,burler-step-2-review.md,burler-step-3-fix.md}`
- Deleted `internal/burlerengine/template.go`; `internal/burlerengine/engine.go` and `internal/burlerengine/prompt.go` now read prompts via `stencilstore.Read` at call time, with `Engine` carrying a `stencilsDir` field
- `internal/burlercli/cli.go` and `internal/perchcli/cli.go` pass `fabricengine.StencilsDir(layout.HubPath)` into `burlerengine.New`
- `internal/burlerengine/{prompt_test.go,engine_test.go,template_test.go,smoke_round_test.go,smoke_cluster_test.go}` updated for the new signatures, `template_test.go` repointed at the `stencils` package's exported defaults with a new `TestComposePrompt_ReadsEditedStencilFromDisk` runtime-read test
- `internal/burlerengine/doc.go` updated to name the new paths
- `.gitattributes` repinned for the relocated files

{"status":"success","commit_sha":"de09c60c3c4bf1ccf27ea1cb5efdbcbef9b20a59","session_id":"aa7de156-96b5-4c29-9146-2b61d982582d","cards_done":[17,18,19,20]}
