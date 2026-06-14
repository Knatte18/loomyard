All claims verified. B8 confirmed: daemon.go implements cmdDaemon with maxRecoveries=3, windowDur=60s, coldRecover. The muxpoc.md content claims (state path, AtomicWrite cross-module reach, sanitizeEnv stripping CLAUDECODE/CLAUDE_CODE_*, lock paths) all match state.go. The overview.md staleness (B1) is confirmed: intro says worktree/mux "coming next", Structure shows only board, dispatch shows only board/init, modules list shows worktree/mux as "Sketch", line 90 lists internal/state, Other docs has worktree.md "(sketch)".

I have enough to render a verdict. The discussion is exceptionally well-grounded: every finding A1-A4, B1-B8, and the muxpoc.md inventory matches the actual source. Decisions all have rationale + rejected alternatives. Scope in/out is explicit. Testing strategy is named (compilation, link integrity, grep guard, checklist). Constraints are covered.

Let me note a couple of minor things I observed: the worktree config.go comment self-references "Mirrors internal/board/config.go" (which is the stale file being fixed) — but that's out of scope per the reference-pattern decision and not a discussion gap. The grep guard pattern in Testing uses regex that should be fine. One genuinely worth-noting item: the Testing grep guard searches `internal/` too, and B8's stale phrase "not yet implemented in this batch" in muxpoc/cli.go matches the guard's `not yet implemented` alternative — which is good (it confirms B8 is caught) but the guard would also flag the legitimate config.md/state.md "now-removed config layer" mentions only if they contain those phrases; they don't. No gap there.

MILL_REVIEW_BEGIN
# Review: docs-stale-sweep

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-14
```

The discussion is exhaustively source-grounded. I verified every load-bearing claim against the tree: the two-layer config model (`internal/config/config.go` Load), the stale board.md blockquote/layered-model/env subsections (~225-272), board cli.go/config.go "layered"/`.mhgo/board.yaml` wording, config.md Status block + "preserved"/"Not supported in v1" lines, the 4-way main.go dispatch (init/board/muxpoc/worktree), overview.md staleness (worktree/mux "Sketch", board-only Structure, `internal/state` listed as existing), daemon.go (`cmdDaemon`, `maxRecoveries=3`, `windowDur=60s`, `coldRecover`) refuting cli.go's "not yet implemented in this batch", and the muxpoc state.go facts (`.mhgo/muxpoc-state.json`, `board.AtomicWrite` cross-module reach, `sanitizeEnv` stripping `CLAUDECODE`/`CLAUDE_CODE_*`). All findings A1-A4, B1-B8 and the muxpoc.md inventory match shipped code.

Scope in/out is explicit and defensible (config fixes, tree-wide sweep, new muxpoc.md; conformance/gofmt/worktree-reference/behaviour-change out). All five `### Decision:` blocks carry rationale + rejected alternatives. Constraints (docs-only, no line-ending changes, no new config grammar in module docs, preserve legitimate runtime-state `.mhgo/` references) are stated. Testing is a named verification strategy (build/vet/test green, markdown link pass, grep staleness guard, A1-B8 checklist) appropriate for a docs task with no TDD candidates. Failure modes for the doc work (legitimate-vs-stale `.mhgo/` disambiguation, link integrity) are addressed.

## Verdict

APPROVE
Fully decided, source-accurate, and scoped; a plan writer can proceed without clarification.
MILL_REVIEW_END
