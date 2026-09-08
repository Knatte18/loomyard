# `loom` fixer report — round 8 (sonnet-xhigh-r8)

Job 2: implementing and verifying every finding from `_mill/loom-review-sonnet-xhigh-r8.md`.
One commit per fix, filled in as each lands green — not reconstructed from memory at the end.

## Table

| ID | Severity | Status | Commit | Change | Test |
|---|---|---|---|---|---|
| SF-1 | BLOCKING | FIXED | (pending) | `internal/shuttleengine/claudeengine/startup.go` — `startupGateNeedles` is now built from `gateAcceptNeedles` plus the prose-only `filesInThisFolderNeedle`, instead of an independently hand-typed list, closing the "Yes, proceed" gap and making the two lists structurally unable to diverge again | New fixture `TestStartup_Classification/trust_prompt_older_wording_no_prose_in_capture` (option list + footer only, no prose) — fails pre-fix (`StartupReady`), passes post-fix (`StartupTrustPrompt`) |

## Notes

- SF-1: confirmed pre-fix via a throwaway test during the review phase (removed before Job 2 began,
  per the sequencing rule); the permanent regression test added in Job 2 reproduces the same failure
  shape as a named, committed fixture.
