```yaml
lens: code-correctness
outcome: reported
```

Branch inventory:

- `ParseFixReviewReport`: missing/unreadable file returns a wrapped error; empty file and unknown verdict fail; valid approval and rejection parse; rejection with an empty Findings section fails; whitespace-only bullets do not count. **Incorrect:** approval without the required Findings section succeeds.
- `fixReviewTable`: each empty required input, unreadable stencil, invalid model specification, and unresolved model returns an error. Valid inputs produce one seat with the configured model, timeout, report output, and optional fixer-report path. Zero timeout is excluded by configuration validation.
- `archiveStaleReport`: absent report succeeds; existing report is renamed; other rename failures propagate.
- `runFixReview`: table/archive failures propagate; startup failures receive the transient marker; infrastructure failures remain errors. With a successfully joined session, dirty tree and moved HEAD take precedence over timeout and report verdict; timeout, missing report, and malformed report produce no-verdict; approval and rejection classify correctly.
- `rerouteCause`: rejection includes findings and report path; no-verdict includes cause and session log.
- `judgeFixerChange`: absent baseline, within-cap change, and matching approved HEAD proceed; HEAD/diff/reroute-read failures propagate; no downstream review preserves the existing halt; approval records HEAD and proceeds; changed branch halts; other review ends reroute. **Incorrect:** the new approval-record rewrite drops unknown JSON fields.
- `reviewFollowsReroute`: nil means false; configured result is returned; read errors carry context.
- `endSpent` and `keepFixerCommits`: failing exhausted entry never starts a solo review; reroute retains commits and names the range; no-review halt records its cause; read/write errors propagate.
- Resume: matching entry and HEAD retain approval; a different HEAD requires another review; a different entry receives a fresh record; successful completion removes the record.

Findings:

1. **BLOCKING — design — `internal/landingshed/finalize.go:741`**  
   **Summary:** The new approval-record rewrite discards unknown JSON keys.  
   **Evidence:** `ReadFixerAttempts` unmarshals into `FixerAttempts` (`internal/landingshed/fixerattempts.go:47–50`), and `WriteFixerAttempts` marshals that struct directly (`internal/landingshed/fixerattempts.go:58–63`). Thus an existing record containing an additional key loses it when this new approval branch writes `Reviewed`. `pattern/PATTERN-tolerant-records.md` explicitly requires rewrites to preserve unknown keys through `internal/state` or equivalent retained fields. The new approval path violates that invariant even though the helper predates this diff.  
   **Fix:** Change the attempt-record rewrite to preserve unknown fields through `internal/state`, or retain and re-emit unknown fields in `FixerAttempts`, and use that writer for approval persistence.

2. **LOW — design — `internal/landingshed/fixreview.go:71–85`**  
   **Summary:** A bare approval is accepted without the report’s required Findings section.  
   **Evidence:** `contracts/specs/landing-fix-review-spec.md:12–14` requires the verdict followed by a section opened with `## Findings`; only its contents may be empty on approval. The parser tracks whether it is currently inside Findings but never requires that heading to occur. Consequently `verdict: approved\n` passes both the parser gate and final parsing, and `runFixReview` approves it at lines 268–273.  
   **Fix:** Track whether the Findings heading was present and reject reports lacking it, including approvals; keep accepting an explicitly present empty section for approval.

Tool calls:

1. Read shuttle gate types and seat-table validation because inherited context lacked the re-prompt-budget contract and single-seat validation details.
2. Read shuttle completion/cleanup, gate evaluation, transient classification references, and timeout validation because inherited context lacked the precise distinction between terminal outcomes and infrastructure errors.
3. Read review format, shuttle error contract, numbered review-run branches, and fabric dirty/diff symbol locations to establish severity vocabulary and precise evidence locations.
