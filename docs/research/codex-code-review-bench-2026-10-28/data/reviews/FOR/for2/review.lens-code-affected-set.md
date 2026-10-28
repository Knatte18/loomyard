```yaml
lens: code-affected-set
outcome: reported
```

Affected-set list:

- Solo-review routing: `Finalize.judgeFixerChange`, `gateAndFix`, `runFixer`, `endSpent`, `keepFixerCommits`, `reviewFollowsReroute`, `Deps.RerouteReviewed`, `loomcli.rerouteReviewed`, and pawl wiring.
  The executable callers are covered; `Finalize.Call`’s overview comment still states the old unconditional over-cap route.
- Review report: `ParseFixReviewReport`, its gate closure and terminal read, `FixReviewReport`, verdict constants, contract registration, parser tests, and `PATTERN-sole-parsers.md`.
  No second parser or missing registry consumer found.
- Review session: `fixReviewTable`, `runFixReview`, `FixerSeats`, `seatengine.Engine.Run`, `join`, `seatValues`, `composeSeatPrompt`, and `seatSpec`.
  The new role, stencil, runtime loading, parent/edit directives, model choice, timeout and language skills reach their existing consumers.
- Attempt-record field: `FixerAttempts.Reviewed`, `ReadFixerAttempts`, `WriteFixerAttempts`, Finalize approval/reuse/removal, fixer writes and Parent-Align record handling.
  Typed readers preserve the field, but the newly added approval rewrite uses a writer that drops unknown JSON keys.
- Shared archive function: both landing callers now use the extracted signature; unrelated same-named webster functions remain separate.
- Configuration: `Config.Fixer`, `FixerTimeoutMin`, template comments, config validation and config registry.
  Existing key names and validation remain compatible.
- Documentation: package documentation, both recipe comments, overview, template comments and sole-parser background cover the new path.
  No declared PATTERN change is missing; the commit has no departure trailer.

Findings:

1. **BLOCKING — design — `internal/landingshed/finalize.go:741`: The new approval rewrite drops unknown attempt-record keys.**
   The approval path reads the attempt record, assigns `Reviewed`, and rewrites it through `WriteFixerAttempts`.
   That writer marshals only the typed struct (`internal/landingshed/fixerattempts.go:58–63`) and replaces the file through `writeBytesAtomic` (`internal/landingshed/mergerecord.go:86–110`).
   Thus an attempt record containing an additional key from another binary loses that key when this new approval path runs.
   `pattern/PATTERN-tolerant-records.md` explicitly requires rewrites to preserve unknown keys through `internal/state` or a type that retains them itself.
   Route this approval persistence through an unknown-key-preserving writer; `internal/state/state.go:30` provides the prescribed primitive.
   The finding concerns the newly introduced rewrite, rather than demanding unrelated cleanup of existing callers.

2. **LOW — consistency — `internal/landingshed/finalize.go:157`: Finalize’s public behavior comment still says every over-cap fix reroutes or halts.**
   `Call`’s overview says “a spent cap or an over-cap fix reroutes or halts,” whereas the changed `judgeFixerChange` approval branch (`internal/landingshed/finalize.go:739–744`) proceeds to landing.
   Update that overview to include solo-review approval and reserve reroute/halt for the remaining outcomes.

Tool-call record:

1. Searched the whole tree for review symbols, stencil/spec names, timeout keys and over-cap routing descriptions to enumerate readers absent from the inherited manifest.
2. Read `Finalize.Call`’s overview, seat execution/join/prompt consumers, and searched attempt-record and changed helper callers because their complete definitions and affected sets were absent.
3. Searched fixer-role/config consumers and read Finalize record cleanup, seat table definitions and state writer declarations to check lifecycle, role compatibility and persistence.
4. Read atomic record replacement, `state.WriteJSON`, Parent-Align attempt handling, seat directive rendering, model-spec documentation and template changes to settle unknown-key preservation and downstream prompt/config coverage.
