MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder

```yaml
duration_s: 245.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:design] Stencil fixture needs a parent mkdir, unstated
**Section:** Testing → `Build`, twelve-engine case
**Issue:** `stencilstore.Path` is `filepath.Join(baseDir, RelPath(name))` and `RelPath` returns `<family>/<name>.md` for any hyphenated name, so "write plain files at `stencilstore.Path(dir, name)`" into a fresh `t.TempDir()` `StencilsDir` fails with ENOENT unless the family directory is created first; the discussion enumerates what is *not* needed (stamp, `Reconcile`, registry entry) but omits this.
**Fix:** Name `internal/shedrecipe/entries_singlellm_test.go`'s `writeStencilFile` (which does `os.MkdirAll(filepath.Dir(path))`) as the pattern, or state the mkdir explicitly.

### [NIT:design] Piece-4 framing ignores `shedrecipe` → `loomshed`
**Section:** Problem ("the only piece still missing before piece 4 can run")
**Issue:** `internal/shedrecipe/entries_simple.go` imports `internal/loomshed`, so `shedbuild` → `shedrecipe` → `loomshed`; piece 4 as the roadmap words it ("replace `internal/loomshed`'s Go literal … using the loader/builder") would make `loomshed` import `shedbuild` and close a production import cycle. Nothing in this task breaks, but the unblocking claim rests on an unexamined premise.
**Fix:** State that the recipe consumer must be `internal/loomcli` (or another package above `loomshed`), never `loomshed` itself, as a carried-forward note for piece 4.

### [NIT:decision] No disposition on a sole-parser invariant
**Section:** Constraints
**Issue:** The only `CONSTRAINTS.md` edit named is the Told-Geometry list append plus its "ten"→"eleven" count; the discussion never says whether `shedbuild` is (or is deliberately not) declared sole parser of the recipe format, despite the directly analogous Planparser Sole-Parser Invariant.
**Fix:** Record an explicit disposition — either add such an invariant in the same commit or state why one is premature until piece 4 ships a real recipe.

### [NIT:decision] "`Config` never contains absolute paths" unaddressed
**Section:** Decisions → Validation split
**Issue:** `manifest/designs/shed-recipe.md` states as a design rule that a recipe's `Config` never contains a path, but the validation split enumerates only shape checks and delegates everything else to `shedengine.validate()`/`shedcheck`, neither of which can see config values; the rule ends up enforced by nobody and the discussion does not say so.
**Fix:** State the disposition — the rule stays a review/authoring obligation (each entry resolves relatives against a told root), or `Parse`/`Build` policies it.

### [NIT:decision] Duplicate YAML mapping keys unaddressed
**Section:** Decisions → Strict unknown-key rejection
**Issue:** Strictness is specified for unknown keys and for duplicate row `name` values, but a repeated mapping key inside one row or at document level (e.g. two `on_done:` keys) has no stated expected behaviour or test case, and the decision's own rationale is about silent last-write-wins-class defects.
**Fix:** State whether duplicate keys are the decoder's problem (pass-through, whatever `yaml.v3` does) or a `shedbuild` check, and add the corresponding `Parse` table case.

## Verdict

APPROVE
Decisions are settled and source-grounded; five recorded NITs, none blocking plan writing.
MILL_REVIEW_END
