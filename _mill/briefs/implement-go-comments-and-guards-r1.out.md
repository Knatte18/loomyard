All 4 cards (9, 10, 11, 12) are complete: 3 committed (matching card Commit: messages) plus card 12 (Commit: none, verified this turn). 4 of 4 cards committed/verified — all complete.

{"status":"success","commit_sha":"f1706640f9761ed5b5038ca66d37749859c67137","session_id":"0f0d6e57-86dc-4b06-8570-d34cc4a37d89","cards_done":[9,10,11,12]}

Summary of work in `/home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix`:

- Card 9 (commit `26786b34`): rewrote the four bare-v3/v2-era comments in `internal/planparser/validate.go`, `internal/planparser/validate_test.go`, and `internal/planparser/parse_test.go` to name the format's actual properties instead of a version.
- Card 10 (commit `e0e12d8a`): dropped the "v2 Scope field" framing in `internal/websterengine/classify.go`; renamed `TestTemplates_NoV2TokensRemain` to `TestTemplates_NoDroppedBatchConceptsRemain` and reworded every v2-labeled comment in `internal/websterengine/template_test.go`.
- Card 11 (commit `f1706640`): renamed `TestCommand_LongStringsHaveNoStaleV2Language` to `TestCommand_LongStringsHaveNoStaleBatchLanguage` in `internal/webstercli/cli_test.go`, dropped the `"v2"` entry from `forbidden`, reworded the failure message.
- Card 12 (Commit: none, verification only): confirmed `grep -rni 'v3' internal/planparser/` returns only the `gopkg.in/yaml.v3` import; confirmed `grep -rniE '\bv2\b' --include='*.go' internal/` returns only the deliberately-untouched sites (`internal/state/state_test.go`, `internal/yamlengine/reconcile_test.go`, `internal/gitrepo/reset_test.go`, `internal/shuttleengine/claudeengine/command.go`, `internal/burlerengine/doc.go`); ran the batch `verify:` (`go test ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/...`) and `go build ./...` — both green.

4 of 4 cards committed/verified — all complete. Working tree is clean (no untracked-tracked modifications remaining).

{"status":"success","commit_sha":"f1706640f9761ed5b5038ca66d37749859c67137","session_id":"0f0d6e57-86dc-4b06-8570-d34cc4a37d89","cards_done":[9,10,11,12]}