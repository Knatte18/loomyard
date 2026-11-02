# PATTERN-heddle-instructions

Heddle's instruction text is stored once per layer and rendered from those stores, never from a copy.

- The shipped layer is `shipped.json`, embedded in `internal/heddleengine`; it holds lym's own entries and changes only with a lym release.
- The repository layer is `heddle.json` at the Bolt checkout's root.
- A hub's lym overrides sit in `heddle-lym-override.json` beside it, and a render reads them only while the hub's `heddle.yaml` sets `lym_override`.
- The `lym heddle` verbs and heddle's Bolt replayer are the only writers of `heddle.json`; another module's verb, such as `lym orch rules`, writes it only through the write path the `lym heddle` verbs share.
- The verbs alone write `heddle-lym-override.json`, which the replayer leaves unclaimed, so fabric replays a commit of it as a plain cherry-pick.
- No verb writes a shipped entry.
- `internal/heddleengine` is the only package that reads the layers and renders them.
- It renders file targets at a worktree's anchor, beside the repository's own CLAUDE and AGENTS files, through fabric's worktree seam or `lym heddle render`.
- It renders a session's appended system prompt through shuttle's prompt seam.
- Fabric hands heddle paths and events and names no heddle file.
