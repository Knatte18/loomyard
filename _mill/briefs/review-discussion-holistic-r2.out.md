Only `mill-config.yaml` (in scope, `short_name`) and `.vscode/settings.json` (explicitly out of scope — gitignored, discussion line 98) contain `mhgo` among config files. The `.millhouse/` files are harness (out of scope) and the grep confirms they contain no `mhgo`. go.sum has no `mhgo` (only go.mod does, which is covered).

Let me verify the `.vscode/settings.json` is actually gitignored as the discussion claims (line 1 of .gitignore showed `.vscode/settings.json`) — confirmed earlier. The discussion's scope is accurate and complete.

I have thoroughly verified the discussion against the source. The discussion is exceptionally well-grounded. My findings are minor NOTE-level observations. No GAPs that block plan writing.

MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-16
```

## Findings

### [NOTE] state_test.go has no full expected-output literal
**Section:** Scope → Test-fixture strings; Technical context → Paired test input/expected
**Issue:** The discussion says input + expected derived output "move together in `muxpoc/state_test.go`", but `TestSocketName` (lines 111-118) only asserts a `"muxpoc-"` prefix and char-class validity — there is no hardcoded `muxpoc-mhgo-mux-design` expected string, so the test passes regardless. The only full example is the `state.go:176` doc comment.
**Fix:** Minor wording: clarify the test has no full expected literal; renaming the input cwds is for brand consistency, not to keep a paired assertion green.

### [NOTE] worktree package `_mhgo` literals not enumerated as a rename site
**Section:** Scope → In (Managed-state directory)
**Issue:** `internal/worktree` carries many `_mhgo` literals in comments/tests (`portals.go`, `add.go`, `config.go`, `cli.go`, `portals_test.go`, `cli_test.go`, `config_test.go`, `remove_test.go`) but the "In" list names worktree only for import-path changes; the dir-literal sites there are covered only by the generic "all tests"/"comments" clauses.
**Fix:** Add `internal/worktree` to the managed-state-dir source-site list (or note its `_mhgo` literals are covered by the prose/test sweep) so a plan writer does not miss them.

## Verdict

APPROVE
Scope is source-grounded and complete; only two minor wording NOTEs, no blocking gaps.
MILL_REVIEW_END
