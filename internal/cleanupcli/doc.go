// Package cleanupcli is the cleanup module: the routine removal of the hub prime's leftover state, one segment per owner.
//
// # The scratch segment
//
// The prime's `.scratch/` directory belongs to no other module, so this package owns its cleanup.
// A direct child of it is removed once nothing under it has changed for the retention age, which matches the default age of the logger's trace sweep, and no board entry that is not done cites its `.scratch/<name>` path in its title, brief or body.
// A link directly under `.scratch/` is judged on its own modification time and removed as a link, never followed.
// Only direct children of the prime's `.scratch/` are touched, and only after the board has been read.
package cleanupcli
