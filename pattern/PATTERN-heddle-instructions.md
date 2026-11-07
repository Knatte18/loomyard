# PATTERN-heddle-instructions

Heddle entries are rendered only via `heddleengine`, and lym's own instructions live in its code, stencils, specs or heddle's shipped layer.

## Heddle rendering

Rule: Rendering heddle entries into a file or prompt: only via `heddleengine`, from its shipped layer, Bolt's `heddle.json` and, under `lym_override`, lym overrides; `heddle.json` written only via `lym heddle` or its replayer; fabric names no heddle file.

Heddle's instruction text is stored once per layer and rendered from those stores, never from a copy.

- The shipped layer is `shipped.json`, embedded in `internal/heddleengine`.
  It holds lym's own entries and changes only with a lym release.
- The repository layer is `heddle.json` at the Bolt checkout's root.
- A hub's lym overrides sit in `heddle-lym-override.json` beside it.
  A render reads them only while the hub's `heddle.yaml` sets `lym_override`.
- The `lym heddle` verbs and heddle's Bolt replayer are the only writers of `heddle.json`.
  Another module's verb, such as `lym orch rules`, writes it only through the write path the `lym heddle` verbs share.
- The verbs alone write `heddle-lym-override.json`, which the replayer leaves unclaimed.
  Fabric therefore replays a commit of it as a plain cherry-pick.
- No verb writes a shipped entry.
- `internal/heddleengine` is the only package that reads the layers and renders them.
- It renders file targets at a worktree's anchor, beside the repository's own CLAUDE and AGENTS files.
  It does so through fabric's worktree seam or `lym heddle render`.
- It renders a session's appended system prompt through shuttle's prompt seam.
- Fabric hands heddle paths and events and names no heddle file.
- Heddle's file targets are the one place a per-worktree file lym seeds is declared, so a new such file is a new target.
  Its path joins the exclude list in fabric's config template,
  and a hub that sets its own list gains the path through a config reinterpretation.
- Claude Code's worktree settings, `.claude/settings.local.json`, render from the `claude-settings` target.
  The shipped layer holds lym's hub-wide Claude settings there, and a repository's entries add to them.
  The effort defaults come from `agents.yaml` through an entry heddle derives at each render, never from a copy in an entry.
  A setting every session of a worktree shares lives only there;
  a setting that differs per session or role lives only in the `--settings` document a session is launched with.
- `HEDDLE.md` at the Bolt checkout's root shows every entry heddle holds.
  Only `heddleengine.RenderOverview` writes it: the `lym heddle` write path after each save, and heddle's Bolt replayer after each replay.

## Lym instructions in code

Rule: Writing lym's own instructions to agents: in lym's code, its stencils and specs, or an entry of heddle's shipped layer in `internal/heddleengine`; never in a hub's `heddle.json`, which holds only a repository's own instructions.
