// annotations.go declares the cobra command annotation keys that cmd/lym's root pre-run consults.
// The keys live in internal/clihelp because the packages that set them already import it.
// So the keys add no dependency edge.

package clihelp

// SkipStencilSeedAnnotation is the cobra-annotation key a command sets to decline the root pre-run's stencil-seed pass.
// Declining is all-or-nothing per command: it is for a command that reads no stencils and is expected to stay silent.
// A value other than "true" never opts out, so a "false" cannot silently read as an opt-out.
const SkipStencilSeedAnnotation = "lym.skip-stencil-seed"

// AnnotationEnabled is the one value that reads as "on" for every annotation key in this package.
const AnnotationEnabled = "true"

// AudienceAnnotation is the cobra-annotation key an invocable command carries to name who calls it.
// Its value is one of the audience values,
// and the command index lists a command only under the audience it names.
const AudienceAnnotation = "lym.audience"

// AudienceOperator marks a command the operator, or the orch acting for them, runs by hand.
const AudienceOperator = "operator"

// AudienceRole marks a command a spawned role agent runs, named by that role's prompt.
const AudienceRole = "role"

// AudienceInternal marks a command lym itself runs, such as a daemon or a pinned key binding.
const AudienceInternal = "internal"

// Audiences is the closed set of audience values, in index order.
var Audiences = []string{AudienceOperator, AudienceRole, AudienceInternal}

// IndexNoteAnnotation is the cobra-annotation key whose value the command index appends to the command's line.
// The value is appended after one space.
// The note prints only when the index renders the audience named by the `lym.index-note-audience` annotation;
// a note without that annotation never prints.
const IndexNoteAnnotation = "lym.index-note"

// IndexNoteAudienceAnnotation is the cobra-annotation key naming the audience whose index prints the command's note.
// The command index appends a command's index note only when rendering that audience.
// It prints no note for a command without this annotation.
const IndexNoteAudienceAnnotation = "lym.index-note-audience"
