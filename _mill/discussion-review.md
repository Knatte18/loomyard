# Review: `_mill/discussion.md` (Shed skeleton task)

Reviewed against `manifest/designs/shed.md` as it currently stands in this worktree (HEAD `32229ad9`, same as `main`) and against the cited source files directly — not just against the discussion's own paraphrase of them.

## Method

Spot-checked essentially every load-bearing code citation in the Decisions/Technical-context sections against the actual files: `internal/treadleengine/result.go`, `run.go:90-139`; `internal/state/state.go` in full; `internal/lock/lock.go` in full; `internal/loomengine/preflight.go:120-170`, `coherence.go:95-110`; `internal/perchengine/profile.go:30-50`; `internal/logger/sink.go:1-30`; `internal/fsx/fsx.go:1-20`; `CONSTRAINTS.md`'s Treadle Runner-Seam Invariant; `docs/reference/status-schema.md`. Also diffed the doc's claims about `shed.md`'s current state against a fresh read of `shed.md` itself.

**Result: every citation checked out, including line numbers.** `run.go:119-128` (the lock-acquire block), `state.go:108-109`/`:111-114`/`:122`/`:127-132`/`:147-153`, `preflight.go:135`/`:151`, `coherence.go:103-110`, `profile.go:43`, `logger/sink.go:21` — all exact, down to the line. The `docs-and-roadmap` decision's inventory of what's still wrong in `shed.md` (step-6 "back to step 2", the "Shed is the file's only writer" sentence, the missing `product` field in the JSON example, the missing `StatusLockPath` field, the "nothing on disk touched" wording) is itself accurate — I confirmed each of those is still present, unfixed, in `shed.md` as of this read. This is a high-precision document; nothing here contradicts the actual codebase.

## Findings

### 1. `strictness-is-scoped-to-the-read-gate`'s corrected behavior has no test (test-coverage gap)

The decision explicitly pins a subtle, previously-mis-stated behavior: an unknown top-level key written by an external actor *after* step 1's read is **silently destroyed** on the next persist, not caught by a later strict read. The decision even flags its own prior error: "An earlier version of this rationale claimed the key would be caught by the next strict read. That was wrong."

The Testing section's closest scenario — "External mid-producer write" (line 370) — only asserts that **known** shared fields (`pause_requested`, `product`) survive a concurrent external write. Nothing in the Testing section asserts the negative case: that a stray, unrecognized top-level key written externally mid-run is *gone* after the next `Shed` persist. Given this is a corrected misconception, not an obvious fact, it's exactly the kind of behavior that should be pinned by a test, not left to review-only memory in the discussion record. Recommend adding it as an explicit scenario in the "Loop scenarios that must be covered" list before this becomes the plan.

### 2. Bounce-budget exhaustion boundary isn't pinned (precision gap)

`total-bounce-budget` and the "Bounce-budget exhaustion" test scenario both say the cycle "terminates at exactly `MaxBounces` bounces," but neither states whether that means `MaxBounces` bounces succeed and the next attempt is refused, or the `MaxBounces`-th attempt itself is the one refused (i.e., whether the counter is checked-then-decremented or decremented-then-checked). This is the classic off-by-one seam, and it's the one place in the document where an exact boundary is left to implementation judgment despite the rest of the doc going out of its way to pin exact formats and edge cases (e.g. `activity.last`'s literal `"<producer> → <outcome>"` string, the exact position of the `done`-short-circuit relative to step 2). Worth one sentence stating the exact semantics — e.g. "`MaxBounces` bounces are permitted; the `(MaxBounces+1)`-th `Stuck` routes to `blocked`" — so the test asserting "the bounce count" has an unambiguous target.

### 3. `ctx-cancellation-as-pause` implicitly assumes producers never return `Stuck` for a cancelled context (minor, worth stating explicitly)

The decision's fix only covers `Call` returning a non-nil `error` alongside `ctx.Err() != nil`. It doesn't address — and doesn't need to mechanically, since `Shed` has no way to detect it — a producer that, on seeing a cancelled context, returns `(Stuck, ..., nil)` instead of an error. That would silently consume bounce budget or escalate to `blocked` for what was actually an operator Ctrl-C, exactly the misrepresentation this decision otherwise goes to some lengths to prevent for the `error`-return case.

This is very likely a non-issue in practice — surfacing cancellation as an `error` is idiomatic Go and the natural thing any adapter would do — but given three of the four engine adapters (`perch`, `Webster`, a bespoke multi-spawn engine) own their own internal error taxonomy and haven't been designed yet, it costs one sentence now to make this a stated producer-contract obligation ("a `ShedProducer` must surface context cancellation as a non-nil `error` from `Call`, never as `Stuck`") rather than an assumption nobody wrote down. Cheaper to pin here than to discover it as a bug report from a future adapter's Discussion phase.

## Non-findings (checked, no issue)

- The apparent contradiction between `shed.md`'s current `Shed` struct (no `StatusLockPath` field) and the `two-lock-paths-never-the-same-file` decision (which requires one) is **not** a bug — `docs-and-roadmap` explicitly lists this as a known, not-yet-applied `shed.md` edit. Consistent.
- `field-ownership-split` vs. `reread-and-merge-persist` vs. `product-field-passthrough`: cross-checked all three against each other for double-ownership or contradictory claims about who writes what. Clean — `pause_requested` is correctly singled out as the one field crossing the ownership boundary, and `product` is consistently "external-owned, round-tripped, never inspected" across every decision that touches it.
- `no-seeding-hard-error-on-missing` vs. the persist-side `found == false` guard in `reread-and-merge-persist`: correctly scoped to two different moments (step-1 read vs. step-5 persist) and both independently enforce "never seed," rather than one silently relying on the other.
- Scope's "Out" list excluding `loom.md` from this task's doc updates is correct, not an oversight — `loom.md`'s own status-file section was already updated on `main` in an earlier, separate commit (`dc97e980`'s companion doc work), confirmed present in this worktree.

## Overall

Structurally sound, internally consistent across ~25 decisions, and unusually well-verified against the codebase (I found zero factual errors in citations after spot-checking essentially all of them). The three findings above are refinements, not blockers — 1 and 2 are cheap to close before planning starts (one test scenario, one sentence), 3 is optional hardening. Nothing here should stop this discussion from moving to Plan.
