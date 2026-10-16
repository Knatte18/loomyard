<!-- This is the RoleReviewFix directive: the variant internal/pattern.Directive renders for the
     burler review half's reviewer, which judges code.
     internal/burlerengine/review.go is its consuming call site, passing pattern.RoleReviewFix.
     internal/pattern.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment and fills the file's required pattern_overview marker with
     the content of PATTERN.md at the repository's worktree root, and its optional pattern_changes marker with the task's declared PATTERN changes.
     The result is injected as a producer template's optional pattern_directive marker value, so it
     is never itself passed through stencil.Fill again.
     This file declares two markers, the required pattern_overview and the optional pattern_changes, and no other. -->

## Constraints — do this before you judge anything

- These are the repo's rules; read them in full before forming any judgment:

{{.pattern_overview}}
{{.pattern_changes}}

- Read every background file under pattern/ that the entries above link to and that touches what you are about to judge.
- Every violation of a listed constraint, an entry the declared changes list judged by its declared text, is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
