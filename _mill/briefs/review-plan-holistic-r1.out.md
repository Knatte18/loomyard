MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Sonnet 5 per platform naming)
reviewed_file: plan/
date: 2026-09-06
```

## Findings

### [BLOCKING:design] quarrycli root resolution breaks in standalone mode
**Location:** Batch 6, Card 26. **Issue:** Requirements say to resolve the repo root via `preflight.ResolveMode(cwd)` and "use the returned location's worktree path as the root," but `ResolveMode` (`internal/preflight/predicates.go:110-134`) returns a **nil** `*lyxcwd.Location` in every `ModeStandalone` branch (all three `return nil, ModeStandalone, nil` sites) — confirmed by `internal/webstercli/wiring_test.go:127` and `internal/burlercli/wiring_test.go:127`'s own comment "loc is nil regardless of which real ResolveMode cause produced ModeStandalone." `webstercli`/`burlercli` handle this by threading the raw `cwd` through to `wire(loc, mode, cwd, ...)` and using `cwd` directly in `wireStandalone` (`internal/webstercli/wiring.go:141-145`), never `loc.WorktreePath()`. **Fix:** card 26 must specify the same two-mode split — hub mode uses `loc.WorktreePath()`, `ModeStandalone` uses `cwd` itself as the root — or the implementer will nil-deref the first time `lyx quarry` runs against a plain (non-hub) repo.

### [BLOCKING:consistency] Plan.SurfaceRefs's type contradicts its own per-card keying rule
**Location:** Batch 2, Card 5 (vs. Card 12). **Issue:** Card 5 declares `SurfaceRefs map[string]string` keyed on "a canonical model string," but then separately requires "store the map per card where two cards' surface spellings for one canonical string differ, keyed on the card's own cardID-style N-<slug> identity" — which a flat `map[string]string` cannot represent (one key, one value; a collision between two cards' surface spellings silently loses one). Card 12 (`RewriteRefs`, `SurfaceRefs`'s stated "only consumer") then treats it as a plain flat map — "map each key through it to the lexeme the file actually carries" — with no card-scoped lookup at all, contradicting card 5's per-card requirement. **Fix:** pick one shape (either accept last-write-wins on a true collision and drop the per-card language, or make the type `map[string]map[string]string`/similar and update card 12's consumption accordingly) and state it once, consistently, across both cards.

### [BLOCKING:scope] canonicalizeCard's `lang` argument has no stated source inside ParsePlan
**Location:** Batch 2, Cards 4 and 5. **Issue:** Card 4 defines `planLanguage(plan *Plan) (glyph.Language, bool)`, taking a `*Plan`. Card 5 requires `ParsePlan` to call `canonicalizeCard(card *Card, lang glyph.Language, surface map[string]string)` **inside the per-card parsing loop**, but per `internal/planparser/parse.go:132-148`, the `*Plan` value is only constructed *after* that loop finishes (mirroring how `root` is computed from `fm` before the loop, `plan.Language` would need the same pre-loop treatment) — there is no `*Plan` to hand `planLanguage` at the call site card 5 names. **Fix:** state explicitly that `lang` is derived from `fm.Language` before the cards loop (the same pattern `root` already uses), and that `planLanguage` is for a different, later caller (e.g. `validate.go`) — not for use inside `ParsePlan`'s loop.

### [NIT:scope] loom-plan-spec.md's deviation-union section is left stale
**Location:** Batch 2, Card 8. **Issue:** `contracts/specs/loom-plan-spec.md`'s "## Deferred / forward-compat" section defines the changes-files/deviation union as "every path-shaped target entry ... plus the files holding every symbol-shaped target entry" — a definition batch 7 (card 35's `ScopeGuard`, card 39's stencil rewrite) replaces with a mechanical glyph/symbol-ID comparison. Card 8's rewrite list for this pinned, "Contract" doc names only the shape-classifier, `language:`, path-resolution, validation-checks, and worked-example sections; card 39 only touches the *stencil* (`webster-body-implementer.md`), not this spec section. **Fix:** add the "Deferred / forward-compat" section to card 8's (or card 39's) edit scope so the pinned contract doc doesn't ship describing a superseded mechanism.

## Verdict

REQUEST_CHANGES
Two source-verified mechanism gaps (quarrycli standalone root, ParsePlan's pre-Plan `lang`) and one internal contradiction (SurfaceRefs shape) need resolving before implementation.
MILL_REVIEW_END
