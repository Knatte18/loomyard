All technical claims in the discussion verify against source: the 8 registered modules, `warp reconcile` having no `--apply` flag, the `warp checkout` wrapped error (`host switch to branch %q failed (git exit %d)`), `config --set`/`--print`/`reconcile [--apply]` with mutual exclusivity, `init --undo` via `initengine.Undo` with per-step outcomes, the `gitignore.Ensure(cwd, ".lyx/")` literal quirk (init.go:101), and weft's five subcommands with the fixed `"weft sync"` commit message. The discussion is round 3 and has already absorbed corrections from rounds 1 and 2.

MILL_REVIEW_BEGIN
# Review: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-02
```

## Findings

### [NOTE] `(discovery)` token vs drift-guard Assert 2
**Section:** Decisions → "Coverage mechanism" and "Coverage test: location and mechanics"
**Issue:** The optional `**Covers:** (discovery)` line for S0/S1 is left as implementer's call, but Assert 2 requires every token in `covered` to be a registered module — a literal `(discovery)` token would fail it unless the parser explicitly strips parenthesized/non-module tokens before Assert 2 runs.
**Fix:** State that if the `(discovery)` form is used, the parser must exclude parenthesized tokens from `covered` prior to the drift-guard assertion (or simply mandate "no `Covers:` line" for S0/S1 to remove the ambiguity).

## Verdict

APPROVE
Scope, decisions, and testing are crisp; all source claims verified; one non-blocking parser NOTE.
MILL_REVIEW_END