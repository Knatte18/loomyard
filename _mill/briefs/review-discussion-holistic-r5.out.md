MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Reason-string reword breaks loom preflight classification
**Section:** junction-health-check (round-4 six-site table)
**Issue:** `loomengine/preflight.go:127-134` classifies `PairInSync`'s reason by prefix — `strings.HasPrefix(reason, "junction")` ⇒ `CheckJunction` **and** `check3BlocksSeed = true`; rewording `drift.go:83`/`:110` to `host <name> junction missing`/`points elsewhere` sends both into the `default` branch, so a broken junction reports as `CheckWeftSync` and stops blocking the seed check (`preflight_integration_test.go:316` fails). `loomengine` appears in no breakage list in the discussion.
**Fix:** State the consumer contract for these strings (update preflight's matcher, or match on a stable prefix/typed reason) and add `internal/loomengine` to the certain-breakage list.

### [GAP] `Layout.WorktreeRoot` is not the value the call sites pass today
**Section:** call-site-plumbing ("`worktreeRoot` is replaced by the Layout … `Layout.WorktreeRoot` already carries the same value")
**Issue:** `webstercli/beginbatch.go:103` passes `WorktreeRoot: c.layout.Cwd` (same in `buildercli/run.go:61`, `spawnbatch.go:143`), and `RenderForkPrompt` feeds it straight into the `{{.worktree_root}}` marker (`render.go:127`). At `RelPath != "."` — the exact geometry this decision exists for — `Cwd != WorktreeRoot`, so replacing the parameter silently changes the fork prompt's worktree root.
**Fix:** Decide explicitly which anchor `worktree_root` must render (`l.Cwd` today) and state that the Layout swap preserves it, rather than asserting the two are identical.

### [GAP] No migration story for the new junction detection
**Section:** junction-health-check / weft-persistence ("no detection or warning surface is in scope")
**Issue:** Every worktree wired before this change lacks the `_pattern` junction, so the generalised loop makes `status` report not-in-sync, `reconcile` report a repair, and loom's preflight fail `CheckJunction` (blocking the run and the seed check) until `lyx init`/`reconcile` is re-run — including this repo's own live worktrees. Only the pathspec migration consequence is documented.
**Fix:** State the junction-side upgrade consequence and the operator remedy (re-run `lyx init` or `lyx fabric reconcile`), and whether a blocked preflight on legacy worktrees is accepted.

### [GAP] Master's role variant contradicts Master's own prompt
**Section:** template-set (webster Master ⇒ `RoleImplementer`)
**Issue:** `master-template.md:21` states "You never edit code yourself" — verbatim the reason the discussion gives for excluding `builderengine/orchestrator-template.md` — yet Master gets `RoleImplementer`, whose block reads "do this before you write any code / Before you edit a single file". The exclusion criterion and the inclusion contradict each other in the same decision.
**Fix:** Restate the criterion (context-inheritance, not code-editing) or give Master wording that matches a non-editing orchestrator role.

### [NOTE] "Whitespace-only renders as nothing" is not what the stated implementation does
**Section:** stencil-optional-marker / Technical context
**Issue:** The prescribed implementation (copy `values`, seed listed-but-absent names with `""`) leaves a whitespace-only optional value rendering its whitespace verbatim, while the decision and the test list both say it "renders as nothing".
**Fix:** Pin whether `FillOptional` normalises whitespace-only optional values to `""`, or soften the contract to "absent or empty".

### [NOTE] Named webster Deps types do not exist
**Section:** call-site-plumbing ("Open plumbing detail for mill-plan")
**Issue:** The types are `BeginDeps` (`beginbatch.go:68`), `RecoverDeps` (`recoverbatch.go:66`) and `RecordDeps` — not `BeginBatchDeps`/`RecoverBatchDeps`/`RecordBatchDeps`; `beginbatch.go:77` is the `WorktreeRoot` field, not the struct.
**Fix:** Correct the names/lines so mill-plan edits the right structs.

### [NOTE] `Directive`'s nil-Layout behaviour is unspecified
**Section:** call-site-plumbing / active-check-semantics
**Issue:** `Directive(l *hubgeometry.Layout, …)` returns only a string, and several Deps structs are assembled by callers that could leave `Layout` unset; a nil Layout panics inside all five prompt-assembly paths.
**Fix:** Pin the nil-Layout contract (empty string, or documented precondition) alongside the zero-`Role` case already on the test list.

## Verdict

GAPS_FOUND
Four verified gaps: preflight reason contract, worktree-root anchor, junction migration, Master's role.
MILL_REVIEW_END
