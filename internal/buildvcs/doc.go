// doc.go carries the package-level doc comment for internal/buildvcs.

// Package buildvcs reads the VCS identity of the running lym binary in-process.
// The identity is the revision, the modified flag and the commit time the toolchain stamps.
// Running reads the running binary's stamp.
// OfFile reads the stamp of a Go binary on disk, such as an installed lym.
// It is a stdlib-only leaf that sits beside internal/buildinfo, not inside it, because buildinfo stays import-free.
package buildvcs
