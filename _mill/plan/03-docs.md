# Batch: docs

```yaml
task: 'loom: Preflight phase (precondition validation)'
batch: docs
number: 3
cards: 2
verify: null
depends-on: [2]
```

## Batch Scope

Documentation updates completing the same-task doc obligation (Documentation Lifecycle): mark
Preflight as built in the loom design doc and roadmap, and realign the pinned `status-schema.md`
wording that this task's strict-parse and presence-handling decisions clarified. Pure prose; no
runnable surface. Split from the code batches only to keep code and doc diffs separate — it is the
same task, squash-merged to `main` in one commit, so the "docs in the same commit as behaviour"
constraint holds at the merge.

## Cards

### Card 9: mark Preflight built in loom.md and roadmap.md

- **Context:**
  - `internal/loomengine/preflight.go`
  - `docs/reference/status-schema.md`
- **Edits:**
  - `docs/modules/loom.md`
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/modules/loom.md`, update the Preflight description (the phase-machine
  section and the module-decomposition table row `| Preflight | uses existing modules | ...`) to
  record that Preflight is now implemented as `internal/loomengine.Preflight`, validating the four
  preconditions (geometry+worktree-root, clean host worktree, weft paired & in sync, seed exists &
  coherent) over git/filesystem state, engine-only (no cobra module yet). In `docs/roadmap.md`,
  mark milestone 12 build-order **build-piece #2 (Preflight)** ✅ Done, with a one-line pointer to
  `internal/loomengine`. Do not add roadmap notes beyond marking the planned sub-milestone done
  (per the repo's roadmap discipline).
- **Commit:** `docs(loom): mark Preflight built in loom.md and roadmap`

### Card 10: realign status-schema.md wording

- **Context:**
  - `internal/loomengine/status.go`
  - `internal/loomengine/coherence.go`
  - `internal/state/state.go`
- **Edits:**
  - `docs/reference/status-schema.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two wording fixes in `docs/reference/status-schema.md`: (1) the **"Parse
  discipline"** paragraph currently describes the JSON status.json strict parse as "the same
  `KnownFields(true)` discipline" (a `yaml.Decoder` API) — realign it to
  `json.Decoder.DisallowUnknownFields()`, the correct API for the JSON seed, matching
  `state.ReadJSONStrict`; (2) the **validation-checklist item 1** ("Required fields … are
  present") — clarify that an absent nullable/bool/slice field (`start_sha`, `next_action`,
  `pause_requested`, `history`) satisfies "present" via its zero/null semantics, and only the
  mandatory string fields (`slug`/`parent`/`phase`/`stage`/`narration`) are structurally
  presence-enforced (matching `loomengine.checkCoherence`). Do not otherwise alter the pinned
  schema.
- **Commit:** `docs(status-schema): realign parse discipline to DisallowUnknownFields and clarify presence`

## Batch Tests

`verify: null` — this batch edits only Markdown documentation (`docs/modules/loom.md`,
`docs/roadmap.md`, `docs/reference/status-schema.md`). There is no runnable surface, no code
change, and no test to scope. Correctness is a review obligation (the plan reviewer / code reviewer
confirms the prose matches the shipped `loomengine` behaviour), consistent with the Documentation
Lifecycle.
