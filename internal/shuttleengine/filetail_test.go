// filetail_test.go pins the bounded tail read of a session record and the capped line count of a file.

package shuttleengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReadFileTail pins the window of a file the tail read returns.
// A file under the budget is read whole, and a larger one is cut to its complete lines.
// A window holding no line end yields nothing.
func TestReadFileTail(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		content string
		budget  int64
		want    string
		wantNil bool
	}{
		{name: "FileUnderTheBudgetIsReadWhole", content: "aaa\nbbb\nccc\n", budget: 100, want: "aaa\nbbb\nccc\n"},
		{name: "FileOverTheBudgetDropsTheCutFirstLine", content: "aaa\nbbb\nccc\n", budget: 6, want: "ccc\n"},
		{name: "WindowWithoutALineEndIsNil", content: "aaa\nbbb", budget: 2, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "transcript.jsonl")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}
			got, err := ReadFileTail(path, tt.budget)
			if err != nil {
				t.Fatalf("ReadFileTail() error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("ReadFileTail(%q, %d) = %q; want nil", tt.content, tt.budget, got)
				}
				return
			}
			if string(got) != tt.want {
				t.Errorf("ReadFileTail(%q, %d) = %q; want %q", tt.content, tt.budget, got, tt.want)
			}
		})
	}
}

// TestCountLinesUpTo pins the capped line count: a count stops at the limit, and an unterminated last line counts.
func TestCountLinesUpTo(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		content string
		limit   int
		want    int
	}{
		{name: "UnderTheLimit", content: "a\nb\nc\n", limit: 10, want: 3},
		{name: "UnterminatedLastLineCounts", content: "a\nb\nc", limit: 10, want: 3},
		{name: "EmptyFile", content: "", limit: 10, want: 0},
		{name: "StopsAtTheLimit", content: "a\nb\nc\nd\ne\n", limit: 2, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}
			got, err := CountLinesUpTo(path, tt.limit)
			if err != nil || got != tt.want {
				t.Errorf("CountLinesUpTo(%q, %d) = %d, %v; want %d", tt.content, tt.limit, got, err, tt.want)
			}
		})
	}
}
