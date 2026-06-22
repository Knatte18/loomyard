# CLAUDE.md — Loomyard (lyx)

## Mill wiki

Never write to the mill wiki directly. Absolutely all interaction with the mill
wiki goes through mill's wiki module — never raw `git` on the wiki, never `Edit`/`Write`
on wiki files (`Home.md`, `_Sidebar.md`, `proposal-*.md`, `tasks.json`), and never
`cp`-into-wiki. Use the daemon client (`wiki._client`: `upsert_task`, `set_phase`,
`merge_tasks`, `list_tasks_*`) or the `/mill-*` skills (`mill-add`, `mill-groom`,
`mill-wiki-push`, …). The daemon owns the wiki repo and serializes every write.

## Filesystem links (fslink)

All cross-OS links go through `internal/fslink`. On Windows it uses **directory
junctions** (mount-point reparse points), which need no special privileges; on other
platforms it uses symlinks. The cross-platform contract is **directory-only**:
`fslink.CreateDirLink` is the entry point, and a `CreateFileLink` is reserved for the
future. Do not rely on Windows **file** symlinks — they require admin / Developer Mode
and are not available on every dev machine, so junctions (directory links) are the only
link type guaranteed to work everywhere.
