MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
duration_s: 186.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] run has no Warnings channel to ride
**Section:** `### standalone-integration-failure-is-recorded-with-unknown-localization`
**Issue:** The decision says the standalone explanation "rides the run's `Warnings` list — the same channel the dirty-worktree and fork-audit notices already use", but `websterengine.RunResult` (`runlevel.go:151-167`) has no `Warnings` field, `runIntegrationStage` (`runlevel.go:763`) returns only `error`, and `run.go:101-107`'s envelope emits no `warnings` key; only `RecordResult` (`recordbatch.go:64-70`) carries one, and only `record-batch`/`recover-batch` surface it.
**Fix:** Decide and state the actual channel — add `Warnings []string` to `RunResult` plus a `warnings` output key on `run` (and add both to Scope "In"), or name a different existing channel; Testing's "must append the standalone-mode explanation to `Warnings`" pins a field that does not exist today.

### [NIT:decision] fabricSync loses its only Location source
**Demoted-from:** BLOCKING
**Section:** `### websterCLI.layout is removed outright` + `### standalone-has-no-fabric`
**Issue:** `fabricSync` (`sync.go:20-26`) needs `layout.AnchorRel` for `ScopedPathspec` **and** the `*lyxcwd.Location` for `fabricengine.Open`; deleting the `layout` field leaves it no source, and the proposed `c.fabric` cannot supply it — `fabricengine.Fabric` (`fabric.go:54-60`) holds only `warp`/`weft`/`warpPath`/`weftPath` and exposes no `AnchorRel`.
**Fix:** State sync.go's disposition explicitly — either a new told `(anchorRel string, fabric *fabricengine.Fabric)` signature, or a named hub-mode-only field that survives the `layout` deletion — and add sync.go to Scope "In".

### [NIT:consistency] Mode trigger contradicts preflight's shipped docs
**Demoted-from:** BLOCKING
**Section:** `### mode-selection-and-the-extracted-wiring-function`
**Issue:** The rationale claims the `HubPresent`-only trigger is "forced by `preflight`'s own documented semantics", but `preflight/doc.go:34-47` and `predicates.go:17-24` state the opposite in shipped prose: `Wired` "is the hub-mode trigger a standalone-capable CLI's pre-run consults", `HubPresent` "is the stencil seed gate", and "that resolved-but-not-wired case is exactly the one a standalone-capable CLI must answer with standalone mode". `internal/preflight` appears nowhere in Scope or in the Documentation Lifecycle list.
**Fix:** Record that this task rewrites `preflight`'s doc.go "Why there are two predicates" section and both function doc comments in the same commit (CLAUDE.md same-commit docs rule), and add `internal/preflight` to Scope "In".

### [NIT:scope] Guard/test enumeration method misses accessor call sites
**Demoted-from:** BLOCKING
**Section:** `## Scope` "In" + `## Testing` (`cmd/lyx` bullet) + "Guards this task runs into"
**Issue:** The affected-file inventory is hand-listed rather than derived, and it misses call sites of the four accessors whose signature changes: `cmd/lyx/notransients_test.go:63,64,73,74,156` (the Durable-vs-Ephemeral enforcement test) and `internal/webstercli/cli_test.go:180-183` both call `websterengine.Dir/ReportsDir/ScratchDir/PromptsDir(l)` and are named nowhere; only `constructoranchoring_test.go` and `verbs_test.go` are.
**Fix:** State the enumeration method as a mechanical grep over the four accessor names repo-wide (as was done for websterengine's seven fixture files) rather than a hand list, and record the disposition of every hit it returns.

### [NIT:consistency] reedengine geometry.go header says "seven-field struct"
**Section:** `### standalone-pane-cwd-is-told-separately-from-anchorpath` / `## Constraints` Documentation Lifecycle
**Issue:** `internal/reedengine/geometry.go:1` opens "declares Geometry, the seven-field struct"; adding `PaneCwd` makes it eight, and the Documentation Lifecycle bullet names only "field docs".
**Fix:** Name the file-header sentence explicitly alongside the new field's own comment.

### [NIT:consistency] Fabric Git Invariant enforcement text narrows silently
**Section:** `### constraints-rewords-land-here-not-in-t8`
**Issue:** CONSTRAINTS' Fabric Git Invariant states the agent half "is machine-checked for webster runs by `fabricengine.RefScanner`"; with a never-matching standalone `RefMatcher` that becomes true of hub-mode runs only, and the Fabric Git Invariant is not on the reword list.
**Fix:** Add a one-clause qualifier to that enforcement bullet in the same commit, scoping the RefScanner check to hub mode.

## Verdict

REQUEST_CHANGES
Three decisions rest on non-existent or unassigned code seams; enumeration method is hand-built.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
