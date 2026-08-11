MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: APPROVE
reviewer_model: claude-sonnet-5
reviewer_self_id: Claude Sonnet 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Scope of this pass

Independently re-derived nearly every checkable claim in the document against the live tree rather than trusting the "already verified" framing: the `HEAD~1` slice-12 landing claim, all eight path-ownership kinds plus the branch-ownership kind and the four-check order in `destroy.go`, `slug.go:30`, `export_test.go`'s package boundary, the `CloneHub(` call-site count (102, across exactly 8 files: 7 tagged `integration` + untagged `clone_test.go`) with the two named per-file counts (59 in `warpbinding_clone_integration_test.go`, 29 in `clone_adopt_test.go`), the two `gitStatusPorcelain` definitions' package/file split, `fabricengine/clone.go:172`'s bootstrap guard, `lyxcwd/anchor.go:112`'s `samePath`, `reconcile_stale_registration_test.go:103`'s `newFabricFixture`, `lyxtest`'s `initBareRemote`/`buildWarpHub`/`buildWeftPrime`/`WarpFixture`/`CopyPaired*`/`CopyWeft` signatures, the four cited `CONSTRAINTS.md` invariants and their enforcement-test file paths, and the two referenced `manifest/designs/` docs. All of it checked out exactly as cited, which is unusually clean for a document this dense with line numbers and counts — the format-docs and batcher discussion reviews earlier in this campaign each turned up several off-by-N citations; this one turned up none in the load-bearing decisions.

Two things worth recording, both non-blocking.

## Findings

### [NIT:consistency] `HEAD~1 (3184cd5a)` no longer matches this worktree's history

**Section:** Problem, "Why now"
**Issue:** The doc states slice 12 "landed 2026-08-11 at `HEAD~1`, routing roughly 29 destructive call sites through one four-check gate." In this worktree, `git log --oneline -3` is `709cd818` (mill-start: write discussion.md) → `6e492297` (spawn: init status) → `3184cd5a` (slice 12 itself) — so slice 12 is `HEAD~2`, not `HEAD~1`. The commit hash is still correct and unambiguous, so no reader is actually misled, but the relative pointer will keep drifting by one on every future commit in this worktree and is already off by one today.
**Fix:** Drop the `HEAD~1` relative pointer and cite the hash alone (`3184cd5a`), which is what the rest of the document does everywhere else it references a commit.

### [NIT:completeness] Constraints section's banned-token list drops two of `CONSTRAINTS.md`'s eight tokens

**Section:** Constraints, "Fabric Destruction Chokepoint Invariant"
**Issue:** The doc lists the banned bypass tokens as `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `fslink.Remove(`, `createdToken{` — six tokens. `CONSTRAINTS.md:233` lists eight: those six plus `warp.ResetHard(` and `weft.ResetHard(`. Doesn't change the paragraph's conclusion (this invariant scopes to `package fabricengine` and explicitly does not apply to `fabrictest`), but a reader skimming this section as the reference list for "what a chokepoint bypass token looks like" gets an incomplete set — notably missing the two `ResetHard` tokens are exactly what tranche 1's `Pull` cell exercises (`Fabric.Pull`'s `ResetHard`, per the verb table).
**Fix:** Either quote the full eight-token list or drop the enumeration in favor of "see `CONSTRAINTS.md`'s Fabric Destruction Chokepoint Invariant for the full token list."

## Verdict

APPROVE
Every load-bearing citation checked — line numbers, call-site counts, package boundaries, ownership-kind enumeration, invariant enforcement paths — reverified accurate against the live tree. Two NIT-level citation/completeness gaps found, neither affects any decision.
MILL_REVIEW_END
