# Batch: tree-sweep

```yaml
task: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep
batch: tree-sweep
number: 3
cards: 6
verify: null
depends-on: [2]
```

## Batch Scope

The tree-wide stale-docs corrections beyond the board config-layer items: overview.md (which
still calls worktree/mux "coming next"/"(sketch)", shows a board-only Structure and dispatch
switch, and lists `internal/state` as existing), roadmap.md and mux.md (which gain
cross-references to the shipped muxpoc POC), state.md (a stale milestone reference),
README.md (the same "layered YAML" wording fixed elsewhere, line 18, plus a planned-library
qualifier on line 21), and benchmarks.md ("YAML layers" wording). Depends on batch 2 because
several cards link to `docs/modules/muxpoc.md`, which batch 2 creates — running after it
keeps the repo's cross-references coherent. Pure Markdown; no `.go` files, so `verify: null`.
Follows `muxpoc-coexist-framing` (no mux milestone is marked Done; mux.md stays "design") and
`preserve-runtime-state-mhgo-references`.

## Cards

### Card 8: overview.md — reflect shipped worktree + muxpoc, fix Structure/dispatch/state

- **Context:**
  - `cmd/mhgo/main.go`
  - `docs/modules/worktree.md`
  - `docs/roadmap.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`: (a) **Intro** (the "the first module, **board** …
  is implemented; **worktree** and **mux** are designed and coming next" sentence): update so
  worktree is **implemented**, the muxpoc **POC** is shipped, and only the clean `internal/mux`
  remains design (link `roadmap.md`). (b) **§Structure** code block: it currently shows only
  `cmd/mhgo/` + `internal/board/` and the line "everything else is `package board` in
  `internal/board`. `main` is the only thing that imports a module." — replace the tree and
  that sentence to reflect the real packages: `internal/board`, `internal/worktree`,
  `internal/muxpoc`, `internal/config`, `internal/git`, `internal/lock`, `internal/output`,
  plus `cmd/mhgo`. (c) **§Module dispatch** `switch` example: it lists only `case "board"` and
  `case "init"`; update it to match `cmd/mhgo/main.go`'s real dispatch (`init`, `board`,
  `muxpoc`, `worktree`). (d) **§Modules** list: change the worktree bullet from "Sketch" to
  implemented; add a **muxpoc** bullet — a shipped proof-of-concept psmux orchestrator,
  linking `modules/muxpoc.md`; keep the mux bullet as design. (e) the line listing shared
  infrastructure "(`internal/config`, `internal/git`, `internal/lock`, `internal/state`)":
  qualify `internal/state` as **(planned)** since the package does not exist yet. (f) **§Other
  docs** list: change the `modules/worktree.md` entry from "(sketch)" to implemented, add a
  `modules/muxpoc.md` entry, keep `modules/mux.md` as the design doc. Do not mark any mux
  milestone Done.
- **Commit:** `docs(overview): reflect shipped worktree + muxpoc, real packages and dispatch`

### Card 9: roadmap.md — note the muxpoc POC across milestones 5–7

- **Context:**
  - `docs/modules/mux.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/roadmap.md`, add a note that a working **muxpoc POC** exists
  (`internal/muxpoc`, documented in `modules/muxpoc.md`) which already proves the risky parts
  of **milestone 6** (subprocess / reviewer panes) and **milestone 7** (daemon crash-recovery)
  ahead of the clean `internal/mux`. Cross-reference `modules/muxpoc.md` from the milestone
  5–7 entries. Do **not** change any mux milestone's status to Done — the clean module is
  still unbuilt (Shared Decision `muxpoc-coexist-framing`). Keep the existing milestone
  numbering and the board/config/worktree "✅ Done" markers unchanged.
- **Commit:** `docs(roadmap): note the muxpoc POC against the mux milestones`

### Card 10: mux.md — cross-reference the muxpoc POC

- **Context:**
  - `docs/roadmap.md`
