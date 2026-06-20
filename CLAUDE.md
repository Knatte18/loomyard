# CLAUDE.md — Loomyard (lyx)

## Mill wiki

Never write to the mill wiki directly. Absolutely all interaction with the mill
wiki goes through mill's wiki module — never raw `git` on the wiki, never `Edit`/`Write`
on wiki files (`Home.md`, `_Sidebar.md`, `proposal-*.md`, `tasks.json`), and never
`cp`-into-wiki. Use the daemon client (`wiki._client`: `upsert_task`, `set_phase`,
`merge_tasks`, `list_tasks_*`) or the `/mill-*` skills (`mill-add`, `mill-groom`,
`mill-wiki-push`, …). The daemon owns the wiki repo and serializes every write.
