MILL_REVIEW_BEGIN
# Review: Migrate planparser.Card to Edits/Uses fields

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:design] path-missing has no disposition for new targets
**Section:** `validator-checks` / Technical context (`validate.go` bullet)
**Issue:** The discussion says `createsUnion`/`movesTargetsUnion` "exist only to serve dropped checks and go away", but `validate.go:529-530` uses both inside `checkPathMissing`, which is a **kept/reworked** check; today `path-missing` also deliberately skips `CreatesFiles` entirely (`validate.go:533`). Reworking it to "applies only to path-shaped entries" with those helpers deleted makes every `Create`/`Rename`/`Move` destination, and every `Edit`/`Uses` entry satisfied by an earlier `Create` card, a spurious finding.
**Fix:** State whether `path-missing` is type-conditional (skip `Create` targets and rename/move destinations) and what replaces `createsUnion` now that `Creates:` is not a field — i.e. the union of every `Create`-type card's path-shaped targets.

### [BLOCKING:design] Enumeration grep does not produce the stated list
**Section:** Technical context, "Files carrying a literal old-format card body"
**Issue:** `internal/websterengine/template_test.go` is listed as grep output, but the file contains neither `**What:**` nor `format: 3` — its only old-format coupling is `card.Moves = []planparser.MovePair{...}` at line 759. A plan writer re-running the stated command (which the discussion instructs them to trust over the hand list) gets a strictly smaller set and drops that file.
**Fix:** Widen the enumeration to also match the Go-level model tokens (`MovePair`, `.Moves`, `ContextFiles`/`EditsFiles`/`CreatesFiles`/`DeletesFiles`, `DependsOn`, `.Intent`) and the bold old field labels, or state that the grep covers markdown carriers only and the Go carriers are found by the compiler.

### [BLOCKING:design] Rename card's targets are unmodelled
**Section:** `target-model` / `rename-grammar`
**Issue:** `target-model` says `Card` carries a flat `Targets []string`; `rename-grammar` says a `Rename` card's bullets parse into `[]MovePair` instead. Whether a `Rename` card also populates `Targets` (and with which side of the pair) is never stated — yet `card-field-overlap` ("entry in both the target list and `Uses:`"), the classifier-driven checks, and Wave 3's edge derivation all key off the target list.
**Fix:** State explicitly whether `Rename` pairs also project into `Targets` (and whether `Old`, `New`, or both), or that `Rename` is deliberately excluded from every target-list-based check.

### [BLOCKING:consistency] Q&A names a check absent from the table
**Section:** Q&A log ("Type-specific checks for `Prosa`, `Custom`, `Create`, `Delete`?")
**Issue:** The answer names `impact-summary-missing` (Edit/Delete), but `validator-checks` has no such ID — required-`ImpactSummary` presence is explicitly folded into the retargeted `card-missing-field`, and the four new checks are `card-type-missing`, `impact-summary-multiline`, `card-field-empty`, `prosa-symbol-target`. Implementing the Q&A answer yields 16 checks and contradicts the pinned banner figure.
**Fix:** Delete `impact-summary-missing` from the Q&A answer and point it at `card-missing-field`, matching the table.

### [NIT:design] Classifier rule undefined for dot-less entries
**Section:** `shape-classifier`
**Issue:** Rule 2 keys on "the segment after the final `.`"; for an entry containing no `.` at all (`Lookup`, `Makefile`, a bare directory name) the rule has no defined value, and rule 3's default silently decides it.
**Fix:** State the no-dot case explicitly ("an entry with no `.` falls to rule 3 → symbol") so the table test in Testing pins a decided behaviour rather than an implementation accident.

### [NIT:decision] Design doc's own "14 checks" line has no disposition
**Section:** Scope ("In", `manifest/designs/plan-card-format.md`)
**Issue:** `plan-card-format.md:84` carries the same stale "existing 14 validator checks" figure; the scope entry covers only the status banner and the three open items, and `validator-checks` notes the miscount without saying whether that line is edited.
**Fix:** Say whether the open-item line is corrected to 15 as it is struck through, or left verbatim as a historical record.

## Verdict

REQUEST_CHANGES
Four unresolved modelling/consistency gaps around path-missing, Rename targets, and the file enumeration.
MILL_REVIEW_END
