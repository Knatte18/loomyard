// Package planindex names the code index a plan gate calls, without linking it.
//
// It declares the finding types the plan gates report, ErrQuarryUnavailable, and the Index and Delta interfaces a package receives instead of importing the package that implements them.
// The real implementation lives in internal/planglyph, which links tree-sitter through quarry; this package imports only the standard library and internal/planparser, so a package that takes an Index stays free of the C grammars.
// The CLI layer is the one place that builds the real Index and hands it down.
package planindex
