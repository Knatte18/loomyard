MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] tools/ Go files are neither owner nor cleanup target
**Section:** `fabric-vocabulary-rule` / `enforcement-test`
**Issue:** `tools/sandbox/main.go:28,33,36,48,67` contains `weftURL`, `fabricWeftURL` and the literal `https://github.com/Knatte18/lyx-test-weft`; the sibling walk in `internal/lyxcwd/enforcement_test.go` starts at repo root and scans every non-`_test.go` `.go` file, so `tools/` is in the scan and the URLs cannot be renamed.
**Fix:** State whether `tools/` is excluded from the walk or added to the owner set, and say so explicitly rather than leaving it to the plan writer.

### [GAP] configcli's operator-facing strings and help text unaddressed
**Section:** `diagnostics-say-fabric-detail-says-weft` / `consumer-renames`
**Issue:** `configcli/configcli.go:125,181` emit `"edited _lyx/config/%s.yaml but weft sync failed: %s"` and `:233` is a cobra `Long` reading `"…and syncs weft on success"`; the decision names only buildercli/webstercli/perchcli, and `configcli` appears only in the comment-only batch, yet these are string literals and help text, not comments. `configcli_test.go:187` asserts `"weft sync failed"`.
**Fix:** Add configcli to the diagnostics decision and to the rename/test-update list, and record the `Long` change against the CLI/Cobra Invariant's help-accuracy obligation.

### [GAP] CheckID string values left unspecified
**Section:** `consumer-renames`
**Issue:** The decision renames `CheckWeftPairing`/`CheckWeftSync` identifiers but is silent on their values, which are `"weft-pairing"`/`"weft-sync"` (`loomengine/report.go:24,26`) and are emitted in loom's report; `preflight.go:109`'s reason string `"weft not paired"` is likewise unlisted. Both are string literals in a non-owner file and would fail the new test as written.
**Fix:** Pin the new values (as was done for `ClassWeftReference`) and name any consumer of the old ones.

### [GAP] Agent prompt templates neither In nor Out
**Section:** Scope
**Issue:** Five embedded templates carry the tokens (`websterengine/master-template.md:136,140,143` — "a weft-reference", "A weft-sync error"; also builderengine/burlerengine templates), and `websterengine/template_test.go:246,257,318` pins literals such as `"NEVER run any git command against the weft"`. Renaming `ClassWeftReference`'s value to `"fabric-reference"` desynchronises the Master template's violation wording.
**Fix:** State explicitly that templates are out of scope (deliberately weft-aware per the Fabric Git Invariant) or list which template lines change.

### [GAP] CONSTRAINTS.md's Fabric Git Invariant goes stale
**Section:** `documentation`
**Issue:** The decision says the Fabric Git Invariant is untouched, but its **Enforced by** bullet (`CONSTRAINTS.md:170`) names `websterengine`'s `weftReferencePattern` as the machine check — a symbol this task deletes in favour of `fabricengine.RefScanner`.
**Fix:** Add that bullet's update to the documentation decision (vocabulary stays, symbol reference changes).

### [GAP] Ready(l)'s non-absence error path vs today's behaviour
**Section:** `ready-not-paired`
**Issue:** `preflight.go:105` treats *any* `os.Stat` error as `CheckWeftPairing` + `check3BlocksSeed`; the decision says `Ready` must not error on absence but does not say how preflight classifies a non-absence error, so "classification unchanged" is not actually pinned.
**Fix:** State the mapping for `Ready` returning a non-nil error, and add it to the `Ready` TDD case list.

### [NOTE] WEFT_SKIP_GIT / WEFT_SKIP_PUSH env names
**Section:** Scope
**Issue:** These operator-facing env vars name the weft (`fabricengine/fabric.go:98-103`, `spawn.go:33`); they live in the owner set so the rule does not force a change, but a visibility cleanup that renames a JSON key leaves them unmentioned.
**Fix:** Record them as explicitly Out with a one-line reason.

### [NOTE] "shares its repo-walk helper" is not accurate
**Section:** `enforcement-test`
**Issue:** `internal/lyxcwd/enforcement_test.go` has no extracted repo-walk helper — `TestEnforcement` and `TestEnforcement_GeometryLiterals` each inline their own `filepath.WalkDir` (lines 112 and 525).
**Fix:** Either say the new test inlines a third walk, or make extracting the helper an explicit part of the work.

## Verdict

GAPS_FOUND
Rule is sound; scope boundaries for tools/, configcli, templates and CheckID values are unpinned.
MILL_REVIEW_END
