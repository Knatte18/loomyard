All 4 of 4 cards (21, 22, 23, 24) are committed, verify passes, and the working tree is clean.

Summary of work in this batch:
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/stencils/treadle/treadle-template-judge-circling.md`, `treadle-template-judge-milestone.md`, `treadle-template-triage.md`, `treadle-template-targeting.md` — relocated via `git mv` from `internal/treadleengine/`.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/stencils/stencils.go` — registered the four new embeds/entries.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/.gitattributes` — re-pinned to the new paths.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/treadleengine/{engine.go,judge.go,targeting.go,run.go}` — `Options.StencilsDir`/`Engine.stencilsDir`, `stencilstore.Read` at the four call sites, `judgeInputs.StencilsDir`, leading `stencilsDir` params on `runTriage`/`runTargeting`; `template.go` deleted.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/treadleengine/{judge_test.go,engine_test.go,template_test.go}` — threaded new params/fields, added `newTestStencilsDir` helper, added `TestRunTriage_ReadsFromDiskAtCallTime`.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/treadleengine/seam_enforcement_test.go` and `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/CONSTRAINTS.md` — admitted `internal/stencilstore` to the Treadle Runner-Seam allowlist.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/perchengine/{engine.go,run_test.go}` and `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/perchcli/run.go` — `Engine.Run` gained a trailing `stencilsDir`; `perchcli` resolves it via `fabricengine.StencilsDir(layout.HubPath)`.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/_mill/plan/05-treadle-runtime-read.md` — extended per protocol to record `engine_test.go`'s undeclared touch (StencilsDir threading through several `Engine.Run`-level round-loop tests that Card 22's note had missed).

`go build ./...` and `go test ./stencils/... ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/lyxcwd/...` both pass.

{"status":"success","commit_sha":"aa66b321b54c9afa2d8b30063e1717582df0055d","session_id":"43c07e8b-0519-4596-acf9-08a1db524f1d","cards_done":[21,22,23,24]}
