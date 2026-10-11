// Package burlercli is the `lyx burler` command tree: the debug-only CLI over internal/burlerengine.
//
// # The verbs
//
// `lyx burler run` drives one review+fix round from a profile file, in hub mode or standalone.
// The round is one seat table on the seats engine: the fixer under the role burler-chair and the reviewer under the role burler-advisor-1.
// The fixer waits on the round's ready marker with `lyx shuttle await`, and the engine writes the marker once the reviewer's review is accepted.
// `lyx burler validate-review <review-file>` is the review gate's read-only self-check and needs no hub, mode or git repository.
//
// # Wiring
//
// The package is the module's claudeengine wiring point: wiring.go builds the engine stack for whichever mode wins, and cli.go holds the command tree alone.
package burlercli
