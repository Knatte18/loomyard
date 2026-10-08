// doc.go carries the package-level doc comment for internal/buildvcs.

// Package buildvcs reads the VCS identity of the running lyx binary in-process.
// It is a stdlib-only leaf that sits beside internal/buildinfo, not inside it, because buildinfo stays import-free.
package buildvcs
