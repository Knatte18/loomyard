// doc.go carries the package godoc for fswatch: the surface, the no-coalescing rule and the fallback contract.

// Package fswatch is the one file-change event source for every lyx process.
//
// Watch opens one fsnotify watcher on a directory and returns a Watcher whose Events channel delivers one Event per create, write, rename or remove of an entry.
// Event carries the entry's base name and the operation as a string.
// Names filter the entries; with none, every entry of the directory is delivered.
//
// The package coalesces and debounces nothing: one filesystem change is one Event, and the caller owns timing.
//
// An fsnotify failure at open or add, such as the inotify watch limit or an unsupported filesystem, is returned from Watch.
// The caller then falls back to its poll cadence.
// fsnotify's own error channel is drained and logged at Debug.
//
// Close stops the watcher and closes the Events channel.
//
// The package imports only the standard library, fsnotify and internal/logger.
package fswatch
