# loom: Planner producer

> **Status: Design — not built.** Split out from [loom.md](loom.md) — the Planner producer is a
> separable unit (a prompt/profile, not a module), distinct from the phase-machine core loom.md
> now focuses on. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle),
> when this lands the durable parts fold into the relevant package doc and this file is deleted.

## What it does

"Read `discussion.md`, write a [plan-format v3](plan-format-v3.md) flat card list." Nothing
else. Like the Discussion producer (already built), this is **not a module** — just a prompt +
profile fed to `shuttle.Run`, one `shuttle.Run` → one artifact. No human interaction: autonomous,
no inputs beyond `discussion.md`. Review is `perch`/`burler`'s job entirely separately — the
Planner producer has no review logic of its own (see [loom.md](loom.md#the-gate) for the
black-box gate every producing phase shares).

## No manifest/CONTEXT.md artifact

A "living vision" document (Matt Pocock–style `CONTEXT.md`/ADR split) was explored at length and
**deliberately rejected** for this project: raddle (code-derived, snapshot-tracked) already owns
"what IS," and `CONSTRAINTS.md`-equivalent files already own "what must remain true" — no
separate "what SHALL become" artifact was judged necessary given code + existing conventions
already cover the need. The Planner producer's only input is `discussion.md`; its only output is
the plan-format-v3 card list.

## Related

- [plan-format-v3.md](plan-format-v3.md) — the schema this producer writes against.
- [loom.md](loom.md) — the phase machine this producer is one phase of.
