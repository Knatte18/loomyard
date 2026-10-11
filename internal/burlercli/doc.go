// Package burlercli is the `lyx burler` command tree: the debug-only CLI over internal/burlerengine.
//
// # The verbs
//
// `lyx burler run` drives one review+fix round from a profile file, in hub mode or standalone.
// The round is two sequential sessions: the reviewer under the role burler-review-chair, then the fixer under the role burler-fix-chair.
// The fixer runs only when the review reached done and the engine accepted it, and it is told the digest of the accepted review.
// A review with no findings spawns no fixer: the engine writes the fixer report itself and the envelope reports `fixSkipped`.
// A reviewer that died or timed out is the envelope's outcome, and runs no fixer.
// `lyx burler validate-review <review-file>` is the review gate's read-only self-check and needs no hub, mode or git repository.
//
// # Wiring
//
// The package is the module's claudeengine wiring point: wiring.go builds the engine stack for whichever mode wins, and cli.go holds the command tree alone.
package burlercli
