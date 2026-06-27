MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-27
```

## Findings

### [GAP] muxpoc/cli_test.go contradicts the new no-arg/seam behavior
**Section:** Testing (regression list + "must still pass")
**Issue:** The plan lists only configcli/ide/weft/main_test.go as needing assertion updates and explicitly says "muxpoc: their existing in-process tests must still pass," but `internal/muxpoc/cli_test.go` asserts no-arg → exit 1 + empty stdout (now exit 0 + listing per no-arg-semantics) and unknown subcommand/flag → empty stdout (now non-empty, since `RunCLI` merges `SetErr` into `out` and cobra writes the error there).
**Fix:** Add `internal/muxpoc/cli_test.go` to the set of tests requiring updated assertions, and drop the claim that muxpoc's existing in-process tests pass unchanged.

### [NOTE] muxpoc PersistentPreRunE resolve-failure error surface unspecified
**Section:** Technical context (muxpoc gotcha) + exit-and-error-contract
**Issue:** muxpoc's pre-switch `paths.Resolve` today emits a JSON `{"ok":false,"error":"not a git repository"}` on stdout (exit 1); moved into `PersistentPreRunE`, returning the error yields cobra human-text on stderr (breaks the JSON contract), while returning nil after `output.Err` lets the subcommand `RunE` run anyway.
**Fix:** State how PersistentPreRunE preserves the JSON error envelope while aborting the subcommand (e.g. record exit via the holder and short-circuit, or a sentinel that stops RunE).

### [NOTE] "all modules write JSON via internal/output" is inaccurate
**Section:** Technical context (Module signature today / Output helpers)
**Issue:** The blanket claim that every handler routes through `internal/output` and it is "the single JSON sink" is false for configcli, whose `editOne` writes plain text via `fmt.Fprintf` (`unknown config module: …`, `aborted: …`) and returns the int directly.
**Fix:** Note configcli (and any other plain-text handlers) return their own int codes, so the `setExit(ctx, output.Err(...))` wrapper pattern is illustrative, not universal.

## Verdict
GAPS_FOUND
One testing-scope contradiction (muxpoc tests) must be resolved before planning.
MILL_REVIEW_END