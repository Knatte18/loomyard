# Plan: lyx mux remove errors when it empties the last session

```yaml
task: "lyx mux remove errors when it empties the last session"
slug: "mux-remove-last-pane-error"
approved: false
started: "20260715-073202"
parent: "cluster-fork-spike"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: remove-empties-session
    file: 01-remove-empties-session.md
    depends-on: []
    verify: go test -tags "integration smoke" ./internal/muxengine/ ./internal/muxcli/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: classify emptied-session via has-session re-probe, never error-text

- **Decision:** When `reconcileApplyPersistLocked` returns `applyErr` inside
  `RemoveStrand`, decide "the removal emptied the session/server" by re-probing
  `e.psmux.hasSession(e.SessionName())` — never by string-matching the psmux
  error text.
- **Rationale:** The bug was caused by *predicting* psmux behavior (the
  remain-on-exit-corpses-last-pane assumption) and being wrong. The fix keys off
  an observed fact: psmux/tmux `has-session` exits 1 for "no server running",
  which `hasSession` (`overlay.go:90`) maps to `(false, nil)` — the same exit-1
  the reproduction showed from `listPanes`. This reuses the exact classification
  `requireSessionLocked` already relies on, and is backend-agnostic.
- **Applies to:** all batches

### Decision: "expected terminal state" = session gone AND no remaining non-hidden strand

- **Decision:** The success-swallow fires only when the session is confirmed
  gone AND no strand left in `st.Strands` (after `removeStrandLocked`) is
  non-hidden — i.e. every remaining strand has
  `Display.Anchor == render.AnchorHidden` (the zero-strands case included).
- **Rationale:** A hidden strand never owns a live pane, so removing the sole
  visible strand legitimately kills the session even when hidden strands remain
  — still an expected terminal state. If a non-hidden strand remained it should
  still own a live pane, so a dead session there is a genuine, surfaceable
  error. Reuses `anyPlacedStrand`'s existing `Anchor != render.AnchorHidden`
  filter (`apply.go:90`) rather than a second, driftable classification.
- **Applies to:** all batches

### Decision: persist the emptied state on the swallow path

- **Decision:** On the success-swallow path, call
  `SaveState(e.layout.DotLyxDir(), st)` before returning success; fail the op
  only if `SaveState` itself errors.
- **Rationale:** `removeStrandLocked` prunes `st.Strands` in memory only; the
  sole `SaveState` for a remove is at the end of `reconcileApplyPersistLocked`
  (`spawn.go:203`), which is skipped when `listPanes` fails first. Swallowing
  the error without persisting would leave the removed strand in `mux.json`, and
  `lyx mux resume` would resurrect it — a data-integrity bug.
- **Applies to:** all batches

### Decision: pure decision helper + integration test (no psmux interface refactor)

- **Decision:** Extract the classification into a pure unexported helper,
  hermetically unit-tested; add an `//go:build integration` end-to-end
  regression that drives a real Engine and self-skips without the binary. Do
  NOT refactor `Engine.psmux` (`lock.go:38`) from a concrete `PsmuxCmd` into an
  interface.
- **Rationale:** `Engine.psmux` is concrete, so the full `RemoveStrand` path
  crosses `reconcileApplyPersistLocked` — "the one seam every public op's
  hermetic unit test must not cross" (`doc.go`). The pure helper keeps the
  decision logic tested everywhere (incl. tmux-less CI); the integration test
  pins the real tmux wire behavior. An interface refactor would touch every
  psmux call site — out of proportion and against the seam rule.
- **Applies to:** all batches

### Decision: the last-pane assumption is BINARY-DEPENDENT, not universally false

- **Decision:** The remain-on-exit-corpses-the-last-pane assumption is **not**
  universally false — it is **binary-dependent**, and the correction must say so
  per backend, in the same commit:
  - **tmux** (the reproduction backend; PATH-resolved default on POSIX per
    `template_posix.go`): killing a session's *true last* pane **destroys the
    session** (and, if it was the server's only session, the server exits). This
    is what the bug's `exit status 1: no server running` repro observed.
  - **psmux** (Windows default): **corpses** the last pane as `pane_dead=1`,
    keeping the session alive — this is **verified**, not unverified, by the
    committed smoke test
    `internal/muxcli/smoke_lifecycle_test.go::TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`
    (remove of the sole strand returns 0, then a subsequent `add` — which calls
    `requireSessionLocked` and never re-boots — yields a *live* second strand,
    which can only hold if the session survived).
  - The code fix (re-probe `hasSession`; swallow only when the session is
    *confirmed* gone) is correctly backend-agnostic: on tmux the session is gone
    so the swallow fires; on psmux the session survives, `reconcileApplyPersistLocked`
    succeeds, and the swallow is never consulted. Only the *docs/comment framing*
    was wrong (declaring the corpse claim universally false and calling psmux
    "unverified"); the correction states both behaviors explicitly and reconciles
    all three in-tree encodings of the claim (`strand.go` comment, `doc.go`
    assumptions list, and the smoke test's comment).
  - The `mux-server-crash` connection remains an unverified note only — a
    separate investigation, not opened here.
- **Rationale:** CLAUDE.md requires docs updated in the same commit as the
  behavior change, and this is exactly the cross-platform assumption `doc.go`'s
  list exists to track. A committed, authoritative smoke test already pins the
  psmux side, so labeling it "unverified" and the assumption "false" would leave
  a second contradicting encoding in the tree and mis-state a verified fact.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across
every batch, sorted alphabetically._

- `internal/muxcli/smoke_lifecycle_test.go`
- `internal/muxengine/contract_integration_test.go`
- `internal/muxengine/doc.go`
- `internal/muxengine/strand.go`
- `internal/muxengine/strand_test.go`
