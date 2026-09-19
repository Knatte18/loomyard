# reed: multi-window ideas (cross-worktree columns, own-window anchoring, session-groups attach)

> **Status: Someday, not yet designed.** Three related, not-yet-scoped ideas about reed outgrowing "one pane per strand, one window per worktree." Grouped here because each is a candidate building block for the others.

## Cross-worktree columns

All worktrees in one tmux window, a column per worktree.

Needs its own name for the per-worktree grouping layer this introduces — it cannot be called "session": that's already tmux's own term, and already 1:1 with a worktree in reed's plumbing today. Also needs a decision on how many columns fit before falling back to tmux windows-as-pages.

Candidate group-layer names surveyed so far and still free: **Heddle, Batten, Bobbin, Sley**. Already taken elsewhere in this codebase: Warp, Weft, Shuttle, Treadle, Shed, Loom, Reed, Strand, Fabric, Quarry, Crucible, and Selvage (claimed by the header-replacement item, see [reed-header-selvage.md](reed-header-selvage.md)).

## Own-window strand anchoring

A `display` anchor that spawns a strand into its own switchable tmux window instead of a pane.

## Independent per-window attach via tmux session groups

Today two `lyx reed attach` invocations join the same tmux session object — a shared "current window" pointer, and mismatched terminal sizes fighting over layout.

tmux's session-groups feature (`new-session -t <existing-session>`) would let each invocation show a different window independently, which reed's `attach` verb does not use today. This is a candidate building block for both ideas above, likely exposed as something like an `--window <n>` flag, or hidden entirely behind one verb that spawns the needed loose terminal windows automatically.

## Related

- [reed-header-selvage.md](reed-header-selvage.md) — claims the "Selvage" name and notes reed has no window support today.
