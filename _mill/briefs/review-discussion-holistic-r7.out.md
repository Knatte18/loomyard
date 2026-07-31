MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (claude-sonnet-5, per system context)
reviewed_file: C:\Code\loomyard\wts\fabric-snapshot-trailer\_mill\discussion.md
date: 2026-07-31
```

## Verification performed

Cross-checked source against the discussion's specific claims rather than trusting prose: `internal/fabricengine/{index.go,trailer.go,commit.go,weftgit.go}`, `internal/gitrepo/{gitrepo.go,snapshot.go,gogit.go,gogit_test.go,parity_test.go,keyvalidation_test.go}`, `cmd/lyx/gitrepoboundary_test.go`, `manifest/roadmap.md`, `manifest/designs/{raddle.md,fabric-unified-view.md}`, `crucible/{gitrepo-review-prompt.md,fabric-review-prompt.md}`, `docs/overview.md`, `docs/shared-libs/README.md`.

Every line-number/content citation checked (dozens: `weftgit.go`'s three fall-through points at 368/374-382/383, `commitWeftLocked`'s exact span 349-400, `RecordCorrespondence` call at 395, `RebuildIndex`'s order-sensitivity comment at 283-287/305, `gogit.go`'s locking-discipline bullets at 82/88-89/108-109/184, `gitrepoboundary_test.go`'s two `SnapshotSHA`-naming comment blocks and pinned-method map, `parity_test.go`'s enumerated test names/lines through 753, `snapshot.go`'s 365-line total, `roadmap.md:74`'s stale API enumeration, `raddle.md`'s four cited lines, `fabric-unified-view.md:67`'s exact wrong clause, and the case-insensitive `snapshot` grep count — 38 = 28+7+2+1 across doc.go/gogit.go/gitrepo.go/push.go, excluding the wholesale-deleted snapshot.go) matched the actual source exactly. No fabricated or stale citation found.

Design review against the stated criteria: no undecided items remain (every `### Decision:` carries rationale + rejected alternatives, including the two mid-discussion reversals — dead-code promotion, date-vs-topo ordering — that were caught and corrected rather than left standing); scope in/out is unambiguous and internally consistent with Technical Context; all relevant CONSTRAINTS.md invariants (gitrepo Client Boundary, Test Tier Purity, Hermetic Git Env) are addressed with concrete mechanics, not just acknowledgment; failure modes (unborn warp, unborn weft, dangling Warp-SHA, dirty index race, concurrent/back-dated branches, misuse via pathspec misclassification) are each named and given an explicit accepted-or-fixed disposition; testing strategy is concrete per-tier with TDD candidates flagged and two coverage-loss judgements explicitly justified (not silently dropped); no unstated technical obstacle found against the actual code read.

No findings.

## Verdict

APPROVE
Extensively source-verified across seven rounds; no remaining gaps.
MILL_REVIEW_END
