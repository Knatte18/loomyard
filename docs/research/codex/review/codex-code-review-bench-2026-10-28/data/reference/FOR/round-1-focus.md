---
round: 1
focus:
  - "internal/landingshed/finalize.go and fixreview.go: settle whether every route in D3 holds (approved lands, rejected and no-usable-verdict reroute with the reason naming findings or cause, dirty tree or moved head halts with LandingStopPrefix, failed start is transient)."
  - "internal/landingshed/finalize.go: settle whether the solo review runs only for an over-cap change with Deps.RerouteReviewed true, and that a spent fixer, a zero Code budget and a pawl run keep today's behavior (D2)."
  - "internal/landingshed/fixerattempts.go and fixreview.go: settle whether the reviewed-HEAD record is written only on approval, is honored only at the same HEAD, and survives a resumed Finalize (D6)."
  - "contracts/specs/landing-fix-review-spec.md and ParseFixReviewReport: settle whether the parser matches the spec, including an empty Findings section on a rejection, and is the sole parser (D7)."
  - "contracts/stencils/.../landing-template-fix-review.md and stencils.go: settle whether the prompt names Baseline..HEAD, the cap reason, the fixer report and the decision record, points at the spec without restating it, and renders parent_directive and edit_directive and is listed in roleOpeningStencils (D4, PATTERN)."
  - "Tests in finalize_gate_test.go, fixreview_test.go, fixerrun_test.go: settle whether each acceptance criterion has coverage without duplicating existing tests."
  - "Docs: settle whether doc.go, both recipe comments, docs/overview.md and PATTERN-sole-parsers.md match the behavior the diff builds."
---

Seed focus from the board entry and decision record; the diff is the subject.
