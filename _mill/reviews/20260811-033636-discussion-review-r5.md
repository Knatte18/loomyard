MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: claude-opus-4-class (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:consistency] stderr-sniffing exit path contradicts "uniform binding"
**Demoted-from:** BLOCKING
**Section:** Technical context → "Uniform prior shape for stderr binding" / migration recipe
**Issue:** `internal/fabricengine/index.go:209-220` binds stderr and *inspects its content* on the non-zero path — `if strings.Contains(stderr, "does not have any commits yet") { return nil, nil }` — so the claim that every stderr binding "appears exactly twice, once bound and once used in an error message, and does nothing else with it" is false, and this recovery-by-stderr-text class (answer vs failure decided by stderr content, not exit code) has no stated disposition anywhere in the document.
**Fix:** Add the class to the taxonomy with a disposition (checked form plus `errors.As` + `gitErr.Stderr` sniff, or raw), name `index.go:209` as its known member, and soften the uniformity claim to "except content-sniffing sites, enumerated below".

### [NIT:scope] Merge rule does not cover a `%d` from a *different* call
**Section:** Migration recipe → "The `(git exit %d)` fragment is dropped along with its argument"
**Issue:** `internal/fabricengine/reconcile.go:546` and `:550` embed `exitCode` from the *earlier* `rev-parse` call at `:534`, not from the call whose error is being merged, so "drop the fragment with its argument" silently discards a second call's diagnostic rather than a duplicate of `GitError.Error()`.
**Fix:** Add a one-line clause: where the message cites a prior call's exit code, keep that call in the raw form (or capture its `*GitError`) rather than deleting the code.

### [NIT:scope] `wrapProbeError` call-path count is wrong and mixes line kinds
**Section:** Migration recipe → "Error-constructing helpers … must be re-signatured"
**Issue:** The document says "all four `warpprobe.go` exit paths route through it" and identifies them as `:71`/`:95` (stderr) and `:69`/`:79` (cause); the file actually has seven `wrapProbeError` calls (`:69`, `:72`, `:79`, `:93`, `:96`, `:134`, `:137`) across two functions, and `probeTreeHasPath` (`:132-139`) is unmentioned — the entries again mix comparison lines with call lines, the exact defect the predicate table was re-keyed to fix.
**Fix:** Re-key this enumeration to call sites like the predicate table, state seven paths in two functions, and label it a 2026-08-10 snapshot.

## Verdict

APPROVE
One unaddressed stderr-content-recovery class falsifies a stated safety premise; two enumeration nits.
MILL_REVIEW_END
