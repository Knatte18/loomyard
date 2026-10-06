// span_test.go covers Span's explicit-parent dotted-path construction, sibling independence (no
// global to leak between spans), End's record levels, and the durable sink's visibility of span=
// records versus open/close records.
// Every sink-touching case calls SetDurableSinkDir(t.TempDir()) at its own start, never sharing one
// call across cases, per sink.go's SetDurableSinkDir doc.
// No test in this file calls t.Parallel: each mutates process-global logger state (verbosity, the
// output writer, the durable sink).

package logger

import (
	"errors"
	"strings"
	"testing"
)

// TestSpan_PathsAreExplicitAndIndependentOfSiblings pins that a span's path comes from its explicit
// parent chain: nesting yields a dotted path, and neither an ended nor a never-ended sibling shows
// up in another span's path.
//
//testtiming:keep pins the dotted span path and sibling independence on stderr, which TestSpan_DurableSinkCarriesInfoSpanPathButNoOpenCloseRecords asserts only for the durable sink
func TestSpan_PathsAreExplicitAndIndependentOfSiblings(t *testing.T) {
	tests := []struct {
		name        string
		logFrom     func() *Span
		wantPath    string
		wantAbsence string
	}{
		{
			name: "nesting produces a dotted path",
			logFrom: func() *Span {
				return StartSpan("a").Child("b").Child("c")
			},
			wantPath: "span=a.b.c",
		},
		{
			name: "ending a sibling leaves the other span's path intact",
			logFrom: func() *Span {
				root := StartSpan("parent")
				first := root.Child("first")
				sibling := root.Child("sibling")
				first.End(nil)
				return sibling
			},
			wantPath:    "span=parent.sibling",
			wantAbsence: "span=parent.first",
		},
		{
			name: "a never-ended child does not corrupt a later sibling's path",
			logFrom: func() *Span {
				root := StartSpan("parent")
				_ = root.Child("first")
				return root.Child("second")
			},
			wantPath:    "span=parent.second",
			wantAbsence: "span=parent.first",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := withCapturedOutput(t)
			SetVerbosity(2)
			t.Cleanup(func() { SetVerbosity(0) })
			span := tt.logFrom()

			buf.Reset()
			span.Info("line from the span under test")

			if !strings.Contains(buf.String(), tt.wantPath) {
				t.Errorf("output = %q; want it to contain %s", buf.String(), tt.wantPath)
			}
			if tt.wantAbsence != "" && strings.Contains(buf.String(), tt.wantAbsence) {
				t.Errorf("output = %q; want it to NOT contain %s", buf.String(), tt.wantAbsence)
			}
		})
	}
}

// TestSpan_EndWithErrorReachesBothSinksAtWarn pins that End(err) records the error text at Warn on
// stderr and in the durable sink.
func TestSpan_EndWithErrorReachesBothSinksAtWarn(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	sp := StartSpan("failing")
	buf.Reset()
	sp.End(errors.New("boom"))

	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("output = %q; want the close record to contain the error text %q", buf.String(), "boom")
	}
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("output = %q; want the close record for a non-nil error to be at Warn level", buf.String())
	}
	content := readSoleSinkFile(t, dir)
	if !strings.Contains(content, "span ended") || !strings.Contains(content, "boom") {
		t.Errorf("durable sink content = %q; want it to contain the End(err) close record with the error text", content)
	}
}

// TestSpan_DurableSinkCarriesInfoSpanPathButNoOpenCloseRecords pins that an Info record inside a
// span carries span=<path> into the durable sink while the span's open and close records, which emit
// at Debug, stay out of it.
//
//testtiming:keep pins that open and close records stay out of the durable sink, which its covering tests do not assert
func TestSpan_DurableSinkCarriesInfoSpanPathButNoOpenCloseRecords(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	root := StartSpan("root")
	work := root.Child("work")
	work.Info("doing the work")
	work.End(nil)
	root.End(nil)

	content := readSoleSinkFile(t, dir)
	if !strings.Contains(content, "doing the work") || !strings.Contains(content, "span=root.work") {
		t.Errorf("durable sink content = %q; want the Info line to carry span=root.work", content)
	}
	if strings.Contains(content, "span started") || strings.Contains(content, "span ended") {
		t.Errorf("durable sink content = %q; want no span open/close records (they emit at Debug)", content)
	}
}
