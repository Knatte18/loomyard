MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-12
```

## Findings

No findings. Verified end-to-end against all three batches:

- **Leaf surfaces (batch 1):** `selfreportengine.DefaultLabels()` returns a fresh slice each call (tested for aliasing); `selfreportcli/cli.go` wired to it with the `create` subcommand's `Long` text byte-identical. `shedadapters.IsLedgerPath`/`ReadLedger`/`Ledger`/`LedgerEntry` derive the round from `ledgerPath` itself (no re-typed literal), reject all four siblings and unrelated paths, and fail closed on a frontmatter/filename round mismatch — all per card 2. `loomengine.LoomSelfreportFiled(Lock)` follow the exact `LoomBootstrapLock` shape, registered correctly in both `cmd/lyx` guard tests (`notransients_test.go` two rows, `constructoranchoring_test.go` all three sites) and in `loomstatus_test.go`'s mirrored pair. `Config.Selfreport` added with matching `template.yaml` line and comment; the four pre-existing fixtures broken by the new template key were switched to `writeLoomConfigWithKey` as required, and `config_test.go` adds both the template-default and explicit-false cases without asserting the forbidden omitted-key case.
- **Anomaly detector (batch 2):** `anomaly.go`/`anomalybody.go` are pure, no I/O, no `shedadapters` import — `detector-stays-pure` decision honored. All five `AnomalyKind` wire spellings match; the two error literals (`errorTextEscalation`/`errorTextBudgetExhausted`) verified byte-identical to `shedengine/run.go`'s own literals. Title rendering (producer#success-count, producer@history-length, row—key) matches spec exactly, including the em-dash separator. `DetectAnomalies`'s pinned order (crash-resume, halt, trigger-5 in ledger order) matches its test table, including the multi-anomaly ordering case. Threshold-3 recurring-finding logic and the round-carry-forward collapse semantics are both correctly implemented and tested (title stability across a resume-appended history, title distinctness across producers/done-counts, deterministic double-run).
- **Drive wiring (batch 3):** `selfreportDeps` carries every told input as specified, including the two ledger seams and the filing seam as function fields. `detectAndFileAnomalies`'s three-skip order (disabled → busy sentinel → cancelled context, with the crash-resume exception) matches the card exactly, verified by the regression test for the reachability defect (non-busy error still runs the pass) and the paired done-context/live-context complement. `observeEntry` probes the run lock before reading status, exactly as required, degrading to `logger.Warn` + zero observation on either failure. `discoverLedgers`'s three non-fatal skips (empty output, predicate-rejected, read/parse failure) are all covered, including the mixed-history and generation-mismatch cases. `runFilingPass`'s four ordered steps (collapse-by-title keeping highest round for recurring findings, marker filter, file, record-after-success) match the card, with the "unreachable by construction" comment following the `targetRepo` guard convention as instructed. `drive.go` places `detectAndFileAnomalies` on one unconditional line above the early error return, with the load-bearing placement comment, and neither envelope changed. `smoke_test.go`'s `fastDeadlineLoomConfig` disarms `selfreport` via the required second `strings.Replace`. Documentation lands together: `self-report-tier1.md` rewritten to shipped shape with Open Questions dropped, `roadmap.md` moved to Done with both sibling Planned entries' stale cross-references corrected, `docs/overview.md`'s selfreport bullet names both triggers.
- **Cross-batch contracts:** `selfreportDeps.ReadLedger`'s signature matches `shedadapters.ReadLedger` exactly; `FileIssue` matches `selfreportengine.CreateIssue`'s signature; `drive.go` fills all three seams with the real production functions. No duplicated helpers across batches — `DefaultLabels()` is consolidated, not reimplemented.
- **Test tier purity:** every new test file is untagged Tier 1, uses only `t.TempDir()`/`httptest`, and `selfreport_github_test.go`/`selfreport_test.go` are cleanly split at the told-seam vs. engine boundary with no double-binding, per the card 8 requirement.

## Verdict

APPROVE
Faithful, fully-tested realization of all three batches with no plan deviations or constraint violations found.
MILL_REVIEW_END
