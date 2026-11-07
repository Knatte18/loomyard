// Package fabricvocab holds the predicates that decide whether a spelling breaks the fabric vocabulary.
// It also holds the data they read:
// the two side tokens, the owner set that may spell them, the doc roots the scan covers,
// and the retired host-geometry identifiers.
//
// The repository scan that enforces PATTERN-fabric-boundary asks it.
// The plan parser's fabric-vocabulary-name check asks it too, so both refuse the same spellings.
// It is a production package because production code cannot import a test file or a testkit kit.
// It imports only the standard library.
package fabricvocab
