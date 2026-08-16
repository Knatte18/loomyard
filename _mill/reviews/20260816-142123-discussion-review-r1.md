MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files

```yaml
duration_s: 150.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] Read returns a stamped banner Directive never strips
**Section:** § Stencil mechanics / § Decisions "Fail loud"
**Issue:** `stencilstore.Reconcile` seeds every file through `writeStamped` → `ApplyStamp`, which prepends `<!-- lyx-stencil: sha256=… -->\n\n` unconditionally (`reconcile.go:89,129-137`, `stencilstore.go:103-111`); `Read` does a plain `os.ReadFile` and strips nothing. Every existing consumer survives this only because it feeds the bytes to `stencil.Fill`, which calls `StripLeadingComment` — but `Directive`'s return goes straight into a `values` map as a *value*, never through Fill, so the stamp banner would be injected verbatim into four producer prompts and `TestDirective_VariantsBeginWithOwnHeading`'s `## ` prefix assertion would fail.
**Fix:** Decide and record that `Directive` strips the leading banner (`stencil.StripLeadingComment`) after `Read`, and state the consequent second allowlist entry `internal/stencilstore` **plus** `internal/stencil` — the discussion's "gains `internal/stencilstore` and nothing else" is currently unachievable.

### [NIT:decision] New .md files' banner comment has no stated disposition
**Demoted-from:** BLOCKING
**Section:** § Scope "Out" / § Naming
**Issue:** All fifteen existing stencils open with an explanatory `<!-- … -->` banner (e.g. `stencils/webster/webster-prefix-recovery.md:1-5`), and `Reconcile` will write a stamp into one whether or not the source file has it. The discussion says the prose moves "byte-for-byte, not one word changes, including the trailing newline" and never says whether the three new files carry a banner — the two statements cannot both hold.
**Fix:** State explicitly whether each file gets the conventional banner, and re-word the "byte-for-byte" scope claim to bind the *body* (post-`StripLeadingComment`) rather than whole-file bytes.

### [NIT:consistency] Verbatim test is vacuous under the fixture/production split
**Demoted-from:** BLOCKING
**Section:** § Testing → `internal/pattern`, new test 3
**Issue:** Test 3 asserts `Directive`'s text equals the `stencils` embedded default byte-for-byte, but the modelled helpers (`loomengine/prompt_test.go:20-35`, `websterengine/template_test.go:122-140`) seed raw `os.WriteFile(embedded)` with no stamp, while production seeds via `Reconcile` with a stamp — so the assertion is green in fixtures and false in a real hub. It is also circular as a relocation guard: the constants are deleted and referenced nowhere else (grep: only `pattern.go`), so it compares the stencil file with itself.
**Fix:** Either route the new fixture through `stencilstore.Reconcile` so tests see production's stamped bytes, or state that the assertion is on stripped-body equality, and name what actually pins the relocation (the existing substring/`## `-prefix assertions).

### [NIT:scope] Allowlist lives in three places in the leaf test, not one
**Section:** § Constraints "Pattern Leaf Invariant"
**Issue:** `internal/pattern/leaf_enforcement_test.go` states the allowlist in its header comment (lines 1-6) and in the failure message (`line 86`: "stdlib + lyxcwd + lyxdirs") beside the `allowedImports` map; the discussion names only the map and `CONSTRAINTS.md`.
**Fix:** Add the header comment and the failure string to the enumerated edits.

### [NIT:scope] pattern.go's file header comment also goes stale
**Section:** § Technical context "The function today"
**Issue:** The discussion flags the lines 56-63 comment but not `pattern.go:1-2`, which likewise says "the three role-specific directive constants Directive selects between".
**Fix:** Include the file header comment in the same rewrite.

## Verdict

REQUEST_CHANGES
Stamp-banner stripping is undecided and invalidates the verbatim claim and its test.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
