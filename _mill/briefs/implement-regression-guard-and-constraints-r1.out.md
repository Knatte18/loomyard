No tracked dirty files. 1 of 1 declared card committed (Card 10). Full report below.

{"status":"success","commit_sha":"caac01cb725289b88659f6b2631d6c54e2a5efb8","session_id":"f48d3556-4bcb-4336-94ab-c669e8b9361b","cards_done":[10]}

Summary: 1 of 1 cards committed for this batch (Card 10). Created `cmd/lyx/rawgitmutation_test.go` with `TestNoRawGitMutation_WebsterBuilderProductionSource`, which bans `gitrepo.New(` and `gitexec.RunGit(` in `internal/websterengine` and `internal/builderengine` production source, allowlisting the two grandfathered read-only exemptions in `internal/websterengine/gitwrap.go` and `internal/builderengine/gitquery.go`. Added the corresponding `allowedSpawners` entry in `cmd/lyx/tierpurity_test.go`. Updated `CONSTRAINTS.md`'s Fabric Git Invariant section to close the "Known gap, tracked" clause and record the new machine check in the Enforced-by bullet. All three verify tests pass (`TestNoRawGitMutation_WebsterBuilderProductionSource`, `TestTierPurity_UntaggedTestsSpawnNothing`, `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`). Working tree is clean of tracked modifications; commit `caac01cb` pushed to `webster-bisect-fabric-migrate`.

Files touched:
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/rawgitmutation_test.go` (new)
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/tierpurity_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/CONSTRAINTS.md`

{"status":"success","commit_sha":"caac01cb725289b88659f6b2631d6c54e2a5efb8","session_id":"f48d3556-4bcb-4336-94ab-c669e8b9361b","cards_done":[10]}
