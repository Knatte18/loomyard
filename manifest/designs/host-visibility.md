# host-visibility — CLAUDE.local.md invisible in host's git history

> **Status: Design — not built.** Split out from [`internal/fabricengine`](../../internal/fabricengine/doc.go) — the vacation-time discussion bundled this into the Fabric cutover step opportunistically ("we're touching that area anyway"), but the mechanism itself is filesystem-linking/worktree setup, not git coordination — it doesn't depend on `fabric`'s own architecture (`gitrepo`, `SyncWeft`, `RevertWithWeft`) at all. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into whichever package ends up owning worktree setup (`warp`/`fabric`'s topology side, or `loom`'s init step) when this lands, and this file is deleted.
>
> **Scope reduced (2026-07):** the `CONSTRAINTS.md`-equivalent half of this task is superseded by the Planned `PATTERN.md` item — `PATTERN.md` lives in `weft`, already invisible to the host repo, so there is no constraints directory left to hide. This task now covers only `CLAUDE.local.md`.

## Design principle

Nothing lyx-related should be visible in the host repo's own git history — everything lives in `weft`.

## `CONSTRAINTS.md`-equivalent — superseded by `PATTERN.md`

**No longer this task's concern.** The loomyard-owned `CONSTRAINTS.md`-equivalent is `PATTERN.md` (roadmap's Planned list), and `PATTERN.md` lives in `weft` (see [board-weft-storage.md](board-weft-storage.md) — "everything that isn't warp already lives in weft"). Weft is a separate repo reached through a junction into the warp worktree, so `PATTERN.md` is **already invisible to the host repo's git history** simply by living there — exactly what the junction-to-hide-a-constraints-dir idea here was for. Once `PATTERN.md` lands, this half is done by the weft architecture; nothing extra to build. This task therefore reduces to `CLAUDE.local.md` below.

## `CLAUDE.local.md` (a single file)

Loads alongside `CLAUDE.md`, additive. Must physically exist in host's working tree, so a directory junction doesn't apply directly:

- **Symlinks, not hard links.** Hard links are inode-based, not path-based — if whatever regenerates the source `CLAUDE.md` in weft uses the standard safe-write pattern (write to a temp file, atomic rename over the target), a hard link on the host side keeps pointing at the *old* inode and silently stops reflecting updates. Symlinks are path-based and always resolve to whatever currently occupies the target path.
- **Windows note:** symlinks normally require admin, but Developer Mode (Settings → For Developers, Windows 10 1703+) grants `SeCreateSymbolicLinkPrivilege` to standard users without elevation — worth checking whether this can be enabled on managed/non-admin machines before assuming it's blocked.
- **Fallback if symlinks aren't available:** explicit re-link (or copy) at `loom` init time, not automatic filesystem-level linking. Accepts staleness only within a single run, not across runs — acceptable since `CLAUDE.md` content changes rarely mid-session.
- `CLAUDE.local.md` is **not** auto-gitignored by Claude Code — `loom`'s init step must explicitly ensure it's listed in host's `.gitignore` (same mechanism already used for weft junctions).
- If host already has its own pre-existing, committed `CLAUDE.md` (someone deliberately approved Claude's use in that repo), that's fine — `CLAUDE.local.md` loads alongside it and takes precedence on conflict, no special handling needed.

## Related

- [board-weft-storage.md](board-weft-storage.md) — establishes that `PATTERN.md` (and all non-warp content) lives in `weft`, which is what supersedes the `CONSTRAINTS.md`-equivalent half of this task.
- Planned `PATTERN.md` (see `manifest/roadmap.md`) — the loomyard-owned `CONSTRAINTS.md`-equivalent whose weft residence makes the constraints-hiding half of this task unnecessary.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — owned the junction re-pointing mechanism the now-superseded constraints-dir half would have reused (`CLAUDE.local.md` uses a symlink, not a junction).
- [loom.md](loom.md) — the init/session-bootstrap step that would trigger the symlink fallback and `.gitignore` entry for `CLAUDE.local.md`.
