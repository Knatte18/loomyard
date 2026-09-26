MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection

```yaml
duration_s: 109.3
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [NIT:consistency] ly-drive SKILL.md is not "true as written"
**Demoted-from:** BLOCKING
**Section:** Scope → Out (step-driven runs bullet); Technical context.
**Issue:** `plugins/ly/skills/ly-drive/SKILL.md` § The loop derives its 40-step cap from "Loom's list is fourteen rows" (thirty-five steps) and repeats "loom's fourteen rows", which the new row makes false; the Out list says the file stays true and is not edited.
**Fix:** Move SKILL.md's row-count/cap arithmetic into scope (or reword it to not pin a count), and drop the "not edited" claim.

### [BLOCKING:design] Row-count doc sites enumerated by hand, and incompletely
**Section:** Technical context (recipe header, `loomshed.go` "fourteen").
**Issue:** The same stale-count text also lives in `internal/loomshed/doc.go`, `internal/loomshed/interruptpolicy.go` (twice), and user-visible `lyx loom` help in `internal/loomcli/cli.go` ("walks fourteen producer rows … and finally Publish and Finalize"), none listed.
**Fix:** State the enumeration method (grep for the row count and for "Finalize" as the final row across code comments, help text, skills and docs) instead of a hand list.

### [NIT:scope] Batten's 12-hour watch window now also covers reflection
**Section:** Scope → Out (batten bullet); Constraints.
**Issue:** Run-Shed's `max_bounces: 1440` × `poll_interval_s: 30` budget now spans up to `friction_timeout_min` more child runtime before `done`, an unstated implicit constraint.
**Fix:** Note the budget consumption and that no batten change is needed.

### [NIT:consistency] shed-recipe-spec.md does not mention reflection
**Section:** Technical context, last bullet.
**Issue:** `contracts/specs/shed-recipe-spec.md` contains no reflection or friction text, so "mentions reflection" is a false premise.
**Fix:** Drop the bullet's spec clause.

### [NIT:consistency] "non-empty friction directory check" misdescribes the gate
**Section:** Blocked outcome keeps today's PostRun reflection; Testing (loomcli bullet).
**Issue:** `shouldReflectFriction` checks only that the directory path string is non-empty (Tier 2 on), not that the directory has notes.
**Fix:** Reword as "Tier 2 enabled (non-empty `frictionDir`)".

## Verdict

REQUEST_CHANGES
The mechanism is sound, but the scope rules SKILL.md unedited while the new row falsifies its row-count text.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
