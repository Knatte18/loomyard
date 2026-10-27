Per-refusal recovery list:

- `plan-language-mismatch` (card 6): set the appropriate `language:` or use `none`; progresses.
- C# `glyph-malformed` detail under Go (card 6): set `language: csharp`; progresses for the prescribed C# input.
- `rename-namespace-pair` (card 7): replace the namespace Rename with edits to declarations and references, or correct a mistaken root-file source; progresses.
- Cross-alphabet `glyph-ambiguous`, Create and done-check details (card 8): name a file or member to disambiguate; progresses.
- C# `rename-old-unresolved` (card 10): correct the new handle’s C# spelling, or drop an unsupported member Rename; progresses for the stated branches.
- C# `resign-head-mismatch` (card 11): preserve parameter-list presence or express a member-kind change as Delete/Create; progresses for the prescribed kind mismatch.
- `resign-overload-collision` against an existing overload (card 11): choose a free signature or remove/re-sign the existing overload earlier; progresses.
- `resign-overload-collision` against a handle declaration (card 11): choose a free signature or drop one declaration; progresses.
- `resign-overload-collision` against another arrow (card 11): change one head to a free signature; progresses.
- `resign-not-done` (card 12): correct the implementation and recover, or correct the head, rebaseline the named card, then recover; progresses. `Rebaseline` permits named edits to failed batches, and recovery explicitly accepts terminal-failed batches despite an existing OK report.

No findings from the way-forward lens.

Additional reading:
1. Read `internal/websterengine/rebaseline.go`, the recovery entry and report guards in `recoverbatch.go`, and recovery-command references; inherited context lacked the implementation of the newly prescribed recovery steps.
2. Read `internal/webstercli/rebaseline.go`, the remaining rebaseline acceptance logic, recovery post-check references, and `contracts/specs/refusal-spec.md`; verified that rebaseline does not require the old glyph to resolve, that failed-batch card edits are accepted, and that the proposed steps satisfy the refusal contract.
