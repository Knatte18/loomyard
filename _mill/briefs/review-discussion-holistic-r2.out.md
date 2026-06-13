No existing porcelain worktree parsing. The discussion's `list` output shape includes `"main":true`, but `git worktree list --porcelain` does not emit a "main" marker — the first entry is conventionally the main worktree, but determining `main` requires a heuristic (first entry, or matching the hub). The discussion doesn't specify how `main` is derived. This is a gap.

I have enough to produce the review. Let me compile findings.

MILL_REVIEW_BEGIN
# Review: mhgo-worktree-module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-13
```

## Findings

### [GAP] `hubPath` used but never resolved
**Section:** Technical context → Worktree existence checks in `add`
**Issue:** Branch-exists check calls `RunGit([]{"rev-parse","--verify",branch}, hubPath)`, but only `sourceWorktreePath` (via `os.Getwd()`) and `container` are defined; `hubPath` has no derivation. `rev-parse` from any worktree resolves the same branch refs, so `sourceWorktreePath` would work — but the spec names an undefined variable.
**Fix:** State the cwd to run the branch check from (use `sourceWorktreePath`, or define how `hubPath` is located).

### [GAP] `list` "main" flag derivation unspecified
**Section:** JSON output shapes / `list` decision
**Issue:** Output shape has `"main":true`, but `git worktree list --porcelain` emits no main marker — only `worktree`/`HEAD`/`branch`/`bare`/`detached` lines. How `main` is determined (first entry? match hub?) is undefined, and the porcelain field-to-JSON mapping (e.g. `head` from `HEAD`) is not specified.
**Fix:** Define the rule for `main` (e.g. first porcelain block = main) and the line-to-field mapping.

### [GAP] Dirty-check includes untracked files; intent unstated
**Section:** Decision: `add` fails if source is dirty / Dirty-check implementation
**Issue:** `git status --porcelain` reports untracked files too, but the decision scopes the check to "staged or unstaged" changes. A repo with only untracked files would fail `add` — possibly intended, possibly not. `remove`'s `--force` check shares this ambiguity.
**Fix:** State explicitly whether untracked files count as dirty for both `add` and `remove`.

### [NOTE] Roadmap says milestone 4 tracks state; discussion defers it
**Section:** Scope (Out) / No state registry decision
**Issue:** `docs/roadmap.md` line 35-36 defines milestone 4 as "First consumer of ... the new state lib" and `docs/modules/worktree.md` describes a state registry and state-reconciled `list`. The discussion defers all state, leaving `list` as a thin git wrapper. Reasonable, but diverges from two existing docs.
**Fix:** Note that `docs/modules/worktree.md` and roadmap milestone 4 wording need follow-up alignment (the discussion already lists doc updates as in-scope).

### [NOTE] Force-remove fallback success/JSON shape on lock failure unstated
**Section:** Decision: on remove failure → force-remove + prune
**Issue:** When `git worktree remove` fails and the `os.RemoveAll` + `prune` fallback runs, it is unstated whether `remove` still returns `{"ok":true,...}` and whether `links_removed` reflects the fallback path. No failure mode for "RemoveAll itself fails" (e.g. directory still locked).
**Fix:** Specify the success contract and JSON on the fallback path, plus behaviour if `os.RemoveAll` also fails.

## Verdict

GAPS_FOUND
Three plan-blocking ambiguities: undefined `hubPath`, `list` "main" derivation, and dirty-check semantics.
MILL_REVIEW_END
