MILL_REVIEW_BEGIN
# Review: Seeded driver choice: ly-drive strand as the child's driver

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, as identified by the harness; no independent way to confirm the exact version
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:consistency] Config section says "two new keys", decision says one
**Section:** Technical context → `internal/loomengine/config.go` vs `### driver-is-a-loom-config-key`
**Issue:** The decision states `Config` gains **exactly one** key (`driver`, explicitly no `driver_timeout_min`), but the technical-context paragraph twice says "the two new keys" ("the pattern the two new keys follow line for line", "the template gains the two keys with their comments") — a plan writer reading only that paragraph adds a second key the decision rejects.
**Fix:** Restate the technical-context paragraph in the singular, so the key count agrees with the decision and with the Testing section's single-key coverage.

### [BLOCKING:decision] Driver strand's `Display` and `Parent` have no disposition
**Section:** `### autonomous-posture-is-shuttle-s-default` / Technical context → `internal/reedengine`
**Issue:** The technical context raises that `Display.Anchor` must be `AnchorBelowParent` or `AnchorHidden` (`AnchorOwnWindow` refused) but never says which the driver takes, nor what `Parent`/`Role`/`Round` are — while the same discussion names `operatorStrandAddSpec`'s comment as the standard that every non-default spec field is pinned with a reason, and the spec-composition test list omits these fields too. A hidden-vs-visible long-lived driver pane is an operator-visible choice, not a detail.
**Fix:** State the driver spec's `Display` (anchor, focus, shrink), `Parent`, and `Role`/`Round` values with the one-line reason each, and add them to the spec-composition assertions.

### [BLOCKING:design] No named site owns "which recipes may be seeded `llm`"
**Section:** Scope (In, bullet 7) and `### driver-branch-lives-in-the-bootstrap-verb` (generalization)
**Issue:** `lyx shed seed --driver llm` is accepted only "when the seeded recipe is `loom`", and the batten `--driver` refusal is rewritten to name a missing bootstrap verb — two independent encodings of the same fact ("loom is the only recipe with a bootstrap verb"), with no statement of where that predicate is declared or which package owns it. The next recipe to grow a bootstrap verb must find both.
**Fix:** Name the single declarer of the recipe→bootstrap-verb capability (e.g. one accessor in `shedrun`/the recipe registry) that both the seed-flag validator and the batten refusal consult.

### [NIT:design] Uniqueness seam for the report suffix is unnamed
**Section:** `### driver-report-is-the-run-s-output-file` / Testing (spec composition)
**Issue:** The test requires two report paths to differ "under a frozen clock", which only works if the 4-hex component comes from an injectable source, but the discussion never says the composer takes a randomness (or clock) seam or where it lives.
**Fix:** State that the report-path composer is a pure function in `bootstrap.go` taking a clock and a random source as injected seams.

## Verdict

REQUEST_CHANGES
Key-count contradiction, unpinned driver strand display fields, and an unowned recipe-capability predicate.
MILL_REVIEW_END