- **Edits:**
  - `docs/modules/mux.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/modules/mux.md`, keep the Status as "design, nothing implemented
  yet" for the clean `internal/mux` module, but add a cross-reference noting that a working
  proof-of-concept of the daemon / pane-recovery / reviewer-pane model already exists in
  `internal/muxpoc` (link `muxpoc.md`). Do not rewrite the design content or claim mux is
  implemented (Shared Decision `muxpoc-coexist-framing`).
- **Commit:** `docs(mux): cross-reference the muxpoc proof-of-concept`

### Card 11: state.md — fix stale milestone reference

- **Context:**
  - `docs/roadmap.md`
- **Edits:**
  - `docs/shared-libs/state.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/shared-libs/state.md`, the closing note "Exact home is decided
  when milestone 2/3 lands — flagged here so it is not forgotten." is stale: milestone 2
  (config/git/lock) has shipped without extracting `AtomicWrite`/`PathGuard`. Change the
  reference to **milestone 3** (the decision is now deferred to milestone 3 only). Optionally
  strengthen the preceding note with the concrete evidence that `internal/muxpoc` already
  reaches into `internal/board.AtomicWrite`, reinforcing that the helper needs a real home.
  Leave the rest of the file unchanged — in particular keep the correct "(now removed) `.mhgo/`
  config layer" wording and the runtime-state-dir description (Shared Decision
  `preserve-runtime-state-mhgo-references`).
- **Commit:** `docs(state): milestone-2 shipped, defer AtomicWrite home to milestone 3`

### Card 12: README.md — fix "layered YAML config" + qualify planned state lib

- **Context:**
  - `docs/shared-libs/config.md`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/shared-libs/README.md`: (a) the library bullet "[config.md] —
  `internal/config`: layered YAML config, env expansion, `.env` loading" carries the same
  two-layer-vs-multi-layer staleness fixed elsewhere — change "layered YAML config" to
  "two-layer YAML config (defaults + `_mhgo/<module>.yaml`)". (b) the "[state.md] —
  `internal/state`: machine-local runtime state registry" bullet lists a package that does
  not exist yet — qualify it as planned, consistent with overview.md's "(planned)". The
  line-(a) fix is required so the handoff staleness grep (which searches for `layered YAML`)
  returns clean.
- **Commit:** `docs(shared-libs): fix README layered-YAML wording, mark state lib planned`

### Card 13: benchmarks.md — two-layer config wording

- **Context:**
  - `docs/shared-libs/config.md`
- **Edits:**
  - `docs/benchmarks.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/benchmarks.md`, the current-revision text describes config-load
  cost as "deep merge from YAML layers" / "configuration resolution from YAML layers" (in the
  "What the suites mean" note, the re-architecture **Note:**, and the Takeaways). Change these
  to reflect the two-layer model — "defaults + the module's `_mhgo/board.yaml`" (a single
  YAML layer over defaults, not multiple layers). Do **not** edit the "### Pre-config baseline
  — synchronous writes (historic reference)" section — it is explicitly labelled historic and
  must stay as-is.
- **Commit:** `docs(benchmarks): describe config load as two-layer, not YAML layers`

## Batch Tests

`verify: null` — this batch edits only Markdown files (`docs/overview.md`, `docs/roadmap.md`,
`docs/modules/mux.md`, `docs/shared-libs/state.md`, `docs/shared-libs/README.md`,
`docs/benchmarks.md`). There is no runnable surface, so there is no per-round `verify`
command. Correctness is validated by plan/code review and, at handoff, by (1) a markdown
link-resolution pass over `docs/` — especially the new inbound links to `modules/muxpoc.md`
from overview.md/roadmap.md/mux.md — and (2) the staleness grep guard
(`grep -rn "\.mhgo/board\.yaml\|three-layer\|Target redesign\|not yet implemented\|layered YAML" docs/ internal/`)
returning only legitimate runtime-state-dir references and the explicit "now-removed config
layer" mentions.
