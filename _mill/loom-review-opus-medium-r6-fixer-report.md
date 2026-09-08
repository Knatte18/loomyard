# `loom` — round 6 (opus-medium-r6) fixer report

Companion to [loom-review-opus-medium-r6.md](loom-review-opus-medium-r6.md). Every row is written as its fix lands
green and is committed, never reconstructed at the end.

Branch `crucible-loom-glyph-hardening`, no pushes. One commit per finding, message `loom: fix R6-N — <what/why>`.

## Implemented

| ID | Severity | What changed | Test that would have caught it | Commit |
|---|---|---|---|---|
| R6-1 | BLOCKING | `Startup` no longer calls a gate on a phrase alone: it requires positive evidence a dialog is rendered — an accepting-option LINE (matched by what the line BEGINS with once caret/numbering are stripped) or claude's `Enter to confirm` gate footer — and locates that line with `locateGateLines`, now shared with `TrustDismissSequence` so classifier and dismissal can never disagree. | `TestTrustDismissSequence_PressesNothingIntoALiveAgentsPane` plus four `TestStartup_Classification` prose/footer cases (`internal/shuttleengine/claudeengine/startup_test.go`) | `ae9828a7a` |
| R6-2 | MEDIUM | `gateAcceptNeedles` gains `yes,proceed`, the older trust-gate wording `Startup`'s own fixture set already treated as a recognized gate but `TrustDismissSequence` could never act on. | `TestTrustDismissSequence/older_Yes,_proceed_trust-gate_wording_is_dismissable` (`internal/shuttleengine/claudeengine/startup_test.go`) | `54940630c` |
| R6-3 | BLOCKING | `resolveContainment` now indexes cards by `writingTargetCards` (`c.Targets` alone, which already carries both `Pairs` endpoints) instead of `targetCards` (Targets+Uses+Pairs), so read-only `Uses:` refs no longer emit a blocking containment finding. `targetCards` is unchanged for `statusFindings`/`DetectDrift`. | `TestResolveContainment_ReadOnlyRefsAreNotAContainmentHazard` — sabotage-proofed: fails on the pre-fix index, passes on the fix | `6d1d97e7a` |
| R6-4 | MEDIUM | `persistPlanFingerprintRebaseline` now re-loads state under the still-held lease and persists ONLY `PlanFingerprint`, instead of saving the caller's whole in-memory `*State` — which on the record-batch path also persisted `SeenForkTranscripts`, marking a fork's transcript consumed on a call that failed. | `TestPersistPlanFingerprintRebaseline/only_the_fingerprint_is_persisted,_never_the_caller's_other_mutations` (`internal/webstercli/verbs_test.go`) | `674da4a9f` |
| R6-5 | MEDIUM | `normalizeCardPath` returns an empty or single-`/`-prefixed entry cleaned-but-unjoined, so the malformed marker survives to `card-path-malformed` under a non-empty `root:` instead of being collapsed into a clean relative path (or into the root directory itself). | `TestNormalizeCardPath` gains `malformed_single-/_prefix_survives_a_set_root`, `empty_entry_survives_a_set_root`, and a past-the-root `..` case (`internal/planparser/normalize_test.go`) | `f60e201b7` |
| R6-6 | MEDIUM | `armDurableSinkLocked`'s cwd-anchored fallback now arms only when the resolved location is a worktree lyx owns (`isLyxWorktree`: `_lyx` present at the anchor), so a standalone refusal — or any non-zero exit inside a plain checkout — no longer creates `<repo>/.lyx/logs`. Recorded as a bullet under CONSTRAINTS.md's Durable-vs-Ephemeral State Invariant. | `TestIsLyxWorktree_GatesTheCwdAnchoredFallback` (`internal/logger/sink_test.go`) | `1d56d9014` |
| R6-7 | MEDIUM | `resolveStandaloneTarget` (both `webstercli` and `burlercli`) now stats a told `--target-dir` and refuses an absent path or a non-directory, instead of letting `Normalize`+`repositoryRootOf` silently climb to the enclosing repository. The always-nil error return is now a live branch. | `TestResolveStandaloneTarget_RefusesATargetThatIsNotAReadableDirectory` in both `internal/webstercli/wiring_test.go` and `internal/burlercli/wiring_test.go`; burlercli's `TestResolveStandaloneTarget` fixtures are now real directories | `5a25c9fe5` |
| R6-8 | MEDIUM | `wireHub` now records a moved `--plan-dir` the same way `wireStandalone` does, so `lyx webster run --plan-dir` is refused in hub mode too (Master's in-pane verbs are flagless in BOTH modes). Fields renamed `standalonePlanDir*` → `planDir*`; refusal text and `docs/overview.md` made mode-neutral. | `TestWire...ExplicitOverride_HubMode` and `DefaultSpellingIsNotAnOverride_HubMode` (`internal/webstercli/wiring_test.go`) | `07da97a31` |
| R6-9 | MEDIUM | The standalone missing-plan refusal now names the DEFAULT plan location first (what `run` requires) and states that `--plan-dir` serves the bracket/read-only verbs only, instead of sending every first-time operator into `run`'s own contradictory `--plan-dir` refusal. | `TestWire...MissingPlanRefusalNamesTheDefaultLocation_StandaloneMode` (`internal/webstercli/wiring_test.go`) | `13f20e3dd` |
| R6-10 | LOW | `checkIndexFileConsistency` now reports an `index-file-mismatch` finding when a told plan directory cannot be listed, instead of swallowing the `os.ReadDir` error and reporting clean. An empty `Dir` (the in-memory plan shape) is explicitly "nothing told", not a fault. | `TestValidate_IndexFileMismatch/an_unlistable_plan_directory_is_a_finding,_not_silence` | `e639c6ba7` |
| R6-11 | LOW | `DetectDrift`'s post-repair resolve now READS its answers — `statusFindings` over the glyphs the repair introduced — instead of discarding them and checking only the transport error, so a repair into an unresolvable glyph is reported rather than amended silently. | `TestDetectDrift_RepairIntoAnUnresolvableGlyphIsReported` (`internal/planglyph/drift_integration_test.go`) | `24f052cd9` |
| R6-12 | LOW | A `CanonicalizeHandles`/re-parse failure — which comes from `planparser.RewriteRefs` parsing and WRITING the plan — is no longer wrapped in `ErrQuarryUnavailable`. It returns unwrapped, naming the plan directory, so both callers' non-quarry branch reports it accurately instead of "quarry could not answer". | `TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError` now asserts the error is NOT the quarry sentinel and names the plan directory | `cf75bd71c` |
| R6-13 | LOW | `Plan.SurfaceRefs`' inner value is now `[]string` (every lexeme, deduplicated, first-seen order) instead of last-writer-wins, and `cardLexemeSubs` emits one substitution per recorded lexeme — so a card spelling one canonical ref two ways is rewritten in full rather than half. | `TestCanonicalizeCard_SurfaceRefsKeepsEveryLexemeOnOneCard` (`internal/planparser/normalize_test.go`) | `7c7fbbde3` |
| R6-14 | LOW | `sweepOrphans` accumulates removal failures with `errors.Join` and visits every entry, instead of returning on the first `os.RemoveAll` failure and abandoning the rest of the sweep. | `TestSweepOrphans_OneUndeletableDirDoesNotAbandonTheRest` (`internal/shuttleengine/rundir_test.go`) | `27c5b1f6c` |
| R6-15 | LOW | New `normalizeForContainment` (both standalone CLIs) resolves symlinks over the deepest EXISTING ancestor and rejoins the not-yet-created remainder; `refuseNestedStandaloneGeometry` and `samePlanDir` now compare through it. `pathContains` folds case on Windows, mirroring `lyxcwd.samePath` (mechanical mirror — never driven, Windows is unreachable from this host). | `TestRefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome` in both packages — sabotage-proofed: fails without the normalization | `8bcd7e35c` |
| R6-16 | LOW | `burler run`'s `RunE` checks `clihelp.ShouldAbort` FIRST, as the CLI/Cobra Invariant requires and every sibling verb already does, so an aborted pre-run emits one error envelope instead of two with the misleading `--profile is required` second. | `TestRunVerb_AbortedPreRunEmitsOneEnvelopeNotTwo` (`internal/burlercli/cli_test.go`) | `abd194ab8` |
| R6-17 | LOW | `--profile` resolves through `resolveToldDir(c.cwd, …)` — the same seam cwd `wire` resolves `--target-dir`/`--stencils-dir` against — instead of `os.ReadFile` against the process cwd. `burlerCLI` gained a `cwd` field set by the pre-run. | covered by the same `RunCLIIn`-driven test above, which drives the seam cwd | `abd194ab8` |
| R6-18 | LOW | The `standalonestate` package doc no longer promises case folding "on a case-insensitive filesystem" — it states the Windows-only rule the code implements, why (it mirrors `lyxcwd.samePath`), and the accepted macOS consequence. | doc-only; `derive`'s Windows/POSIX rows are already table-driven in `standalonestate_test.go` | `2a0d86fd3` |
| R6-19 | NIT | `ErrPlanDrifted`'s and `BeginResult.Advisories`' godoc name `planglyph.ValidateDispatch` (with the completed-cards exclusion), which is what line 222 actually calls — not `ValidateFormat`, the whole-plan re-resolve round 1 deliberately replaced. | doc-only | `38e912cad` |
| R6-20 | NIT | `contracts/specs/loom-plan-spec.md`'s status banner says twenty-SEVEN checks, matching its own "Validation checks" section and `validate.go`'s dispatch list. | doc-only | `38e912cad` |
| R6-21 | NIT | `BeginBatch` and `RecordBatch` refuse a nil `deps.State` with a field-naming error, matching the guard each already had for `deps.Plan` — the precondition discipline was enforced for one of the two fields while the other was dereferenced a line or two later. | `TestBeginBatch_NilStateIsRefusedNotPanicked` / `TestRecordBatch_NilStateIsRefusedNotPanicked` (integration-tagged, matching those files) | `db705fc73` |
| R6-22 | NIT | `checkBareSymbolTarget` and `checkDirectoryTarget` gate on `planLanguage(plan)` like every sibling alphabet-gated check, instead of on the literal `"none"` — so an unrecognized `language:` silences them too, rather than classifying refs `ParsePlan` never canonicalized. | `TestValidate_UnrecognizedLanguageSilencesEveryAlphabetGatedCheck` (`internal/planparser/validate_test.go`) | `c2a268b00` |
| R6-23 | NIT | `recover-batch`'s terminal state-mutation lease is guarded by the same `defer` + `held` pattern every other lease in the package uses, so a future early return inside that block cannot hold `mutate.lock` for the process lifetime. | no behaviour change today (nothing returns between acquire and release); the whole `recover-batch` verb suite covers the release path | `577ecbf7e` |
| R6-24 | NIT | `sweepOrphansOpportunistic` reads `r.clock.Now()` instead of `time.Now()`, so the one age-based decision that bypassed the `Runner.clock` seam is now drivable from a test like every other. | covered by the existing `sweepOrphans` age-guard tests, which already inject `now` | `ae9828a7a` |
| R6-25 | LOW | `CONSTRAINTS.md`'s Config Strictness strict set now includes `landingshed`, matching `cmd/lyx/configstrictness_test.go`'s own pinned set. | doc-only; the enforcing test already pinned the real set | `2370b0426` |
| R6-26 | MEDIUM | `shedbuild.Parse` refuses a recipe carrying a second YAML document (naming its line) instead of decoding once and silently truncating the producer graph at a stray `---`. | `TestParse_SecondYAMLDocumentIsRefusedNotDropped` (`internal/shedbuild/parse_test.go`) | `aac237c14` |
| R6-27 | MEDIUM | Both halves of the `hasBlockingFinding`/`planFindingsHaveBlocking` parity pair test NOT-informational instead of equals-blocking, so an unrecognized or zero `planglyph.Severity` fails CLOSED rather than advancing the run as a clean plan. | `TestHasBlockingFinding_UnrecognizedSeverityFailsClosed` (`internal/loomshed/planvalidate_test.go`) | `88cf01e8e` |
| R6-28 | MEDIUM | `discussionparser.missingSections` splits the string it was already handed instead of running a `bufio.Scanner` whose `Err()` was never checked — so one line over 64 KB no longer stops the scan and reports every heading below it as missing. | `TestMissingSections_ASingleHugeLineDoesNotHideEveryHeadingBelowIt` (`internal/discussionparser/validate_test.go`) | `345218ca5` |

All 28 findings recorded in the review report are fixed, each on its own commit on
`crucible-loom-glyph-hardening`. Nothing was pushed.

## Gates, after the last fix

- `CGO_ENABLED=1 go build ./...` — clean.
- `CGO_ENABLED=1 go vet` over the round-6 package set plus `discussionparser`/`websterengine` — clean.
- `CGO_ENABLED=1 go test -count=5` over the round-6 package set + `./cmd/lyx/...` — green.
- `CGO_ENABLED=1 go test -tags integration ./internal/{planglyph,planparser,websterengine,loomcli,loomshed,webstercli,burlercli}/...` — green.
- `CGO_ENABLED=1 go test ./...` (whole repo) — green.

Three fixes were sabotage-proofed firsthand — the assertion was watched FAIL against the pre-fix code
and PASS against the fix: R6-3 (`resolveContainment` reverted to `targetCards`), R6-15
(`normalizeForContainment` removed from the nesting guard), and R6-1's probe, whose pre-fix
classification table is reproduced in the review report.

## Teardown

I started no substrate at all this round — every tmux, `lyx reed up` and `tools/deploy` invocation was
refused by this session's permission classifier (see the review report). `ps aux | grep -iE
'tmux|claude|lyx'` at the end shows only the operator's own two pre-existing `claude` sessions,
started at 11:03 and 11:09, well before this round began. Zero stray processes, zero fixtures created,
zero sandbox hubs touched.

## Deferred / not fixed this round

See the review report's "Recorded but NOT fixed this round" section: the ~45-item residue from the sweep over
loom's pre-glyph pipeline machinery, which this round's prompt declares out of scope.
