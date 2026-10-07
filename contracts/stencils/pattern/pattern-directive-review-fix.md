<!-- This is the RoleReviewFix directive: the variant internal/pattern.Directive renders for the
     burler round's two halves: the reviewer, which judges code, and the fixer, which changes it.
     internal/burlerengine/engine.go is its consuming call site, passing pattern.RoleReviewFix for both halves.
     internal/pattern.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment and fills the file's one marker, the pattern_overview marker, with
     the content of PATTERN.md at the repository's worktree root.
     The result is injected as a producer template's optional pattern_directive marker value, so it
     is never itself passed through stencil.Fill again.
     This file declares that one marker and no other, and the marker is required. -->

## Constraints — do this before you judge or change anything

- These are the repo's rules; read them in full before forming any judgment:

{{.pattern_overview}}

- Read every background file under pattern/ that the entries above link to and that touches what you are about to judge or change.
- In part A, which binds the reviewer's findings, every violation of a listed constraint is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- In part B, which binds the fixer's fixes, the fix must not introduce a violation of its own: a fix that trades one finding for a constraint breach is not a fix.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
