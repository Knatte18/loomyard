| Change | Affected set found | Covering cards |
|---|---|---|
| Machine identity | Owner readers/setter, batten, loom, landing, orch, tender, adopt; hostname and leaf-package tests | 1, 3, 6, 8–9, 12–14, 16 |
| `owner_machine`, `prime_owner` | Task/store decoding, migration, persistence, replay, run-lock comparison, board views and tests | 2–4 |
| Owner writes and payload validation | Single/batch upsert, merge, claim/resume, prime claim, adopt, CLI help and board skill | 3, 8, 13, 16 |
| Board owner output | Brief/full views, `find` through `ListTasksBrief`, get envelope, Running table and golden | 4 |
| `owned_elsewhere` and replay restriction | Classification, summary, mover, push retry, **pull replay followed by a successful initial push**, reconcile/pull/push tests | 5–6; pull-to-push path uncovered |
| Owner-reader wiring | Hub seam builder; batten, landing, hibernate, orch and cleanup reconciles; root push-reader installation | 6, 8–13 |
| Owner-pause metadata | Pause creation, coverage, InnerRun lock handling, wake/orch pauses, identity-based lift, status serialization and tests | 7, 10–11, 13–16 |
| Start/resume refusals | Batten arm/pre-run/step/window/claim, loom start/resume, child refusal-envelope reader and spawn paths | 8–11 |
| Sleep-triggered owner check | InnerRun watch, shared `SleepWatch.Observe`, its boundary test, other sleep-watch consumers | 10–11; shared boundary uncovered |
| Finalize owner halt and exemption | Landing deps, reconcile verdicts, parent-lock handling, repair message, adopt halt recognition, orch instructions and tests | 10–12, 16–17 |
| Wake/orch owner outcome | Hibernate mapping/hold/notice, orch start/envelope, cleanup reporting, root wiring | 13 |
| Tender owner output | Hub deps, status report, CLI envelope, status/pair-owner tests, overview and orch instructions | 14, 17 |
| Window reopening | Reed client listing, batten resume/command settings, help, package docs, overview and tests | 15 |
| Adopt module and destructive primitive | Root registration, sandbox exclusion, mutation guards, clone fixtures, docs, **CLI seam-signature assertion lists** | 16; signature lists uncovered |
| Orch interpretation and ownership rule | Role stencil, PATTERN overview and new background | 17 |

1. **MEDIUM · design · `_lyx/plan/05-reconcile-owner-outcome.md:42` — The replay affected set omits pull followed by a fast-forward push.**  
   Card 5 guards reconcile replay and rejected-push retry but explicitly leaves `Fabric.Pull` unchanged. `settleUnmovedWeft` can first replay the old machine’s commits onto the adopter’s tip; the subsequent initial push then succeeds without entering the guarded retry. The missed members are `Fabric.settleUnmovedWeft`, `pushClassified`’s successful initial-push path, and the pull-to-push test scenario. Evidence: `internal/fabricengine/pull.go:379`, `internal/fabricengine/pushclassified.go:120`, `internal/fabricengine/pushclassified.go:127`, `internal/fabricengine/pull_integration_test.go:609`.

2. **LOW · design · `_lyx/plan/10-innerrun-owner-check.md:40` — The one-minute trigger omits the shared detector’s conflicting boundary contract.**  
   Card 10 requires a sleep of one minute or more to trigger the check, but derives it from `SleepWatch`, which records only gaps strictly greater than one minute. Its existing test explicitly asserts that equality is not sleep. No card covers this helper/test boundary; changing the shared detector would also affect hibernate, alignment, shed watchdog, shuttle and existing InnerRun timing. Evidence: `internal/pauserecord/sleep.go:28`, `internal/pauserecord/sleep_test.go:28`, `_lyx/plan/10-innerrun-owner-check.md:51`.

3. **LOW · design · `_lyx/plan/16-adopt-module.md:7` — New-module coverage omits both CLI seam-signature assertion lists.**  
   Card 16 covers registration, sandbox and mutation-guard inventories, but neither it nor another card covers adding adopt’s `RunCLI` and `RunCLIIn` to the existing compile-time lists that pin module seam signatures. Evidence: `cmd/lyx/seamsignature_test.go:1`, `cmd/lyx/seamsignature_test.go:33`, `cmd/lyx/seamsignature_test.go:60`; card coverage: `_lyx/plan/16-adopt-module.md:27`.