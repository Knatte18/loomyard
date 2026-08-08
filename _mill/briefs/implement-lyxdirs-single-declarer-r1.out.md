8 commits match all 8 declared cards exactly (Card 1 through Card 8, each with its own commit, matching the `Commit:` messages in the batch file). All 8 of 8 cards committed and verified.

{"status":"success","commit_sha":"63e9bbd09e5f1cfa5e7ff361010885751518c80b","session_id":"571d7fc6-095c-47e7-bc02-55351851eedc","cards_done":[1,2,3,4,5,6,7,8]}

Summary: All 8 cards of batch `lyxdirs-single-declarer` are committed (8 of 8 — verified against the batch-start commit `79e55a1a` via `git log <start>..HEAD --oneline`, each commit subject matching a card's `Commit:` message). The batch's `verify:` command (`go vet -tags integration ./... && go test ./...`) passes cleanly with no failures.

Key files touched:
- `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/internal/lyxdirs/dirs.go`, `doc.go` (new leaf package)
- `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/internal/configengine/config.go`
- 18 production files across fabricengine/websterengine/builderengine/perchengine/loomengine/planparser/ideengine/buildercli/webstercli/perchcli/lyxtest retargeted to `lyxdirs.LyxDirName`
- 25 test files retargeted to `lyxdirs.LyxDirName`
- 5 private `dotLyxDirName` declarers removed (logger/sink.go, scoutengine/daemonstate.go, shuttleengine/rundir.go, reedengine/lifecycle.go, burlerengine/engine.go) plus their same-package test files, all now using `lyxdirs.DotLyxDirName`
- `internal/fabricengine/status.go` (raw `_lyx` literals → `lyxdirs.LyxDirName`)
- `internal/lyxcwd/enforcement_test.go`, `internal/lyxtest/leaf_enforcement_test.go`, `internal/lyxtest/doc.go` (policing + allowlist)
- `CONSTRAINTS.md`, `docs/overview.md` (new invariant section + doc references)

One incidental fix folded into card 7's commit: `internal/lyxdirs/dirs.go`'s `LyxDirName` godoc originally said "weft-synced," which tripped `TestEnforcement_FabricVocabulary`'s bare-token check outside fabric's owner set — reworded to "fabric-synced" to keep the full test suite green without weakening any check.
