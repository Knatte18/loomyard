# Stencil intent audit, 2026-10-15

An audit of every stencil under `contracts/stencils/` for wrong design assumptions, run for the board note `stencil-intent-audit`.
Line numbers in both files refer to `main` at `aa925bdcc554c36ec01aa7bf6e12311dae8ba9cc`.

- `catalogue.md` lists every sentence that states a design assumption, per stencil.
- `findings.md` holds the flagged entries, F0 to F17, each with `file:line`, a restatement and the flag reason.

The review with the operator has not started: no entry has been confirmed or rejected.

## Resuming

1. Check each finding's lines against current `main`, since stencils may have moved since the commit above.
2. Show the operator the flagged entries in short batches, in Norwegian, short and plain, with terms explained, for the operator to confirm or reject.
3. Each rejected entry becomes one board note through `lyx board upsert` with `"kind":"note"` and the labels `bug`, `stencil` and the module labels.
   Record only what the operator decides as a decision; label an operator remark that is not a decision "Operator view, not a decision", and label the agent's own suggestions as its own.
4. Mark each entry's outcome in `findings.md`.
