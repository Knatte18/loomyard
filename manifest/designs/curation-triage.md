# board — curation/triage automation (DRAFT — deferred out of `board: move storage to weft:main`)

> **Status: Someday, deferred.** This is a stub, not a settled design — carried forward from `board-weft-storage.md`'s now-deleted "Curation flow" section (its host design, `board: move storage to weft:main`, shipped without this piece). Do not implement from this doc yet; it captures the substance of what was scoped and deferred so the idea isn't lost, not a ready-to-build spec.

## What already shipped (out of scope here)

`board: move storage to weft:main` delivered the mechanical primitives this item would automate on top of:

- `notes.json` and `tasks.json` share one schema and one global slug namespace — raw/uncurated entries live in `notes.json`, claimable work lives in `tasks.json`.
- `promote-note` is a plain, mechanical, human-triggered CLI command that moves one entry from `notes.json` into `tasks.json`.

Neither of those is this stub's remaining scope. This item is the **automation layer** on top of that already-shipped primitive — GitHub-issue ingestion and periodic/triggered triage — not the primitive itself.

## Curation flow (carried forward from `board-weft-storage.md`)

- **Anyone can add a raw note, no intake gatekeeping.** Any worktree's LLM session can add one directly; a human can add one via a GitHub issue on the `weft` repo. Requiring every spontaneous idea to go through a single owner would create friction and lose ideas from discusser/planner sessions in other worktrees.
- **Only the orchestrating thread (running in prime) curates notes.** This is where LLM judgment is actually needed — is this note coherent, well-formed, worth keeping — and where consistency of voice/format matters.
- **Task extraction from the manifest is a deliberate, explicit, human-triggered command** (a skill, invoked by the operator), not an autonomous background loop. It promotes a note into a task via `promote-note`, consistent with this project's general pattern of starting cautious and only removing human-in-the-loop steps once behavior is observed and trusted.

## What this item still needs to design

- The GitHub-issue-intake mechanism itself: how an issue on the `weft` repo becomes a `notes.json` entry (webhook? polling? a `lyx board` subcommand an operator or a scheduled job runs?).
- The periodic/triggered triage workflow: what "extract a logical next task from the manifest" means as a skill an operator invokes, and how it selects which note(s) to act on.

Both remain undesigned; this doc exists to record the deferral, not to specify the mechanism.
