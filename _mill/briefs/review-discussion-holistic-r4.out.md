MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md -> manifest/designs/producers-standalone.md
date: 2026-08-17
```

## Findings

### [BLOCKING:consistency] Sixth task-shared file breaks the contention premise
**Section:** "no additive twins — parallelism comes from wave scheduling" + T3/T4/T5/T7/T9 Files
**Issue:** The decision rests on contention concentrating "in exactly two files", but `cmd/lyx/constructoranchoring_test.go` asserts `pattern.FileHere` (80,129), `reedengine.HubLogsDir` (96,144), `perchengine.RunsDir`/`ScratchDir` (79,89,128,138), `websterengine.Dir`/`ReportsDir`/`PromptsDir`/`ScratchDir` (77-78,87-88), `scoutengine.DaemonStateFile`/`DaemonLock` (91-92,140-141) — so T3, T4, T5, T7 and T9 must all edit it, yet only T1 names it, and T3/T4/T5/T7 omit `./cmd/lyx/...` from Verify. It is also a named enforcer of the Durable-vs-Ephemeral State Invariant (`CONSTRAINTS.md:84`).
**Fix:** State this file's disposition once (rewritten in place per task, split per package, or retired) and reconcile the parallel-safety claims for T5∥T7 and T1∥T4 against it.

### [BLOCKING:design] `buildChannel` is unreachable from the CLI packages
**Section:** "stencils — a told directory" and T6's `mode` row
**Issue:** T6 pins reuse of the `buildChannel == "dev"` selector "verbatim", but `buildChannel` is an unexported `package main` var in `cmd/lyx/stencilseed.go:29`, stamped by `tools/deploy/main.go:62` as `-X main.buildChannel=dev`; T6's Files list contains no production file under `cmd/lyx` and `internal/burlercli`/`internal/perchcli` cannot read it.
**Fix:** Decide how the mode reaches the CLI package (exported into a shared package with the ldflags path changed, or injected from `cmd/lyx`) and add the affected files, including `tools/deploy/main.go` if the flag path moves.

### [NIT:decision] `--stencils-dir` omission behaviour unstated
**Section:** T6 pinned-values table, "stencils dir" row
**Issue:** Every other told value has a default; this one is only "told", with no statement of whether omitting it in standalone is a hard error or defaults under `<state>` — and no statement of what it does when a real worktree resolves.
**Fix:** Pin required-vs-defaulted and the in-hub behaviour (ignored or refused) in the same table.

### [NIT:design] `hash8` input normalization unspecified
**Section:** T6, `<state>` derivation
**Issue:** `hash8` is "the target directory's absolute-path hash" with no hash function and no normalization rule, so two spellings of the same directory (symlinked path, differing case) yield different `<state>`, socket and session — undercutting the "one tmux server per target directory, resumable" property this section makes load-bearing.
**Fix:** Name the hash and whether the input is `filepath.Abs` alone or `EvalSymlinks`-resolved.

## Verdict

REQUEST_CHANGES
Two blocking gaps: a shared enforcement test contradicting the contention premise, and an unreachable build-channel selector.
MILL_REVIEW_END
