MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:consistency] New-test count stated three different ways
**Section:** §Scope / §Testing / §Q&A log
**Issue:** §Scope says "plus three new tests", §Testing says "Add four new tests" and lists four, and the Q&A answer says "Three: lazy-read, missing-stencil error, byte-for-byte equality" — omitting the banner-strip test (§Testing item 3), the discussion's own named regression guard for the one correctness bug it says would otherwise ship.
**Fix:** Make all three enumerations name the same four tests, with the banner-strip test present in each.

### [BLOCKING:consistency] Q&A restates test 4 in the exact terms §Testing forbids
**Section:** §Q&A log ("Which new tests beyond migrating the existing ones?")
**Issue:** The Q&A calls test 4 "byte-for-byte equality with the `stencils` embedded defaults"; §Testing item 4 explicitly requires it be stated as equality with `stencil.StripLeadingComment` of the default, "not as whole-file byte equality" — verified: `Reconcile`→`writeStamped`→`ApplyStamp` (`internal/stencilstore/reconcile.go:133`) means the on-disk file never equals the return value.
**Fix:** Reword the Q&A answer to the stripped-body formulation §Testing already fixes.

### [BLOCKING:scope] Stale stencil count in the sandbox suite not in the doc inventory
**Section:** §Scope ("Four doc updates") / §Decisions ("All four doc updates land in this commit")
**Issue:** `tools/sandbox/SANDBOX-CORE-SUITE.md:232` reads "Does `lyx stencil list` name all fifteen registered stencils" — eighteen after this task; the discussion enumerates exactly four docs and gives this file no disposition.
**Fix:** Name `tools/sandbox/SANDBOX-CORE-SUITE.md` as a fifth required update, or state explicitly why the count is left stale.

### [BLOCKING:decision] Design doc's step 4 falsehood has no disposition
**Section:** §Decisions "All four doc updates land in this commit"
**Issue:** The design-doc update is scoped to "status flip plus the step-3 correction", yet §Technical context proves step 4 is also false — `manifest/designs/pattern-directive-stencils.md:31-33` claims the change is "plumbing-free" because "websterengine's functions already take it as a parameter", which `render.go`'s `RenderRecoveryPrompt`/`RenderMasterPrompt` (they derive `fabricengine.StencilsDir(l.HubPath)` internally at lines 181 and 240) contradicts.
**Fix:** Add step 4's correction to the design-doc update scope alongside the step-3 correction.

### [NIT:consistency] Superseded "one allowlist entry" left standing in the Q&A log
**Section:** §Q&A log entry 1
**Issue:** It says "Extend the invariant's allowlist by one entry (`internal/stencilstore`)", contradicting §Decisions and the final Q&A entry, both of which say two (`stencilstore` + `stencil`); verified three in-test statement sites at `internal/pattern/leaf_enforcement_test.go` lines 1-6, 22-25, 86.
**Fix:** Correct entry 1 to two entries, or mark it superseded in place.

### [NIT:scope] Burler fixture seeding left as an open choice
**Section:** §Testing "`internal/burlerengine`"
**Issue:** "mill-plan may add it or not" leaves a decision for the plan writer rather than making one; verified no burler test activates PATTERN and `template_test.go:142` uses a hardcoded placeholder.
**Fix:** State a default (seed or do not seed) and keep the "must not be presented as a fix" caveat.

### [NIT:consistency] Line-number drift in §Technical context
**Section:** §Technical context "The four call sites" / "Test-fixture landscape"
**Issue:** Webster derives `fabricengine.StencilsDir(l.HubPath)` at `render.go:181` and `:240` (discussion says 180/239); `seedHubStencils` is declared at `internal/websterengine/template_test.go:122` (discussion says 128, the `files` map inside it). The four `pattern.Directive` call sites, the constant/leaf-test/`doc.go:53-54`/`roadmap.md:20` references all verified exact.
**Fix:** Correct the three drifted references.

## Verdict

REQUEST_CHANGES
Test-count and doc-inventory contradictions must be resolved before plan writing.
MILL_REVIEW_END
