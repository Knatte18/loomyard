{"status":"success","commit_sha":"d2a32c19e5c2e8f8aee816cda1571d03b2c06e82","session_id":"35d7fccf-e2df-41ea-a58e-2bc43b0a7aa2","cards_done":[6,7,8,9,10,11,12,13]}

All 8 of 8 cards in this batch are committed and verified — this is a genuine "all complete" (not partial): cards 6–13 each have a matching commit subject in the log since the batch-start commit `37a06412`, and the batch's `verify:` command (`go test -tags integration ./internal/fabricengine/ ./internal/buildercli/ ./internal/webstercli/ ./internal/perchcli/`) plus `go build ./...` both pass cleanly with no uncommitted tracked changes.

Summary of what changed (all paths absolute from `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface`):

- `internal/fabricengine/commit.go` — `Fabric.Commit` now classifies against the resolved `l.RelPath` instead of a hardcoded `"."` (card 6).
- `internal/fabricengine/weftgit.go` — deepened the lock exclude glob to `**/_lyx/*/**/*.lock` (card 7); unexported `PushWeftAt` → `pushWeftAt` (card 11).
- `internal/fabricengine/unwire.go`, `internal/fabricengine/bolt.go` — updated to the new `pushWeftAt` casing (card 11).
- `internal/buildercli/weft.go`, `internal/webstercli/weft.go`, `internal/perchcli/run.go` — migrated onto `Fabric.Commit` with positive-only pathspecs, dropping all `:(exclude)` magic and the standalone `PushWeftAt` calls (cards 8–10).
- `internal/buildercli/weft_test.go`, `internal/webstercli/cli_test.go` — dropped now-dead pathspec-shape unit tests for deleted helpers (discovered mid-batch; plan file `_mill/plan/02-commit-migration.md` was updated first to add these as an in-scope surprise-file fix, per protocol).
- `CONSTRAINTS.md` — retired the "Anchored exclusions" bullet and revised "Cross-module exclusions" to describe the git-exclude-only guardian (card 12).
- `internal/fabricengine/commit_integration_test.go`, `internal/buildercli/weft_integration_test.go`, `internal/webstercli/weft_integration_test.go`, `internal/perchcli/run_integration_test.go` — new nested-RelPath and exclude-file regression coverage (card 13), including fixture fixes (seeding the repo-wide `fabric.yaml` and `.fabric-anchor` marker) needed because `Fabric.Commit` now re-resolves geometry itself rather than trusting a caller-built `Layout`/pathspec.

Three plan-edit commits (`4c0bbe92`, `b8b9f644`, `3ecc4920`) extended `_mill/plan/02-commit-migration.md`'s `Edits:` lists ahead of each surprise-file fix, per the brief's mid-batch scope-extension protocol.
