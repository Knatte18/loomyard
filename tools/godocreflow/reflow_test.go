package main

import (
	"go/token"
	"reflect"
	"strings"
	"testing"
)

// wideEnough is a max width at which the width fallback never fires.
const wideEnough = 1000

// pkgHeader is the file header and package clause shared by most sources below.
const pkgHeader = `// pkg.go implements the pkg package.

package pkg

`

func mustReflow(t *testing.T, src string) string {
	t.Helper()
	return mustReflowWidth(t, src, wideEnough)
}

func mustReflowWidth(t *testing.T, src string, maxWidth int) string {
	t.Helper()
	fset := token.NewFileSet()
	out, err := reflowSource(fset, "test.go", src, maxWidth)
	if err != nil {
		t.Fatalf("reflowSource: %v", err)
	}
	return out
}

// TestReflowSource_ReflowsDocComments covers every doc-comment kind that is reflowed: an
// exported func, a package doc, a file header with a package doc, a grouped const spec, and a
// hard-wrapped comment that is rejoined before it is split again.
func TestReflowSource_ReflowsDocComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "exported func doc",
			src: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio, and it returns an error if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
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
			want: pkgHeader + `const (
	// MaxRetries caps the number of retry attempts,
	// and it exists to bound worst-case latency under failure.
	MaxRetries = 3
)
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mustReflow(t, tt.src)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestReflowSource_LeavesUntouched covers every source the tool must return byte-for-byte:
// out-of-scope declarations and comment shapes, generated files, an already-reflowed comment,
// and the width fallback when it is disabled or every line already fits.
func TestReflowSource_LeavesUntouched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		maxWidth int
	}{
		{
			name: "unexported func doc",
			src: pkgHeader + `// loadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio, and returns an error on failure.
func loadPortfolio(dir string) {}
`,
			maxWidth: wideEnough,
		},
		{
			name: "single-line comment",
			src: pkgHeader + `// Foo does a thing.
func Foo() {}
`,
			maxWidth: wideEnough,
		},
		{
			// A comment that already occupies exactly one physical source line is left alone regardless of
			// length -- there is no existing hard-wrap to reflow, matching pydocreflow.py's own
			// single-line-docstring/single-line-comment-block skip.
			name: "single physical line group",
			src: `// Package pkg provides utilities for testing this reflow tool, and it exists solely for that purpose.
package pkg
`,
			maxWidth: wideEnough,
		},
		{
			name: "go:generate directive",
			src: pkgHeader + `// Foo does a thing across several files, and it needs a mock for testing purposes.
//go:generate mockgen -source=foo.go
func Foo() {}
`,
			maxWidth: wideEnough,
		},
		{
			name: "indented code example",
			src: pkgHeader + `// Foo does a thing, and here is an example of calling it in practice.
//
//	Foo()
func Foo() {}
`,
			maxWidth: wideEnough,
		},
		{
			name: "generated file",
			src: `// Code generated by foogen. DO NOT EDIT.

package pkg

// Foo does a thing, and it needs no further explanation beyond this single sentence run on long.
func Foo() {}
`,
			maxWidth: wideEnough,
		},
		{
			name: "struct field doc is out of scope",
			src: pkgHeader + `// Config holds settings.
type Config struct {
	// Timeout bounds how long a request may run, and callers should set it explicitly rather than relying on the zero value.
	Timeout int
}
`,
			maxWidth: wideEnough,
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
			maxWidth: wideEnough,
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
			maxWidth: wideEnough,
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
			maxWidth: wideEnough,
		},
		{
			name: "width fallback disabled at zero",
			src: pkgHeader + `// Foo validates the request thoroughly and writes a well-formed structured response back to the caller immediately.
func Foo() {}
`,
			maxWidth: 0,
		},
		{
			// Max width well above every existing line's length: the fallback must never fire.
			name: "width fallback leaves short lines alone",
			src: pkgHeader + `// LoadPortfolio reads every position file in dir and validates each one against the schema.
// It merges the valid files into a single Portfolio,
// and it returns an error if any file fails validation or two files declare the same position ID.
func LoadPortfolio(dir string) (*Portfolio, error) {
	return nil, nil
}
`,
			maxWidth: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mustReflowWidth(t, tt.src, tt.maxWidth)
			if got != tt.src {
				t.Errorf("source was changed, want left alone:\n%s", got)
			}
		})
	}
}

func TestWidthFallbackAppliesToOverwideAtomicLine(t *testing.T) {
	t.Parallel()

	// One long compound-predicate sentence with no semicolon/conjunction boundary to split at: the
	// ordinary semantic splitter produces a single atomic line, which the width fallback must then wrap.
	src := pkgHeader + `// Foo validates the request thoroughly and writes a well-formed structured response back to the caller immediately.
func Foo() {}
`
	got := mustReflowWidth(t, src, 60)
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") && len(line) > 60 {
			t.Errorf("line exceeds max-width=60: %q", line)
		}
	}
	if !strings.Contains(got, "// Foo validates the request thoroughly") {
		t.Errorf("expected wrapped content preserved, got:\n%s", got)
	}
	// No word may be lost or reordered: whitespace-collapsed content must match the original.
	collapse := func(s string) string {
		return strings.Join(strings.Fields(strings.ReplaceAll(s, "//", " ")), " ")
	}
	if collapse(got) != collapse(src) {
		t.Errorf("wrap fallback changed comment content:\ngot:  %s\nwant: %s", collapse(got), collapse(src))
	}
}

//testtiming:keep pins the exact slices wrapLongLine returns for each case, where the width-fallback test only bounds line length
func TestWrapLongLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		content   string
		prefixLen int
		maxWidth  int
		want      []string
	}{
		{
			name:      "fits, no wrap",
			content:   "short line",
			prefixLen: 3,
			maxWidth:  100,
			want:      []string{"short line"},
		},
		{
			name:      "disabled",
			content:   "a very long line that would otherwise need wrapping for sure",
			prefixLen: 3,
			maxWidth:  0,
			want:      []string{"a very long line that would otherwise need wrapping for sure"},
		},
		{
			name:      "wraps at word boundary",
			content:   "one two three four five six seven eight",
			prefixLen: 0,
			maxWidth:  12,
			want:      []string{"one two", "three four", "five six", "seven eight"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := wrapLongLine(c.content, c.prefixLen, c.maxWidth)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("wrapLongLine(%q, %d, %d) = %#v, want %#v", c.content, c.prefixLen, c.maxWidth, got, c.want)
			}
		})
	}
}
