MILL_REVIEW_BEGIN
# Review: CLI help & error ergonomics from sandbox run

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [GAP] W16 guard under-scoped: ide+muxpoc also have PersistentPreRunE
**Section:** Decisions → W16 / Technical context "Groups needing the W16 RunE"
**Issue:** The decision says only "the two groups with a `PersistentPreRunE` (`weft`, `board`)" need the early-return guard, but `internal/ide/cli.go` (line 34) and `internal/muxpoc/cli.go` (line 81) also carry a layout-resolving PersistentPreRunE that aborts without a git repo; once W16 makes those groups runnable, bare `lyx ide`/`lyx muxpoc` and `lyx ide bogus`/`lyx muxpoc bogus` will trigger git/layout resolution — breaking the no-git-repo subcommand listing and masking the unknown-subcommand error.
**Fix:** Apply the `cmd.Name()==<group>` early-return guard to all four PersistentPreRunE groups (board, weft, ide, muxpoc), and add the bare-group/no-git-repo tests for ide and muxpoc to the Testing section.

### [NOTE] W14 production stream for wrapped JSON error unspecified
**Section:** Decisions → W14 (root wrapping in `cmd/lyx/main.go`)
**Issue:** `main()` keeps stdout/stderr split (lines 37-38); the decision wraps the Cobra error via `output.Err` but does not say which stream receives the JSON in production.
**Fix:** State that the wrapped JSON error goes to stdout (matching domain errors via `cmd.OutOrStdout()`) so programmatic callers reading stdout see it.

### [NOTE] config domain errors stay plain-text, inconsistent with W12 JSON
**Section:** Decisions → W12 / W5
**Issue:** `configcli` already emits unknown-module and edit errors as plain text via `fmt.Fprintf` (configcli.go lines 43, 56, 70), not `output.Err`; W12 adds a JSON envelope for `--print` errors only, so `lyx config bogus` is plain text while `lyx config bogus --print` is JSON — contradicting W12's "consistency with the rest of the CLI" rationale.
**Fix:** Decide explicitly whether to harmonize config's existing edit/menu errors to `output.Err` or accept the asymmetry, and record it.

### [NOTE] W7 rename leaves stale "warp status" references in weft tests
**Section:** Decisions → W7 (verified-no-callers claim)
**Issue:** The "only warp.go and helptree_test.go reference status" claim misses doc comments at `internal/weft/cli_test.go:121` and `internal/weft/status_test.go:45` that mention "warp status".
**Fix:** Update those comments to "warp pairs" when renaming (no code break, doc rot only).

## Verdict

GAPS_FOUND
W16 guard must cover ide and muxpoc, not just weft and board.
MILL_REVIEW_END
