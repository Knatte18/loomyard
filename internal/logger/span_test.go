// span_test.go covers Span's explicit-parent dotted-path construction,
// sibling independence (no global to leak between spans), End's record
// levels, and the durable sink's visibility of span= records versus
// open/close records. Every sink-touching case calls
// SetDurableSinkDir(t.TempDir()) at its own start, never sharing one call
// across cases, per sink.go's SetDurableSinkDir doc.

package logger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpan_NestingProducesDottedPath(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	root := StartSpan("a")
	child := root.Child("b")
	grandchild := child.Child("c")

	buf.Reset()
	grandchild.Info("innermost line")

	if !strings.Contains(buf.String(), "span=a.b.c") {
		t.Errorf("output = %q; want it to contain span=a.b.c", buf.String())
	}
}

func TestSpan_EndDoesNotCorruptSiblingSpan(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	root := StartSpan("parent")
	firstChild := root.Child("first")
	sibling := root.Child("sibling")

	firstChild.End(nil)

	buf.Reset()
	sibling.Info("sibling line after first child ended")

	if !strings.Contains(buf.String(), "span=parent.sibling") {
		t.Errorf("output = %q; want it to contain span=parent.sibling, unaffected by the ended sibling", buf.String())
	}
	if strings.Contains(buf.String(), "span=parent.first") {
		t.Errorf("output = %q; want it to NOT contain the ended sibling's path span=parent.first", buf.String())
	}
}

func TestSpan_UnendedChildDoesNotCorruptSiblingPath(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	root := StartSpan("parent")
	// firstChild is deliberately never ended.
	_ = root.Child("first")
	second := root.Child("second")

	buf.Reset()
	second.Info("second child line")

	if !strings.Contains(buf.String(), "span=parent.second") {
		t.Errorf("output = %q; want it to contain exactly span=parent.second", buf.String())
	}
	if strings.Contains(buf.String(), "span=parent.first") {
		t.Errorf("output = %q; want it to NOT contain the unended first child's path", buf.String())
	}
}

func TestSpan_EndWithErrorRecordsErrorText(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	sp := StartSpan("failing")
	buf.Reset()

	wantErr := errors.New("boom")
	sp.End(wantErr)

	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("output = %q; want the close record to contain the error text %q", buf.String(), wantErr.Error())
	}
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("output = %q; want the close record for a non-nil error to be at Warn level", buf.String())
	}
}

// TestSpan_OpenCloseRecordsAbsentFromDurableSink verifies open/close records don't reach the durable sink.
func TestSpan_OpenCloseRecordsAbsentFromDurableSink(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	sp := StartSpan("root")
	child := sp.Child("child")
	child.End(nil)
	sp.End(nil)

	Info("unrelated info line opens the sink")

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	content := string(data)
	if strings.Contains(content, "span started") || strings.Contains(content, "span ended") {
		t.Errorf("durable sink content = %q; want no span open/close records (they emit at Debug)", content)
	}
}

// TestSpan_EndWithErrorReachesDurableSink verifies End(err) reaches the durable sink.
func TestSpan_EndWithErrorReachesDurableSink(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	sp := StartSpan("failing-root")
	sp.End(errors.New("durable boom"))

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	content := string(data)
	if !strings.Contains(content, "span ended") || !strings.Contains(content, "durable boom") {
		t.Errorf("durable sink content = %q; want it to contain the End(err) close record with the error text", content)
	}
}

// TestSpan_InfoCarriesSpanPathIntoDurableSink verifies Info carries span= into the durable sink.
func TestSpan_InfoCarriesSpanPathIntoDurableSink(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	sp := StartSpan("root").Child("work")
	sp.Info("doing the work")

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	content := string(data)
	if !strings.Contains(content, "doing the work") || !strings.Contains(content, "span=root.work") {
		t.Errorf("durable sink content = %q; want the Info line to carry span=root.work", content)
	}
	if strings.Contains(content, "span started") || strings.Contains(content, "span ended") {
		t.Errorf("durable sink content = %q; want no span open/close records present", content)
	}
}
