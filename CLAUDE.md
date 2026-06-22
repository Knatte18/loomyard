# CLAUDE.md — Loomyard (lyx)

## Mill wiki

Never write to the mill wiki directly. Absolutely all interaction with the mill
wiki goes through mill's wiki module — never raw `git` on the wiki, never `Edit`/`Write`
on wiki files (`Home.md`, `_Sidebar.md`, `proposal-*.md`, `tasks.json`), and never
`cp`-into-wiki. Use the daemon client (`wiki._client`: `upsert_task`, `set_phase`,
`merge_tasks`, `list_tasks_*`) or the `/mill-*` skills (`mill-add`, `mill-groom`,
`mill-wiki-push`, …). The daemon owns the wiki repo and serializes every write.

## Agent execution: interactive psmux sessions, NOT `claude -p`

Every LLM agent lyx spawns (loom producers, the review handler, cluster reviewers,
the progress-judge) runs as an **interactive session inside psmux** — never headless
`claude -p`.

**Why (economic, not technical):** Anthropic announced that headless `claude -p` will
no longer draw on a Pro/Max subscription — it will be billed as API and reserved
interactive sessions for the subscription. (Slated for 2026-06-15, postponed, but
expected to land.) Headless is technically possible but would force API cost, so we do
not use it. Interactive sessions keep subscription coverage; psmux is what makes a
programmatically-driven session *interactive*.

**Consequences for design:**
- The orchestrator drives agents by launching an interactive session in a psmux
  pane/window, injecting the prompt, and detecting completion via Claude Code hooks.
  I/O still rides the **file contract** (the agent writes its output files; Go reads
  them) — that part is unchanged from a headless model.
- Therefore `internal/agent` depends on the **mux** module; it cannot be built purely
  on a headless `exec`. mux is on loom's critical path for this reason.
- Agents are provider-agnostic via **engines** — per-LLM adapters (a Claude engine now;
  Gemini etc. later) that know how to launch/drive their provider as a psmux session.
  The verdict/output contract is provider-invariant, which is what makes engines
  swappable. **Non-Claude support is not a current priority.**
- Cluster-reviews (N parallel reviewers) scale via psmux **windows** (spawned clusters
  land in their own windows, not a pane explosion) — long-term mux work, not now.
