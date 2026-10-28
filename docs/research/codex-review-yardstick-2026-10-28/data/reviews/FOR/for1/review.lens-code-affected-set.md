```yaml
lens: code-affected-set
outcome: reported
```

Affected-set inventory:

- `keepFixerCommits` signature: its three callers in `Finalize.judgeFixerChange` and `endSpent` pass the new eligibility argument; covered.
- Shared `archiveStaleReport`: fixer and fix-review callers pass the timestamp, row and session description; covered.
- Fix-review role, stencil and spec: `fixreview.go`, stencil registry, `roleOpeningStencils`, spec registry, registry tests and `PATTERN-sole-parsers.md`; covered.
- Stencil values and directives: `seatengine.seatValues`, `composeSeatPrompt`, `seatSpec` and `SeatRole` supply the directives and propagate model, timeout, skills and role; covered.
- `Fixer` / `FixerTimeoutMin`: configuration validation, template, existing fixer table and new review table remain compatible; no key migration required.
- `FixerAttempts.Reviewed`: Finalize writes approval, checks the same HEAD, retains it through ordinary record reads, and clears the record on completion; Parent-Align uses a separate row record.
  The new approval rewrite does not preserve unknown keys.
- Finalize routing: package documentation, overview, recipe comments and changed gate tests describe the new route; the public `Call` comment still describes the superseded route.
- Report parsing: both production consumers use `ParseFixReviewReport`; no second parser found.
- Commit and declared PATTERN changes: no departure trailer and no declared PATTERN change requiring an additional landing.

Findings:

1. **BLOCKING — design — `internal/landingshed/finalize.go:740–741`: The new approval rewrite discards unknown attempt-record keys.**
   The newly added approval path rewrites the existing Go-written JSON record through `WriteFixerAttempts`.
   That function serializes only the declared struct fields with `json.Marshal` (`internal/landingshed/fixerattempts.go:58–63`), after `ReadFixerAttempts` decoded into the struct (`:47–54`).
   Therefore an existing unknown key disappears when this new approval path saves `Reviewed`.
   `pattern/PATTERN-tolerant-records.md:3–8` explicitly requires rewrites to preserve unknown keys through `internal/state` or equivalent preservation in the type.
   `writeBytesAtomic` only writes and renames the supplied bytes (`internal/landingshed/mergerecord.go:86` onward).
   Fix the approval record write through a preserving writer.
   The underlying helper predates this diff, but this finding concerns the newly added rewrite operation.

2. **LOW — consistency — `internal/landingshed/finalize.go:157`: `Finalize.Call` still documents every over-cap fix as rerouting or halting.**
   Its comment says “a spent cap or an over-cap fix reroutes or halts,” while the new approved branch returns `proceed()` (`:739–744`).
   Update the public producer comment to describe the review-approved landing route.
   The detailed comments and `doc.go` were updated, but this affected description was missed.

Tool-call record:

1. Searched landing/config/report readers and read both recipe diffs; the inherited context lacked the complete affected documentation set and recipe changes.
2. Read atomic-write implementation, `state` preservation contract, Parent-Align record lifecycle and complete seat directive/naming functions; inherited outputs omitted those callees or truncated them.
3. Searched the whole tree for changed configuration/report names, read registry behavior and configuration validation, and inspected the base attempt writer; these established external readers, registry integration and which part of the JSON defect predates the diff.
4. Read numbered evidence for the approval writer, stale `Call` description and tolerance rule, and checked remaining helper call sites and explicit registry consumers; these supplied precise locations and confirmed the caller set.
