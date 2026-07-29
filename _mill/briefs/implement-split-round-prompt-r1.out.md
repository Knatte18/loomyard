All 9 of 9 cards committed this turn (verified by matching each `### Card N:` heading's `Commit:` message against `git log 63340915..HEAD --oneline`). Verify (`go test ./internal/burlerengine/... ./internal/stencil/...`) passes cleanly with `-count=1`. No uncommitted tracked changes, no gofmt drift.

Summary of work (all paths relative to `/home/knatte/Code/loomyard/wts/burler-prompt-split`):
- Created `internal/burlerengine/round-orchestrator-template.md`, `instruction-1-explore-template.md`, `instruction-2-review-template.md`, `instruction-3-fix-template.md`; deleted `internal/burlerengine/review-prompt-template.md`; updated `.gitattributes`.
- `internal/burlerengine/template.go`: four `//go:embed` vars replacing the single `reviewPromptTemplate`.
- `internal/burlerengine/prompt.go`: `composePrompt` now has signature `(p *Profile, patternDirective, inst1Path, inst2Path, inst3Path string) (string, []instructionFile, error)`, rendering all four assets with their own marker subsets.
- `internal/burlerengine/engine.go`: `Run` mints a per-round dir under `layout.DotLyxDir()/burler`, materializes the three instruction files, and hands the shuttle only the orchestrator string.
- `internal/burlerengine/template_test.go`, `prompt_test.go`, `engine_test.go`: pins relocated to the new assets, new tests added (`TestTemplate_OrchestratorExcludesDownstreamBodies`, `TestComposePrompt_ReturnsThreeInstructionFiles`, `TestComposePrompt_BlockHelpersLandInIntendedAsset`, `TestEngine_Run_MaterializesInstructionFiles`, `TestEngine_Run_MaterializeFailure`), and every test `hubgeometry.Layout` now sets `Cwd`.
- `internal/burlerengine/doc.go` and `CONSTRAINTS.md`: updated to describe the new four-asset layout and the relocated Review Round Invariant enforcement tests.

{"status":"success","commit_sha":"3df6e21debc0bace529ac842fcd3a711aa7be896","session_id":"4966c302-4780-4f78-8f22-7d64ca17bdca","cards_done":[1,2,3,4,5,6,7,8,9]}
