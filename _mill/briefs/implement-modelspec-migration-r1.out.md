{"status":"success","commit_sha":"50ba4983b6cc02c4bd043e592b1c2e26ed477808","session_id":"79b605ae-1a59-4ded-94ce-b107e09a7a31","cards_done":[12,13]}

Summary: 2 of 2 cards committed (card 12 and card 13, both fully complete — no partial work). All batch verify tests pass fresh (`go test -count=1 ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/configreg/... ./cmd/lyx/...`), `go build ./...` and `go vet ./...` are clean, `goimports -l` shows no drift, and `golangci-lint run` on the touched packages is clean. Working tree has no uncommitted tracked changes.

Files touched:
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchengine/template.yaml` — removed `judge_effort`, reworded `judge_model` comment to model-spec notation.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchengine/config.go` — `Config.JudgeEffort` now `yaml:"-"`; added `ResolveModelSpec` (shared Parse→Resolve→effort-only-params-check helper) and `LoadConfigWithRegistry`; `LoadConfig` delegates after loading its own registry; strict `KnownFields(true)` decode.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchengine/config_test.go` — six new/updated resolution and fail-loud test cases.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchengine/doc.go` — updated configuration section.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchcli/cli.go` — loads `modelspec.Registry` once in `PersistentPreRunE`, threads it into `LoadConfigWithRegistry` and `decodeProfile`.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchcli/run.go` — `profileYAML` drops `judge-effort`/`effort`; `decodeProfile` gains a registry param and resolves `judge-model`/`model` via `perchengine.ResolveModelSpec`; help text updated.
- `/home/knatte/Code/loomyard/wts/treadle/internal/perchcli/run_test.go` — fixtures migrated to model-spec strings; added `testModelRegistry` and new fail-loud/default-effort test cases.
- `/home/knatte/Code/loomyard/wts/treadle/tools/sandbox/SANDBOX-PERCH-SUITE.md` — updated the one prose line describing perch's config.

{"status":"success","commit_sha":"50ba4983b6cc02c4bd043e592b1c2e26ed477808","session_id":"79b605ae-1a59-4ded-94ce-b107e09a7a31","cards_done":[12,13]}
