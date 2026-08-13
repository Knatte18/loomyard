// doc.go carries the package-level doc comment for internal/lyxdirs.

// Package lyxdirs is a stdlib-free leaf existing solely so internal/configengine,
// internal/logger, internal/gitkit, internal/fabricengine and every module engine can name the
// two lyx directory tokens without any of them owning the pair, and without risking the
// internal/fabricengine -> internal/logger -> internal/lyxcwd import cycle.
package lyxdirs
