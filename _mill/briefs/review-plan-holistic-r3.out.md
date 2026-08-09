MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, per system self-identification)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Old-order guard shadows the pre-existing stale-`.fabric-anchor` migration error
**Location:** batch 2, card 3 (`probeWeftBinding`/`WeftLooksLikeWeft`) and card 4 (guard placement)
**Issue:** `WeftLooksLikeWeft` (card 3 step 6) probes only for `lyxcwd.AnchorFileName` (`.lyx-anchor`), never the pre-rename `.fabric-anchor` name. Card 4's new guard runs *before* `clone.go`'s existing steps 4-9, which is where the specific "found stale `.fabric-anchor` marker … re-clone this hub to migrate" hard error lives (`clone.go` lines 181-187). A real operator re-cloning a legacy hub (pre-anchor-rename) with an explicit two-arg clone now gets the generic "refusing to bootstrap … check the argument order" guard message instead of the specific, actionable stale-marker message — a genuine UX/correctness regression this plan does not acknowledge anywhere, unlike its careful documentation of every other edge case.
**Fix:** Either have the probe's "looks like a weft" discriminator also recognize the stale `.fabric-anchor` name, or explicitly document (and accept) that a legacy stale-marker hub now requires `--force-bootstrap` on its first post-migration clone, superseding the old specific error for that path.

**Corollary (batch 2, card 7):** because of the same gap, `TestCloneHub_StaleFabricAnchorHardErrors`'s weft fixture (seeded via `commitFileOnBranch(..., ".fabric-anchor", ...)`, not `lyxcwd.AnchorFileName`) does **not** satisfy card 7's "already carries `.lyx-anchor`" ForceBootstrap exception as literally stated, even though it's seeded via `commitFileOnBranch`. Without `ForceBootstrap: true` on this specific call, the guard intercepts first and the test's `strings.Contains(err.Error(), "re-clone")` assertion fails (the guard's message never contains "re-clone"). Card 7's two-bucket heuristic needs an explicit carve-out naming this call site.

### [BLOCKING:scope] Card 13's Requirements name `BoardDir` from a file absent from its Context/Edits
**Location:** batch 5, card 13 (`reconcileWarpBinding`)
**Issue:** Requirements step 1 says "Resolve the board directory as `BoardDir(l.HubPath)`." `BoardDir` is declared in `internal/fabricengine/junctionnames.go`, which is in neither card 13's `Context:` (`warpbinding.go`, `boardweft.go`, `topology.go`, `gitexec.go`, `anchor.go`) nor its `Edits:` (`reconcile.go`). `reconcile.go` itself only *mentions* `BoardDir` in a doc comment (line 15), never calls it — so an implementer confined to Context has no confirmed signature for this same-package helper.
**Fix:** Add `internal/fabricengine/junctionnames.go` to card 13's `Context:` list.

### [NIT:consistency] "board branch tracks its remote from the clone" overstates the orphan-bootstrap case
**Location:** batch 5, card 14 (push-on-`present` comment)
**Issue:** For a hub bootstrapped against a genuinely empty weft remote (`ensureBoardWorktree`'s `--orphan` path), `_board`'s branch is created fresh with no upstream at all — `HasUnpushed` would treat it as unpushed on every reconcile, not just "not the normal case." The comment's claim holds only for the adopt path (an already-existing default branch carries upstream from the initial `git clone`).
**Fix:** Narrow the comment to say this is the adopt-path case; note the orphan-bootstrap case attempts a (harmless, no-op-on-remote) push every reconcile.

### [NIT:consistency] Card 18 perpetuates a pre-existing roadmap/design-doc contradiction on slice 8
**Location:** batch 6, card 18 (roadmap.md edit)
**Issue:** `manifest/designs/fabric-unified-view.md`'s own Slice 8 section is headed "(shipped)" and states "The CLI-wording question below was resolved," while `manifest/roadmap.md` (edited by the same card) is to keep saying "leaving only [slice 8's] open CLI-wording policy question." These two files, both touched by card 18, will still disagree about whether slice 8's CLI-wording question is open. Pre-existing, unrelated to slice 10, but card 18 is the natural place to note or fix it.
**Fix:** Either reconcile the roadmap wording against the design doc's own "resolved" claim, or add a one-line acknowledgment that this is out of scope for this task.

## Verdict

REQUEST_CHANGES
Two BLOCKING findings: a guard/legacy-marker masking gap (batch 2) and a Context-completeness gap (batch 5, card 13).
MILL_REVIEW_END
