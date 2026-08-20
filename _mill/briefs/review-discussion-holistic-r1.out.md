MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] `internal/lock` is not dependency-free
**Section:** `dependency-strategy-copy-vs-replace` + Constraints ("Discovered during discussion")
**Issue:** `internal/lock/lock.go:12` imports `github.com/gofrs/flock`, so "Copy verbatim: `internal/lock` (222 lines, no deps)" is false and copying it makes quarry's dependency count four, which the stated budget (stdlib + cobra + yaml.v3) says "must stop".
**Fix:** State the disposition explicitly — accept `gofrs/flock` as a fourth budgeted dependency, or move `lock` to the replace side — and correct the budget line.

### [BLOCKING:consistency] Test tier tag is `scout`, not `integration`
**Section:** Scope; Constraints (Test Tier Purity); Testing step 3
**Issue:** Five of the six tagged scout test files carry `//go:build scout` (`ensureserver_integration_test.go`, `refs_integration_test.go`, `supervised_integration_test.go`, `toolchain_integration_test.go`, `supervised_scout_test.go`); only `internal/scoutcli/cli_integration_test.go` is `//go:build integration`. Verification step 3 (`go test -tags integration ./...`) would therefore pass while exercising almost none of the ported live-server suite, and "port the `//go:build integration` tier separation" mis-describes what exists.
**Fix:** Name the tag scheme quarry adopts (keep `scout`, rename to `integration`, or both) and spell the verify commands against it.

### [BLOCKING:scope] gitkit/hubforge fixture inventory is wrong
**Section:** Testing → "Rewritten fixtures"; "The exact used surface" table
**Issue:** Only two files touch those packages — `internal/scoutcli/cli_integration_test.go` (`hubforge.NewHub`/`SeedConfig`) and `internal/scoutcli/testmain_test.go` (`gitkit.HermeticGitEnv`, listed in no disposition list at all). The other five files named as "the ones reaching for `gitkit`/`hubforge`" import neither; `scoutdaemon_test.go` uses no fixtures and no build tag. The one hubforge test covers `lookupContext`'s hub-mode branch, whose subject disappears in quarry — it is not a `t.TempDir()` swap.
**Fix:** Re-derive the three test lists from the actual import sets, give `testmain_test.go` a disposition, and say what replaces the hub-mode branch test.

### [BLOCKING:design] Ownership of the new state path is undecided
**Section:** `config-and-state-paths`; Constraints (Told-Geometry)
**Issue:** `DaemonStateFile`/`DaemonLock` live in the engine (`daemonstate.go:39,47`) and join `<anchorRoot>/.lyx/scout/<lang>/`, but the discussion says resolution belongs in `internal/cli/` and that `DotLyxDirName` "disappears entirely" — irreconcilable unless the engine's signature/`Options.AnchorRoot` changes. "Redirecting those three is the entire state-path change" hides that decision.
**Fix:** Decide and state whether the engine is told a leaf state directory (signature change) or keeps `anchorRoot` plus its own segments, and name the consequences for `scoutdaemon_test.go`.

### [BLOCKING:scope] Removal checklist claims completeness it lacks
**Section:** Technical context → "Loomyard removal checklist" ("Every site, enumerated")
**Issue:** A repo grep surfaces sites the list omits: `cmd/lyx/sandbox_coverage_test.go:31` (`excludedModules["scout"]`, required by the Sandbox Suite Coverage invariant), `README.md:87`, `manifest/designs/review-finding-classification.md:67` (a live markdown link to a moved doc, machine-checked by `TestEnforcement_MarkdownLinks`), `docs/benchmarks/running-tests.md`, and the now-dead `scout` build tag in `cmd/lyx/tierpurity_test.go` / CONSTRAINTS' Test Tier Purity.
**Fix:** State the enumeration method (a pinned grep the plan re-runs) rather than a hand-listed set, and drop the "every site" claim.

### [NIT:consistency] `configengine` used-surface omits `ConfigDir`
**Section:** "The exact used surface" table; Testing → "Ported unchanged"
**Issue:** `load_test.go:19-20` calls `configengine.ConfigDir`, so the table ("`ConfigFile` (3 sites)") is incomplete and `load_test.go` cannot be ported with "only import paths and package clauses change".
**Fix:** Add `ConfigDir` to the table and move `load_test.go` to the rewritten list.

### [NIT:consistency] Two miscounted/misattributed statements
**Section:** Problem ("Why now"); `mechanical-move-not-hand-transcription`
**Issue:** "Two things changed" is followed by First/Second/Third; and the `sed` ban is attributed to "the `mill:conversation` rule" when it comes from the global CLAUDE.md "Don't use `sed`" instruction.
**Fix:** Say "Three things changed" and cite CLAUDE.md for the `sed` ban.

## Verdict

REQUEST_CHANGES
Dependency budget, test-tier tags, fixture inventory, state-path ownership, and removal checklist all need resolution.
MILL_REVIEW_END
