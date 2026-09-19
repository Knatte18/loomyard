# Batch: docs

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
batch: 'docs'
number: 5
cards: 2
verify: null
depends-on: [3]
```

## Batch Scope

This batch delivers the task's documentation surface: the design doc's watchdog section gains the orphan-reap rule and drops its forward reference to this item as future work, and the roadmap item moves from Planned to Done.
It is one batch because both cards are prose edits to `manifest/` describing the same shipped behaviour, with no runnable surface between them.
It depends on batch 3 — the behaviour being documented is the wired daemon, not the seams — and it is independent of batch 4, so the two can run in parallel.

Batch-local decision beyond `## Shared Decisions`: this batch touches neither `docs/overview.md` nor `CONSTRAINTS.md`. The task adds no module and changes no execution stack, so the module table is unchanged, and the rules it introduces are module-local to reed and belong in the design doc rather than as a new cross-cutting invariant.

## Cards

### Card 20: design doc gains the orphan-reap rule

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/spawnwatchdog.go`
  - `internal/reedengine/overlay.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/reed-header-selvage.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the `### Watchdog daemon → a detached, per-hub background process` section with the orphan-reap rule, written as prose continuing that section's existing voice rather than as a bulleted list of decisions.

  Cover, in the section's own terms: that each discovery cycle also decides whether every live session name's `filepath.Join(hub, name)` is still a live directory; that the gone verdict is only-proven-gone — a not-exist result or a successful stat of a non-directory — with every other stat outcome, including a permission or I/O error, counting as not gone; that the hub directory itself is probed once per cycle first and a non-live hub skips the reap pass entirely without advancing, resetting or pruning any counter, because an unmounted or moved hub makes every name look gone at once while the socket keeps listing every session; that a name must be observed gone on `watchdogOrphanGoneCycles` (3) consecutive affirmative cycles before it is reaped, with a non-affirmative cycle leaving every counter untouched rather than resetting or advancing them; that the reap captures the session's pane process closure **before** issuing an exact-match `kill-session` and reaps that closure afterwards, which is `Engine.Down`'s own ordering and the reason the agent processes actually die rather than being reparented; that it runs in its own goroutine with a loop-owned in-flight set excluding the name from both re-selection and the appeared set until it finishes; that `reedengine.ReapSession` is the second engine-less exported function in that package and holds no lock and persists nothing; and that `--shell` is told to the daemon by the same spawn construction that tells it `--tmux`, accepted on every platform but never required, degrading the reap rather than refusing to start when empty.

  State the two accepted limitations plainly, since both are operator-visible: an orphan on a hub with no running daemon survives until the next lyx command against that hub re-spawns one, and a hand-made session on the hub's socket with no matching directory is reaped ~15s later.

  Update the `## Related` section by removing only its last bullet, the one forward-referencing this item as future work. The bullet above it, referencing the loom-step design and the born-as-strand item, stays exactly as it is, and the section itself stays.
- **Commit:** `docs(reed): document the watchdog daemon's orphan-session reap`

### Card 21: roadmap item moves to Done

- **Context:**
  - `manifest/designs/reed-header-selvage.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move the `reed: per-hub daemon reaps orphaned sessions` item out of the `## Planned` section and into `## Done`, as the first entry under that heading, matching the shape the entries already there use: a bolded title, an em-dash, and one or two sentences in the past tense describing what shipped.

  Rewrite the summary to describe the shipped behaviour rather than the intent the Planned entry stated — the per-hub daemon now checks each live session name's worktree directory every discovery cycle, reaps a session confirmed gone across three consecutive affirmative cycles by capturing its pane process closure and then killing the session by exact target, and refuses to act at all while the hub directory itself does not stat live.

  The `## Done` entries do not carry a `See [designs/...]` line, so the moved entry drops the one the Planned version had; the design doc is reachable from the module's own documentation. Renumber nothing — the file uses the `1.` repeated-marker form throughout, so the surrounding entries need no edit.

  Leave every other item in `## Planned` in its existing order.
- **Commit:** `docs(manifest): move the per-hub daemon orphan reap to Done`

## Batch Tests

`verify: null` — this batch edits two markdown files under `manifest/` and has no runnable surface at all.
Neither file is read by any Go test or by the build;
there is no doc-link checker or markdown lint gate in this repo to run against them.
The behaviour the prose describes is verified by batches 3 and 4, whose own verify commands already ran by the time this batch starts.
