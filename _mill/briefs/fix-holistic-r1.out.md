HEAD (92b1af61a7baf9b7ff2e8705874749580ae4cb6f) differs from baseline (841e0865...), and `git status --porcelain --untracked-files=no` shows no tracked modifications. All work is committed and pushed.

{"status":"success","commit_sha":"92b1af61a7baf9b7ff2e8705874749580ae4cb6f","session_id":"db468b57-f020-4e1b-8c22-7734965e1ae6"}

Summary of work done in `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene`:

- `internal/fabriccli/weft_verbs.go`: fixed the `lyx fabric commit --help` `Long` text (BLOCKING finding) to state `_lyx`/`.lyx` are structural/code-injected, only `_pattern` is the configurable default.
- `internal/buildercli/sync.go`, `internal/webstercli/sync.go`: swept the stale "excluded solely by `.git/info/exclude`" header comments (NIT finding), reworded to say the artifacts live under `.lyx`.
- `internal/builderengine/state.go`, `internal/perchengine/engine.go`, `internal/websterengine/state.go`: fixed bare `weft`/`warp` tokens that tripped `TestEnforcement_FabricVocabulary` (batch 1 verify regression from this task's own earlier batches, caught by the newly-merged fabric-vocabulary enforcement test).
- `internal/perchcli/run_integration_test.go`: updated two lock-exclusion tests to plant `run.lock`/`state.json.lock` under the `.lyx` scratch dir instead of `_lyx`, matching real production geometry now that the exclude mechanism is retired.
- `internal/buildercli/sync_integration_test.go`, `internal/webstercli/sync_integration_test.go`: same fix for `TestFabricSync_CommitsAtEveryRelPathDepth` -- machine-local artifacts (locks, pause flags, prompts) now planted under `.lyx`.
- `internal/loomengine/preflight_integration_test.go`: `setupPreflightFixture` now wires the `.lyx` junction too (matching batch 8's fold into the wired name-set), and the now-redundant test-local `.lyx` git-exclude helper was removed.

All 8 batch `verify:` commands plus the full `go build ./... && go vet -tags integration ./... && go test ./...` pass clean. Every commit was pushed to `dotlyx-scratch-hygiene`.
