MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-26
```

The discussion is fully scoped and decided. Spot-verified against source: `configreg.go`
imports+rows for warp/weft (`:18,:20,:51,:53`) and the already-registered fabric row (`:45`)
match exactly; consumer sites in `initengine/undo.go` (`:54,:89,:90,:96`), `buildercli/weft.go`,
and `perchcli/run.go` (`:367-391`) match the call-site map, and each has both host
(`WorktreeRoot`/`layout.WorktreeRoot`) and weft path in scope for the `New(host,weft)`
rewrite. Fabric API signatures all confirmed: `New(warpPath, weftPath) (*Fabric, error)`,
`(*Fabric).CommitWeft(...) (sha, committed, err)`, free `PushWeftAt(weftPath, opts)`,
`NewTopology`, one-arg `LoadConfig`, and the `fabriccli/weft_verbs.go:124` error-checked
pattern. Signature gotchas, nil-deref-on-discarded-`New`-error, the two-tier grep gate,
transition-window mixed-trailer tolerance, batch DAG with C-atomicity, and all CONSTRAINTS
touchpoints are correctly identified. Testing names targeted-per-batch plus a full final gate.
Prior-round gaps (comment sweep, crucible links, Tier-2b fabric self-de-parallel-build) are
resolved. No GAPs.

## Verdict

APPROVE
Scope, decisions, and source claims are complete, accurate, and ready for plan writing.
MILL_REVIEW_END
