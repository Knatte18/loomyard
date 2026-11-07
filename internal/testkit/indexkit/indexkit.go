// Package indexkit stubs the code index that a plan gate calls.
// It serves the CLI tests that pin which index a verb validates through.
// It asserts nothing.
package indexkit

import (
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ValidateStub is a planindex.Index whose Validate answers with Finding alone.
// A verb's output carries Finding only when the verb validated through this index.
// Every other method forwards to Index, which panics when nil.
type ValidateStub struct {
	planindex.Index
	Finding planindex.Finding
}

func (s ValidateStub) Validate(*planparser.Plan, string) ([]planindex.Finding, error) {
	return []planindex.Finding{s.Finding}, nil
}
