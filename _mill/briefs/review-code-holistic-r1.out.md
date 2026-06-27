MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-27
```

## Findings

### [NIT] cobra marked `// indirect` in go.mod despite direct imports
**Location:** `C:\Code\loomyard\wts\builtin-cli-help\go.mod:13`
**Issue:** `github.com/spf13/cobra v1.10.2` and `github.com/spf13/pflag v1.0.9` sit in the indirect `require` block even though cobra is directly imported in `internal/clihelp`, `cmd/lyx`, and every module's `cli.go`. Card 1 required `go get` + `go mod tidy`; tidy would move them to the direct block. A CI `go mod tidy && git diff --exit-code go.mod` check would fail.
**Fix:** Run `go mod tidy` so cobra and pflag appear in the direct `require` block without `// indirect`.

### [NIT] board abort guard uses `b == nil` instead of `clihelp.ShouldAbort`
**Location:** `C:\Code\loomyard\wts\builtin-cli-help\internal\board\cli.go:91,116,138` (and all other 11 subcommands)
**Issue:** Every board subcommand guards with `if b == nil { return 0 }` inside the `WrapRun` closure instead of the standardized `clihelp.ShouldAbort(cmd.Context())` used consistently by `ide/cli.go` and `weft/cli.go`. Functionally correct because `Abort` already set `es.code = 1` and `SetExit(ctx, 0)` is a no-op, but inconsistent with the module-level contract and fragile if future code changes the abort pattern.
**Fix:** Replace `if b == nil { return 0 }` guards with `if clihelp.ShouldAbort(cmd.Context()) { return nil }` or wrap bodies with `clihelp.WrapRun` after a closure-local abort check, consistent with ide and weft.

### [NIT] weft PersistentPreRunE reads flag via `cmd.Flags()` not `cmd.PersistentFlags()`
**Location:** `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\cli.go:48`
**Issue:** `--weft-path` is registered on the parent via `cmd.PersistentFlags()`, and in the `PersistentPreRunE` it is read via `cmd.Flags().GetString("weft-path")` where `cmd` is the leaf subcommand. In cobra, `cmd.Flags()` on a leaf includes inherited persistent flags so this works, but it is at odds with how muxpoc reads its persistent flags (also via `c.Flags()`, consistent) and creates a subtle dependency on cobra's merging behavior. No bug, but worth noting.
**Fix:** Use `cmd.InheritedFlags().GetString("weft-path")` or note that `cmd.Flags()` on a cobra leaf includes inherited flags, to make the intent explicit.

## Verdict

APPROVE
Implementation is complete, plan-aligned, and all cross-batch contracts hold; two cosmetic nits and a go.mod tidy omission only.
MILL_REVIEW_END
