7 of 7 cards committed, matching all `Commit:` messages declared in `_mill/plan/04-burler-cluster-round.md`. Verify (`go test ./internal/burlerengine/ ./internal/burlercli/ ./internal/perchengine/ ./internal/perchcli/`) passed cleanly, and a full-repo `go build ./...` / `go vet ./...` also succeeded with no uncommitted tracked changes remaining.

{"status":"success","commit_sha":"34bc7189","session_id":"1c027ca8-8034-446f-821d-5c06e077e5a5"}

Summary of work (all 7 of 7 cards committed — verified against the batch file's Cards list):

- Card 8 — `internal/burlerengine/profile.go`, `profile_test.go`, `engine.go`, `engine_test.go`, `smoke_round_test.go`: replaced `ClusterN`/`ErrClusterUnsupported` with `ClusterFan string` + unexported `clusterLenses []Lens`; `validate` now takes a `Config` and resolves via `ResolveFan`; `Engine` gained a `cfg` field and `New` a `cfg` parameter; `Run` sets `spec.ForkSubagents = p.ClusterFan != ""`.
- Card 9 — `internal/burlerengine/prompt.go`, `prompt_test.go`, `review-prompt-template.md`, `template.go`, `template_test.go`: added the ninth `{{.cluster_rules}}` marker and `clusterRulesBlock`, composing the fork phase/spawn/consolidation prose from `p.clusterLenses`.
- Card 10 — `internal/burlerengine/verdict.go`, `verdict_test.go`: added `Finding.Origin` (optional, backward compatible).
- Card 11 — new `internal/burlerengine/cluster.go`, `cluster_test.go`, plus `engine.go`/`engine_test.go`: `ErrClusterForksMissing`, `auditClusterRound`, `mutatingGitPattern`; `Result` gained `ForkAudit`/`ClusterWarnings`; `Run` enforces the policy before the review-file read.
- Card 12 — `internal/burlercli/cli.go`, `run.go`, `cli_test.go`: `cluster-fan` profile key, `burlerengine.LoadConfig` wiring, envelope refactored into a unit-testable `resultEnvelope`.
- Card 13 — `internal/perchengine/profile.go`, `roundfiles.go`, `roundfiles_test.go`: `ClusterFan` passthrough.
- Card 14 — `internal/perchcli/cli.go`, `run.go`, `run_test.go`: same key swap and config wiring, mirroring card 12.

All file paths above are relative to `/home/knatte/Code/loomyard/wts/burler-fork-cluster`.
