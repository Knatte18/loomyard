MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-16
```

## Findings

### [BLOCKING:scope] Card 1 Context omits the files its own Requirements name
**Location:** batch 01-stencil-files, Card 1 **Issue:** Card 1's Context lists only `internal/pattern/pattern.go` (still pre-Card-3, old constants, no stencil calls at this point in the batch) plus two example stencil files, but the Requirements dictate banner text that must accurately state `stencilstore.Read`, `stencil.StripLeadingComment`, `stencil.Fill`, and `stencilstore.Validate`'s behavior — none of these functions' defining files appear anywhere in Context, forcing cold-start exploration or guesswork to write an accurate banner. **Fix:** Add `internal/stencilstore/reconcile.go`, `internal/stencilstore/stencilstore.go`, `internal/stencilstore/validate.go`, and `internal/stencil/stencil.go` to Card 1's Context list.

### [NIT:consistency] Card 10 asks doc.go to remove wording it doesn't contain
**Location:** batch 03-docs, Card 10 **Issue:** Card 10's second instruction says the file-header comment and package-doc opening paragraph must be edited "so neither describes Directive as returning constants" — but `doc.go`'s only "constant" mention is in the "Why the pointer stays relative" section, already corrected by the card's first instruction; the file-header (line 1) and the package-doc opening paragraph never made that claim (that description belongs to `pattern.go`'s own header, already fixed in Card 3). **Fix:** Reword so the header/opening-paragraph edit is scoped to adding the new read-path subsection, not to removing a "returns constants" claim that isn't there.

## Verdict

REQUEST_CHANGES
Card 1's Context omits files its own banner-text Requirements name (BLOCKING); one doc-accuracy nit in Card 10.
MILL_REVIEW_END
