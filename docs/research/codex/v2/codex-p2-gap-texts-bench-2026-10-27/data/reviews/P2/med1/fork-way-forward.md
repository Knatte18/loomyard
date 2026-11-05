Refusal and recovery paths checked:

- `plan-language-mismatch` and Go `glyph-malformed`: set `language: csharp` or `language: none`, as appropriate.
  No failing precondition found in the prescribed plan-edit route.
- `rename-namespace-pair`: replace the namespace pair with Edit cards, or correct the old filename.
  Both address the predicate the card specifies.
- Cross-alphabet ambiguity: name a file or member instead of the ambiguous self glyph.
  No competing instruction found.
- C# `rename-old-unresolved`: correct the new handle spelling, or drop an unsupported pair.
  No recovery command is required before execution.
- `resign-head-mismatch`: preserve parameter-list presence or express a kind change through Delete and Create.
  The latter provides a route when the intended operation exceeds a re-sign.
- `resign-overload-collision`: choose a free signature, remove the conflicting overload earlier, or drop a duplicate declaration.
  These correspond to the planned collision predicates and exceptions.
- `resign-not-done`: correct the code and recover the batch, or correct the card head, rebaseline that card, then recover.
  `failCardNotDone` leaves the batch terminal failed; `Rebaseline` accepts named edits to that failed batch; recovery reads the edited plan and repeats the shared post-batch checks.
  Its archived report does not obstruct recovery.
- `rename-tool-missing`: configure `rename_tools` through `lyx config language` from the prime, or recast the card, then retry.
  This is the existing refusal and route.

**No findings retained for the way-forward lens.**

Additional reads, one line per tool call:

1. Recovery/rebaseline entry-point searches, root pre-run, and `refusal-spec.md`: inherited context lacked the actual recovery command contracts.
2. `failCardNotDone`, `Rebaseline`, recovery command setup and spawning, rebaseline CLI, root construction: traced the proposed correction through failed-batch state and command ordering.
3. Recovery spawn and terminal persistence, plus wrapper searches: checked report archival, recovery prerequisites, and repeated post-batch checks.
4. Webster pre-run and `executionBatches`, plus refusal/fingerprint searches: checked command wiring and the source of recovery’s batch selection.
5. `BatchFailedError`, `failBatch`, `renameToolRefusal`, fingerprint guards/restamping: checked complete rendered errors and the state preserved for retry.
6. `RewriteRefs`, root stencil-seeding skips, and existing edited-card recovery test location: checked the rewrite primitive and remaining root pre-run bypasses.
