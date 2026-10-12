// Package fabricvocab holds the predicates that decide whether a spelling breaks the fabric vocabulary.
// It also holds the data they read:
// the two side tokens, the owner set that may spell them, the doc roots the scan covers and the retired host-geometry identifiers.
//
// Its callers, by role, are the repository scan that enforces PATTERN-fabric-vocabulary and the plan parser's fabric-vocabulary-name check.
// Both refuse the same spellings because both ask this package.
// It is a production package because production code cannot import a test file or a testkit kit.
//
// It imports only the standard library, and holds only the vocabulary predicates and their data.
package fabricvocab
