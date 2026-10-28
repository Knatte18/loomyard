```yaml
lens: code-affected-set
outcome: reported
```

Affected-set inventory:

- `Finalize.judgeFixerChange`, `reviewFollowsReroute`, and `keepFixerCommits`: callers in `gateAndFix`, `runFixer`, and `endSpent`; dependency wiring in `loomcli/landingdeps.go`; budget and pawl wiring tests; recipe comments and landing docs.
  Covered, apart from the new approval rewrite described below.
- `FixerAttempts.Reviewed`: approval writer in `finalize.go`, same-HEAD reader in `judgeFixerChange`, JSON reader/writer and removal paths in `fixerattempts.go`, shared callers in `fixerrun.go` and `parentalign.go`, seeded resume test, and package documentation.
  Unknown-key preservation is not covered.
- `archiveStaleReport`: both landing-session callers updated; existing similarly named webster helper is unrelated.
  Covered.
- Fix-review role, stencil, model, and timeout: registered stencil and `roleOpeningStencils` entry; table construction; seatengine parent/edit directives, model and timeout propagation; existing `FixerSeats` wiring; config and template comments.
  Covered.
- Report format and sole parser: spec registry/default, registry tests, stencil reference, parser, gate closure, final verdict reader, parser tests, and `PATTERN-sole-parsers.md`.
  Covered except for the gate’s required CLI self-check.
- Commit message and declared PATTERN changes: no departure trailer and no declared PATTERN change requiring an additional landing.
  Covered.

Findings:

1. **BLOCKING — design — `internal/landingshed/fixreview.go:226` — The new report gate has no CLI self-check or parity check.**
   The diff installs the new mechanical `fix-review-report` gate at lines 226–231, whose closure validates the report with `ParseFixReviewReport`.
   Whole-tree searches find its only production parser callers in `fixreview.go`, at lines 227 and 268; no CLI verb calls it.
   `pattern/PATTERN-gate-slot.md:45` expressly requires “Adding a mechanical gate means adding its verb and its parity check in the same task,” and line 49 says implementing the gate inside a producer closure does not remove that obligation.
   Add a self-check verb using the same parser and a parity check in this change.

2. **BLOCKING — design — `internal/landingshed/finalize.go:741` — Persisting the approval drops unknown attempt-record keys.**
   The new approval path rewrites the existing attempt record through `WriteFixerAttempts`.
   `ReadFixerAttempts` decodes into an ordinary struct (`fixerattempts.go:47–49`), and `WriteFixerAttempts` marshals only that struct (`fixerattempts.go:58–63`).
   `writeBytesAtomic` replaces the file with those bytes (`mergerecord.go:96–105`); it does not preserve unknown fields.
   Thus an existing record containing a key from another binary loses that key when the new approval path runs.
   This path formerly rerouted without rewriting that record.
   `pattern/PATTERN-tolerant-records.md:3–7` requires every rewrite to retain unknown keys through `internal/state` or a type that preserves them itself.
   Route the approval update through such a preserving writer.

Additional tool calls:

1. Searched new parser/gate/role/spec readers and read spec registry and seat prompt construction; inherited output lacked the complete external reader set and directive injection definitions.
2. Searched parser and gate consumers, inspected existing CLI validation implementation and config/template consumers, and obtained numbered gate-rule evidence; inherited context did not establish absence of a CLI self-check.
3. Searched shared helper/record readers, inspected landing seam documentation and pawl wiring, and obtained numbered approval/record-rule evidence; inherited context lacked the complete affected record consumer set and precise evidence locations.
4. Read `rerouteReviewed` and `writeBytesAtomic`, and inspected the Finalize diff; inherited context lacked the record writer’s ultimate replacement operation and exact comparison with the previous reroute path.
