All 8 cards in the `module-rearm` batch are complete and committed. 2 commits cover cards 21-27 (combined per file-overlap allowance), and card 28 (Commit: none) was satisfied by re-running the batch's full two-tier `verify:` this turn with all named regression suites green.

Summary of work:
- /home/knatte/Code/loomyard/wts/shed-generic-watchdog/internal/loomcli/arm.go (new): loomcli.Arm/arm/specFor plus loom's PreRun/PostRun/PreStep/PostStep/StatusExtras hook implementations.
- /home/knatte/Code/loomyard/wts/shed-generic-watchdog/internal/loomcli/cli.go: added spec/parentFlag/entryObservation fields, loomVerbTexts, rewired Command() onto shedverbs.Verbs, rewrote resolvePersistentPreRun to call c.arm.
- /home/knatte/Code/loomyard/wts/shed-generic-watchdog/internal/loomcli/run.go, step.go: trimmed to the pure helpers that survive the extraction (shouldReflectFriction/reflectFriction, stepKindForBootstrapStage retargeted onto shedverbs.Kind*).
- Deleted internal/loomcli/status.go, internal/loomcli/pause.go; removed the now-orphaned ensureStatusLockDir from sharedbootstrap.go.
- Retargeted internal/loomcli/cli_test.go, step_test.go, status_test.go onto shedverbs' exported names via a new loomVerbCommand test helper built on c.specFor.
- /home/knatte/Code/loomyard/wts/shed-generic-watchdog/internal/lifecyclecli/arm.go (new): lifecyclecli.Arm/arm/specFor plus PreRun/PostRun/StatusExtras hooks.
- /home/knatte/Code/loomyard/wts/shed-generic-watchdog/internal/lifecyclecli/cli.go: added spec field, lifecycleVerbTexts (including new pause text and --watch in status's Long), registered run/status/pause from shedverbs.Verbs.
- Deleted internal/lifecyclecli/run.go, status.go.
- Added the three agreed additive-surface-change tests to run_test.go (history_length, pause, --watch/--interval) and lifecycle_integration_test.go (history_length), and updated cli_test.go's exact-verb-set/arg-count assertions to include pause.

Both tiers of the batch's verify: command are green: go test ./internal/loomcli/... ./internal/lifecyclecli/... ./internal/shedverbs/... ./cmd/lyx/... and go test -tags integration ./internal/loomcli/... ./internal/lifecyclecli/.... go build ./... and go vet ./... are clean across the whole repo, including the smoke-tagged suites (compile-checked with -tags smoke, not run, per the plan's exclusion).

Card-count self-check: 8 of 8 cards in this batch are done (21-27 via the two content commits above; 28 is the Commit: none verification card, satisfied this turn).

{"status":"success","commit_sha":"783e57d290a13747dd482a85631eadcd3731c314","session_id":"46dd139f-6fa9-4eb9-b5c0-ae09256b9026","cards_done":[21,22,23,24,25,26,27,28]}
