MILL_REVIEW_BEGIN
# Review: Rename the fabric host vocabulary to warp, and name the composite repo Fabric

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] Fabric-sense `host` outside the in-scope set is unclassified
**Section:** Scope (In/Out), Testing ("repo-wide completeness check")
**Issue:** Fabric-sense `host` also lives in non-owner packages the scope never mentions — `internal/loomengine/preflight_integration_test.go` (31 hits, `TestPreflight_HostDirty`, "paired host+fabric worktree"), `internal/perchcli/run_integration_test.go` (14), `internal/webstercli` (13), `internal/buildercli` (13), `internal/configcli` (13, including production `configcli.go:269` "the host _lyx parent"), `internal/configengine/config_test.go` (4), `internal/websterengine/audit_test.go` (4); the Out list covers only machine/verb sense, so these are neither in nor out.
**Fix:** State explicitly whether this residue is deliberately deferred (and why the task's premise still holds), or fold it in.

### [GAP] `tools/sandbox` inventory misattributes and omits files
**Section:** Scope → `tools/sandbox/*.go`
**Issue:** The enumerated `hostRepoDir` (`:136`–`:189`) and error strings (`:139`, `:141`) are correct for `main.go`, but `suite.go` independently holds `hostDirName = "lyx-test"` (`:27`–`:29`), `hostRepoDir` (`:169`–`:380`) and two more user-visible errors (`:323` "hub host repo not found at %s", `:325`), `report.go` holds the same pair (`:54`–`:94`, `:57`, `:59`), and `main_test.go`/`suite_test.go`/`report_test.go` carry ~157 further occurrences — none named.
**Fix:** Either enumerate `suite.go`/`report.go`/the three test files, or state the scope is the glob and the per-line list is illustrative.

### [GAP] Historical-record exclusion names only one of three such files
**Section:** Technical context → Deliberate documentation exclusion
**Issue:** `docs/benchmarks/fixture-copy.md` (`:214`, `:236`, `:242` — `hostLayoutFor`, `hostPath`, `internal/warpengine`) and `docs/research/scout-spike.md` (`:111`, `:116`, `:118`, `:123`, `:133` — `hubgeometry.WeftHostSlug`, `internal/warpengine`, `warpcli`) are the same historical-record class as the excluded `test-suite-timing.md`, yet appear in neither the swept list nor the exclusion.
**Fix:** Add both to the deliberate-exclusion list with the same rationale, or explain why they differ.

### [GAP] Two docs citing retired identifiers are missing from the sweep list
**Section:** Technical context → Documentation surface
**Issue:** `docs/shared-libs/lyxcwd.md:82` cites `HostLyxLink`/`HostJunctions` (the doc mirror of the CONSTRAINTS Cwd bullet the task does rename) and `manifest/designs/fabric-unified-view.md:86` cites `Host*Link`/`HostLyxLink`/`HostJunctions`; the per-file hit table lists neither, and `fabric-unified-view.md` appears only as owner prose and an inbound link at `:203`.
**Fix:** Add both files to the documentation surface with their line citations.

### [GAP] The "five ambiguous occurrences" figure covers only the Go sweep
**Section:** Decisions → Ambiguous compounds are reported, not guessed
**Issue:** The five measured hits are the Go set, but commit (d)'s doc sweep adds more `host`+lowercase occurrences the tool must also report — `docs/overview.md:302`, `manifest/designs/loom.md:131`/`:198`, `manifest/designs/fabric-unified-view.md:88`, `docs/sandbox-hub.md`, `tools/sandbox/SANDBOX-REED-SUITE.md:225` — so "a sixth occurrence appearing in the report is a finding" is a false tripwire for the later commits.
**Fix:** Scope the count of five to commit (a), and state the expected report shape (or that no count is pinned) for commits (c)/(d).

### [NOTE] `CopyHostHub`/`HostFixture` caller list is slightly wrong
**Section:** Scope → `internal/lyxtest`'s exported test-fixture seam
**Issue:** `internal/configcli` is listed among the 8 caller packages but references neither symbol; the actual 8 are `cmd/lyx`, `buildercli`, `idecli`, `lyxcwd`, `webstercli`, `fabricengine`, `fabriccli`, `lyxtest` (`cmd/lyx/tierpurity_test.go:50` is a comment-only mention).
**Fix:** Correct the package list.

### [NOTE] Forced-scope set is smaller than stated; no further growth found
**Section:** Decisions → `weftname`, `boardengine`, `tools/sandbox` are in scope too
**Issue:** Only `weftname.go:10`, `board.go:15` and `:25` match `hostPhrases`; `board.go:17` ("the host's own default branch") does not, so it is prose polish rather than forced by the tightening — verified that `internal/configsync` is clean and no owner dir contains `.md`, so the tightening forces no additional packages.
**Fix:** Note the distinction so a plan writer does not treat `:17` as compile-gating.

## Verdict

GAPS_FOUND
Scope boundaries and file inventories are incomplete in five places; decisions themselves are sound.
MILL_REVIEW_END
