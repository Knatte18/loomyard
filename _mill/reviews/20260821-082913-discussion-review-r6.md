MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry

```yaml
duration_s: 255.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Env validation rule does not cover `WebsterDeps`
**Section:** "Every entry validates the `Env` fields it reads" **Issue:** The rule enumerates only two shapes — "a path root must be non-empty and absolute, an injected seam must be non-nil" — but `Env.WebsterDeps` is a `websterengine.RunDeps` **value struct** (`runlevel.go:104-131`, carrying `Starter`, `Reed`, `Engine`, `Geom`, `RefMatcher`), so neither clause applies to it; `loomshed.newWebsterProducer` (`webster.go:41-43`) validates nothing, so a zero `WebsterDeps` constructs cleanly and fails at every `Call` — exactly the failure mode the decision exists to prevent for `SingleLLM`. **Fix:** State what the `Webster` entry checks on `Env.WebsterDeps` (which inner seams, or explicitly nothing and why), and reconcile it with Testing item 10's "nil seam fails at construction" for that entry; note `Env.Landing` is already covered by `NewPublish`/`NewFinalize`'s own nil-closure checks, so `WebsterDeps` is the only unguarded value-struct field.

### [NIT:consistency] "Every producer-hosting package carries one" is false
**Section:** Technical context → "Import-allowlist tests" **Issue:** `internal/shedadapters` and `internal/preflightshed` — both producer-hosting — carry no `seam_enforcement_test.go`/`leaf_enforcement_test.go`; only `loomshed` and `landingshed` do, matching CONSTRAINTS.md's machine-enforced list. **Fix:** Reword to "the two producer-hosting packages already on the Told-Geometry machine-enforced list carry one".

### [NIT:consistency] `landingshed.Deps` field count is wrong
**Section:** "`landingshed.Deps` rides `Env` as a whole-struct passthrough" **Issue:** The rationale says "already carries eleven fields"; `internal/landingshed/deps.go:31-91` declares fourteen. **Fix:** Correct the count, or drop the number and keep the "already told wholesale" argument, which is what actually carries the decision.

### [NIT:scope] No `doc.go` for the new package in the work inventory
**Section:** Scope → In **Issue:** Every sibling in this family ships one (`loomshed/doc.go`, `landingshed/doc.go`, `preflightshed/doc.go`, `shedadapters/doc.go`, `batcher/doc.go`), and the Documentation Lifecycle is cited in Constraints, but `internal/shedrecipe/doc.go` is not listed among the files this task creates. **Fix:** Add it to the In list alongside the registry table file and `seam_enforcement_test.go`.

## Verdict

REQUEST_CHANGES
One unspecified `Env` validation case; everything else verified accurate against source.
MILL_REVIEW_END
