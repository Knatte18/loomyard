MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:design] Plan-Revalidate is not a per-card-merge boundary
**Section:** `drift-boundaries` (and Scope bullet on extending the row)
**Issue:** The decision states `Plan-Revalidate` gives "one batched `Resolve` over the remaining plan **after each card merge**", but `contracts/recipes/loom-recipe.yaml:187-213` places that row once, after the Plan-Review segment, with `on_done: Batchifier` — it runs before `Webster` and therefore before any card merge exists; "the remaining plan" is the whole plan, and the rationale "covering only `Plan-Revalidate` would catch drift a merge late" rests on a boundary that never fires post-merge.
**Fix:** Restate `Plan-Revalidate` as a one-shot pre-Webster baseline resolve and say explicitly that post-merge drift is carried by `begin-batch`/`record-batch` alone — or name the actual mechanism that re-runs it per merge.

### [BLOCKING:consistency] Surface lexeme per ref contradicts the ref-model decision
**Section:** `rewrite-write-path` "Keyspace" vs `parse-time-canonicalization` "Rejected"
**Issue:** `parse-time-canonicalization` rejects "storing both forms per ref (a two-field ref type rippling through `Targets`/`Uses`/`Pairs` and every consumer)", while `rewrite-write-path` decides each parsed ref "retains the surface lexeme it was written as, beside its canonical form" — the same thing, with no statement of where it is stored; `websterengine.refsIntersect`/`deriveEdges` consume plain `[]string`, so whether `Targets` stays `[]string` with a side table or becomes a struct slice is an open, rippling design question.
**Fix:** State the storage shape explicitly (e.g. a `Plan`-level canonical→surface map keeping `Targets`/`Uses`/`Pairs` as `[]string`) and reconcile the rejected alternative's wording.

### [NIT:scope] Interim window where the two path checks go silent
**Section:** `quarry-unitpath-precondition`
**Issue:** `path-missing` and `card-path-malformed` are both gated on `isPathRef` (`internal/planparser/validate.go:282,580,594,634,644,651`); once the classifier is rewritten and refs are canonicalized to glyphs, both stop matching anything, yet their repair is deferred to late `UnitPath()`-gated cards — so the plan lands merges with two checks dead.
**Fix:** State the interim posture — accept the gap for the card window, or hold the classifier rewrite in the same card as the disk-check repair.

### [NIT:consistency] Facade import path stated as the module root
**Section:** Problem "Why now" and Technical context "quarry's surface"
**Issue:** The facade is `github.com/Knatte18/quarry/quarry` (`/home/knatte/Code/quarry/wts/quarry/quarry/repo.go`, module path `github.com/Knatte18/quarry`, no package at the module root); the wording "public Go facade (`github.com/Knatte18/quarry`, package `quarry`)" reads as an import path a plan writer would copy.
**Fix:** Spell both import paths verbatim — `github.com/Knatte18/quarry/quarry` and `github.com/Knatte18/quarry/glyph`.

## Verdict

REQUEST_CHANGES
One drift boundary rests on a false premise; ref-model storage shape is contradictory and undecided.
MILL_REVIEW_END
