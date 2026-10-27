- `plan-language-unrecognized` / `glyph-malformed` (cards 2 and 6): choose the recognized language, including `csharp` for the named C# glyph; progresses before execution.
- `plan-language-mismatch` (card 6): set the files’ language or `none` for mixed-language work; progresses before execution.
- `rename-namespace-pair` (card 7): replace the namespace pair with Edit cards, or correct a mistaken root-file path; progresses.
- Cross-alphabet ambiguity (card 8): name a file or member instead of the ambiguous self glyph; progresses.
- C# `rename-old-unresolved` branches (card 10): correct the new handle’s C# spelling, or drop an unsupported member pair; progresses for the stated cases.
- C# `resign-head-mismatch` (card 11): correct the head while preserving parameter-list presence, or express a kind change through Delete/Create; progresses.
- `resign-overload-collision` (card 11): choose a free signature, remove the existing overload earlier, or drop one competing declaration; progresses within D7’s rules.
- `resign-not-done` (card 12): correct the implementation and recover, or correct the card’s head, rebaseline that card, then recover; progresses. `failCardNotDone` leaves a terminal failed batch, and `Rebaseline` explicitly accepts named edits to such cards while preserving the batch’s card set.
- Infrastructure failures (card 12): no separate finding; the rubric exempts propagated raw I/O errors.

No findings from the way-forward lens.

Additional reads:
1. Read `internal/websterengine/rebaseline.go`, `internal/webstercli/rebaseline.go`, and searched recovery references because the inherited context did not include the rebaseline admission rules or CLI argument format.
2. Read `internal/websterengine/recordbatch.go:45–180,475–560`, `internal/websterengine/recoverbatch.go:440–560,635–700`, and `contracts/specs/refusal-spec.md` because the inherited context lacked failure terminalization, recovery-spawn admission, and the refusal contract.
3. Read `internal/websterengine/rebaseline.go:238–365` and `internal/webstercli/recoverbatch.go:1–140`, and searched batch failure routing because the inherited context lacked the exact failed-card acceptance branches and recovery CLI behavior.
