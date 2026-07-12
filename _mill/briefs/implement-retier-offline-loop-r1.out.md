All 6 of 6 cards committed, matching the batch's `## Cards` list exactly. The batch verify command passed cleanly.

```json
{"status":"success","commit_sha":"db9543d","session_id":"9d14fbc4-2936-44fc-9ae1-85be3d059200"}
```

Summary: 6 of 6 cards committed for batch `retier-offline-loop`.

Files touched (all paths relative to `C:\Code\loomyard\wts\test-suite-regression`):
- `internal/boardcli/cli_test.go`, `internal/boardcli/cli_unit_test.go` (new)
- `internal/perchcli/cli_test.go`, `internal/perchcli/cli_integration_test.go` (new)
- `internal/perchcli/run_test.go`, `internal/perchcli/run_integration_test.go` (new)
- `internal/muxcli/cli_test.go`, `internal/muxcli/cli_integration_test.go` (new)
- `internal/configcli/configcli_test.go`, `internal/configcli/configcli_integration_test.go`, `internal/configcli/reconcile_test.go`, `internal/configcli/reconcile_integration_test.go` (new)
- `cmd/lyx/main_test.go`, `cmd/lyx/main_integration_test.go` (new), `cmd/lyx/crosscompile_test.go`

Verification performed:
- `go build` / `go vet` (both untagged and `-tags integration`) clean for all five touched packages after every card.
- `go test ./<pkg> -count=1` and `go test -tags integration ./<pkg> -count=1` green after every card.
- Card 8's required equivalence check: built a detached worktree at the batch-start commit (`0f74b0c`), ran `go test -tags integration ./internal/boardcli ./internal/perchcli ./internal/muxcli ./internal/configcli ./cmd/lyx -list '.*'` before and after — the emitted test-name sets are identical (only per-run timing differs); confirmed the untagged `-list` still exposes each package's Tier 1 remainder. Worktree removed after the check.
- Final batch verify (`go test -tags integration ./internal/boardcli ./internal/perchcli ./internal/muxcli ./internal/configcli ./cmd/lyx -count=1`) passed: all five packages ok.
- `git status --porcelain --untracked-files=no` clean (no uncommitted tracked changes) before this report.

Tier 1 (untagged) wall-clock dropped substantially per package as expected from the plan's dry run (e.g. boardcli ~2.7s → ~1.4-1.9s in this run's package-scoped timing, perchcli 14s → ~4s pre-card-5 to ~2s after, muxcli ~16-18s → ~1-2s), confirming the re-tiering achieved its purpose without losing any test — every moved test is verbatim (name, body, doc comment unchanged) per the `moved-code-is-verbatim` and `test-names-are-preserved` Shared Decisions.

{"status":"success","commit_sha":"db9543d","session_id":"9d14fbc4-2936-44fc-9ae1-85be3d059200"}
