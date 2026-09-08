# loom glyph-plan-format surface — independent review, round 10 (fable-high-r10)

Clean-room review in progress. Findings and test log appended incrementally; executive summary written last.

## What was tested

- Read in full (clean-room, before any prior-round material): `CONSTRAINTS.md`, `internal/planparser/{classify,glyphref,normalize,validate,handle,containment,parse,plan}.go`.

## Provisional observations (pre-verification jottings)

- P-1 (to verify): a bare extensionless filename under a non-"." `root:` ("Makefile" with `root: internal/foo`) normalizes to `internal/foo/Makefile`, which `canonicalizablePath` declines (slashed, no extension) and `checkDirectoryTarget` then flags as a directory — classify.go rule 4's own comment calls that resolution "the intended behaviour". Check spec, and check whether the finding's remedy (append `#`) actually works for a nested extensionless FILE glyph.
- P-2 (to verify): `checkRenamePairShape` admits `old = SELF glyph` -> `new = plan: handle` (a file's self glyph renamed "to a symbol handle"). Verify what planglyph's binding does with a self-glyph old side; possible unvalidated shape sibling of R9-2.
- P-3 (to verify): two SELF glyphs, one a file self glyph (`a/b.go#`) and one its containing package's unit self glyph (`a#`), are never compared by syntacticContainment (member-vs-self only). Check whether planglyph's resolve-backed containment covers file-in-package overlap.
