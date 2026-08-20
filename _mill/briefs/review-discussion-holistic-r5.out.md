MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:decision] Roadmap's Bouncer item still states the superseded discriminator
**Section:** Scope ("Out": Bouncer) + Round resolution decision
**Issue:** `manifest/roadmap.md:23` specifies the Bouncer's seed-vs-judge test as "if the round producer's report artifact for the current round does not exist yet" — a single-artifact predicate, which is exactly the review-only-orphan wedge the round-4 pair-predicate decision closed; the discussion asserts "the `Bouncer` uses exactly that pair predicate" but never says whether that roadmap wording is amended, and its roadmap edit is scoped to "a Planned item completing" only.
**Fix:** State the disposition of `roadmap.md:23`'s discriminator sentence (amend in this commit, or name the durable doc — e.g. `internal/shedadapters/doc.go` / `shed.md`'s Engine-adapters section — that records the pair predicate as the binding two-sided contract the Bouncer task must read).

### [NIT:design] Focus-file token: read side defined, writer side unstated
**Section:** "The next-round focus file: structured, fail-safe, defined here"
**Issue:** The file is read at `round-<N>-focus.json` for the round about to run, but the discussion never states that the writer must therefore name it for the *not-yet-run* round (`N+1` relative to the round it judged); a Bouncer naming it after the round it judged degrades silently to "no directive" by the file's own fail-safe rule, losing the whole trimming feature undetectably.
**Fix:** State the token's meaning explicitly ("`round-<N>-focus.json` carries directives *for* round `N`; the seed call writes `round-1-focus.json`") in the same place the read contract is stated.

### [NIT:consistency] Two source citations imprecise
**Section:** Q&A (runDir creation) and Technical context (`ResolveFan`)
**Issue:** `PerchProducer.resolveRunID`'s mandatory `MkdirAll` creates the *scratch* dir, not a run dir (`perch.go:120-126` — "run dirs are tracked and the scratch tree never is"), so the "a fresh clone has no run dir" analogy misstates the cited code; and `ResolveFan` *rejects* a fan with more than `maxClusterN` entries (`config.go:102-104`) rather than "caps a fan at `maxClusterN`".
**Fix:** Reword both to what the cited code does; the underlying decisions (Call-time `MkdirAll`, fan size limit) stand unchanged.

## Verdict

REQUEST_CHANGES
One superseded cross-task specification left standing; the rest is sound.
MILL_REVIEW_END
