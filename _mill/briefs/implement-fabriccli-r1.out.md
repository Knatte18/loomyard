All 3 cards in this batch are complete: 3 of 3 cards committed/verified (card 39 and card 40 each got their own real commit matching the batch's declared `Commit:` messages exactly; card 41 is a `Commit: none` verification-only card whose grep/mapping checks were re-run and confirmed pass, so it needs no log entry). `verify:` (`go vet -tags integration ./...` plus `go test -tags integration ./internal/fabriccli/...`) passed clean, and the working tree has no uncommitted tracked changes.

Files touched:
- `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/fabriccli/cli_test.go`
- `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/fabriccli/pushbypass_integration_test.go`

Notable finding worth flagging: the push-bypass test's original `headSHA` assertion against the weft bare's plain `HEAD` was silently wrong once the fixture became a real hub — a real hub's weft bare carries two branches (`main-weft` for the prime pair and `main` for `weft:main`/board), so the bare's default `HEAD` symref doesn't reliably name the branch under test. Re-expressed that assertion as a branch-scoped `refs/heads/<branch>` rev-parse instead of silencing it.

{"status":"success","commit_sha":"aa906bd38e6aa1127274ab8850f2b286b893a987","session_id":"0d815f62-7fd1-419b-9b01-fc27654edf2f","cards_done":[39,40,41]}
