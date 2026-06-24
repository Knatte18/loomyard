# Batch: durable-docs

```yaml
task: "ly-git-clone hub-creator (host, weft, board)"
batch: "durable-docs"
number: 2
cards: 2
verify: null
depends-on: []
```

## Batch Scope

Corrects the two durable design docs that contradict the landed decisions: `docs/roadmap.md`
(milestone 6 still describes a `ly-*` skill that wires junctions; the out-of-scope bullet bans
board-URL derivation outright) and `docs/overview.md` (the weft-overlay model places `_board/`
in the weft worktree). Independent of batch 1 — disjoint files, no code, so it carries no
dependency and runs in parallel. `verify: null` because the batch has no runnable surface;
correctness is the plan-reviewer's and holistic reviewer's concern.

## Cards

### Card 7: align roadmap milestone 6 + out-of-scope bullet

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/roadmap.md`, rewrite milestone 6 ("Task 007 — Hub-creator /
  `ly-git-clone`") so it describes a Go **`lyx git-clone`** subcommand (not a `ly-*` skill)
  that creates a **fresh** Hub (`<name>-HUB/`) and clones the host, weft, and board Primes into
  it, with **no** junction wiring (junctions/activation are a separate later step). Remove the
  "wiring the host↔weft junctions", "The clone **skill** is `ly-*`", and "neighbors in an
  existing hub" phrasings; keep the "board-repo creation is milestone 16, not part of this
  clone" note. In the "Explicitly out of scope" section, amend the bullet that currently reads
  "Heuristic inference of home-file content shape and board-URL derivation" to clarify that the
  **deterministic** weft→wiki URL rewrite (`.git`→`.wiki.git`) performed by `lyx git-clone` is
  **in** scope; only *heuristic* inference of board URLs / home-file shape stays out.
- **Commit:** `docs(roadmap): align milestone 6 with lyx git-clone command`

### Card 8: correct board location in overview weft model

- **Context:**
  - `docs/roadmap.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`'s weft-overlay model, change the `_board/` row of the
  "Artifacts location" table so its Location column reads **Hub** (was "Weft worktree"); the
  Repo column stays "Board". In the "Topology" diagram, add `_board/` as a Hub child sibling of
  `<prime>/` and `<prime>-weft/` (e.g. a line `├── _board/   (board repo; the task store)`),
  so the diagram and table agree that the board lives at the Hub top level. Also add the new
  subcommand wherever `docs/overview.md` enumerates the `lyx` subcommand surface — the
  "Module dispatch" switch snippet and any Modules list — so `git-clone` is listed alongside
  `init`/`board`/`config`/`update`/`ide`/`worktree`/`weft` (mirrors the `main.go` package-doc
  update in Card 6). If `docs/overview.md` contains no such dispatch enumeration, skip this
  addition (the table + topology corrections remain required).
- **Commit:** `docs(overview): correct board location to Hub`

## Batch Tests

No runnable surface — this batch only edits Markdown design docs, so `verify: null`. The edits
are validated by the holistic plan review (and, post-implementation, by the holistic code
review reading the diff against the decisions in `discussion.md`).
