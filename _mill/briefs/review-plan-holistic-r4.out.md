MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Claude Sonnet 5, Anthropic)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [NIT:consistency] Card 9 omits the new `cobra`/`clihelp` imports it requires
**Location:** batch 2 / card 9 **Issue:** `skipStencilSeed(cmd *cobra.Command)` and `seedStencils(cmd *cobra.Command)` need `github.com/spf13/cobra` and `internal/clihelp` added to `cmd/lyx/stencilseed.go`'s import block, neither of which is currently imported there; card 9's Requirements never call this out, breaking the precedent card 1 sets by explicitly naming import adds/drops (`lock.go`'s `testing` add, `lifecycle.go`'s `testing` drop) for the same kind of existing-file edit. **Fix:** add one sentence to card 9's Requirements naming the two new imports, matching card 1's convention.

## Verdict

APPROVE
Plan is internally consistent, source-grounded, and faithfully implements every Shared Decision; one trivial import-mention gap.
MILL_REVIEW_END
