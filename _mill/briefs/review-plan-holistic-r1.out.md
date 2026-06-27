MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-27
```

## Findings

### [BLOCKING] Bad-flag path emits "unknown flag", not "unknown command"
**Location:** Batch 5 Card 17 (also help-and-unknown-surfaces decision; Batch 2 Card 8)
**Issue:** cobra/pflag print `unknown flag: --no-such-flag` for bad flags, but Card 17 tells the implementer to assert the `unknown command` substring for "unknown subcommand/bad flag"; the existing `TestRunCLIUnknownFlagFails` ({"--no-such-flag","status"}) will then fail, breaking batch-5 verify.
**Fix:** Split the assertion — unknown *subcommand* asserts `unknown command`; bad *flag* asserts `unknown flag` (or just exit 1). Correct the decision wording in 00-overview that conflates the two cobra messages, and the same caveat in Card 8.

### [NIT] board card says "12 subcommands" but there are 11
**Location:** Batch 4 Cards 12/14 (and scope)
**Issue:** board/cli.go has 11 switch cases (upsert, upsert-batch, set-phase, remove, get, list, list-full, merge, set-deps, rerender, sync); the cards repeatedly say "12".
**Fix:** Change the count to 11 ("one subcommand per existing case" is otherwise correct).

### [NIT] SilenceUsage only set on root, not module Command()s
**Location:** exit-and-error-contract decision; module cards 9/10/12/15/16
**Issue:** The in-process `RunCLI` seam executes a module `Command()` as its own root; without `SilenceUsage` cobra dumps full usage text into the merged buffer on cobra-error paths (harmless to substring asserts, but noisy/inconsistent with production root).
**Fix:** Optionally set `SilenceUsage: true` on each module `Command()` (or in `clihelp.Execute`) for parity; not required for tests to pass.

## Verdict

REQUEST_CHANGES — fix the unknown-flag vs unknown-command assertion guidance; remainder is sound.
MILL_REVIEW_END