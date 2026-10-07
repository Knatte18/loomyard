// estimate_test.go verifies SegmentCost against a fake size source and hand-built cards, and DiskSizes against a temporary worktree tree.
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

// testWeights are distinct round coefficients, so a hand-computed cost pins each one entering once.
var testWeights = batcher.Weights{
	StartupContext:   10,
	ForkMessages:     2,
	TargetMessages:   3,
	TestFileMessages: 4,
	UsesMessages:     5,
	ContextPerLine:   0.5,
	PackageContext:   7,
}

// editCard returns the card numbered number that edits targets and uses uses.
func editCard(number int, targets []string, uses ...string) planparser.Card {
	return planparser.Card{
		Number:       number,
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: targets}},
		Uses:         uses,
	}
}

// TestSegmentCost asserts the hand-computed cost of one-card segments over a fake size source.
// It also asserts that merging cards prices a shared read set once, so identical read sets cost less merged than apart and a large added file costs more.
func TestSegmentCost(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	sizes := fakeSizes{
		lines: map[string]int{
			"internal/a/a.go":   100,
			"internal/a/b.go":   50,
			"internal/b/b.go":   50,
			"internal/s/s.go":   10,
			"internal/big/b.go": 1000,
		},
		testFiles: map[string][]string{
			"internal/a": {"internal/a/a_test.go", "internal/a/b_test.go"},
		},
	}

	tests := []struct {
		name  string
		cards []planparser.Card
		want  float64
	}{
		{
			// fork 2*10, messages 3 + 2 test files*4 + 5, read set a.go 50, b.go 25, package 7, tests 7.
			name:  "one card with a use and tests",
			cards: []planparser.Card{editCard(1, []string{"internal/a/a.go"}, "internal/b/b.go")},
			want:  20 + 16*(10+50+25+7+7),
		},
		{
			name:  "test files count once per distinct directory",
			cards: []planparser.Card{editCard(1, []string{"internal/a/a.go", "internal/a/b.go"})},
			want:  20 + (2*3+2*4)*(10+50+25+7+7),
		},
		{
			name:  "a nonexistent target weighs nothing but counts its messages",
			cards: []planparser.Card{editCard(1, []string{"internal/new/n.go"})},
			want:  20 + 3*(10+7+7),
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
			want: 20 + 3*(10+25+7+7),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := batcher.SegmentCost(plan, tt.cards, sizes, testWeights)
			if err != nil {
				t.Fatalf("SegmentCost: %v", err)
			}
			if got != tt.want {
				t.Errorf("SegmentCost = %v; want %v", got, tt.want)
			}
		})
	}

	apart := func(t *testing.T, cards ...planparser.Card) float64 {
		t.Helper()
		var total float64
		for _, card := range cards {
			cost, err := batcher.SegmentCost(plan, []planparser.Card{card}, sizes, testWeights)
			if err != nil {
				t.Fatalf("SegmentCost: %v", err)
			}
			total += cost
		}
		return total
	}
	merged := func(t *testing.T, cards ...planparser.Card) float64 {
		t.Helper()
		cost, err := batcher.SegmentCost(plan, cards, sizes, testWeights)
		if err != nil {
			t.Fatalf("SegmentCost: %v", err)
		}
		return cost
	}

	t.Run("identical read sets cost less merged than apart", func(t *testing.T) {
		t.Parallel()
		first := editCard(1, []string{"internal/a/a.go"})
		second := editCard(2, []string{"internal/a/a.go"})
		if m, a := merged(t, first, second), apart(t, first, second); m >= a {
			t.Errorf("merged = %v; want less than apart = %v", m, a)
		}
	})

	t.Run("a card adding a large file costs more merged than apart", func(t *testing.T) {
		t.Parallel()
		small := editCard(1, []string{"internal/s/s.go"})
		large := editCard(2, []string{"internal/big/b.go"})
		if m, a := merged(t, small, large), apart(t, small, large); m <= a {
			t.Errorf("merged = %v; want more than apart = %v", m, a)
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
