MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:consistency] Scope says one MkdirAll, Decision says three
**Section:** `## Scope` In-list ("A single `os.MkdirAll` … in `internal/loomcli/run.go`") vs `### internal/loomcli/run.go owns creating the directory`
**Issue:** The authoritative work inventory still claims a single create site, while the Decision mandates one ensure-helper called from three sites (`run.go`, `drive.go` startup, and inside `Reflect` after archiving).
**Fix:** Rewrite the Scope bullet to name the exported ensure-helper and all three call sites, so the plan writer inventories three edits, not one.

### [BLOCKING:consistency] Unknown `Role`: silent empty vs hard error
**Section:** `### The directive is injected the way internal/pattern already does it` vs `## Testing` → `internal/friction`
**Issue:** The Decision states "an unknown or zero `Role` behaves the same way" (returns `("", nil)`, no read — matching `internal/pattern/pattern.go:94–98`), but Testing asserts "An unknown `Role` value is a hard error, not a silent empty string." These cannot both hold.
**Fix:** Pick one and state it in both places; if it diverges from `pattern.Directive`'s documented default case, say why.

### [BLOCKING:scope] All four webster composers need a new told parameter, not one
**Section:** `## Scope` In-list and `### How the friction directory reaches each of the three engines`
**Issue:** The discussion says `render.go`'s composers "read it from deps" and that only `RenderForkPrompt` gains a told parameter. Source shows all four are package-level free functions taking explicit args and no `RunDeps` (`render.go:145,177,210,264`); `anchorRoot` on recovery/Master is `pattern`'s probe root, not a note path. Every one of the four — plus their callers at `beginbatch.go:311`, `recoverbatch.go:154`, `runlevel.go:548,564` — needs the per-spawn note path threaded through.
**Fix:** State that all four composers gain a told note-path parameter and that their four call sites compose the id → `NotePath` from `RunDeps.FrictionDir`.

### [NIT:consistency] Q&A log carries superseded answers unmarked
**Section:** `## Q&A log` entries at "Which spawn sites get the directive?" (five) and "Who creates `.lyx/loom/friction/`?" (one `MkdirAll`)
**Issue:** Both are superseded by later entries, but neither carries the "Correction this supersedes" marking the migration decision uses, so a skimming plan writer can act on the stale answer.
**Fix:** Annotate the two superseded entries as corrected, pointing at the later ones.

### [NIT:design] `frictionengine` construction semantics unstated
**Section:** `## Testing` → `internal/frictionengine` ("Nil `Shuttle` seam and an empty stencils dir are rejected at construction")
**Issue:** The stated API is `Reflect(Deps) (Report, error)` with no constructor, so "at construction" has no referent, and when `Reflect`'s `error` is non-nil is never defined (reflection failures are explicitly nil-error).
**Fix:** Say whether validation happens inside `Reflect` or via a `New(Deps)` form, and name the only cases yielding a non-nil error.

## Verdict

REQUEST_CHANGES
Three contradictions between Scope, Decisions, Testing, and the actual composer signatures.
MILL_REVIEW_END
