// doc.go carries the package-level doc comment for internal/lymdirs.

// Package lymdirs is a stdlib-free leaf existing solely so internal/configengine, internal/logger, internal/gitkit, internal/fabricengine and every module engine can name the two lym directory tokens without any of them owning the pair, and without risking the internal/fabricengine -> internal/logger -> internal/lymcwd import cycle.
package lymdirs
