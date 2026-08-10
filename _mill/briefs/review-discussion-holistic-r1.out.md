MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:consistency] Client Boundary guard does not tolerate the gitrepo pair
**Section:** Constraints (gitrepo Client Boundary Invariant) / gitrepo-run-is-covered
**Issue:** `cmd/lyx/gitrepoboundary_test.go:174` asserts `gitexecTotal != 1` — exactly ONE non-comment `gitexec.` occurrence in all of `internal/gitrepo` non-test source, and `:177` requires it to sit inside `run`'s body; a checked sibling calling `gitexec.Run` makes it 2 and fails, and any method migrated from `r.run` to the sibling also drops out of the `r.run`-keyed `gitrepoPinnedRunBoundMethods` set-equality check at `:167`.
**Fix:** Replace "the set-equality check tolerates [the shape change], but it must confirm that" with the recorded fact that both assertions in `TestGitrepoBoundary_PinnedRunCallSites` must change, and state which.

### [BLOCKING:scope] Three token-keyed guards go blind to `gitexec.Run`
**Section:** Constraints / guard-test-with-justification-comments
**Issue:** `cmd/lyx/tierpurity_test.go:54` (`bannedTokens`), `cmd/lyx/hermeticenv_test.go:49` (`gitSpawnTokens`) and `cmd/lyx/rawgitmutation_test.go:37` all key on the literal `gitexec.RunGit`; `gitexec.Run(` does not contain that substring, so an untagged test, a non-hermetic package, or websterengine can spawn git through the new entry point undetected — three CONSTRAINTS invariants silently holed, none named in the discussion.
**Fix:** Record in the verdict that Test Tier Purity, Hermetic Git Test Environment and Fabric Git Invariant token lists must gain `gitexec.Run` in the implementation commit.

### [BLOCKING:design] Regeneration query misses helper-constructed errors
**Section:** The predicate-site inventory — regeneration query
**Issue:** The query inspects ~8 following lines for `fmt.Errorf`/`errors.New`, but `warpprobe.go` builds errors via `wrapProbeError`; lines 71, 95 and 136 all `return warpProbeResult{}, wrapProbeError(...)` yet the inventory files them as "non-zero means this is not a weft, returned as a value with a nil error" — only `:81` is genuinely a predicate, so the 15-site predicate count (the load-bearing evidence for two entry points) is inflated.
**Fix:** Broaden the classifier to any error-returning branch (helper constructors included) and re-state the 48/15 split, or record it as approximate.

### [BLOCKING:design] Two inventories carry no regeneration query
**Section:** verdict-carries-shapes-and-a-regeneration-recipe / Testing
**Issue:** The acceptance bar requires shape + regenerating query for each inventory, but the ~51-of-~70 two-message-merge count (which drives the implementation size estimate) and the gitrepo 21-site / six-discard classification are given as bare numbers with no stated method.
**Fix:** Supply a query for the two-block merge shape and for gitrepo's `r.run` sites, or drop them to explicitly-unmeasured status.

### [BLOCKING:design] Checked-Call guard's call-site key and test scope unspecified
**Section:** guard-test-with-justification-comments
**Issue:** "Keyed by call site" is not defined — file:line keying rots on every unrelated edit (the same staleness the document elsewhere designs around), and the discussion never says whether the ~50 `*_test.go` RunGit sites are inside the pinned set or exempt.
**Fix:** State the key (e.g. file + enclosing function, not line) and whether test files are pinned.

### [NIT:consistency] Outside-fabric count contradicts its own enumeration
**Section:** Call-site inventory
**Issue:** The table gives `internal/gitrepo` 2 sites and 75 total and the prose says "five production sites outside fabric", but only four are enumerated and `internal/gitrepo` has one production `gitexec.RunGit` call (`gitrepo.go:60`).
**Fix:** Reconcile the table, the total, and the enumerated list before the number is copied into the verdict.

## Verdict

REQUEST_CHANGES
Guard-test interactions and one inventory classifier are wrong; verdict itself is sound.
MILL_REVIEW_END
