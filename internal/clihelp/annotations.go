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

// AudienceAnnotation is the cobra-annotation key an invocable command carries to name who calls it.
// Its value is one of Audiences; the command index lists a command only under the audience it names.
const AudienceAnnotation = "lyx.audience"

// AudienceOperator marks a command the operator, or the orch acting for them, runs by hand.
const AudienceOperator = "operator"

// AudienceRole marks a command a spawned role agent runs, named by that role's prompt.
const AudienceRole = "role"

// AudienceInternal marks a command lyx itself runs, such as a daemon or a pinned key binding.
const AudienceInternal = "internal"

// Audiences is the closed set of AudienceAnnotation values, in index order.
var Audiences = []string{AudienceOperator, AudienceRole, AudienceInternal}

// IndexNoteAnnotation is the cobra-annotation key whose value the command index appends, after one space, to the line of the command carrying it.
// The note prints only when the index renders the audience the command's IndexNoteAudienceAnnotation names; a note without that annotation never prints.
const IndexNoteAnnotation = "lyx.index-note"

// IndexNoteAudienceAnnotation is the cobra-annotation key naming the one audience, a member of Audiences, whose index prints the command's index note.
// The command index appends a command's index note only when rendering that audience, and prints no note for a command without this annotation.
const IndexNoteAudienceAnnotation = "lyx.index-note-audience"
