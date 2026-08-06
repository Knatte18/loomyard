MILL_REVIEW_BEGIN
# Review: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic) — exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] Scout re-anchor names the wrong sites
**Section:** `dotlyx-and-lyx-are-directory-siblings` / Technical context re-anchor list
**Issue:** `DaemonStateFile`/`DaemonLock` (`daemonstate.go:42,49`) take a plain `worktreePath string`; the anchor is chosen by `scoutcli/cli.go:479` (`resolveWorktreeRoot`, with a non-hub `abs(targetDir)` fallback) and threaded via `Options.WorktreeRoot` → `ensureserver.go:299-300`, and scoutengine's leaf allowlist excludes `lyxcwd` so it cannot derive `AnchorPath()` itself — editing only `daemonstate.go:43,50` changes nothing.
**Fix:** Name `scoutcli/cli.go:479,584` and `ensureserver.go:298-300` in the sweep, and state whether `WorktreeRoot` is re-purposed as the anchor path (it also keys the daemon singleton/LSP root) or a separate anchor value is threaded.

### [GAP] fabric-unified-view's anchoring table asserts the old anchor
**Section:** Scope/In (docs) and "Docs that assert the old behaviour"
**Issue:** `manifest/designs/fabric-unified-view.md:60-64` states the `.lyx` group (`WorktreeLogsDir`, `ScoutDaemonStateFile`, `ScoutDaemonLock`) joins onto `WorktreePath()` and claims the table is mirrored verbatim in CONSTRAINTS' Cwd Resolution Invariant; scope names only "fabric-unified-view.md slice 9", and `docs/shared-libs/README.md:35` ("worktree-anchored sink `.lyx/logs`") is unlisted.
**Fix:** Add the anchoring-table paragraph and `docs/shared-libs/README.md:35` to the docs-to-correct list, and state whether the CONSTRAINTS mirror it references still exists.

### [GAP] Hub-level `<hub>/.lyx` has no named creator
**Section:** `hub-level-dotlyx-is-a-recognised-geometry-element`
**Issue:** The decision says `<hub>/.lyx` is "created and named by fabric" but names no fabric call site, while `reedengine/lifecycle.go:250-253` currently `MkdirAll`s it on every boot for a documented reason (dir must exist and be swept before the boot loop); whether that call stays is undecided, and Testing lists no assertion for hub-level creation.
**Fix:** Name the fabric call site that creates `<hub>/.lyx`, state explicitly whether reed keeps its idempotent `MkdirAll` (needed for pre-fix hubs), and add a test line.

### [GAP] `logger.WorktreeLogsDir` rename left non-committal
**Section:** Testing — "Anchor re-parenting"
**Issue:** "`logger.WorktreeLogsDir`'s name should change with its anchor" decides nothing — no new name, no call-site/doc list, and it is an exported symbol referenced from `sink.go:97`, `worktreelogs_test.go` and `cmd/lyx/constructoranchoring_test.go`.
**Fix:** Either fix the new name and list its call sites, or record explicitly that the rename is deferred and the symbol keeps its name this slice.

### [NOTE] Shuttle rundir keeps two anchors in one function
**Section:** Technical context re-anchor list
**Issue:** Only `shuttleengine/rundir.go:51` (the `.lyx/shuttle` default) is re-anchored; `:56`'s config-supplied `cfg.RunDir` branch stays `filepath.Join(layout.WorktreePath(), …)`, so the same function resolves against two different bases when `AnchorRel != "."`.
**Fix:** State whether the configured-RunDir branch re-anchors too or is deliberately left worktree-relative.

### [NOTE] Weft-pair teardown with live `.lyx` state unaddressed
**Section:** `unwire-never-deletes-weft-content` (final sentence)
**Issue:** ".lyx … disappears with the weft worktree when `Remove` tears the pair down" is asserted without covering the open-handle case the adoption decision treats as significant on Windows — `Remove`'s `git worktree remove --force` now deletes live reed/scout state inside the weft worktree.
**Fix:** State the expected behaviour (and error contract, if any) for `Remove` when a process holds files under the weft-side `.lyx`.

## Verdict

GAPS_FOUND
Four gaps: scout re-anchor call sites, stale anchoring doc, hub `.lyx` creator, undecided logger rename.
MILL_REVIEW_END
