MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5), per this session's own system-provided identification
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [BLOCKING:scope] Cards 3 and 10 omit the file declaring a symbol their Requirements name
**Location:** batch 1 / card 3 (`01-cluster-exclude.md`); batch 3 / card 10 (`03-burler-producer.md`)
**Issue:** Card 3's Requirements name `auditClusterRound` and `ErrClusterForksMissing`, both declared in `internal/burlerengine/cluster.go` — but card 3's `Context:` lists only `profile.go`/`config.go`, omitting `cluster.go`. Card 10's Requirements name `ClusterExclude` (a `burlerengine.Profile` field declared in `profile.go`), but card 10's `Context:` (burler.go, ctx.go, archive.go, three *_test.go files, shedengine/producer.go, burlerengine/engine.go+verdict.go, shuttleengine/engine.go) omits `profile.go` entirely.
**Fix:** Add `internal/burlerengine/cluster.go` to card 3's `Context:`, and `internal/burlerengine/profile.go` to card 10's `Context:`.

### [BLOCKING:consistency] Card 14's roadmap move leaves stale "above"/"two items" cross-references in Planned
**Location:** batch 4 / card 14 (`04-docs.md`); `manifest/roadmap.md` lines 22, 38, 39, 48, 53
**Issue:** Card 14 moves the **shedadapters: Burler-round producer** item out of `## Planned`'s `### Perch → Shed flattening` subsection (currently exactly two items: Burler-round producer + Bouncer) into `## Done`, but its own instruction is "Change no other Planned or Someday item." Four other sentences in that same Planned section depend on the item staying there and staying paired: the Bouncer item itself reads "unlike the Burler-round producer above" (line 22) and "an instance of `shedadapters: Burler-round producer` above" (line 38); three separate loom review-producer items each read "Depends on the two 'Perch → Shed flattening' items above" (lines 39, 48, 53). After the move, only one item (Bouncer) remains in that subsection, "above" no longer points at anything, and "the two ... items" is literally false — all four become incorrect the moment card 14 lands, and card 14's own scope forbids fixing them.
**Fix:** Expand card 14's scope to also correct these four now-stale phrasings in the same commit (e.g. "the Bouncer item" / "the one 'Perch → Shed flattening' item above, plus the now-Done Burler-round producer"), rather than leaving them to silently go wrong.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps (cards 3, 10) and one card-14-induced roadmap staleness (four dangling cross-references) need fixing.
MILL_REVIEW_END
