# Review: `_mill/discussion.md`

## Method

Every code citation in the discussion was checked directly against the worktree source, not taken on faith: `internal/pattern/pattern.go` (constant/function line numbers), `internal/pattern/leaf_enforcement_test.go` (all three allowlist sites), `internal/stencil/stencil.go` (`Fill`, `FillOptional`, `StripLeadingComment`), `internal/stencilstore/reconcile.go` and `stencilstore.go` (`Read`, `ApplyStamp`, `BodyHash`), both `internal/websterengine/render.go` call sites, `internal/burlerengine/engine.go:103`, `internal/loomengine/plan.go:70`, `.gitattributes`, `internal/pattern/doc.go:53-54`, `internal/stencilstore/validate.go:52,56`, `CONSTRAINTS.md`'s Pattern Leaf and Stencil Ownership Invariants, `internal/pattern/pattern_test.go`'s existing test list, and `internal/loomengine/discussion_test.go:128-146`.

**Result: zero citation errors.** Every line number, function name, and quoted string matched exactly.

## Verdict: sound, nothing should block Plan

The discussion does not just restate the pre-written `manifest/designs/pattern-directive-stencils.md` — it corrects three real defects in it:

1. **Banner-strip finding.** Confirmed genuine. `stencilstore.Read` is a bare `os.ReadFile` (`reconcile.go:28-34`) and strips nothing. `Reconcile` seeds every file through `ApplyStamp` (`reconcile.go:133`, `stencilstore.go:103`), which inserts a `lyx-stencil:` banner line. Only `stencil.Fill`/`FillOptional` strip that banner, and only because their first act is `stripLeadingComment` (`stencil.go:27`). `Directive`'s return value is a plain string placed into a `values` map — it is never itself passed to `Fill`, so nothing would strip it without the discussion's explicit `stencil.StripLeadingComment` call. Without this fix, a real hub would leak the stamp comment into every producer prompt that carries `pattern_directive`. The design doc does not mention this at all. Correct and necessary catch.

2. **"Plumbing-free" claim is false for 2 of 4 call sites.** Confirmed by direct read. Design doc step 4 claims "`websterengine`'s functions already take [`stencilsDir`] as a parameter" — they don't. `RenderRecoveryPrompt` and `RenderMasterPrompt` (`render.go:179`, `:238`) derive it internally via `fabricengine.StencilsDir(l.HubPath)`, and both embed the `pattern.Directive(...)` call inline inside a map literal. Once `Directive` returns `(string, error)`, Go can no longer accept that as an inline map value — both sites genuinely need the call hoisted out with its own error check. `burlerengine/engine.go:103` and `loomengine/plan.go:70` are correctly identified as the two simple-assignment sites.

3. **Fail-loud override of the design doc's fail-silent spec.** Design doc step 3 specifies `logger.Warn` + return `""` on a read failure. The discussion overrides this to `(string, error)` with propagation, backed by three independent, verified precedents: `stencilstore.Read`'s own doc comment ("a missing file is reported as an error... per the missing-board-is-a-hard-error Shared Decision"), every other stencil consumer in the repo already hard-erroring on read failure, and `internal/pattern/doc.go`'s own stated posture that silently stripping an active PATTERN's constraints is worse than surfacing the failure. Well-grounded, not just a stylistic preference — and cheap, since all four call sites already return an error from their enclosing function.

## Non-findings

Checked and confirmed *not* to be problems, despite looking plausible at first glance:

- The leaf-invariant amendment (adding `internal/stencilstore` + `internal/stencil` to `pattern`'s allowlist) does not introduce an import cycle — `internal/stencil` imports no internal package at all, and `internal/stencilstore`'s closure (`stencil` + `logger` → `lyxcwd`/`lyxdirs`/`proc`) never reaches back to `pattern`.
- `internal/burlerengine`'s tests are correctly identified as unaffected — `template_test.go:142`'s `pattern_directive` value is a hardcoded placeholder, never a `Directive` call.
- Test 4 ("verbatim move") is honestly flagged by the discussion itself as a weak, near-tautological guard once the source constants are deleted — this is the document catching its own test's limitation rather than overclaiming coverage.

## One nitpick (non-blocking)

Lines 185-190 describe the `burlerengine`/`loomengine` call-site edits as "a one-line change each." Once `Directive` returns `(string, error)`, those simple assignments need an added error-check block, not literally one line. The substance (mechanical, trivial, no design risk) is correct — only the phrasing undersells the diff size. Worth a one-word fix ("trivial" instead of "one-line") so `mill-plan` doesn't under-scope the batch.

## Overall

High-quality discussion. All citations verified exact. The three corrections to the pre-written design doc are substantive and correct, especially the banner-strip finding, which is a real bug the design doc would have shipped. Nothing here should block Plan.
