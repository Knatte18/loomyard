// doc.go carries the package-level doc comment for internal/buildvcs.

// Package buildvcs reads the VCS identity of the running lyx binary in-process.
// The identity is the revision, the modified flag and the commit time the toolchain stamps.
// Its readers are the hub reconcile build stamp, the stencil and spec seeding that orders a board copy against the running binary, and the seed-commit label.
// It is a stdlib-only leaf that sits beside internal/buildinfo, not inside it, because buildinfo stays import-free.
package buildvcs
