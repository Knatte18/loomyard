# Batch: docs-lifecycle

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: docs-lifecycle
number: 7
cards: 3
verify: null
depends-on: [6]
```

## Batch Scope

The documentation lifecycle for this landing: delete the two module docs (`shuttle.md` —
design now built; `mux.md` — as-built doc that has already rotted, per operator
directive), retarget every inbound link, update `overview.md` (module table + execution
stack) and `roadmap.md` (milestone 10 ✅), and record the new provider-seam invariant in
`CONSTRAINTS.md`. Durable design already lives in the package headers written by batches
2–5; this batch only moves the shared-reference layer.

## Cards

### Card 25: delete module docs and retarget inbound links

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `docs/modules/loom.md`
  - `docs/modules/review.md`
  - `docs/modules/README.md`
  - `docs/research/mux-exploration.md`
  - `docs/research/mux-hooks-exploration.md`
  - `docs/research/mux-proposal.md`
  - `docs/reviews/README.md`
  - `docs/reviews/mux-review-prompt.md`
- **Creates:** none
- **Deletes:**
  - `docs/modules/shuttle.md`
  - `docs/modules/mux.md`
- **Moves:** none
- **Requirements:** Delete the two files. Then retarget every markdown link that pointed
  at `modules/mux.md` or `modules/shuttle.md` (any anchor): links whose target concept
  now lives in `overview.md` point at `docs/overview.md#modules` (or the execution-stack
  section where that is the better fit); links to deep mux behaviour point readers at
  the `internal/muxengine` godoc instead (plain-text reference, since godoc has no
  stable markdown URL); links to shuttle design point at the `internal/shuttleengine`
  package header the same way. In `loom.md`/`review.md` keep the surrounding prose
  intact — only the link targets change; where a sentence says "see shuttle.md" reword
  minimally to "see the `internal/shuttleengine` package documentation". Grep for
  `mux.md` and `shuttle.md` across `docs/` and the repo root after editing to confirm
  zero dangling references outside `_mill/` (CONSTRAINTS.md is card 27's edit; leave its
  mux.md reference to that card).
- **Commit:** `docs: delete shuttle.md/mux.md per documentation lifecycle, retarget links`

### Card 26: overview module table, execution stack, roadmap milestone 10

- **Context:**
  - `internal/shuttleengine/doc.go`
  - `internal/shuttleengine/claudeengine/doc.go`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `overview.md`: add shuttle to the module table (packages
  `internal/shuttleengine` + `internal/shuttleengine/claudeengine` +
  `internal/shuttlecli`; one-paragraph as-built summary: one agent run as an interactive
  psmux strand over the file contract; Stop-hook completion via events file; four
  outcomes done/asking/died/timeout with asking as the escalation channel; PreToolUse
  guardrails — Agent always, AskUserQuestion when autonomous; Interrupt/Send; engine
  seam with Claude as the only v1 engine) and update the execution-stack section so
  shuttle shows ✅ between mux and review. `roadmap.md`: flip milestone 10 to
  ✅ **Done** with a pointer to the overview module entry (the module doc is deleted);
  update the "Build order" spine line (`proc ✅ ──▶ mux ✅ ──▶ shuttle ✅ ──▶ review ──▶
  loom`) and the "immediate front" paragraph to name `review` as next. Do NOT add any
  other roadmap notes (roadmap is planned-milestones only).
- **Commit:** `docs: shuttle in overview module table + roadmap milestone 10 done`

### Card 27: CONSTRAINTS — provider-seam invariant

- **Context:**
  - `internal/shuttleengine/seam_enforcement_test.go`
  - `docs/overview.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new short section "Shuttle Provider-Seam Invariant": provider
  specifics (CLI flags, hook schema/steer texts, TUI markers, key choreography) live
  ONLY under `internal/shuttleengine/claudeengine`; `internal/shuttleengine` and
  `internal/muxengine` stay provider-invariant (`shuttleengine` never imports
  `claudeengine`; wiring happens in `internal/shuttlecli`); enforced by
  `internal/shuttleengine/seam_enforcement_test.go` on every `go test`, plus review
  discipline for the semantic half (no Claude marker strings outside claudeengine).
  While editing, also fix the file's existing reference to
  `docs/modules/mux.md#attach-is-a-documented-envelope-exception` (deleted in card 25)
  — repoint it at the `internal/muxcli` attach command's godoc/`Long` and
  `docs/overview.md#modules`.
- **Commit:** `docs(constraints): record shuttle provider-seam invariant`

## Batch Tests

`verify: null` — pure documentation batch with no runnable surface of its own; the
seam-enforcement test named by card 27 already runs in batches 3–6 and via the overview's
module-wide `go test ./...` boundary check, and card 25's zero-dangling-references check
is a grep the implementer runs and reports in its notes.
