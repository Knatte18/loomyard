MILL_REVIEW_BEGIN
# Review: Reed and Fabric as standalone modules: public API design

```yaml
duration_s: 145.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude, Opus-class (reported model id claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [NIT:consistency] Q&A log carries the retracted count of 15
**Demoted-from:** BLOCKING
**Section:** Q&A log, "extract-or-not verdict for Reed" **Issue:** The answer says "15 identifiers, 9 importers", but `## Technical context` states the number is **13** and names 15 as the first draft's error from comment-prose contamination (`burlerengine/doc.go:205`, `reedcli/add.go:1,4`, `reedcli/attach.go:52`). **Fix:** Correct the Q&A entry to 13 so the doc writer cannot transcribe the retracted figure into the very doc whose value rests on its measurements.

### [NIT:consistency] "2–3k-loc kernel" has no producing command
**Demoted-from:** BLOCKING
**Section:** `fabric-verdict-do-not-extract-and-say-why-precisely`; Q&A log **Issue:** The ~2–3k-loc generic-kernel size is the load-bearing number in the Fabric verdict, yet it is a range with no stated method, contradicting the discovered constraint that "every number the doc states must be reproducible by a named command" — every other figure (14,610, 74, 33, 1,614) names one. **Fix:** Either name the file set and `wc -l` command the range is derived from, or state it in the doc explicitly as an unmeasured estimate, not a measurement.

### [NIT:decision] Design doc with no owning roadmap entry
**Demoted-from:** BLOCKING
**Section:** `## Scope` (Out: any edit to `manifest/roadmap.md`) vs `doc-belongs-in-manifest-designs-despite-shipped-modules` **Issue:** `manifest/roadmap.md`'s own Maintenance note scopes `designs/<name>.md` to the detail of a Planned/Someday **entry** and says the roadmap "is the single home for everything not scheduled … add new speculative ideas directly to Someday"; this task lands a designs doc with no entry pointing at it and no recorded deletion trigger. **Fix:** State a disposition for the orphan — either why an entry-less designs doc is permitted here, or that a follow-up decision adds the Someday entry.

### [BLOCKING:design] Fabric split recommendation has no stated depth
**Section:** `fabric-recommendation-is-an-in-repo-seam-not-a-repo-boundary` **Issue:** The recommendation is "a named generic kernel and a named hub-layout half" but neither name is given, and it is never said whether the split is sub-packages, file grouping inside `package fabricengine`, or documentation-level sectioning — unlike creel, which gets an explicit `creel-is-sketched-not-specified` boundary. **Fix:** Decide the recommendation's depth (name the two halves, or state that naming and mechanism are deliberately left to the follow-up item).

### [NIT:consistency] "Active churn" rests on three unscheduled items
**Section:** `reed-verdict-defer-and-gate-on-a-second-consumer` **Issue:** All three cited items (`reed: cross-worktree columns`, `reed: own-window strand anchoring`, `reed: daemon Slack relay`) sit in roadmap.md's **Someday** section — committed but unscheduled — which is weaker evidence of churn than the phrasing implies. **Fix:** Say the items are Someday, so the argument stands on the second-consumer gate rather than on imminent change.

### [NIT:consistency] Prep item (c)'s success condition is unachievable as worded
**Section:** `reed-preparation-work-is-worth-doing-regardless` (c) **Issue:** "no consumer holds `*reedengine.Engine` concretely except Reed's own CLI" cannot hold: `burlercli/wiring.go:99,158` and `shuttlecli/cli.go:86,94` call `reedengine.New` and so hold the concrete value before handing it to `NewRunner`. **Fix:** Reword to "no consumer *retains* the concrete `*Engine` as a field" — which is the true statement about `loomcli/cli.go:44`.

### [NIT:scope] quarry cited by a machine-local absolute path
**Section:** `### The quarry precedent — measured` **Issue:** `/home/knatte/Code/quarry/wts/quarry` is not reachable by any other reader, so the layout and façade claims are the one block of the doc nobody else can re-verify. **Fix:** Cite the module path `github.com/Knatte18/quarry` as the reference and mark the layout claims as observed off a local checkout.

## Verdict

REQUEST_CHANGES
One retracted number, one unmethodded estimate, an orphan doc, and an undefined Fabric split.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
