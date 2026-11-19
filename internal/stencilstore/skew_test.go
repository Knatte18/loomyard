// skew_test.go covers when a read refuses a board copy that a newer lym binary wrote.

package stencilstore_test

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// skewRegistry is a stencilstore.Registry of fixed shipped defaults.
type skewRegistry map[string][]byte

// Names implements stencilstore.Registry.
func (r skewRegistry) Names() []string {
	return slices.Sorted(maps.Keys(r))
}

// Default implements stencilstore.Registry.
func (r skewRegistry) Default(name string) ([]byte, bool) {
	content, ok := r[name]
	return content, ok
}

// writeBoardFile writes content at name's path under baseDir, creating its directories.
func writeBoardFile(t *testing.T, baseDir, name string, content []byte) {
	t.Helper()
	path := stencilstore.Path(baseDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%s) failed: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s) failed: %v", path, err)
	}
}

// skewReadEntry is one public read path that reaches a board copy.
// It holds the copy's stencil name, the stencil that includes it when it is not read directly, and the read itself.
type skewReadEntry struct {
	target  string
	top     string
	topBody string
	read    func(baseDir string) (string, error)
}

// TestReadRefusesBoardCopyFromNewerBinary pins when a read refuses a board copy and when it returns it as it stands.
// A copy a newer binary wrote with a body of its own is refused, edited or not, whatever read path reaches it.
// Every refusal and every pass leaves the board file unchanged.
// It is not parallel: stencilstore.SetSkewStateForTest replaces process-global state.
func TestReadRefusesBoardCopyFromNewerBinary(t *testing.T) {
	tiergate.Require(t, 0)

	const shippedBody = "The running binary's text.\n"
	const newerBody = "The newer binary's text.\n"
	runningTime := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	running := stencilstore.Writer{Revision: "running", Time: runningTime}
	newer := stencilstore.Writer{Revision: "newer", Time: runningTime.Add(time.Hour)}
	older := stencilstore.Writer{Revision: "older", Time: runningTime.Add(-time.Hour)}
	sibling := stencilstore.Writer{Revision: "sibling", Time: runningTime}
	descendantEqualTime := stencilstore.Writer{Revision: "descendant", Time: runningTime}
	descendantEarlierTime := stencilstore.Writer{Revision: "descendant", Time: runningTime.Add(-time.Hour)}

	orderedAs := func(ordering stencilstore.Ordering) func(stencilstore.Writer) stencilstore.Ordering {
		return func(stencilstore.Writer) stencilstore.Ordering { return ordering }
	}

	const topLevel = "gatefix-body-test"
	readTopLevel := skewReadEntry{
		target: topLevel,
		read: func(baseDir string) (string, error) {
			content, err := stencilstore.Read(baseDir, topLevel)
			return string(content), err
		},
	}
	const plainInclude = "gatefix-frag-test"
	readPlainInclude := skewReadEntry{
		target:  plainInclude,
		top:     "gatefix-top-test",
		topBody: "Top.\n{{template \"" + plainInclude + "\"}}\n",
		read: func(baseDir string) (string, error) {
			content, err := stencilstore.ReadForEngine(baseDir, "claude", "gatefix-top-test")
			return string(content), err
		},
	}
	readEngineBlock := skewReadEntry{
		target:  stencilstore.EngineBlockName("claude", "gatefix-block"),
		top:     "gatefix-top-test",
		topBody: "Top.\n{{template \"engine-gatefix-block\"}}\n",
		read: func(baseDir string) (string, error) {
			body, blocks, err := stencilstore.ReadSet(baseDir, "claude", "gatefix-top-test")
			var text strings.Builder
			text.Write(body)
			for _, block := range blocks {
				text.Write(block)
			}
			return text.String(), err
		},
	}

	tests := []struct {
		name       string
		entry      skewReadEntry
		recorded   stencilstore.Writer
		body       string
		edited     bool
		running    stencilstore.Writer
		older      func(stencilstore.Writer) stencilstore.Ordering
		unshipped  bool
		wantRefuse bool
	}{
		{name: "NewerWriterTopLevel", entry: readTopLevel, recorded: newer, wantRefuse: true},
		{name: "NewerWriterPlainInclude", entry: readPlainInclude, recorded: newer, wantRefuse: true},
		{name: "NewerWriterEngineBlock", entry: readEngineBlock, recorded: newer, wantRefuse: true},
		{name: "EditedCopyFromANewerWriter", entry: readTopLevel, recorded: newer, edited: true, wantRefuse: true},
		{name: "OlderWriter", entry: readTopLevel, recorded: older},
		{name: "EqualCommitTime", entry: readTopLevel, recorded: sibling},
		{name: "NoRecordedWriter", entry: readTopLevel},
		{name: "NewerWriterWithTheShippedBody", entry: readTopLevel, recorded: newer, body: shippedBody},
		{name: "RunningBinaryWithNoCommitTime", entry: readTopLevel, recorded: newer, running: stencilstore.Writer{Revision: "running"}},
		{name: "StencilTheBinaryDoesNotShip", entry: readTopLevel, recorded: newer, unshipped: true},
		{name: "DescendantWriterWithEqualCommitTime", entry: readTopLevel, recorded: descendantEqualTime, older: orderedAs(stencilstore.RecordedNotOlder), wantRefuse: true},
		{name: "DescendantWriterWithEarlierCommitTime", entry: readTopLevel, recorded: descendantEarlierTime, older: orderedAs(stencilstore.RecordedNotOlder), wantRefuse: true},
		{name: "AncestorWriterWithLaterCommitTime", entry: readTopLevel, recorded: newer, older: orderedAs(stencilstore.RecordedOlder)},
		{name: "UnknownOrdering", entry: readTopLevel, recorded: newer, older: orderedAs(stencilstore.OrderingUnknown)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.body == "" {
				tt.body = newerBody
			}
			if tt.running == (stencilstore.Writer{}) {
				tt.running = running
			}
			registry := skewRegistry{tt.entry.target: []byte(shippedBody)}
			if tt.unshipped {
				registry = skewRegistry{}
			}
			stencilstore.SetSkewStateForTest(t, registry, tt.running, tt.older)

			baseDir := t.TempDir()
			boardCopy := stencilstore.ApplyWriter(stencilstore.ApplyStamp([]byte(tt.body), stencilstore.BodyHash([]byte(tt.body))), tt.recorded)
			if tt.edited {
				boardCopy = append(boardCopy, "An operator's edit.\n"...)
			}
			writeBoardFile(t, baseDir, tt.entry.target, boardCopy)
			if tt.entry.top != "" {
				writeBoardFile(t, baseDir, tt.entry.top, []byte(tt.entry.topBody))
			}

			got, err := tt.entry.read(baseDir)

			if tt.wantRefuse {
				var skew *stencilstore.NewerBoardCopyError
				if !errors.As(err, &skew) {
					t.Fatalf("read of %q = %q, %v; want a *NewerBoardCopyError", tt.entry.target, got, err)
				}
				want := stencilstore.NewerBoardCopyError{Stencil: tt.entry.target, WrittenBy: tt.recorded.Revision, Running: tt.running.Revision}
				if *skew != want {
					t.Errorf("read of %q error = %+v; want %+v", tt.entry.target, *skew, want)
				}
				for _, part := range []string{tt.entry.target, tt.recorded.Revision, tt.running.Revision, "restart the run's driver on the current lym binary"} {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("read of %q error %q lacks %q", tt.entry.target, err.Error(), part)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("read of %q failed: %v", tt.entry.target, err)
				}
				if !strings.Contains(got, strings.TrimSuffix(tt.body, "\n")) {
					t.Errorf("read of %q = %q; want it to hold the board body %q", tt.entry.target, got, tt.body)
				}
			}

			onDisk, readErr := os.ReadFile(stencilstore.Path(baseDir, tt.entry.target))
			if readErr != nil {
				t.Fatalf("os.ReadFile of the board copy failed: %v", readErr)
			}
			if !bytes.Equal(onDisk, boardCopy) {
				t.Errorf("board copy of %q changed to %q; want it unchanged", tt.entry.target, onDisk)
			}
		})
	}
}
