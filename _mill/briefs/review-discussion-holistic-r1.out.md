MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Nothing ever commits weft-side `_pattern/`
**Section:** Scope / junction-ownership
**Issue:** `internal/fabricengine/template.yaml:2` defaults `pathspec: _lyx`, and `undo.go:93` uses `ScopedPathspec(RelPath, []string{LyxDirName})` — no weft-git caller stages `_pattern`, so a `PATTERN.md` written through the junction is never committed or pushed and never reaches another machine or another worktree's weft pull.
**Fix:** Decide explicitly whether `_pattern` joins fabric's default `pathspec` (a config-schema change requiring configsync reconcile) or whether content persistence is stated as out of scope with the consequence named.

### [GAP] `lyx init --undo` behaviour on weft `_pattern/` undecided
**Section:** unwire-generalisation / Testing → initengine
**Issue:** `initengine/undo.go:72-96` does `RemoveAll(WeftLyxDirFor(slug))` then commits and pushes the deletion; the discussion says only that `undo` "is updated to match" the result shape and never decides whether undo also deletes weft `_pattern/` — i.e. destroys the host repo's own PATTERN.md and pushes that deletion.
**Fix:** Decide and pin the weft-content behaviour for `_pattern` in undo, and state the resulting `UndoResult` shape (`LyxJunction string` today) since it is CLI-observable output.

### [GAP] `worktreeRoot` is the wrong base at `RelPath != "."`
**Section:** Scope / call-site-plumbing
**Issue:** the junction sits at `filepath.Join(Hub, slug, RelPath, DirName)` (`hubgeometry.go:524`, mirrored by `HostLyxLinkHere`), but `Directive(worktreeRoot string, role Role)` and the named sources (`deps.WorktreeRoot`, webster's `worktreeRoot`) are worktree-root-anchored, so in a nested-hub geometry the stat misses and PATTERN silently renders as inactive in all five agents.
**Fix:** State which base the five call sites pass (RelPath-mirrored, e.g. via a `HostPatternLinkHere`-style accessor) and add a RelPath-non-`.` case to the `internal/pattern` test list.

### [GAP] fabric reconcile's junction health check stays `_lyx`-only
**Section:** Scope (fabricengine changes)
**Issue:** `reconcile.go:146-148` health-checks `HostLyxLinkHere()`/`WeftLyxDir()` only, so a missing or mis-pointed `_pattern` junction reports `ReconcileActionAlreadyHealthy` and is never repaired; the scope names only the unwire path and `status.go`.
**Fix:** Bring `reconcile.go`'s health check into the per-junction generalisation, or state explicitly why it is deferred.

### [GAP] Junction created without materialising its weft target
**Section:** junction-ownership / Technical context → initengine
**Issue:** `checkout.go:152` and `reconcile.go:153` call `WireJunctions` but never `MkdirAll` through the junction (only `Init` does); a `_pattern` link created there points at a nonexistent weft dir, and the next `seedLyxJunction` re-check hard-errors at `junction.go:83` ("weft directory does not exist at ..."), breaking init/checkout/reconcile for that worktree.
**Fix:** Decide where the weft-side `_pattern` directory is materialised for the non-init `WireJunctions` callers, or make the seed path tolerate a not-yet-materialised target.

### [GAP] Burler gets the reviewer variant but also writes code
**Section:** template-set / directive-shape-and-wording
**Issue:** `review-prompt-template.md:10-17` states the round is "review, then fix" — phase B edits target files and commits — so the reviewer-only checklist ("before you judge anything", "every violation is BLOCKING") never states the implementer obligation not to write a violation; burler targets are also frequently prose, where the directive is noise.
**Fix:** Decide whether burler receives both variants (or a combined block) and whether the directive is gated on a code-touching target.

### [NOTE] `UnwireResult` replacement shape left open
**Section:** unwire-generalisation
**Issue:** the decision offers "`JunctionsRemoved []string`, or a count" without choosing, and does not say what a mid-loop failure reports — `UnwireJunctions` today returns a zero `UnwireResult` on junction error (`junction.go:163`), which becomes a lie once one junction was already removed.
**Fix:** Pick one shape and state the partial-failure contract.

### [NOTE] Enforcement claim is broader than the guard
**Section:** Testing → Repo-wide; hubgeometry
**Issue:** "No new `_pattern` string literal outside `internal/hubgeometry` (enforced automatically)" overstates `TestEnforcement_GeometryLiterals`, which flags only `filepath.Join` args, `+` operands and const values (`enforcement_test.go:237-303`); `status.go`'s pollution scan will legitimately carry `"_pattern"` in a pathspec slice and a `HasPrefix` comparison.
**Fix:** Restate the repo-wide assertion in the same construction-context terms the Constraints section already uses correctly.

### [NOTE] `Directive` cannot distinguish stat error from absent
**Section:** active-check-semantics
**Issue:** the signature returns only `string`, so a permission or I/O error on `PATTERN.md` renders identically to "inactive" — constraints silently vanish from five agents with no signal.
**Fix:** Decide the non-`IsNotExist` stat-error behaviour and pin it with a test.

### [NOTE] Marker-placement rule contradicts its own per-template targets
**Section:** marker-placement
**Issue:** the uniform rule is "immediately after the opening role paragraph, before the first concrete work instruction", but master is placed at `master-template.md:25` (inside `## Orientation`, splitting it from line 27's `_lyx` rule) and plan at `plan-template.md:19` (inside `## Step 2`) — and the block carries its own `##` heading, so it terminates the section it lands in.
**Fix:** Either place the block before the first `##` in every template or state the per-template exceptions as the rule.

## Verdict

GAPS_FOUND
Persistence, undo semantics, RelPath base, reconcile scope and burler role need decisions.
MILL_REVIEW_END
