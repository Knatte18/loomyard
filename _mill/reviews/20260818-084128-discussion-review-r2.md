MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
duration_s: 158.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] Render signatures contradict the two-roots ruling
**Demoted-from:** BLOCKING
**Section:** §render-takes-told-strings vs §worktree-root-token-value-preserved
**Issue:** The first pins the new signature at `(anchorRoot, stencilsDir string)` and rejects a struct because "they need two strings", while the second requires the renderers to take *both* roots; source shows `RenderForkPrompt` uses the `Location` only for `worktree_root` (`render.go:149`) so it needs `worktreeRoot`, not `anchorRoot`, `RenderRecoveryPrompt` needs all three (`:173,174,183`), and `RenderMasterPrompt` needs only `anchorRoot`+`stencilsDir` (`:236,237`).
**Fix:** State the three signatures explicitly per function, and correct the rejected-alternative rationale that assumes two strings for all of them.

### [BLOCKING:design] Audit workdir has no told-field assignment
**Section:** §webster-geometry-struct / §the-two-roots
**Issue:** `deps.Layout.AnchorPath()` is the audit workdir at `recordbatch.go:102,129,133` and `runlevel.go:724,728`, where it resolves transcript-relative write paths (`audit.go:129 resolveWritePath`) — i.e. it must be the pane's actual cwd; the `PaneCwd` Decision says so in prose but `Geometry`'s eight fields and the two-roots table contain no row for it, so a mechanical conversion to `Geom.AnchorRoot` silently mis-resolves every relative write path in standalone.
**Fix:** Add an explicit row deciding which told value the audit workdir (and `AuditForksIncremental`'s workdir) takes in each mode, and pin it in the standalone test list.

### [BLOCKING:design] `(wired=false, hubPresent=true)` is not always a broken hub
**Section:** §mode-selection-and-the-extracted-wiring-function truth table
**Issue:** That row is declared a hard error naming `lyx fabric reconcile`, but `preflight.Wired` is per-worktree (`fabricengine.Ready` stats the weft sibling, `ready.go:18`) and `predicates.go:22-24` documents `<hub>/_board`, an unpaired sibling, and a pair-removed worktree as *healthy* situations producing exactly this combination — all of which run `lyx webster validate`/`status` today.
**Fix:** Decide and record how those healthy-but-unwired locations are treated (hub mode, standalone, or refusal with a different message), rather than folding them into "broken hub".

### [NIT:decision] `webstercli.layout` and `validate.go` have no standalone disposition
**Demoted-from:** BLOCKING
**Section:** §Scope (In, `internal/webstercli`) / Technical context
**Issue:** Only the four verb Deps are converted, yet `validate.go:73` calls `planparser.Validate(plan, c.layout.AnchorPath())` and the `websterCLI.layout` field survives; in standalone there is no `Location`, so the field is nil and `validate` nil-derefs — and `Validate`'s second argument resolves move source/target files (`validate.go:64-65`), so it is a worktree root, not an anchor root.
**Fix:** State what `c.layout` holds (or that it is removed) and which told value `validate` passes in each mode.

### [NIT:design] Standalone degraded-escalation channel unspecified
**Section:** §standalone-has-no-fabric
**Issue:** "Report the failing suite with an explicit warning naming standalone mode" names no channel (logger, output envelope, or `summary.md`) and does not say whether `RecordIntegrationFailure`/`AppendIntegrationFailure` still run — `BisectAndEscalate` already has an "unknown" SHA/card fallback (`integration.go:190-200`) that fits exactly this case.
**Fix:** Name the channel and say whether state.json/summary.md still record the failure with the `unknown` localization.

## Verdict

REQUEST_CHANGES
Four decisions are missing or self-contradictory; the rest of the discussion checks out against source.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
