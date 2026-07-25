MILL_REVIEW_BEGIN
# Review: loom: Planner producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-25
```

## Findings

### [BLOCKING] Stray `</content>` tag embedded at end of plan-template.md
**Location:** `internal/loomengine/plan-template.md:120`
**Issue:** The file's final line is a literal `</content>` tag with no matching open tag anywhere in the file — an evident copy/paste artifact from a tool-output wrapper, not part of the specified prose (card 3's spec ends the file at the "Never use `AskUserQuestion`" guard, and the sibling `discussion-template.md` ends cleanly with no such tag). Because the file is `//go:embed`-ed verbatim and passed through `stencil.Fill` unmodified (it is plain prose, not a `{{.X}}` marker), this string is embedded byte-for-byte into every Plan producer prompt shipped to the agent.
**Fix:** Delete the trailing `</content>` line so the template ends cleanly at "…never block on a dialog."

### [NIT] ConfigTemplate() doc comment still names only the discussion knobs
**Location:** `internal/loomengine/configtemplate.go:14-19`
**Issue:** `ConfigTemplate`'s doc comment reads "the discussion role model-spec and the discussion_timeout_min knob the discussion producer consults," but `template.yaml` (edited by card 2) now also carries `plan`/`plan_timeout_min`; the comment under-describes the asset it documents.
**Fix:** Extend the doc comment to also name the `plan`/`plan_timeout_min` knobs, mirroring the refresh already done to `config.go`'s `Config`/`LoadConfig` comments.

## Verdict

REQUEST_CHANGES
Stray `</content>` artifact leaked into the shipped plan-template.md prompt asset; everything else aligns with the plan.
MILL_REVIEW_END
