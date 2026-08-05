Summary: All 5 cards of the production-sweep batch are committed (5 of 5, verified against the git log range `6c68580c..HEAD`), and the batch's `verify: go build ./...` passes cleanly.

Key files touched (all under `/home/knatte/Code/loomyard/wts/fabric-illusion-core`):
- `internal/fabriccli/*.go`, `internal/fabricengine/*.go` (card 8, plus `worktreelist.go` added mid-batch)
- `internal/webstercli/*.go`, `internal/websterengine/*.go` (card 9)
- `internal/buildercli/*.go`, `internal/builderengine/spawn.go`, `internal/burlercli/cli.go`, `internal/burlerengine/{config,engine}.go`, `internal/loomengine/{discussion,plan,preflight}.go`, `internal/planparser/parse.go` (card 10)
- `internal/boardcli/cli.go`, `internal/configcli/*.go`, `internal/configsync/configsync.go`, `internal/idecli/cli.go`, `internal/ideengine/{menu,spawn}.go`, `internal/logger/sink.go`, `internal/pattern/pattern.go`, `internal/tokenvocab/tokenvocab.go`, `internal/vscode/color.go` (card 11)
- `internal/perchcli/*.go`, `internal/perchengine/{doc,engine}.go`, `internal/reedcli/cli.go`, `internal/reedengine/{lock,lifecycle,strand}.go`, `internal/scoutcli/cli.go`, `internal/scoutengine/{daemonstate,doc,ensureserver,load}.go`, `internal/shuttlecli/cli.go`, `internal/shuttleengine/{run,rundir,wait}.go` (card 12)

Two plan-extension commits were required and made first, per protocol: `internal/fabricengine/worktreelist.go` (missing from card 8's original scope) and `internal/reedengine/lifecycle.go`/`strand.go` (missing from card 12's original scope) -- both discovered because `go build ./...` failed against files that read `.Cwd`/`.WorktreeRoot` on the same Location-typed variables the listed files retyped.

{"status":"success","commit_sha":"e70f312ccaf8087057fb9c374bb22dd9df113852","session_id":"e3bc0823-3a34-49f8-a82e-f4a75ed0b5ba","cards_done":[8,9,10,11,12]}