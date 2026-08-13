All 3 cards (card 30, 31, 32) committed — matching the batch's declared 3 cards. Verify passed fully (build, Tier 1, and Tier 2 test suites all green).

{"status":"success","commit_sha":"edff8d727bd0b74848158d9dd4824cabdb2f4dbc","session_id":"67298108-874c-4c01-9322-7b6495420f80","cards_done":[30,31,32]}

Summary: 3 of 3 cards committed (card 30 lyxcwd, card 31 fabriccli, card 32 websterengine). All migrated from `gitexec.RunGit` to `gitexec.Run` with `errors.As` recovery or clean message merges as specified. Verify (`go build ./...`, Tier 1 tests for lyxcwd/fabriccli/websterengine/configcli/reedcli/idecli/loomengine, Tier 2 integration tests for lyxcwd/fabriccli/websterengine) all passed.

Files touched:
- internal/lyxcwd/lyxcwd.go
- internal/fabriccli/fabric.go
- internal/websterengine/gitwrap.go

{"status":"success","commit_sha":"edff8d727bd0b74848158d9dd4824cabdb2f4dbc","session_id":"67298108-874c-4c01-9322-7b6495420f80","cards_done":[30,31,32]}
