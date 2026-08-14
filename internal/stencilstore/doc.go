// doc.go carries the package doc comment for stencilstore.

// Package stencilstore owns the entire stencil lifecycle -- seeding, hash-stamping, edit detection,
// reading, and validation -- against a caller-supplied absolute stencils directory.
// It never derives that directory's geometry itself: baseDir always arrives fully resolved from the
// caller (fabricengine.StencilsDir), which is what keeps this package's tests hermetic against a
// bare t.TempDir() and keeps it free of the _board/_lyx literals those packages own.
package stencilstore
