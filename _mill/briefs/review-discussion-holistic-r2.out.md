MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: /home/knatte/Code/loomyard/wts/treadle/_mill/discussion.md
date: 2026-07-26
```

## Findings

### [GAP] "perchcli compiles untouched" contradicts the migration
**Section:** Scope (In, pkg-layout bullet) vs Technical context (perchcli)
**Issue:** Scope asserts "perchcli compiles untouched," but the model-spec migration must change perchcli source: `run.go`'s profile struct yaml tags (`judge-model`/`judge-effort`/`model`/`effort` at lines 65-68), the parse, the embedded help text (lines 256-259) and `run_test.go` fixtures — tech context even says "the file-schema migration lands here."
**Fix:** Reword to distinguish "perchcli's use of the perchengine Go API is unchanged (still compiles against it)" from "perchcli's profile-parsing + help-text source DOES change for the model-spec migration," so a plan writer scopes those edits.

### [NOTE] Treadle targeting capability-flag location unspecified
**Section:** Decisions/pre-round-targeting
**Issue:** Targeting is enabled by "A Treadle profile capability flag," but Treadle owns no config and receives only resolved data/settings structs — the field's home in that resolved-data API is never named, and "profile" overloads perch's byte-identical `Profile`.
**Fix:** Name where the flag lives in Treadle's resolved settings input (not perch's `Profile`) and confirm perch simply leaves it unset.

### [NOTE] configreg template update stated non-committally
**Section:** Technical context (template.yaml)
**Issue:** "`configreg` registration may need the updated template" is soft, yet removing the `judge_effort` line from `perchengine/template.yaml` deterministically requires the strict-template/configreg update if perch.yaml stays configreg-validated.
**Fix:** State the template edit as required, not conditional.

## Verdict

GAPS_FOUND
One scope contradiction on perchcli's source-vs-API stability needs resolving before plan writing.
MILL_REVIEW_END
