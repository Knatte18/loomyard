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
    verify: go test -tags integration ./internal/muxengine/
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

### Decision: docs corrected in the same commit; Windows + crash-link are unverified notes

- **Decision:** The false `kill-pane` comment in `strand.go` and the `doc.go`
  load-bearing-assumptions list are corrected in the same batch. Windows/psmux
  last-pane behavior and any `mux-server-crash` connection are documented as
  unverified notes only — not investigated here.
- **Rationale:** CLAUDE.md requires docs updated in the same commit as the
  behavior change, and this is exactly the cross-platform assumption `doc.go`'s
  list exists to track. Windows verification needs hardware not available; the
  crash link is a separate investigation.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across
every batch, sorted alphabetically._

- `internal/muxengine/contract_integration_test.go`
- `internal/muxengine/doc.go`
- `internal/muxengine/strand.go`
- `internal/muxengine/strand_test.go`
