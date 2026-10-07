// estimate_test.go verifies PeakContext against a fake size source and hand-built cards, and DiskSizes against a temporary worktree tree.
// Tier-1 (pure logic, no git, no spawn).

package batcher_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// fakeSizes is an in-memory SizeSource.
type fakeSizes struct {
	lines     map[string]int
	testFiles map[string][]string
}

func (f fakeSizes) Lines(path string) (int, bool, error) {
	n, ok := f.lines[path]
	return n, ok, nil
}

func (f fakeSizes) TestFiles(dir string) ([]string, error) {
	return f.testFiles[dir], nil
}

// testWeights are distinct round coefficients, so a hand-computed peak pins each one entering once.
var testWeights = batcher.Weights{
	StartupContext:   10,
	ForkMessages:     2,
	MessageContext:   1,
	TargetMessages:   3,
	TestFileMessages: 4,
	UsesMessages:     5,
	ContextPerLine:   0.5,
	PackageContext:   7,
	WritePerCardLine: 2,
}

// editCard returns the card numbered number that edits targets and uses uses.
func editCard(number int, targets []string, uses ...string) planparser.Card {
	return planparser.Card{
		Number:       number,
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: targets}},
		Uses:         uses,
	}
}

// TestPeakContext asserts the hand-computed peak of one-card segments over a fake size source,
// and that merging cards counts a shared read-set entry once and never lowers the peak.
func TestPeakContext(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	sizes := fakeSizes{
		lines: map[string]int{
			"internal/a/a.go":   100,
			"internal/a/b.go":   50,
			"internal/b/b.go":   50,
			"internal/s/s.go":   10,
			"internal/big/b.go": 1000,
			"_lyx/plan/01-x.md": 40,
		},
		testFiles: map[string][]string{
			"internal/a": {"internal/a/a_test.go", "internal/a/b_test.go"},
		},
	}
	// Every peak below starts from the startup context 10 plus 2 fork messages at 1 each.
	const startup = 12

	withText := editCard(1, []string{"internal/s/s.go"})
	withText.SourcePath = "_lyx/plan/01-x.md"

	tests := []struct {
		name  string
		cards []planparser.Card
		want  float64
	}{
		{
			// messages 3 + 2 test files*4 + 5; read set a.go 50, b.go 25, package 7, tests 7.
			name:  "one card with a use and tests",
			cards: []planparser.Card{editCard(1, []string{"internal/a/a.go"}, "internal/b/b.go")},
			want:  startup + 16 + 50 + 25 + 7 + 7,
		},
		{
			name:  "test files count once per distinct directory",
			cards: []planparser.Card{editCard(1, []string{"internal/a/a.go", "internal/a/b.go"})},
			want:  startup + (2*3 + 2*4) + 50 + 25 + 7 + 7,
		},
		{
			name:  "a nonexistent target weighs nothing but counts its messages",
			cards: []planparser.Card{editCard(1, []string{"internal/new/n.go"})},
			want:  startup + 3 + 7 + 7,
		},
		{
			name: "a rename counts once and is sized by its old side",
			cards: []planparser.Card{{
				Number: 1,
				TargetGroups: []planparser.TargetGroup{{
					Type:  planparser.CardTypeRename,
					Refs:  []string{"internal/b/b.go#", "internal/new/n.go#"},
					Pairs: []planparser.MovePair{{Old: "internal/b/b.go#", New: "internal/new/n.go#"}},
				}},
			}},
			want: startup + 3 + 25 + 7 + 7,
		},
		{
			// The card's 40 lines are read at 0.5 each and written back at 2 each.
			name:  "a card's own text is read and drives its write allowance",
			cards: []planparser.Card{withText},
			want:  startup + 3 + 5 + 7 + 7 + 20 + 80,
		},
		{
			name: "a read-set entry two cards share is counted once",
			cards: []planparser.Card{
				editCard(1, []string{"internal/s/s.go"}),
				editCard(2, []string{"internal/s/s.go"}),
			},
			want: startup + 3 + 3 + 5 + 7 + 7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := batcher.PeakContext(plan, tt.cards, sizes, testWeights)
			if err != nil {
				t.Fatalf("PeakContext: %v", err)
			}
			if got != tt.want {
				t.Errorf("PeakContext = %v; want %v", got, tt.want)
			}
		})
	}

	t.Run("adding a card never lowers the peak", func(t *testing.T) {
		t.Parallel()
		cards := []planparser.Card{
			editCard(1, []string{"internal/big/b.go"}),
			editCard(2, []string{"internal/s/s.go"}),
			withText,
		}
		previous := 0.0
		for n := 1; n <= len(cards); n++ {
			peak, err := batcher.PeakContext(plan, cards[:n], sizes, testWeights)
			if err != nil {
				t.Fatalf("PeakContext: %v", err)
			}
			if peak < previous {
				t.Errorf("peak of %d cards = %v; want at least %v", n, peak, previous)
			}
			previous = peak
		}
	})
}

// TestDiskSizes asserts DiskSizes counts the lines of present files and reports absent ones as missing.
// It also asserts DiskSizes lists only the *_test.go files directly in a directory, with an absent directory listing none.
func TestDiskSizes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files := map[string]string{
		"internal/a/a.go":       "x\ny\nz\n",
		"internal/a/open.go":    "x\ny",
		"internal/a/empty.go":   "",
		"internal/a/a_test.go":  "package a\n",
		"internal/a/b_test.go":  "package a\n",
		"internal/a/sub/s_test": "not a test file",
	}
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	sizes := batcher.DiskSizes(root)

	lineTests := []struct {
		path       string
		wantLines  int
		wantExists bool
	}{
		{"internal/a/a.go", 3, true},
		{"internal/a/open.go", 2, true},
		{"internal/a/empty.go", 0, true},
		{"internal/a/missing.go", 0, false},
	}
	for _, tt := range lineTests {
		n, exists, err := sizes.Lines(tt.path)
		if err != nil {
			t.Fatalf("Lines(%q): %v", tt.path, err)
		}
		if n != tt.wantLines || exists != tt.wantExists {
			t.Errorf("Lines(%q) = %d, %v; want %d, %v", tt.path, n, exists, tt.wantLines, tt.wantExists)
		}
	}

	dirTests := []struct {
		dir  string
		want []string
	}{
		{"internal/a", []string{"internal/a/a_test.go", "internal/a/b_test.go"}},
		{"internal/absent", nil},
	}
	for _, tt := range dirTests {
		got, err := sizes.TestFiles(tt.dir)
		if err != nil {
			t.Fatalf("TestFiles(%q): %v", tt.dir, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("TestFiles(%q) = %v; want %v", tt.dir, got, tt.want)
		}
	}
}
