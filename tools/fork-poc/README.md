# fork-poc — Claude session-fork measurement rig (throwaway)

Spike rig for the `session-fork-diversity-spike` task: measure whether
`claude --resume <id> --fork-session` supports a fork-based review cluster
(one explorer forked into N reviewers) before any burler/perch/mux work.

**This is a spike artifact, not product code.** It is driven manually by the
orchestrating session (mill-start); the scripts here are the reproducible
pieces: prompts, the usage summarizer, and this choreography note.

## Where things run

- **Rig, prompts, results, findings:** this repo (`tools/fork-poc/`,
  `docs/research/session-fork-spike.md`), committed as produced.
- **Live sessions:** the sandbox hub (`~/Code/lyx-test-HUB/lyx-test`), as
  `lyx mux` strands, so the operator can watch via `lyx mux attach`. mux's
  `CleanClaudeEnv` strips `CLAUDECODE`/`CLAUDE_CODE_*` at server spawn — the
  proven fix for the attempt-1 "no transcript persisted" blocker.
- **Review target:** `internal/modelspec` in THIS worktree, read by sessions
  via `--add-dir <this worktree>` (read-only; sessions are told not to write).

## Choreography (per session)

1. Generate a UUID; launch via
   `lyx mux add --name <n> --cmd "claude --session-id <uuid> --add-dir <wt> \"$(cat prompt)\""`.
2. Transcript appears deterministically at
   `~/.claude/projects/-home-knatte-Code-lyx-test-HUB-lyx-test/<uuid>.jsonl`.
3. Wait until the `.jsonl` exists and stops growing (the smoke-test pattern
   from `internal/muxcli/smoke_resume_test.go`).
4. Harvest: final assistant text + `usage` blocks via `usage_sum.py`.
5. Fork: new strand with `claude --resume <explorer-uuid> --fork-session
   --session-id <fork-uuid> "<prompt>"` (plus `--model <m>` for M2/B3).

## Arms

- **step-0** — plain session, codeword; proves transcript persistence.
- **M1** — fork probed for inherited nonce + code facts, tools forbidden.
- **M2** — fork with `--model` switch, same probe.
- **B1** — 3 forks, identical generic review prompt.
- **B2** — 3 forks, one lens each (correctness / error-handling / test-gap).
- **A** — 3 cold sessions, same lenses, each explores from scratch.

Coverage judging: findings deduped/clustered by the orchestrating session
against a rubric recorded in the findings doc; raw outputs committed under
`results/` so the judgment is auditable.
