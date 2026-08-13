MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [NIT:design] ExecuteIn's own contract for cwd == "" is unstated
**Section:** `the-injected-cwd-contract` / Scope bullet 2
**Issue:** `WithCwd(ctx, "")` panics and the empty-string sentinel is scoped "only in `RunCLIIn`", so a uniform one-line `RunCLIIn → ExecuteIn(Command(), cwd, out, args)` delegation would panic on every existing `RunCLI` call; the text never says whether `ExecuteIn` tolerates `""` or `RunCLIIn` must branch to `Execute`.
**Fix:** State the branch explicitly — `RunCLIIn` calls `Execute` when `cwd == ""` and `ExecuteIn` otherwise (or `ExecuteIn` treats `""` as "seed nothing").

### [NIT:consistency] "eight packages / roughly fourteen files" undercounts and overcounts
**Section:** `guard-bans-both-chdir-spellings`, "Why per-file and not per-package"
**Issue:** Eleven packages gain a seam change (10 `RunCLIIn` modules plus `loomengine`), not eight, and the untouched chdir-using files in those packages number exactly eleven — the list names all eleven while the prose says "roughly fourteen".
**Fix:** Replace both numbers with the verified counts (eleven packages, eleven untouched non-smoke files); the census elsewhere in the document (37 files / 118 occurrences) is otherwise exact and verified.

### [NIT:consistency] Clone's usage string is duplicated and needs `--into` in both places
**Section:** `clone-gets-an-explicit-destination`
**Issue:** The flag list appears verbatim at `fabriccli/fabric.go:64` (`Use:`) and again in the usage-error literal at `clone.go:125`; the discussion only says commit 2 carries "help text", so one of the two can silently go stale under the CLI/Cobra help-accuracy obligation.
**Fix:** Name both sites as part of commit 2's help-text update.

## Verdict

APPROVE
No blocking gaps; enumerations, line cites and counts verified against source.
MILL_REVIEW_END
