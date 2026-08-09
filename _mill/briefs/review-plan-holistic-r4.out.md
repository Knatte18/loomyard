MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5 / Claude Agent SDK)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:scope] rawgitmutation_test.go card leaves three var-doc comments naming builderengine
**Location:** batch 1, card 1 (`cmd/lyx/rawgitmutation_test.go`)
**Issue:** Requirements say to rewrite only "the file-header and function doc comments," but `rawGitMutationScanPackages` (doc: "exactly the two packages... internal/websterengine and internal/builderengine"), `rawGitMutationAllowlist` (doc: "the two grandfathered read-only exemptions"), and `rawGitMutationMinScannedFiles` (doc: "this guard's two-package walk... across both packages combined") also name `builderengine`/"two packages" and are left unaddressed — verbatim "builderengine" text would fail card 18's pattern-1 package-name zero-hit grep, and this file is not on the exclusion list.
**Fix:** Extend card 1's requirement for this file to also rewrite these three var doc comments to name websterengine alone / a one-package walk.

### [BLOCKING:scope] sync_integration_test.go card misses a doc comment naming BUILDER
**Location:** batch 1, card 1 (`internal/webstercli/sync_integration_test.go`)
**Issue:** Requirements enumerate "the file-header comment, the newWarpWeftPairAt doc comment, and both in-body comments" to rewrite, but `TestFabricSync_CommitsAtEveryRelPathDepth`'s own doc comment ("a webster commit must hold back BUILDER's pause flag too...") is a fourth, distinct comment block containing the bare word "BUILDER" that the enumerated list does not cover. Card 6 explicitly forbids re-editing this file ("must not be edited again here"), so this site is never fixed by any later card. It is not among card 18's enumerated bare-word exclusion phrases, so it fails the acceptance sweep's pattern-6 grep.
**Fix:** Extend card 1's requirement to also rewrite `TestFabricSync_CommitsAtEveryRelPathDepth`'s doc comment to drop "BUILDER" (e.g. "hold back the sibling round-loop module's pause flag too").

## Verdict

REQUEST_CHANGES
Two BLOCKING scope gaps in batch 1 card 1's Requirements leave literal "builder"/"BUILDER" text that fails batch 5's own acceptance grep.
MILL_REVIEW_END
