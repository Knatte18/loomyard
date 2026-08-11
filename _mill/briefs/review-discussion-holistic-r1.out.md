MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Most refusals never reach the gate, so RefusedBy fails
**Section:** `### refusal-check-assertion`, `### tranche-1-state-matrix`
**Issue:** The verbs pre-flight their own checks and return plain `fmt.Errorf`, not `*destructiveRefusal`: `remove.go:45` (slug), `remove.go:68-75` (`"worktree has uncommitted changes; use --force"`), `checkout.go:42-48` (dirty weft) — none contains `"<check> check failed"`, so `RefusedBy(err, dirtiness)` is false on the most obvious dirty/`Remove` cell and the matrix's central assertion silently mis-specifies.
**Fix:** Decide and state a second expectation kind — refused-before-the-gate (assert on the verb's own message) versus refused-by-the-gate — and say which cells are which.

### [BLOCKING:consistency] "remove .. refuses on containment" is not what the code does
**Section:** `## Testing` → scenarios list
**Issue:** `Remove` calls `validateWorktreeSlug` at `remove.go:45`, returning `invalid slug "..": a slug must name a directory…` before any `pathRequest` is built; the gate's containment refusal at `destroy.go:528` is unreachable for that input, so a cell asserting `RefusedBy(err, containment)` would fail against a correct binary.
**Fix:** Restate the R5 scenario as "refuses at slug validation, hub survives", or say explicitly that the check name is not asserted there.

### [BLOCKING:scope] fabrictest is outside the fabric-vocabulary owner set
**Section:** `## Constraints` → Fabric Vocabulary Invariant
**Issue:** `fabricVocabularyOwners` (`internal/lyxcwd/enforcement_test.go:597`) is an exact-directory map and the tree scan covers all non-`_test.go` `.go` files under `internal/`; `internal/fabricengine/fabrictest`'s `hub.go`/`states.go`/`verbs.go` are production files whose every bare `weft`/`warp` identifier or comment fails `TestEnforcement_FabricVocabulary`, and importing `internal/weftname` (needed for the `-weft`-suffix hostile input) fails `weftnameImportOwners` too.
**Fix:** Decide that the new dir gets owner rows in both maps plus a CONSTRAINTS.md owner-set update in the same commit, and record it in Constraints.

### [BLOCKING:design] The CloneHub{Reset:true} column has no coherent expectation
**Section:** `### tranche-1-verb-table`, `### clean-state-effect-assertions`
**Issue:** `resetHub` declares `dirtinessNA(...)` (`clone.go:585`), so no dirtiness state can refuse it — total hub destruction is correct behaviour — which makes the permitted root the hub root and the manifest diff vacuous for all nine states; and the rebuilt hub is `CloneHub`-partial (no junctions, no repo-wide `fabric.yaml`), the exact shortfall `hub-factory-fidelity` cites.
**Fix:** State this column's expectation explicitly — what may vanish, that dirtiness is by-design not refused, and whether the rebuild is asserted at all.

### [BLOCKING:design] Undecided expectation for non-slug hostile inputs
**Section:** `### tranche-1-verb-table` → hostile-input paragraph
**Issue:** A leading `-` is left as "asserts on what the verb does with it", and the slug-shaped hostile set is applied to `Checkout`, which validates no branch at all (`checkout.go:38-85` goes straight to `git switch`) — both leave the cell's expected outcome undefined, so it will be written regenerated-from-actual and assert nothing.
**Fix:** Name the expected outcome for the leading-`-` cells and for `Checkout`'s hostile inputs, or move them out of tranche 1.

### [NIT:consistency] The "four checks" assertion surface is really three
**Section:** `### refusal-check-assertion`
**Issue:** `checkForce` is declared (`destroy.go:39`) but never emitted in any `destructiveRefusal`, so a `force` `Check` constant can never match.
**Fix:** Say the exported `Check` set is containment/ownership/dirtiness, with force noted as never-emitted.

### [NIT:scope] CloneHub call-site arithmetic is inconsistent
**Section:** `## Scope` → Out, `### extraction-scope`
**Issue:** The tree holds 100 `CloneHub(` calls across the 7 external test files; the quoted 102 includes `clone_test.go`'s 2, which the same section lists separately as out of scope.
**Fix:** Quote 100 across 7 files, plus 2 in-package.

## Verdict

REQUEST_CHANGES
Refusal-assertion mechanism, vocabulary owner set, and the reset column need decisions.
MILL_REVIEW_END
