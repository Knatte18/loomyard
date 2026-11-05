// doc.go carries the package-level doc comment for internal/lymdirs.

// Package lymdirs is a stdlib-free leaf existing solely so internal/configengine, internal/logger, internal/gitkit, internal/fabricengine and every module engine can name the two lym directory tokens without any of them owning the pair, and without risking the internal/fabricengine -> internal/logger -> internal/lymcwd import cycle.
// It also declares BoltDirName, the name of the Bolt checkout inside a hub, and the names of the directories modules keep under a worktree's or the hub's never-tracked directory, among them HeddleDirName for heddle's render lock and LocksDirName for the locks of a checkout's tracked files.
// UsageDirName and RaddleDirName are the tracked names among them, the directories under a worktree's LymDirName that hold a run's usage records and a raddle run's files.
// Under the hub's never-tracked directory sit AdoptDirName for the adopt check's files, FrontTaskDirName for the front-task record and DriftJudgeDirName for the drift judge's snapshots of the checkouts.
package lymdirs
