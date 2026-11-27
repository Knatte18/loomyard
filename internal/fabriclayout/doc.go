// Package fabriclayout holds fabric's names, path constants and pure path functions.
// Those functions work over absolute paths and plain strings.
// It also holds plain option and result types and the mutation record data.
//
// It imports only the standard library, internal/lymdirs and internal/weftname.
// It runs no git, starts no process, touches no filesystem and takes no lock.
// A function that touches the filesystem or git stays in internal/fabricengine.
// So does an option type with a seam or callback field.
//
// A file outside fabric that uses only these names imports this package and not the engine,
// so a change to the engine leaves that file out of the gate scope.
package fabriclayout
