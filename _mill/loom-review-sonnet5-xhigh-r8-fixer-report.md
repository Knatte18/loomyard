# `loom` fixer report — round `sonnet5-xhigh-r8`

Companion to `_mill/loom-review-sonnet5-xhigh-r8.md`. Two findings recorded, both MEDIUM, both
addressed here.

## F1 — AuditForks-failure orphan (documented as an Accepted residual, not code-fixed)

**Disposition: documented, not code-fixed — a genuine design tradeoff.**

Fixing this safely requires `RunState` to distinguish two states it currently conflates: an
`AuditForks`-preserved run (meant to be diagnosed once by an operator, then reclaimed) and a
`KeepPane`-preserved run (`internal/shuttlecli/run.go`'s `--keep-pane` debugging flag, meant to
stay alive indefinitely on purpose). Neither `finalize` nor `RunState` records *why* a run's
strand/directory survived cleanup, so any sweep or `Attach` change generalized from "Outcome is
terminal" would reclaim (or fail to reclaim) both alike — silently breaking `KeepPane`'s own
guarantee that a kept pane stays kept. Resolving this cleanly needs either a persisted reason
field on `RunState` or a `websterengine`-style dedicated entry-time reclaim step generalized to
`BurlerProducer`'s cluster-fan path — a real architecture decision, not a mechanical fix, and
exactly the kind of tradeoff this campaign's own rule reserves for an explicit operator call
rather than a guess made unilaterally mid-round.

This is the same disposition the codebase already gives two structurally identical gaps: the
done-but-not-persisted crash window and the `AddStrand`/`run.json` crash-mid-registration window,
both documented as "Accepted residual" in `manifest/designs/loom.md`'s "Crash recovery" section
rather than code-fixed, for the same reason (closing them requires a new durable-state contract
this round is not positioned to design unilaterally).

**What was done:** added a third "Accepted residual (fork-audit-failure orphan)" entry to
`manifest/designs/loom.md`'s "Crash recovery — resume on output files, not live processes"
section, immediately after the existing crash-mid-registration entry, matching that section's
established voice and level of detail: what the gap is, why it exists (the deliberate,
already-tested `fable5-high-r2` design choice it builds on), the concrete consequence for both
production call sites (`burlerengine`'s cluster-fan `Burler` rounds — permanent leak; `websterengine`'s
Master row — bounded but still forces a redundant re-run), and why it stays documented rather
than fixed. Commit: `loom: fix F1 (r8) — document the AuditForks-failure orphan as a third
Accepted residual`.

**Verification:** doc-only change. `go build ./...` and the full `go test ./...` both stay green
(re-confirmed after this commit — see Test commands below). No markdown-link-integrity test
exists for this file's own internal anchors beyond the existing `#crash-recovery--...` heading,
which this change does not touch or rename.

**No regression test added:** this is a documentation fix recording a known, deliberately
unfixed gap — there is no code behavior to pin with a test, and inventing one would misrepresent
an accepted tradeoff as a guarded invariant it is not.

## F2 — `validate-discussion`/`validate-plan` use the full `wire()` (code-fixed)

_(status: in progress — see commit log for the exact change; filled in as it lands)_

## Test commands run (fixer phase)

_(filled in as fixes land)_

## Deferred items

None beyond F1's documented residual above, which is deferred by design (a genuine architecture
tradeoff), not by omission — see its own section for the full reasoning.
