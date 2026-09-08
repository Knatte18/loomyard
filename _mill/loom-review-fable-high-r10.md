# loom glyph-plan-format surface — independent review, round 10 (fable-high-r10)

Clean-room review in progress. Findings and test log appended incrementally; executive summary written last.

## What was tested

- Read in full (clean-room, before any prior-round material): `CONSTRAINTS.md`, `internal/planparser/{classify,glyphref,normalize,validate,handle,containment,parse,plan}.go`.

## Provisional observations (pre-verification jottings)

- P-1 (to verify): a bare extensionless filename under a non-"." `root:` ("Makefile" with `root: internal/foo`) normalizes to `internal/foo/Makefile`, which `canonicalizablePath` declines (slashed, no extension) and `checkDirectoryTarget` then flags as a directory — classify.go rule 4's own comment calls that resolution "the intended behaviour". Check spec, and check whether the finding's remedy (append `#`) actually works for a nested extensionless FILE glyph.
- P-2 (to verify): `checkRenamePairShape` admits `old = SELF glyph` -> `new = plan: handle` (a file's self glyph renamed "to a symbol handle"). Verify what planglyph's binding does with a self-glyph old side; possible unvalidated shape sibling of R9-2.
- P-3 (to verify): two SELF glyphs, one a file self glyph (`a/b.go#`) and one its containing package's unit self glyph (`a#`), are never compared by syntacticContainment (member-vs-self only). Check whether planglyph's resolve-backed containment covers file-in-package overlap.
- P-4 (strong candidate): `doneCheckVerdicts` (internal/planglyph/donecheck.go) reads `r.Status` through the single boolean `resolved := found || multipart` — an unrecognized status or a pre-resolution rejection (Status "") reads as "not resolved", which fails OPEN for `delete-not-done` and `rename-not-done-old` (target reads as gone → check passes). Exact sibling of R9-6's shape, on the one remaining unswitched Status consumer in planglyph.
- P-5 (to verify severity): `CanonicalizeHandles` guards N drafts→1 canonical (`handle-canonical-collision`) but not 1 draft→N canonicals (same draft handle declared by two sources naming different declarations): `subs[owners[0]] = canonical` is assigned once per canonical, last-wins over sorted order, and `RewriteRefs` then rewrites the plan ON DISK with an arbitrarily chosen canonical. The pure `handle-collision` finding fires in the same composite report (every multi-source case is a claim collision), so the plan is blocked — but only after its bytes were mutated with a wrong, arbitrary substitution.
- P-6 (read): `rewriteBulletLine`'s bind-time arrow-collapse only triggers for a handle LEFT side; a Rename pair's handle NEW side bound by BindHandles becomes a bare-glyph right side — that is OBS-2's known post-execution `rename-to-not-handle` shape, re-evaluate against real callers.

Additional files read: planparser/{sections,amendment,approve,rewrite}.go, planglyph/{planglyph,resolve,create,handle,drift,donecheck,containment,scope,delta,repo,doc}.go.
