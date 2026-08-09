All 3 declared cards (8, 9, 10) match commits made this turn. 3 of 3 cards committed — all complete.

Summary of work done, files touched (absolute paths):
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabriccli/fabric.go` — card 8: rewrote clone's `Use`/`Long`, registered `--force-bootstrap`, threaded it through the RunE closure.
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabriccli/clone.go` — card 9: added `forceBootstrap` param to `runCloneWithReset`, wired it into `CloneOptions.ForceBootstrap`, extended the success envelope with `"warp"` and `"warp_binding_recorded"`.
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabriccli/cli_test.go` — card 10: renamed/rewrote the arity test to `TestRunCLI_CloneAcceptsOneOrTwoArgs`, added `TestRunCLI_CloneUnboundWeftErrorNamesTwoArgForm`, flipped the two end-to-end tests to weft-first args, extended `TestRunCLI_CloneEndToEnd` with the new envelope-key and tracked-binding-file assertions.

Verify command (`go build ./... && go test -tags integration ./internal/fabriccli/ && go test ./cmd/lyx/`) passed. All tracked files are committed; `git status --porcelain --untracked-files=no` is clean.

{"status":"success","commit_sha":"8f26e61bf504b5ab53887b4d3d8d931b41dcaa1d","session_id":"a6c66ff5-3d5b-4229-8d62-c8fd075e4dc3","cards_done":[8,9,10]}