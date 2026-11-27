// doc.go carries the package-level doc comment for internal/lymdirs.

// Package lymdirs is a stdlib-free leaf that names the lym directory tokens for every package that needs them.
// It exists so no engine owns them.
// Naming them from it cannot create an import cycle through internal/logger or internal/lymcwd.
// LymDirName is the tracked directory in a worktree, and DotLymDirName is the one git never tracks.
// BoltDirName names the Bolt checkout inside a hub.
// The other constants name the directories modules keep under a worktree's or the hub's never-tracked directory.
// HandOverDirName sits beside the other hub-level names and holds a machine's hand-over marks.
// PusherDirName holds the locks of the background pushers, one per push target.
// PushLedgerDirName holds the ledger of owed pushes, one entry per push target.
// IgnoreCheckDirName holds the scratch repositories that ignore rules are checked in.
// HardenSandboxDirName names the directory under a worktree's `.lym` that holds a run's throwaway sandbox hubs.
// UsageDirName, RaddleDirName, WeaverDirName and HardenDirName are the exceptions:
// they sit under a worktree's tracked LymDirName.
// Each constant's doc comment says where its directory sits and what it holds.
package lymdirs
