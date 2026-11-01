# PATTERN-heddle-instructions

Heddle's instruction text is stored once and rendered from that one store, never from a copy.

- The store is `heddle.json` at the Bolt checkout's root.
- The `lym heddle` verbs and heddle's Bolt replayer are its only writers.
- `internal/heddleengine` is the only package that reads the store and renders it.
- It renders file targets at a worktree's root, through fabric's worktree seam or `lym heddle render`.
- It renders a session's appended system prompt through shuttle's prompt seam.
- Fabric hands heddle paths and events and names no heddle file.
