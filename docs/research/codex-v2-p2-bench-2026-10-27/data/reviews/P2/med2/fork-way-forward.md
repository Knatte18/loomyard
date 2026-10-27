Refusal and recovery inventory:

- Card 6, `plan-language-mismatch`: set the matching language or `none`; progresses at the plan gate.
- Card 6, Go `glyph-malformed` for a C# ref: set `language: csharp`; progresses.
- Card 7, `rename-namespace-pair`: express namespace changes as Edit cards, or correct an intended root-file source; progresses.
- Card 8, cross-alphabet ambiguity: name a file or member; progresses without the ambiguous namespace/directory spelling.
- Card 10, rejected Rename handle: use the C# handle spelling; progresses.
- Card 10, a symbol with no substitutable declared identifier: drop the pair; progresses.
- Card 11, `resign-head-mismatch`: preserve the member shape, or use Delete and Create for a kind change; progresses.
- Card 11, existing-overload collision: choose a free signature or remove/re-sign the overload first; progresses.
- Card 11, handle or second-arrow collision: change a head or remove a competing declaration; progresses.
- Card 12, `resign-not-done`: correct the implementation and recover, or correct the head, rebaseline that card, then recover; progresses. `failCardNotDone` makes the batch terminal-failed, `refuseRecoveringDoneReport` permits recovery from that state, and `Rebaseline` accepts explicitly named edits to failed cards.
- Card 12, successful rewrite: `restampFingerprint` also advances matching begun-card hashes, so it addresses both the run fingerprint and the per-card edit guard.
- Existing `rename-tool-missing`: configure `rename_tools` through `lyx config language` or recast the card; progresses.

No findings from the way-forward lens.

Tool-call record:

1. Read recovery/rebaseline call sites and `contracts/specs/refusal-spec.md`; the inherited manifest lacked the refusal contract and executable recovery route.
2. Read record/recovery handling of `ErrCardNotDone`, recovery’s finished-report guard, rebaseline acceptance, and rewrite entry points; needed to establish the state left by `resign-not-done`.
3. Read fingerprint hash consumers, plan rewrite mechanics, rebaseline partition preservation, and Rename-tool refusal location; needed to check rewrite recovery and card-hash implications.
4. Read `failCardNotDone`, `restampFingerprint`, `renameToolRefusal`, validation entry points, and card 12 line locations; needed to confirm the recovery conclusions and precise evidence.
