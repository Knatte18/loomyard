MILL_REVIEW_BEGIN
# Review: modelspec: bare-effort shorthand and version-to-v rename

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model; exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:consistency] Check order (unknown-key before duplicate) only implied
**Section:** § Decisions (duplicate-key), Note at line 63; § Technical context line 99
**Issue:** `parse.go:146-151` runs the duplicate check *before* the unknown-key check; line 63's claim that `opus[v=4.8,version=4.8]` "fails as an unknown key before any duplicate check" requires that order to flip, while line 99 says the `key=value` branch "keeps every one of those checks" without saying the order changes.
**Fix:** State outright that canonicalization/unknown-key rejection now precedes the duplicate check, for both branches.

### [NIT:consistency] "four named files" contradicts the five-file Scope list
**Section:** § Technical context ("No programmatic spec construction exists"), Q&A line 167
**Issue:** Scope In names five files (`parse.go`, `modelspec.go`, `parse_test.go`, `contracts/specs/llm-model-spec.md`, `docs/overview.md`), but two later passages pin a "four-file blast radius".
**Fix:** Say five, or name which four are meant.

### [NIT:scope] Load-time validators are the real operator-facing break surface
**Section:** § Decisions (`version=` is removed…)
**Issue:** The migration hint is justified against "an operator's `webster.yaml`", but three config validators call `modelspec.Parse` at load time and will hard-fail on a stale `version=` bracket: `internal/websterengine/config.go:71`, `internal/landingshed/config.go:52`, `internal/loomengine/config.go:255,259,263,270` (loom's four role keys).
**Fix:** Name these three load-time sites as where the hint surfaces, so the plan does not treat the break as spawn-time only.

### [NIT:scope] `internal/modelspec/template.yaml` disposition unstated
**Section:** § Scope
**Issue:** Out says "No `.yaml` outside `internal/modelspec` is touched", implying one inside might be, yet the In list omits `internal/modelspec/template.yaml` — whose header comment is the operator-facing grammar pointer (at the acknowledged-dangling `docs/reference/model-spec.md`).
**Fix:** State explicitly that `template.yaml` is unchanged (its `defaults: effort:` keys are canonical and unaffected).

### [NIT:scope] Duplicate-error message format has no test assertion
**Section:** § Testing (`TestParse_Rejects` new cases)
**Issue:** The duplicate decision specifies a precise message shape — canonical key plus both as-written segments — but the two new cases assert only the substring `duplicate param key`, so the deliberately-designed part is untested; the `v` migration hint by contrast does get a second assertion.
**Fix:** Add a segment-quoting assertion to `opus[high,effort=max]`, or say the format is deliberately unpinned.

## Verdict

APPROVE
Decisions complete and source-verified; five NITs on wording, ordering, and test pinning.
MILL_REVIEW_END
