MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] `Clean`'s reason strings leak into loom's report
**Section:** Scope / `healthy-typed-reason`
**Issue:** `fabricengine.Clean` returns `"host: %s"` / `"weft: %s"` (`hostclean.go:39,42`) and `loomengine/preflight.go:92-98` prints that reason verbatim under `CheckWorktreeClean` — the identical leak class as `drift.go:58`, but Scope names only "`drift.go`'s five reason strings" and no decision mentions `Clean`.
**Fix:** Decide `Clean`'s reason wording (and whether it too needs a typed cause) explicitly, or record why a two-sided dirty report is allowed to name both sides to an operator.

### [GAP] `host` is in the rule but not in the machine check
**Section:** `enforcement-test` vs `fabric-vocabulary-rule` / `documentation`
**Issue:** `enforcement-test` says the test fails a non-owner file "containing the token `weft` or `warp`", while `fabric-vocabulary-rule` polices three tokens and `documentation` says the CONSTRAINTS section states "(`weft`, `warp`, `host`)" under **Enforced by** — so whether `host` is machine-checked or review-only is undecided.
**Fix:** State in `enforcement-test` whether the predicate includes whole-word `host`, and if not, record `host` as a review obligation in the CONSTRAINTS wording.

### [GAP] `host` exception granularity is package-level, the need is per-occurrence
**Section:** `fabric-vocabulary-rule`
**Issue:** The exception mechanism is "an explicit exception in the owner map" (package-granular, `internal/shell` named), but non-fabric `host` sits in packages that also carry fabric-sense `host`: `websterengine/audit.go:156,159` ("the host OS's native sense", "regardless of host workdir") alongside `beginbatch.go:62` / `recordbatch.go:40` ("the host repo checkout"). A `websterengine` owner-map row would silently permit the real leak.
**Fix:** Decide the exception unit (per-line marker, per-symbol allowlist, or reword the OS-sense occurrences instead) rather than leaving it to the plan.

### [GAP] Template inventory misses `master-template.md:30` and one pinned test
**Section:** `templates-describe-one-repo` / Testing
**Issue:** The 22-row table covers `master-template.md:29` but not `:30` ("The `_lyx` path is your one sanctioned window **into it**"), whose antecedent is the weft worktree — rewriting `:29` leaves it dangling, and it is pinned at `websterengine/template_test.go:259`. Separately `template_test.go:412` pins `"Commit the card to the HOST repo"` (the `implementer-body.md:31` rewrite) but the update list names only `:246,257,318`.
**Fix:** Add the `:30` row with its replacement wording and extend the pinned-test list to `:259` and `:412`.

### [NOTE] "five templates" contradicts the seven-file inventory
**Section:** `templates-describe-one-repo` (opening sentence)
**Issue:** The decision opens "the five `go:embed`-ed prompt templates are rewritten", then the inventory immediately below says **7 files, 30 occurrences**, as does Scope.
**Fix:** Correct the opening sentence to seven.

## Verdict

GAPS_FOUND
Four gaps: `Clean`'s reasons, `host` enforcement scope and granularity, and template/test inventory misses.
MILL_REVIEW_END
