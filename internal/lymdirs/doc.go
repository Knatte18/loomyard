// doc.go carries the package-level doc comment for internal/lymdirs.

// Package lymdirs is a stdlib-free leaf existing solely so internal/configengine, internal/logger, internal/gitkit, internal/fabricengine and every module engine can name the two lym directory tokens without any of them owning the pair, and without risking the internal/fabricengine -> internal/logger -> internal/lymcwd import cycle.
// It also declares the names of the directories modules keep under a worktree's or the hub's never-tracked directory, among them HeddleDirName for heddle's render lock.
// Under the hub's never-tracked directory sit AdoptDirName for the adopt check's files and FrontTaskDirName for the front-task record.
package lymdirs
