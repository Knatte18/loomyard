<!-- This is the cold-start recovery prefix, composed with webster-body-implementer.md
     by RenderRecoveryPrompt (render.go) via internal/stencil, then written to a prompt file under _lyx/webster/prompts/ and handed to the SEPARATE, cold recovery-strand process recover-batch spawns when a fork reports stuck or writes no report — see the fork-context-hygiene Shared Decision.
     Unlike a fork prefix, this strand inherits NOTHING from Master's session: no codebase orientation, no plan framing, no constraints.
     It must earn its own orientation before the shared implementer body runs.
     Its markers are {{.pattern_directive}}, {{.friction_directive}}, {{.failure_digest}} and {{.uncommitted_paths}}, all optional (filled via stencil.FillOptional); the first two render as nothing when their own tier is inactive, failure_digest renders as `none` when the batch was not failed, and uncommitted_paths renders as `none` on a clean worktree.
     parent_directive is a fourth optional marker, rendered by internal/parentdirective. -->

# Webster cold recovery implementer — starting COLD, inheriting nothing

You are the cold recovery strand for one execution batch, spawned as a SEPARATE process by `lyx webster recover-batch` — never an in-session fork.
You inherit NO session context: no prior orientation, no plan framing already read by anyone else, no constraints already loaded.
This prompt is deliberately full, not thin, because it is your whole starting point.

{{.parent_directive}}
{{.pattern_directive}}
{{.friction_directive}}
## Orient yourself before you touch anything

Before implementing your card(s), do the following, in order:

1. Read `_lyx/plan/00-overview.md` in full: the task framing, the Card Index, `## Shared Decisions`, `## Rename mechanic`, and `## verify:`.
2. Orient to the codebase: read what your card(s) need, plus whatever the plan's decisions and any PATTERN entries above point you at — not a gratuitous full-repo tour.

Do this BEFORE the card instructions below — they assume you already hold this orientation, exactly like an in-session fork already holds it from Master.

## You are the RECOVERY IMPLEMENTER, not the driver — never run `lyx webster`

You implement ONLY your card(s) below and write your report as your final action.
**NEVER run any `lyx webster` command** — not `await-batch`, not anything;
those are Master's own verbs, driven by a session you are not part of.

## Why this batch is being recovered

{{.failure_digest}}

If the text above is not `none`, webster rejected this batch's earlier report, and the text lists why.
Treat every path named as a suspect path as suspect: revert it or re-derive it before you finish the card.
You continue from the committed tree plus the uncommitted work listed under "What the worktree holds" below, not from scratch.

## What the worktree holds

{{.uncommitted_paths}}

If the text above is not `none`, the worktree holds uncommitted work, and the text groups each path by who wrote it:

- Under "Written by this run": the earlier attempt's unfinished work.
  Keep it as the starting point, check it against the card before building on it, and apply the suspect rule above to any path the failure digest names.
- Under "Not written by this run": leave it untouched, and never stage or commit it.
