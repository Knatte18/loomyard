# host-visibility — CLAUDE.local.md / CONSTRAINTS.md invisible in host's git history

> **Status: Design — not built.** Split out from [fabric.md](fabric.md) — the vacation-time
> discussion bundled this into the Fabric cutover step opportunistically ("we're touching that
> area anyway"), but the mechanism itself is filesystem-linking/worktree setup, not git
> coordination — it doesn't depend on `fabric`'s own architecture (`gitrepo`, `SyncWeft`,
> `RevertWithWeft`) at all. Per the [documentation
> lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into whichever
> package ends up owning worktree setup (`warp`/`fabric`'s topology side, or `loom`'s init step)
> when this lands, and this file is deleted.

## Design principle

Nothing lyx-related should be visible in the host repo's own git history — everything lives in
`weft`.

## `CONSTRAINTS.md`-equivalent (a directory)

Can use the same junction mechanism raddle already uses — junctions work for directories, and
this is a directory of docs.

## `CLAUDE.local.md` (a single file)

Loads alongside `CLAUDE.md`, additive. Must physically exist in host's working tree, so a
directory junction doesn't apply directly:

- **Symlinks, not hard links.** Hard links are inode-based, not path-based — if whatever
  regenerates the source `CLAUDE.md` in weft uses the standard safe-write pattern (write to a
  temp file, atomic rename over the target), a hard link on the host side keeps pointing at the
  *old* inode and silently stops reflecting updates. Symlinks are path-based and always resolve
  to whatever currently occupies the target path.
- **Windows note:** symlinks normally require admin, but Developer Mode (Settings → For
  Developers, Windows 10 1703+) grants `SeCreateSymbolicLinkPrivilege` to standard users without
  elevation — worth checking whether this can be enabled on managed/non-admin machines before
  assuming it's blocked.
- **Fallback if symlinks aren't available:** explicit re-link (or copy) at `loom` init time, not
  automatic filesystem-level linking. Accepts staleness only within a single run, not across
  runs — acceptable since `CLAUDE.md` content changes rarely mid-session.
- `CLAUDE.local.md` is **not** auto-gitignored by Claude Code — `loom`'s init step must
  explicitly ensure it's listed in host's `.gitignore` (same mechanism already used for weft
  junctions).
- If host already has its own pre-existing, committed `CLAUDE.md` (someone deliberately approved
  Claude's use in that repo), that's fine — `CLAUDE.local.md` loads alongside it and takes
  precedence on conflict, no special handling needed.

## Related

- [fabric.md](fabric.md) — owns the junction re-pointing mechanism this could reuse for the
  `CONSTRAINTS.md`-equivalent directory.
- [loom.md](loom.md) — the init/session-bootstrap step that would trigger the symlink fallback
  and `.gitignore` entry for `CLAUDE.local.md`.
