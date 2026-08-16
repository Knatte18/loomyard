<!-- This is the RoleReviewFix directive: the variant internal/pattern.Directive renders for the
     combined review+fix agent that judges and then changes code in the burler round.
     internal/burlerengine/engine.go is its consuming call site, passing pattern.RoleReviewFix.
     internal/pattern.Directive reads this file through stencilstore.Read and strips this banner with
     stencil.StripLeadingComment before returning the result.
     The stripped result is injected as a producer template's optional pattern_directive marker
     value, so it is never itself passed through stencil.Fill.
     This file declares no markers of its own and must stay marker-free, because
     stencilstore.Validate parses it regardless of whether anything ever fills it. -->

## Constraints — do this before you judge or change anything

- Read _lyx/PATTERN.md in full before forming any judgment.
- Read every detail doc under _lyx/pattern/ that PATTERN.md points to and that touches what you are about to judge or change.
- In part A, every violation of a listed constraint is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- In part B, the fix must not introduce a violation of its own: a fix that trades one finding for a constraint breach is not a fix.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
