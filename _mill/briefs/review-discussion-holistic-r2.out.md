MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-27
```

## Findings

### [GAP] Assembled-root exit code on handler failure
**Section:** Decisions → exit-and-error-contract; Technical context (main.go)
**Issue:** Handlers record exit 1 in a holder and return `nil` to cobra, so `root.Execute()` returns nil; the spec gives `RunCLI` a `return exitCodeFromHandlers` but never says how `cmd/lyx/main.go` reads the firing module's holder — a naive `Execute()==nil → exit 0` would silently break the "exit codes unchanged" core constraint for real `{"ok":false}` failures (e.g. `lyx weft status`).
**Fix:** Specify the retrieval path for the production root — a single shared holder passed into each `Command()`, or read the executed command via `root.ExecuteC()` — so main maps a recorded exit 1 to `os.Exit(1)`.

### [NOTE] Holder source vs fixed `Command() *cobra.Command` signature
**Section:** Scope; integration-style-c-preserve-seam (adapter snippet)
**Issue:** Scope fixes `func Command() *cobra.Command` with no out-param, yet the `RunCLI` snippet returns `exitCodeFromHandlers` from a source that signature can't expose; "closure vs struct is mill-plan's job" doesn't pin where RunCLI actually reads it.
**Fix:** State whether the holder is a package var, stored on the `*cobra.Command`, or returned alongside it, so RunCLI and main share one contract.

### [NOTE] Which flagset feeds the `--json` `flags` array
**Section:** Decisions → json-help-form (schema)
**Issue:** The schema shows a `flags` array but doesn't say whether inherited persistent flags (the root `--json`, muxpoc's persistent tuning flags) appear at a subcommand node or only local flags — affects the `--json` schema test's expectations.
**Fix:** Specify local-only vs inherited (`LocalFlags`/`Flags`/`InheritedFlags`) for the `flags` array; verified no current module defines `--json`, so no name collision exists.

## Verdict
GAPS_FOUND — production exit-code retrieval for handler failures via the assembled root is unspecified.
MILL_REVIEW_END