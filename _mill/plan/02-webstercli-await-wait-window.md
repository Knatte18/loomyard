# Batch: webstercli-await-wait-window

```yaml
task: Audit and overhaul engine test suites
batch: webstercli-await-wait-window
number: 2
cards: 2
verify: go test -tags integration ./internal/webstercli/...
depends-on: []
```

## Batch Scope

Shrinks `TestAwaitBatchCmd_ReportPresenceEnvelope/NoReport_WindowElapses` (`internal/webstercli/verbs_test.go`, tagged `//go:build integration`, so this whole file is Tier 2) from a real ~30-second wait to a near-instant one by passing the already-existing `--wait` cobra flag with a near-zero duration, matching the `"--wait", "1ns"` convention the same file already uses twice for `recoverBatchCmd`'s equivalent near-instant-wait tests. Fixes that test's own docstring, which currently misattributes its wait window to a nonexistent `PollWaitS = 1s` (the fixture's actual `PollWaitS: 1` field, `verbs_test.go:242`, configures an unrelated webster `Config` field, not `await-batch`'s own `--wait`/`websterengine.DefaultAwaitWaitS` mechanism). Also folds two near-duplicate tests, `TestRecordBatchCmd_DigestEnvelope` and `TestRecordBatchCmd_NoReportEnvelope`, into one table-driven test — a clean, low-risk refactor in a file already being touched this batch (see `_mill/discussion.md`'s "Test-consolidation scope" Decision for why only this one pair, not the other two candidates found during exploration). No product-code change anywhere in this batch — every edit is confined to `verbs_test.go`, and every test's assertions are preserved (either unchanged, or carried forward as a table row). This batch is independent of `githubclient-timeout-seam` (batch 1) — different package, no shared file — so both are root batches with no `depends-on`.

## Cards

### Card 2: pass `--wait 1ns` to the window-elapses subtest and fix its stale docstring

- **Context:** none
- **Edits:**
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In the `NoReport_WindowElapses` subtest inside `TestAwaitBatchCmd_ReportPresenceEnvelope` (`verbs_test.go`, currently `t.Run("NoReport_WindowElapses", func(t *testing.T) { ... })`), change the `clihelp.Execute` call's args slice from
```go
		exitCode := clihelp.Execute(fx.CLI.awaitBatchCmd(), &out, []string{"1"})
```
    to
    ```go
    exitCode := clihelp.Execute(fx.CLI.awaitBatchCmd(), &out, []string{"1", "--wait", "1ns"})
    ```
    matching the exact `"--wait", "1ns"` two-token arg-slice shape already used in this same file for `recoverBatchCmd` (two other call sites). Do not change the sibling `ReportPresent` subtest — it returns before the window ever matters, so it needs no `--wait` addition.
  - Correct `TestAwaitBatchCmd_ReportPresenceEnvelope`'s doc comment, currently:
    ```go
    // TestAwaitBatchCmd_ReportPresenceEnvelope proves await-batch's two
    // envelopes: {"report": true} the moment the batch's report file exists,
    // and {"report": false} once the bounded wait (PollWaitS = 1s in this
    // fixture) elapses with no report -- with no state.json ever read or
    // written, since the verb is deliberately stateless.
    ```
    Replace the inaccurate "(PollWaitS = 1s in this fixture)" parenthetical — `PollWaitS` is an unrelated `websterengine.Config` field (`verbs_test.go:242`), not what bounds `await-batch`'s own wait — with an accurate description of the window this test now actually exercises: the `--wait` flag `NoReport_WindowElapses` now passes explicitly (`1ns`, near-instant), versus the production default (`websterengine.DefaultAwaitWaitS`, ~30s) used whenever `--wait` is omitted. Keep the rest of the comment's content (the two-envelope description, the statelessness claim) intact — only the wait-window parenthetical is wrong and needs replacing.
- **Commit:** `test(webstercli): shrink await-batch's window-elapses test to near-instant via --wait`

### Card 3: consolidate `TestRecordBatchCmd_DigestEnvelope`/`NoReportEnvelope` into one table-driven test

- **Context:** none
- **Edits:**
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Replace `TestRecordBatchCmd_DigestEnvelope` (currently lines 447-485) and `TestRecordBatchCmd_NoReportEnvelope` (currently lines 490-526) with one new test function (suggested name `TestRecordBatchCmd_Envelope`) that table-drives over the two scenarios via `t.Run`. Both original functions build the identical fixture: `t.Setenv("WEFT_SKIP_GIT", "1")`, `fx := newVerbsFixture(t)`, `st := fx.initState(t, "master-model")`, `startSHA := commitFile(t, fx.Worktree, "internal/only/impl.go", "package only\n", "01.1: add impl")`, `st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}`, `st.CurrentBatch = 1`, `websterengine.SaveState(fx.CLI.websterDir, st)`, and `fx.Engine.auditForks = shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}}}` — keep this setup identical, inside the shared `t.Run` body, driven once per table row.
  - Table fields per row: whether `writeBatchReport(t, fx.CLI.reportsDir, startSHA)` is called before invoking `recordBatchCmd` (`true` for the digest-envelope case, `false` for the no-report case); the envelope substrings to assert present in `out.String()` after `clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})` (digest case: `` `"batch":"01-only"` ``, `` `"status":"done"` ``, plus a dynamic `fmt.Sprintf(`"head_sha":%q`, startSHA)` check computed per-row since `startSHA` is generated fresh inside each `t.Run` — this one assertion needs its own conditional rather than a static table string, since only the digest-envelope row checks it; no-report case: `` `"no_report":true` ``, `` `"batch":"01-only"` ``); the expected `loaded.Batches[1].Terminal` value after `websterengine.LoadState` (`true` for digest, `false` for no-report); and whether `loaded.Batches[1].Digest` is expected non-nil (`true` for digest, `false` for no-report).
  - Preserve every original assertion's failure-message text verbatim where the two originals already agree (e.g. `t.Fatalf("record-batch 1 = %d; want 0, output: %s", exitCode, out.String())`); where the originals differ only in exit-code variable naming (`"record-batch 1 = %d..."` vs `"record-batch 1 with no report = %d..."`), keep whichever phrasing reads correctly for both table rows or parametrize the message text per row — implementer's judgment, since this is presentation only and not itself asserted on.
  - Update the doc comment above the new function to describe both scenarios it now covers (adapt from the two originals' existing doc comments: `TestRecordBatchCmd_DigestEnvelope`'s "proves the terminal success envelope is the digest verbatim plus warnings, once one new fork transcript and a matching batch report are both present" and `TestRecordBatchCmd_NoReportEnvelope`'s "proves a call with a new fork transcript but no report file yet is a ladder signal, not an error").
  - Delete the two original functions entirely — they are fully replaced, not left alongside the new table-driven test.
- **Commit:** `test(webstercli): fold record-batch digest/no-report tests into one table-driven test`

## Batch Tests

`go test -tags integration ./internal/webstercli/...` covers the whole package's integration-tagged suite, including both cards' changes (`TestAwaitBatchCmd_ReportPresenceEnvelope` and the new `TestRecordBatchCmd_Envelope`) plus every sibling test in the same file (`TestBeginBatchCmd_*`, `TestRecoverBatchCmd_*`, `TestRunCmd_*`, etc.) as a regression check — this whole file is one `//go:build integration` unit sharing the `newVerbsFixture` helper both cards' edits sit inside, so a `-run` filter would risk missing a fixture-level regression. This is the package's Tier 2 suite; it is not run under plain `go test ./...`.

Manually confirm timing after implementing: `go test -tags integration -run TestAwaitBatchCmd_ReportPresenceEnvelope -v ./internal/webstercli/...` should report near-instant elapsed time for `NoReport_WindowElapses`, not ~30s. `go test -tags integration -run TestRecordBatchCmd_Envelope -v ./internal/webstercli/...` should show both table rows passing.
