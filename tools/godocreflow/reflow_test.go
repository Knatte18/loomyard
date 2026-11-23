package main

import (
	"go/token"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wideEnough is a line-length limit no test line reaches.
const wideEnough = 1000

// pkgHeader is the file header and package clause that the test sources start with.
const pkgHeader = `// pkg.go implements the pkg package.

package pkg

`

// reflowSource returns src with its reflow edits applied, or src unchanged when nothing qualifies.
// err is non-nil only on a parse failure.
func reflowSource(fset *token.FileSet, filename, src string, limit int) (string, error) {
	edits, _, err := collectEdits(fset, filename, src, limit)
	if err != nil {
		return src, err
	}
	if len(edits) == 0 {
		return src, nil
	}
	return applyEdits(src, edits), nil
}

func mustReflow(t *testing.T, src string, limit int) string {
	t.Helper()
	fset := token.NewFileSet()
	out, err := reflowSource(fset, "test.go", src, limit)
	if err != nil {
		t.Fatalf("reflowSource: %v", err)
	}
	return out
}

// TestReflowSource_ReflowsDocComments covers every doc-comment kind that the tool reflows,
// and the breaks it keeps, joins and makes.
func TestReflowSource_ReflowsDocComments(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		limit int
		want  string
	}{
		{
			name: "exported func doc",
			src: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio, and it returns an error if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
			limit: wideEnough,
			want: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio,
// and it returns an error if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
		},
		{
			name: "hard-wrapped lines are rejoined then split",
			src: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the
// schema. It merges the valid files into a single Portfolio, and it returns an error
// if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
			limit: wideEnough,
			want: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio,
// and it returns an error if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
		},
		{
			name: "package doc comment",
			src: `// Package pkg provides utilities for testing this reflow tool, and it
// exists solely for that purpose.
package pkg
`,
			limit: wideEnough,
			want: `// Package pkg provides utilities for testing this reflow tool,
// and it exists solely for that purpose.
package pkg
`,
		},
		{
			name: "file header and package doc both reflowed",
			src: `// pkg.go implements the pkg package, and this header line alone is
// long enough to want a break.

// Package pkg provides utilities for testing, and this doc comment is
// also long enough to want a break.
package pkg
`,
			limit: wideEnough,
			want: `// pkg.go implements the pkg package,
// and this header line alone is long enough to want a break.

// Package pkg provides utilities for testing,
// and this doc comment is also long enough to want a break.
package pkg
`,
		},
		{
			name: "grouped const doc on exported spec",
			src: pkgHeader + `const (
	// MaxRetries caps the number of retry attempts, and it exists to
	// bound worst-case latency under failure.
	MaxRetries = 3
)
`,
			limit: wideEnough,
			want: pkgHeader + `const (
	// MaxRetries caps the number of retry attempts,
	// and it exists to bound worst-case latency under failure.
	MaxRetries = 3
)
`,
		},
		{
			name: "prose paragraph beside a heading",
			src: pkgHeader + `// Foo does a thing.
//
// # Usage
//
// Call Foo once at startup, and it
// sets up the cache.
func Foo() {}
`,
			limit: wideEnough,
			want: pkgHeader + `// Foo does a thing.
//
// # Usage
//
// Call Foo once at startup,
// and it sets up the cache.
func Foo() {}
`,
		},
		{
			name: "prose paragraph beside a list",
			src: pkgHeader + `// Foo supports two modes, and the
// caller picks one.
//
//   - fast, which skips validation
//   - safe, which validates every field
func Foo() {}
`,
			limit: wideEnough,
			want: pkgHeader + `// Foo supports two modes,
// and the caller picks one.
//
//   - fast, which skips validation
//   - safe, which validates every field
func Foo() {}
`,
		},
		{
			// The conjunction opens no clause, so a break before it would be a wrapped break.
			name: "compound predicate joined onto one line",
			src: pkgHeader + `// Foo reads the file, and returns
// an error on failure.
func Foo() {}
`,
			limit: wideEnough,
			want: pkgHeader + `// Foo reads the file, and returns an error on failure.
func Foo() {}
`,
		},
		{
			name: "written break at a colon kept",
			src: pkgHeader + `// Foo writes two files:
// the index and the log, and it
// returns the count.
func Foo() {}
`,
			limit: wideEnough,
			want: pkgHeader + `// Foo writes two files:
// the index and the log,
// and it returns the count.
func Foo() {}
`,
		},
		{
			name: "line reaching the limit split at a colon",
			src: pkgHeader + `// Foo writes two files: the index of every record and the log.
func Foo() {}
`,
			limit: 50,
			want: pkgHeader + `// Foo writes two files:
// the index of every record and the log.
func Foo() {}
`,
		},
		{
			name: "line reaching the limit split at the latest break that fits",
			src: pkgHeader + `// Foo opens the store: it reads every record in order: it then writes a summary.
func Foo() {}
`,
			limit: 60,
			want: pkgHeader + `// Foo opens the store: it reads every record in order:
// it then writes a summary.
func Foo() {}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mustReflow(t, tt.src, tt.limit)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestReflowSource_LeavesUntouched covers every source the tool must return byte-for-byte.
// Those are out-of-scope declarations, generated files, and already-reflowed comments.
// A line reaching the limit is left as written when no accepted or written break can split it.
func TestReflowSource_LeavesUntouched(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		limit int
	}{
		{
			name: "unexported func doc",
			src: pkgHeader + `// loadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio, and returns an error on failure.
func loadPortfolio(dir string) {}
`,
			limit: wideEnough,
		},
		{
			name: "single-line comment",
			src: pkgHeader + `// Foo does a thing.
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			// A comment that occupies one physical source line under the limit has no hard wrap to reflow.
			name: "single physical line group",
			src: `// Package pkg provides utilities for testing this reflow tool, and it exists solely for that purpose.
package pkg
`,
			limit: wideEnough,
		},
		{
			name: "go:generate directive",
			src: pkgHeader + `// Foo does a thing across several files, and it needs a mock for testing purposes.
//go:generate mockgen -source=foo.go
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			name: "lym directive",
			src: pkgHeader + `// Foo does a thing across several files, and
// it needs a seam.
//lym:seam
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			name: "indented code example",
			src: pkgHeader + `// Foo does a thing, and here is an example of calling it in practice.
//
//	Foo()
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			name: "generated file",
			src: `// Code generated by foogen. DO NOT EDIT.

package pkg

// Foo does a thing, and it needs no further explanation beyond this single sentence run on long.
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			name: "struct field doc is out of scope",
			src: pkgHeader + `// Config holds settings.
type Config struct {
	// Timeout bounds how long a request may run, and callers should set it explicitly rather than relying on the zero value.
	Timeout int
}
`,
			limit: wideEnough,
		},
		{
			name: "doc comment heading",
			src: pkgHeader + `// Foo does a thing.
//
// # Deprecated
//
// Use Bar instead, and remove all call sites before the next major release.
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			name: "doc comment list",
			src: pkgHeader + `// Foo supports three modes:
//
//   - fast, which skips validation entirely and trusts the caller
//   - safe, which validates every field before proceeding
//   - strict, which also rejects unknown fields
func Foo() {}
`,
			limit: wideEnough,
		},
		{
			name: "already reflowed",
			src: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio,
// and it returns an error if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
			limit: wideEnough,
		},
		{
			name: "sentence reaching the limit keeps its written break",
			src: pkgHeader + `// Foo opens the store and reads every record in
// order and writes a summary of the records.
func Foo() {}
`,
			limit: 40,
		},
		{
			name: "line reaching the limit with no break point stays whole",
			src: pkgHeader + `// Foo opens the store and reads every record in order.
func Foo() {}
`,
			limit: 40,
		},
		{
			// The only break leaves a piece that still reaches the limit, so splitting would add a lint finding.
			name: "line whose only break leaves a piece over the limit stays whole",
			src: pkgHeader + `// Foo writes two files: the index of every record and the log.
func Foo() {}
`,
			limit: 30,
		},
		{
			// Each line is under the limit as written, but the laid-out second line would be joined onto the first.
			name: "paragraph whose layout would join a line over the limit stays as written",
			src: pkgHeader + `// Foo reads every record from the store and writes a detailed summary of the results, and the
// collection of data from the current transaction and the prior transaction
// is validated against the complete collection before the final result can be returned.
func Foo() {}
`,
			limit: 120,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mustReflow(t, tt.src, tt.limit)
			if got != tt.src {
				t.Errorf("source was changed, want left alone:\n%s", got)
			}
		})
	}
}

// TestCollectEdits_ReportsLongProseLinesInReflowedNumbering covers the lines the run names for a hand rewrite.
// It reports a prose line in a group that needs no edit, numbered after the lines an earlier edit adds.
// It reports no list-item line.
func TestCollectEdits_ReportsLongProseLinesInReflowedNumbering(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	src := pkgHeader + `// Foo does a thing. It sets up the cache,
// and it logs.
func Foo() {}

// Bar opens the store and reads every record in order.
//
//   - a list item that is long enough to reach the limit.
func Bar() {}
`
	_, got, err := collectEdits(token.NewFileSet(), "test.go", src, 40)
	if err != nil {
		t.Fatalf("collectEdits: %v", err)
	}
	// The first group grows from two lines to three, so Bar's line 9 in the source is line 10 in the reflowed file.
	want := []int{10}
	if !slices.Equal(got, want) {
		t.Errorf("collectEdits long lines = %v; want %v", got, want)
	}
}
