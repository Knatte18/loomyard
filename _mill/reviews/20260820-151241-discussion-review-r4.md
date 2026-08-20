MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer

```yaml
duration_s: 212.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Injected rubric carries its stencil stamp banner
**Section:** "Stencils — two generic templates, rubric injected by name"
**Issue:** Every on-disk stencil begins with `<!-- lyx-stencil: sha256=... -->` (`stencilstore.ApplyStamp`, `StampPrefix`); `stencil.Fill` strips that banner only from the *template* it parses (`stencil.go:27`), never from a marker *value*, so reading the rubric with `stencilstore.Read` and filling it into the `rubric` marker injects the hash banner verbatim into the judge/seed prompt. No existing call site does stencil-into-stencil injection — `burlerengine/prompt.go:65` fills `"rubric"` from a plain `Profile` string — so this is a new pattern with no precedent to copy.
**Fix:** State that the rubric bytes pass through `stencil.StripLeadingComment` before filling, and assert the filled prompt contains no stamp banner in the `Call` tests.

### [NIT:consistency] Re-bounce pointer contradicts itself
**Demoted-from:** BLOCKING
**Section:** "Cancellation and the output pointer" table vs "Testing" vs Q&A log
**Issue:** The pointer table says re-bounce → `Stuck` with an **empty** pointer (matching the body's `N == 0` re-bounce branch, where no ledger can exist), but the Testing section's pointer-discipline scenario says "the re-bounce names a ledger that exists", and the unlabelled r1 Q&A entry (last line of the log) still says re-bounce returns "`Stuck` with the existing ledger as pointer" — a formulation the body explicitly says was replaced by replay.
**Fix:** Correct the Testing bullet to empty-pointer and mark the trailing r1 Q&A entry superseded, as the r1 budget entry already is.

### [NIT:decision] `Spec.Version` has no stated disposition
**Demoted-from:** BLOCKING
**Section:** "The constructor surface — a config struct, validated, returning an error"
**Issue:** `shuttleengine.Spec` carries `Version` beside `Model`/`Effort` (`spec.go:55`), and loom's own spec builders already pass all three from one resolved modelspec (`internal/loomengine/discussion.go:46`, `plan.go:85`). `BouncerConfig` carries `Model` and `Effort` only, so a `version:` pin in a loom review role's model spec would be silently dropped by the Bouncer's judge spawn.
**Fix:** Say explicitly whether `Version` joins the told triple in `BouncerConfig` or is deliberately omitted (and why), the same way `Model`/`Effort` are pinned.

### [BLOCKING:design] Focus-file writer's overwrite behaviour unspecified
**Section:** "Focus-file synthesis" / "Seed call — spawns, with a mechanical fallback"
**Issue:** Both synthesis paths fire when the focus file is "absent **or does not parse**", and the Testing seed-failure scenario asserts `round-1-focus.md` afterwards holds `round: 1` with empty lists — i.e. the writer must destroy an existing unparseable agent-written file. That directly contradicts this discussion's own stale-outputs rationale ("Rejected: deleting stale outputs — destroys the partial artifact an operator would want").
**Fix:** Pin whether the unparseable file is archived via `archiveStaleOutputs` before the synthetic write, or overwritten, and state which and why.

### [NIT:consistency] Mode and path counts disagree with their own lists
**Section:** "Three modes, told apart by file existence only" / "Focus-file synthesis"
**Issue:** The heading says three modes while the section defines four (seed, re-bounce, judge, replay) and Scope calls it a "four-branch `Call`"; likewise the synthesis paragraph says "exactly three paths" over a list whose third item is "never anywhere else", while Scope says "exactly two paths".
**Fix:** Retitle to four modes and reword the synthesis count to two paths plus the negative clause.

### [NIT:scope] doc.go update inventory is stated twice, differently
**Section:** Scope vs "Technical context"
**Issue:** Scope names the outcome table, the cancellation rule, and a "pointer rule" section; Technical context names the outcome table, "told, never derived", the cancellation rule, and limitations — and `internal/shedadapters/doc.go` has no standalone pointer-rule section. Neither list mentions the package sentence's "the three shedengine.ShedProducer adapters", which becomes four.
**Fix:** Reconcile the two lists against doc.go's actual headings and add the opening-sentence count.

## Verdict

REQUEST_CHANGES
Four blocking items: rubric stamp leak, contradictory re-bounce pointer, undecided `Version`, unspecified overwrite.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
