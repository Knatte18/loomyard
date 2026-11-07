// doc.go carries the package-level doc comment for internal/lymdirs.

// Package lymdirs is a stdlib-free leaf that names the lym directory tokens for every package that needs them.
// It exists so no engine owns them.
// Naming them from it cannot create an import cycle through internal/logger or internal/lymcwd.
// LymDirName is the tracked directory in a worktree, and DotLymDirName is the one git never tracks.
// BoltDirName names the Bolt checkout inside a hub.
// The other constants name the directories modules keep under a worktree's or the hub's never-tracked directory.
// UsageDirName and RaddleDirName are the exceptions: they sit under a worktree's tracked LymDirName.
// Each constant's doc comment says where its directory sits and what it holds.
package lymdirs
