MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
duration_s: 154.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md → manifest/designs/producers-standalone.md
date: 2026-08-17
```

## Findings

### [NIT:scope] T4 blast-radius enumerated by constructors only
**Demoted-from:** BLOCKING
**Section:** T4 "shuttle+reed+tokenvocab told-geometry"
**Issue:** T4 says "all their construction sites are the same five CLI files", but `shuttleengine.FindRun` — which T4 changes to take `anchorRoot` — is also called at `internal/websterengine/recoverbatch.go:182` and `internal/websterengine/runlevel.go:529`, and `internal/websterengine` is absent from T4's Files list; the enumeration method counted constructor call sites, not callers of every signature being changed.
**Fix:** State the enumeration method as "every caller of every changed exported symbol", and re-derive each task's Files list and dependency edges from it (T4↔T7 in particular).

### [BLOCKING:design] Standalone reed geometry left undecided
**Section:** T4 / T6
**Issue:** T4 makes `reedengine.New` take `sessionName` and `HubLogsDir` a told hub path (today `fabricengine.HubScratchDir(l.HubPath)` + `/logs`, verified at `lifecycle.go:35`), but T6 decides only the target directory and a per-invocation `socketKey` — nothing says what names the session or where reed writes logs when there is no hub.
**Fix:** Pin standalone values (or a documented non-hub logs location) for `sessionName` and the reed logs directory in T6's brief, as explicitly as `socketKey` is pinned.

### [NIT:decision] Where standalone writes its ephemeral artifacts
**Demoted-from:** BLOCKING
**Section:** T5 / T6, "the standalone CLI path"
**Issue:** Standalone passes the target directory as `anchorRoot`, so `burlerengine`'s `.lyx/burler` and `perchengine`'s `RunsDir`/`ScratchDir` land inside an arbitrary user folder; the doc states a disclosure obligation for `--stencils-dir` only and never gives these a disposition or relates them to the Durable-vs-Ephemeral State Invariant.
**Fix:** State in T6 where producer scratch/state goes in standalone mode (target dir vs. a user-scoped location) and whether the invariant's `_lyx`/`.lyx` sibling rule engages.

### [NIT:consistency] Stencil invariant reword deferred out of its commit
**Demoted-from:** BLOCKING
**Section:** T6 Files / T10
**Issue:** T6 introduces the told stencils directory but lists no `CONSTRAINTS.md` edit, deferring the Stencil Ownership Invariant reword to T10 — so between waves 4 and 5 the shipped code contradicts an invariant that pins reads to `<hub>/_board/_lyx/stencils/`, against CLAUDE.md's same-commit docs rule that T1, T3 and T4 do honour.
**Fix:** Move the Stencil Ownership reword into T6's own commit and leave T10 the three-tier invariant and cross-doc consolidation.

### [BLOCKING:design] Core capability verified only by a manual run
**Section:** T6 "Verify"
**Issue:** The one behaviour the whole design exists for — pre-run degrading outside a git repository — is checked by a manual acceptance run; T6 names no new automated test, whereas T2 explicitly requires "a new test per loader", so nothing pins the branch against regression.
**Fix:** Require an automated test in T6 asserting `burlercli`/`perchcli` build their engine stack from told values when `lyxcwd.Resolve` fails, and name its tier (untagged is feasible — asserting the branch needs no spawn).

### [NIT:consistency] Task numbers non-monotonic across waves
**Section:** "Task decomposition"
**Issue:** Waves run T1/T2/T4, then T3/T8, then T5/T7, then T6/T9, then T10, so a reader tracking dependencies jumps backwards repeatedly.
**Fix:** Either renumber in wave order or add a one-line ID→wave index at the top of the section.

## Verdict

REQUEST_CHANGES
Sourcing is excellent; enumeration method and standalone-mode geometry decisions need closing first.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
