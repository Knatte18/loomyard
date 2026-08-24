MILL_REVIEW_BEGIN
# Review: Migrate planparser.Card to Edits/Uses fields

```yaml
duration_s: 176.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:decision] Old `**verify:**` spelling has no disposition
**Demoted-from:** BLOCKING
**Section:** `### field-mapping` + `### retired-label-disposition`
**Issue:** `field-mapping` capitalises the on-disk label (`verify:` → `Verify:`) and `card-grammar` lists `**Verify:**`, but the seven retired labels do not include the format-3 spelling `**verify:**` (`parse.go:307` `cardVerifyLabel = "**verify:**"`, matched case-sensitively via `strings.HasPrefix`), so a stale lowercase line falls through `parseCardBody`'s `default: i++` and is silently dropped — the exact misparse class `retired-label-disposition` exists to prevent, and `card-unknown-label` is explicitly out of scope.
**Fix:** State the disposition: either keep `**verify:**` recognised as an eighth retired label mapping to `**Verify:**` (`card-retired-label`), or accept both spellings; also note the deliberate asymmetry with the plan-level `## verify:` section, which stays lowercase since `sections.go` is unchanged.

### [NIT:consistency] `Custom` + `path-missing`: targets only, or `Uses:` too?
**Demoted-from:** BLOCKING
**Section:** `### path-missing-rework`
**Issue:** "What it checks" says path-shaped entries in **any** card's `Uses:`, and "What it never checks" scopes the carve-out to "`Custom` card **targets**" — but the following precision bullet lists "Exempt: `path-missing`" unqualified, so an implementer cannot tell whether a `Custom` card's `Uses:` paths are checked; the stated rationale (a `Custom` card creating something) only covers targets.
**Fix:** Qualify the exempt bullet to "`path-missing` **on its targets**" (or state the wider exemption and why), so exactly one reading survives.

### [NIT:consistency] `webster-parallel-execution.md` mis-attributed to Wave 3
**Section:** `## Scope` → Out, and the Q&A entry on the stale design docs
**Issue:** `manifest/roadmap.md:61-62` is the **Someday** item "webster: worktree-per-card parallel execution"; the Wave 3 DAG item (`roadmap.md:31`) does not mention that doc at all, and `roadmap.md:14` assigns reconciliation to the whole Card-format group — so "explicitly assigned to the Wave 3 DAG task" is wrong, and the design-doc banner rewrite risks encoding it.
**Fix:** Re-word to cite `roadmap.md:14`/`:62` as the owning items (Someday worktree-per-card parallel execution), keeping the line references, which are correct.

### [NIT:scope] `Intent` → `Summary` rename touches one render function, not two
**Section:** `## Technical context` → Consumers outside `planparser`
**Issue:** `render.go:268` (`RenderBatchIndex`) is the only `Card.Intent` read; `RenderProgress` (`render.go:277-294`) reads `Number`/`Slug` only, so "the rename touches those two functions" overstates the surface.
**Fix:** Name `RenderBatchIndex` alone as the rename site.

## Verdict

APPROVE
Two implementer-facing gaps: retired `**verify:**` spelling and `Custom`'s `path-missing` scope.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
