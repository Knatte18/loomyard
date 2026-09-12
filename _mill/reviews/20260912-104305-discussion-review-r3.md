MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies

```yaml
duration_s: 230.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus (Anthropic), opus-class reasoning model
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] history-length discriminator defeats the marker
**Section:** `### one-issue-per-anomaly` **Issue:** The premise "the same halt re-observed on resume yields byte-identical values" is false for triggers 2 and 3: `run.go`'s blocked arms call `appendHistory()` before persisting, and `StateBlocked` does not short-circuit (`run.go:95-97`), so every resume of an unfixed escalation re-calls the producer, appends another `stuck` entry, and lands at a longer `len(history)` — a new title, a new issue, every single resume. Only the empty-outcome hard-failure case (`appendHistory`'s skip) stays stable. **Fix:** Decide a discriminator that is stable across re-observations of one unresolved halt (e.g. the entry-time history length, or `episodeStuckCount` of the halting producer) and state it, or state explicitly that repeated resumes re-file.

### [BLOCKING:design] recurring-finding title has no segment discriminator
**Section:** `### one-issue-per-anomaly`, trigger 5 **Issue:** The title is `recurring-finding — <slug> — <ledger-key>`, but the key is an LLM-authored "short-stable-finding-identity" (`contracts/stencils/bouncer/bouncer-template-judge.md:72`) scoped to one segment's `run_subdir`; nothing makes it unique across `discussion`/`plan`/`webster`, so two unrelated findings with the same key collapse into one issue and the second is permanently suppressed by the marker. **Fix:** Add the segment/ledger identity (run_subdir or the Bouncer row name from the history entry) to the trigger-5 title and say so.

### [BLOCKING:design] carry-forward duplicates one key across ledger files
**Section:** `### ledger-discovery` + `### trigger-list-and-thresholds` **Issue:** The judge prompt's lossless carry-forward rule means an `open` entry reappears in every later round's ledger, and discovery reads *every* distinct accepted path in `history[]` — so one recurring finding yields one anomaly per ledger file from round 3 on, all with identical titles; whether identical titles are collapsed before filing (and when the marker is written relative to the filing loop) is never stated. **Fix:** State the per-pass de-duplication rule (collapse by title before filing, or read only the highest-round ledger per run directory) and add a test case for it.

### [BLOCKING:design] mandatory drive-wiring tests are not Tier-1 reachable
**Section:** `## Testing`, "`internal/loomcli` — the drive wiring" **Issue:** Two of the three "mandatory" cases (clean `Result`, non-`ErrShedBusy` error) require control over `shed.Run`'s return, but `drive.go` constructs the Shed inline via `loomrecipe.New` inside the cobra closure with no seam, and reaching that line needs `reed.Up()` + `fabricengine.Open`; today's Tier-1 test (`cli_test.go:143`) can only reach drive's early refusal. Tier 1 is declared the only tier, so the regression test for the reachability defect is currently unwritable. **Fix:** Decide and scope the seam (e.g. extract the detect-and-file step into a testable function, or a package-level shed-constructor var) or move those two cases to a tagged tier.

### [NIT:design] crash-resume trigger over- and under-matches
**Section:** `### trigger-list-and-thresholds`, trigger 1 **Issue:** `state: running` + non-empty history does not only mean "a driver died": `Run`'s step-1/step-2 hard errors (unreadable status, invalid state, `current_producer` names no producer — reachable on a recipe rename, which the recipe header warns about) return without persisting and leave `running` behind; conversely a crash during the *first* producer leaves `running` with empty history and is invisible. **Fix:** Record both, at least as accepted labelling imprecision, rather than asserting the signal means a died driver.

### [NIT:scope] predicate rejection cases omit the Burler neighbours
**Section:** `## Testing`, shedadapters predicate **Issue:** The reject list names `verdictPath`, `focusPath`, `decision-record.md`, a plan dir and the empty string, but not `roundReviewPath`/`round-%d-fixer-report.md` (`internal/shedadapters/burler.go:133,139`) — which live in the same `runDir` and are published into `history[].output` (`burler.go:299,420`), making them the nearest false-accept risk. **Fix:** Add both Burler filenames to the predicate's rejection cases, asserted against the real helpers.

### [NIT:design] no owner named for title/body rendering
**Section:** `## Scope` (in, bullet 6) **Issue:** "A filing path that renders each anomaly into a deterministic title plus a body" names no package; the testing section implies titles come from `loomengine` (title cases sit in the detector table) while body rendering is unassigned. **Fix:** State which package owns title and body rendering.

## Verdict

REQUEST_CHANGES
Dedupe-key stability, trigger-5 identity, and the drive-wiring test plan need resolving.
MILL_REVIEW_END
