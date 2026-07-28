MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Third hardcoded `_lyx` reason string not named
**Section:** junction-health-check
**Issue:** `checkJunctionHealth` has **three** `_lyx` literals, not two — `reconcile.go:318` "host _lyx junction missing", `:326` "host _lyx is not a junction", and `:341` **"host _lyx junction points elsewhere"**, which the decision never mentions; with a singular `JunctionReason`, a mis-pointed `_pattern` junction would report the wrong junction name, defeating the "the reason names which" premise the `PairStatus`-keeps-its-shape decision rests on. `drift.go`'s inline copy is worse: only `:94` matches `checkJunctionHealth` verbatim, while `:83` ("junction missing") and `:110` ("junction points elsewhere") name no junction at all.
**Fix:** Name all three `checkJunctionHealth` strings and both unparameterised `drift.go` strings as sites that gain a junction name, and state the required wording for each.

### [GAP] Materialisation placement leaves the dangling-junction case unfixed
**Section:** weft-target-materialisation
**Issue:** The decision says `os.MkdirAll(target)` "before `fslink.CreateDirLink(link, target)`", but `seedLyxJunction` has two `CreateDirLink` calls (`junction.go:105` re-point, `:125` create) and the hard error the decision exists to remove is at `:83`, reached from the link-already-exists branch (`EvalSymlinks(target)` at `:81`) **before either** — so that placement still hard-errors on a worktree whose junction a pre-fix `checkout`/`reconcile` already left dangling, which is exactly the "no self-repair path" state the rationale cites.
**Fix:** Specify `MkdirAll(j.Target)` at the top of each loop iteration, before the `os.Lstat`/`EvalSymlinks` re-check, not adjacent to `CreateDirLink`.

### [GAP] Filter can leave an exclusions-only pathspec
**Section:** weft-pathspec-tolerance
**Issue:** The predicate always passes `:` entries through and drops non-matching positive entries, but says nothing about the case where **no positive entry survives** — `buildercli`/`webstercli`/`perchcli` pathspecs are one positive entry plus `:(exclude)` entries, so a worktree where `_lyx` matches nothing yields a non-empty, all-negative pathspec, and git treats exclusions with no positive pathspec as "everything except", i.e. `git add` stages the whole weft worktree.
**Fix:** State that when no positive entry survives, `CommitWeft` returns `("", false, nil)` without calling `StageAndCommit` at all.

### [GAP] Webster render signature change is not package-internal
**Section:** call-site-plumbing / Technical context
**Issue:** "`RenderForkPrompt` and `RenderMasterPrompt` are exported within the package; adding parameters is a package-internal signature change only" is wrong — both are exported and `websterengine_test` calls `RenderForkPrompt` at `template_test.go:484, 498, 522, 536, 559, 578, 603, 612, 637, 648`, none of which is on the Testing section's "certain breakages, verified" list, and it is left open whether `worktreeRoot string` survives alongside the new Layout (which already carries it, so the two can disagree).
**Fix:** List those ten call sites as certain breakages and decide whether `worktreeRoot` is replaced by the Layout or kept.

### [GAP] Test-tier invariant unacknowledged for the mandated real-git tests
**Section:** Constraints / Testing
**Issue:** The Constraints section enumerates Hub Geometry, CLI/Cobra, lyxtest Leaf and Documentation Lifecycle, but not the **Test Tier Purity Invariant**, while the Testing section mandates a dozen new tests that must spawn git or copy fixtures (`remove` at `RelPath != "."`, the `:(exclude)` pathspec case, materialisation, undo) — an untagged new `_test.go` containing `lyxtest.Copy*`/`exec.Command` fails `cmd/lyx/tierpurity_test.go`.
**Fix:** State that every new git-spawning test file carries the `integration` build tag, and that `internal/pattern`'s own tests stay untagged and spawn nothing.

### [NOTE] "Empty directory matches nothing" premise unverified
**Section:** weft-pathspec-tolerance
**Issue:** The claim that `git add -- _lyx _pattern` fails as a unit for an *existing-but-empty* `_pattern/` is asserted as fact; git's unmatched-pathspec `die` is guarded by a path-existence check, so plausibly only a wholly-absent entry errors — which would change what the mandatory regression test asserts.
**Fix:** Have the plan derive the behaviour from the real-git test (both shapes are already listed) rather than pinning an assertion on the asserted premise.

### [NOTE] Filter evaluation mechanism and anchor unstated
**Section:** weft-pathspec-tolerance
**Issue:** The predicate is pinned semantically but not mechanically: no statement of which command evaluates "matches in the worktree or the index", that it must run against the weft worktree (`f.weftPath`) with entries relative to the weft root, or that it runs inside the already-held weft write lock.
**Fix:** Name the evaluation command and the directory it runs in.

## Verdict

GAPS_FOUND
Five gaps: reason strings, MkdirAll placement, exclusions-only pathspec, webster signatures, test tiering.
MILL_REVIEW_END
