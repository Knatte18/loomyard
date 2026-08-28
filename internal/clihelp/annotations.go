// annotations.go declares the cobra command annotation keys internal/clihelp exposes for cmd/lyx's
// root pre-run to consult. internal/clihelp is where this belongs: it already owns the CLI-wide seams
// both cmd/lyx and every *cli package import, so this constant creates no new dependency edge in
// either direction.

package clihelp

// SkipStencilSeedAnnotation is the cobra-annotation key a command carries to decline the root
// pre-run's stencil-seed pass.
// Declining is all-or-nothing per command: it is for a command that reads no stencils and is expected
// to stay silent.
// A value other than AnnotationEnabled never opts out -- so a "false" cannot silently read as an
// opt-out.
const SkipStencilSeedAnnotation = "lyx.skip-stencil-seed"

// AnnotationEnabled is the one value that reads as "on" for any annotation key in this package,
// SkipStencilSeedAnnotation included.
const AnnotationEnabled = "true"
