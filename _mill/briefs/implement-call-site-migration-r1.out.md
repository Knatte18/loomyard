All 4 cards (5, 6, 7, 8) committed — 3 commits, cards 6 and 7 combined into one commit as allowed. All 4 of 4 cards committed and verified.

{"status":"success","commit_sha":"c692d54c7fb7fd3241349453db452645c302f9d2","session_id":"fdc6f31b-f3e1-4b43-9002-dcbd2badd601","cards_done":[5,6,7,8]}

Summary: All 4 cards in batch `02-call-site-migration` were completed and committed this turn (4 of 4 cards committed).

- Card 5 (`324235ac`): removed `Config.Batcher` from `internal/websterengine/config.go`/`template.yaml`/`template.go`, added `ErrNilBatcher` sentinel and `RunDeps.Batcher` field in `internal/websterengine/runlevel.go`, switched `internal/webstercli/cli.go`'s `PersistentPreRunE` to `batcher.Active(layout.AnchorPath())`, threaded `c.batcher` into `RunDeps.Batcher` in `internal/webstercli/run.go`, and updated `internal/websterengine/config_test.go`.
- Cards 6+7 (`8194835f`, combined commit named after card 7 per the one-combined-commit allowance): `internal/websterengine/runlevel_test.go`'s `newRunFixture` now injects the identity batchifier via `batcher.Select("")` into `RunDeps.Batcher`, and a new `TestRun_NilBatcherRefuses` pins the sentinel.
- Card 8 (`c692d54c`): `internal/webstercli/verbs_test.go`'s `seedPersistentPreRunFixture` now seeds `batcher.yaml` and unconditionally seeds `webster.yaml`; `TestPersistentPreRunE_UnknownBatcherFailsFast`/`TestPersistentPreRunE_DefaultBatcherResolves` now drive `batcher.ConfigTemplate()`'s `active:` key.

`verify:` (`go build ./...`, untagged tests, and `-tags integration` tests over `internal/websterengine`/`internal/webstercli`) all pass. Working tree is clean of tracked in-scope modifications.

{"status":"success","commit_sha":"c692d54c7fb7fd3241349453db452645c302f9d2","session_id":"fdc6f31b-f3e1-4b43-9002-dcbd2badd601","cards_done":[5,6,7,8]}
