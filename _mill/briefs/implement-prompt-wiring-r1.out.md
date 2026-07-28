All cards committed, verify run (with 3 confirmed pre-existing failure clusters unrelated to this batch), and formatting/vet clean.

{"status":"success","commit_sha":"62e0b3dd46587df33961ade166487459b6b6996c","session_id":"3eabb6b3-4b49-4cb0-af7b-41fce493d689","cards_done":[24,25,26,27,28,29]}

## Summary

All 6 of 6 cards in batch `prompt-wiring` (cards 24-29) are committed. This is a complete batch — not a partial one.

**Commits made this turn** (`cb7babeb..HEAD`):
- `e5ef07ea` — builder: inject the PATTERN directive into the implementer prompt (card 24)
- `0a5a8a32` — builder: tighten blank-line spacing + fix a stale pre-existing test assertion in `internal/builderengine/template_test.go` (discovered while verifying card 24)
- `ee2af4c4` — plan: extend prompt-wiring refs for `internal/burlerengine/prompt_test.go` (scope-extension commit per protocol)
- `d34a0c84` — burler: inject the PATTERN directive into the round prompt (card 25)
- `cec1a857` — plan: extend prompt-wiring refs for `internal/websterengine/beginbatch_test.go` (scope-extension commit)
- `75a6d4a9` — webster: inject the PATTERN directive into the fork prompt (card 26)
- `5a4d3cfc` — webster: inject the PATTERN orchestrator directive into the Master prompt (card 27)
- `b6c557a0` — loom: inject the PATTERN directive into the plan prompt (card 28)
- `62e0b3dd` — docs: correct the PATTERN design doc and record the landed wiring (card 29)

**Files touched** (all absolute paths under `/home/knatte/Code/loomyard/wts/pattern-wiring/`):
- `internal/builderengine/{implementer-template.md,spawn.go,template_test.go}`
- `internal/burlerengine/{review-prompt-template.md,prompt.go,engine.go,template_test.go,prompt_test.go}`
- `internal/websterengine/{fork-template.md,master-template.md,render.go,beginbatch.go,recoverbatch.go,runlevel.go,template_test.go,beginbatch_test.go}`
- `internal/webstercli/beginbatch.go`
- `internal/loomengine/{plan-template.md,plan.go,plan_test.go}`
- `manifest/designs/pattern.md`, `manifest/roadmap.md`
- `_mill/plan/07-prompt-wiring.md` (extended twice, per protocol, when a card's signature change forced editing a same-package test file not in its declared `Edits:` list)

**Verify**: ran `go test -tags integration ./...`. Every package touched by this batch (or by any commit in the whole 7-batch chain) is green. Three pre-existing, unrelated failures remain: `internal/buildercli` and `internal/webstercli`'s `TestWeftCommit_*` tests (a `.gitignore`/git-config environment issue unrelated to any PATTERN code) and `internal/reedengine`'s `TestTmuxProcessName` (a Windows-path-parsing test failing on this Linux sandbox). I confirmed via `git log main..HEAD -- <path>` that zero commits in this entire task's 7-batch chain touch `internal/buildercli` or `internal/reedengine`, and the one `internal/webstercli` file I touched (`beginbatch.go`, adding a `Layout` field passthrough) is unrelated to the gitignore/weft-commit logic that fails. `go vet ./...` is clean; `gofmt -l` on every file I touched is clean.
