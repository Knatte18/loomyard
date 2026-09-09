# LLM-driven orchestration as an alternative (or supplement) to `loom`'s Go phase machine

> **Status: Speculative, not scoped.** Recorded from a 2026-09-09 conversation so the idea and its reasoning aren't lost — not a commitment to build it. See the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle): if this is ever picked up, fold the durable parts into `manifest/designs/loom.md` or `docs/overview.md`; if abandoned, delete this file.

## The concern this responds to

`loom`'s whole design bet — stated outright in `docs/overview.md`'s Principle 7 ("Go where it can be; LLM only for judgment") — is that phase sequencing, gating, and retries should be deterministic Go, not something an LLM orchestrator decides turn to turn. Millhouse (the Python predecessor `loom` is meant to replace) worked the opposite way: an LLM was *always* the orchestrator, present continuously, and could notice and fix a small defect on the fly, mid-flow, without waiting for a defined checkpoint to catch it.

The concern raised: `loom`'s gates are only as good as what they were specifically built to check. Anything that falls between two gates — a defect no reviewer rubric named, no test asserts, no validator flags — passes through completely, where a continuously-present LLM orchestrator might have noticed it by osmosis.

## The evidence this isn't a hypothetical worry

The `crucible-loom-glyph-hardening` campaign (10 rounds, 5 models, see `_mill/loom-review-HANDOFF.md` at `archive/crucible-loom-glyph-hardening` — deleted from the branch pre-merge per this repo's convention, recoverable from that tag's history) is itself an instance of "an LLM orchestrator watching live" — each round's human+Claude orchestrator pair drove a fresh reviewer/fixer agent and verified its work.

That campaign's own findings cut both ways on the question:

- **For continuous supervision mattering**: rounds 5, 6, and 7 each found a real defect *inside the previous round's own fix*, in the exact same provider-startup-gate seam, four rounds in a row. A single live-watching orchestrator, however attentive, did not catch it the first time, the second time, or the third time.
- **Against treating continuous LLM supervision as sufficient on its own**: it took *four independent rounds*, with *rotated models*, to finally close that seam — not one attentive orchestrator noticing it in the moment. The thing that actually converged it was the same discipline `loom`'s Go gates encode structurally: independent re-verification, never trusting a self-report, repeated until clean.

Read together: an LLM's continuous presence catches things a fixed rubric didn't anticipate; but a *single* LLM's continuous presence is not reliably enough by itself — the crucible campaign needed the same "never self-grade, verify independently, repeat" discipline `loom`'s own gates already encode, just applied by rotating LLM judgment instead of by a Go retry loop.

## The idea: neither pure Go nor pure LLM — both, at different layers

Not proposed: ripping out `loom`'s Go phase machine and replacing it with an LLM sequencer wholesale. That throws away exactly what Go gives for free — a check either ran or it didn't, auditable, with zero chance of "the orchestrator forgot to look."

What was actually drafted (see "The full prompt" below): an LLM plays `loom`'s sequencing role directly — driving `board`/`fabric`/`reed`/`shuttle`/`webster`/`burler`'s own primitives phase by phase, in the same order `manifest/designs/loom.md`'s 15-row producer table already defines — but:

- Every row already marked **mechanical** in that table (`Preflight`, `Loom-Preflight`, `Discussion-Validate`, `Plan-Validate`/`Revalidate`, `Batchifier`) stays a plain Go verb call the LLM orchestrator trusts outright, never re-judged. No reason to spend LLM judgment on something already deterministic and free.
- Every row that already needs an LLM (`Discussion-Write`, `Plan-Write`, the three review segments, `Webster`'s batch loop) gets the SAME clean-room / never-self-grade discipline `loom`'s Go-driven review segments and crucible's method both already enforce.
- The one thing added on top, which Go structurally cannot offer: the orchestrating LLM stays *present* — reads every artifact before advancing, watches `Webster`'s batch implementer live via `lyx reed attach` rather than firing-and-forgetting, and is explicitly told to flag anything that looks wrong even if it technically passed its check.

This is closer to "run `loom`'s own phase list by hand, with a human+LLM pair supervising it live" than to "replace loom." The Go gates are not weakened; a live layer is added on top of them, in the one place — continuous attention — Go cannot provide.

## The full prompt (as drafted)

A complete, paste-ready "you are acting as Loom" prompt was written against `manifest/designs/loom.md`'s exact 15-row producer table, porting crucible's clean-room and independent-verification rules onto each LLM-necessary phase. It is not committed anywhere yet — recovered from this conversation's own transcript if this idea is picked up. Rewrite it against `loom.md`'s table at pickup time rather than trusting a stale copy, since that table is itself flagged as living documentation that may have moved past this doc.

## Open questions (genuinely unscoped)

- **Is this ever worth building as a standing mode**, or is "attach and watch while `lyx loom run` executes" (the existing capability, no new machinery) already good enough for the continuous-attention gap? The crucible evidence above suggests continuous presence alone doesn't reliably close gaps *either* — so the marginal value of a full LLM-driven sequencing loop over "loom runs normally, a human watches via attach" is not established.
- **Cost.** An LLM re-deciding "run the next phase now" at every step burns tokens/turns a Go loop does for free. Not measured here.
- **Where this would live if built for real** — a new mill-wiki-driven mode, a documented alternative invocation, or purely an ad hoc prompt an operator pastes when they want it — not decided.

## Related

- [loom.md](loom.md) — the producer table and phase details this idea drives by hand instead of via `internal/loomengine`.
- `crucible/README.md` and `crucible/orchestrator-prompt.md` — the method this idea's discipline is ported from, and the evidence (the R5-R7 provider-startup seam) informing the "not sufficient alone" caveat above.
