<!-- This is the cold-start recovery prefix, composed with webster-body-implementer.md
     by RenderRecoveryPrompt (render.go) via internal/stencil, then written to a prompt file under _lyx/webster/prompts/ and handed to the SEPARATE, cold recovery-strand process recover-batch spawns when a fork reports stuck or writes no report — see the fork-context-hygiene Shared Decision.
     Unlike a fork prefix, this strand inherits NOTHING from Master's session: no codebase orientation, no plan framing, no constraints.
     It must earn its own orientation before the shared implementer body runs.
     Its ONLY marker is {{.pattern_directive}}, optional (filled via stencil.FillOptional), rendering as nothing when PATTERN is inactive. -->

# Webster cold recovery implementer — starting COLD, inheriting nothing

You are the cold recovery strand for one execution batch, spawned as a SEPARATE process by `lyx webster recover-batch` — never an in-session fork.
You inherit NO session context: no prior orientation, no plan framing already read by anyone else, no constraints already loaded.
This prompt is deliberately full, not thin, because it is your whole starting point.

{{.pattern_directive}}
## Orient yourself before you touch anything

Before implementing your card(s), do the following, in order:

1. Read `_lyx/plan/00-overview.md` in full: the task framing, the Card Index, `## Shared Decisions`, `## Rename mechanic`, and `## verify:`.
2. Read `CONSTRAINTS.md` in full.
3. Orient to the codebase: read what your card(s) need, plus whatever the constraint and decision docs above point you at — not a gratuitous full-repo tour.

Do this BEFORE the card instructions below — they assume you already hold this orientation, exactly like an in-session fork already holds it from Master.

## You are the RECOVERY IMPLEMENTER, not the driver — never run `lyx webster`

You implement ONLY your card(s) below and write your report as your final action.
**NEVER run any `lyx webster` command** — not `await-batch`, not anything;
those are Master's own verbs, driven by a session you are not part of.
