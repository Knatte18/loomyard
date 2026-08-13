MILL_REVIEW_BEGIN
# Review: gitexec: add the checked entry point and migrate the call sites

```yaml
duration_s: 211.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:design] Raw-vs-checked discriminator is under-specified
**Section:** `rev-parse-probes-are-mixed-not-pure-predicates` vs `the-checked-call-invariant` + the two out-of-fabric raw rows
**Issue:** Structurally identical sites — exec path returns a real error, every non-zero exit is an answer — are classified both ways with no stated discriminator: `add.go:59`, `boardweft.go:24`, `pull.go:134` (`weftHasUpstream`) go checked, while `gitrepo/push.go:132` (`HasUnpushed`), `lyxcwd.go:147`, `fabriccli/fabric.go:494` stay raw; the decision's own rationale ("the marker would have to claim every exit code is an answer, which is false on the exec path") applies verbatim to all three raw sites, and neither of the two named raw classes (pure predicate / pinned deliberate-suppression) actually covers them.
**Fix:** State the discriminator explicitly (e.g. "raw only where no exit-code branch and no exec branch are separately reported, or where a test pins the surface"), and re-file `HasUnpushed`, `lyxcwd`, and `fabriccli` under whichever side it selects — including the marker wording that is then truthfully fillable.

### [NIT:consistency] "The eight call sites merge their two messages" is superseded
**Demoted-from:** BLOCKING
**Section:** `destroy-executors-are-re-signatured`
**Issue:** The decision bullet says "The eight call sites merge their two messages under the default merge rule" and the Technical Context repeats "8 call sites", while the bold clause and the shape table immediately below say nine and explicitly exclude two of them (shape D) from the merge rule — the code shows nine production call sites (remove.go:205, prune.go:278, checkout.go:225, cleanup.go:289, weftwiring.go:218/238, add.go:116/284/308).
**Fix:** Correct both "eight" occurrences to nine and reword the bullet so it no longer asserts that all of them merge.

### [NIT:consistency] One listed rev-parse probe has no exec-error path
**Section:** `rev-parse-probes-are-mixed-not-pure-predicates`
**Issue:** The rationale claims each of the seven sites "already separates `if err != nil { return …, fmt.Errorf(…) }` from `if exitCode == 0`", but `checkout.go:74` (best-effort weft branch capture) is `werr == nil && code == 0` with no error return at all — the premise is false for that site.
**Fix:** Note that site as best-effort and state its migrated form (`if out, err := gitexec.Run(…); err == nil`), rather than asserting a shape it does not have.

### [NIT:design] The guard's `r.run` token spelling is left ambiguous
**Section:** `the-checked-call-invariant` / Testing
**Issue:** The invariant and the test plan write the raw-site token as `r.run` unparenthesised, but `r.runChecked(` contains that substring — a raw-substring guard keyed that way demands a `//gitexec:raw` marker at all 18 migrated sites, the inverse of the deliberate prefix-matching chosen for `gitexec.Run`.
**Fix:** Pin the token as `r.run(` (with the paren) everywhere it appears, and say why the paren is load-bearing here while the `gitexec.Run` prefix deliberately is not.

### [NIT:decision] `export_test.go`'s executor call has no disposition
**Section:** Scope "Out" / `destroy-executors-are-re-signatured`
**Issue:** Scope declares test files out of the change, but `internal/fabricengine/export_test.go:123` returns `deleteBranch(...)` directly, so the re-signature changes that test seam's own signature and its callers.
**Fix:** Name the test seam as an in-scope consequence of the re-signature, so "test files are exempt" is not read as "no test file changes".

## Verdict

REQUEST_CHANGES
Raw-vs-checked classification lacks a discriminator; the executor call-site count contradicts itself.
MILL_REVIEW_END
