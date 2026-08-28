# Review: Audit internal/logger coverage across spawn/hard-error paths

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING][consistency] CONSTRAINTS.md amendment conflicts with the file's own "not a test-coverage index" rule

**Section:** Scope > In (line 41), Decisions > `audit-doc-location` (line 134)

**Issue:** The discussion commits to "A CONSTRAINTS.md amendment sharpening **Live-Substrate Spawn Observability** to name the guard and its allowlist, so the invariant stops being review-discipline-only" (line 41), and to CONSTRAINTS.md gaining "a pointer to it and to the new guard" (line 134). `CONSTRAINTS.md`'s own current top-of-file blurb (verified directly, line 4) reads: *"Not a test-coverage index: a new constraint may get its own enforcing test in the same change, but which tests exist today is not tracked here."* Naming `cmd/lyx/spawnobservability_test.go` inside the invariant's own text is exactly the pattern that sentence disclaims — it was written deliberately, in this same repo, to strip "Enforced by `<test>`"-style references out of this file (`d66cefe5`, this session's own earlier work: "CONSTRAINTS.md: strip to pure form rules"). The discussion never mentions this rule or reconciles the amendment against it, despite otherwise citing CONSTRAINTS.md's Documentation Lifecycle for the same edit.

**Suggested fix:** Either (a) sharpen the invariant's *prose* only — tighten "for a round/strand/session" without naming the guard file, and let the guard's own header comment (already planned, per the `enforcement-guard` decision) be where the mechanism is documented, or (b) if naming the guard in CONSTRAINTS.md is genuinely wanted, make that an explicit decision in this discussion that argues for reversing/carving an exception into the just-established "not a test-coverage index" rule, rather than silently reintroducing the pattern it removed.

## Verdict

REQUEST_CHANGES
One well-grounded conflict with an established, current repo convention (CONSTRAINTS.md's own "not a test-coverage index" rule, added this same session); every other load-bearing technical claim spot-checked against source (the gitexec import cycle, singlellm.go's branch structure, burler.go's precedent, the githubclient leaf enforcement test, sibling cmd/lyx guard tests) verified accurate, and the rest of the discussion (scope, decisions, testing strategy, blind-spot disclosure) is unusually thorough.
