// estimate.go — the card-size estimator behind the cost-model batchifier.
//
// Declares SizeSource, the file-size and test-file reads the estimator needs, and DiskSizes, its
// on-disk implementation; Weights, the coefficients of the cost model, and ProfileWeights, which
// reads them from a batcher.yaml profile; and SegmentCost, the estimated token cost of running a
// contiguous run of cards in one fork.

package batcher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// SizeSource reads the worktree facts the estimator weighs.
// Every path is worktree-relative and slash-separated.
type SizeSource interface {
	// Lines reports the line count of the file at path, and false when the file does not exist.
	Lines(path string) (n int, exists bool, err error)

	// TestFiles lists the *_test.go files directly in dir; an absent dir has none.
	TestFiles(dir string) ([]string, error)
}

// DiskSizes returns a SizeSource over the worktree rooted at the told absolute root.
func DiskSizes(root string) SizeSource {
	return diskSizes{root: root}
}

// diskSizes implements SizeSource over a worktree directory.
type diskSizes struct {
	root string
}

func (d diskSizes) Lines(relPath string) (int, bool, error) {
	data, err := os.ReadFile(d.absolute(relPath))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	lines := strings.Count(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	return lines, true, nil
}

func (d diskSizes) TestFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(d.absolute(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, path.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

func (d diskSizes) absolute(relPath string) string {
	return filepath.Join(d.root, filepath.FromSlash(relPath))
}

// Weights are the cost model's coefficients, one per key of a batcher.yaml profile's weights: map.
type Weights struct {
	// StartupContext is the context a fork starts with, paid by every message it sends.
	StartupContext float64

	// ForkMessages is the messages spent starting a fork, before any card.
	ForkMessages float64

	// TargetMessages is the messages per target a card edits.
	TargetMessages float64

	// TestFileMessages is the messages per test file in a card's target directories.
	TestFileMessages float64

	// UsesMessages is the messages per entry in a card's Uses list.
	UsesMessages float64

	// ContextPerLine is the context a read file adds per line.
	ContextPerLine float64

	// PackageContext is the context a target directory adds, once for its package and once for its tests.
	PackageContext float64
}

// coefficients maps each weights: key to the Weights field it fills; it is the one declaration of
// the key set.
var coefficients = []struct {
	key   string
	field func(*Weights) *float64
}{
	{"startup_context", func(w *Weights) *float64 { return &w.StartupContext }},
	{"fork_messages", func(w *Weights) *float64 { return &w.ForkMessages }},
	{"target_messages", func(w *Weights) *float64 { return &w.TargetMessages }},
	{"test_file_messages", func(w *Weights) *float64 { return &w.TestFileMessages }},
	{"uses_messages", func(w *Weights) *float64 { return &w.UsesMessages }},
	{"context_per_line", func(w *Weights) *float64 { return &w.ContextPerLine }},
	{"package_context", func(w *Weights) *float64 { return &w.PackageContext }},
}

// ProfileWeights loads batcher.yaml under baseDir and returns the named profile's weights.
// It errors naming batcher.yaml when the profile is absent, a coefficient is missing or negative,
// or a weights: key is not a coefficient.
func ProfileWeights(baseDir, profileName string) (Weights, error) {
	cfg, err := loadConfig(baseDir)
	if err != nil {
		return Weights{}, err
	}
	prof, ok := cfg.Profiles[profileName]
	if !ok {
		return Weights{}, fmt.Errorf("batcher.yaml has no profile %q", profileName)
	}

	known := make(map[string]bool, len(coefficients))
	for _, c := range coefficients {
		known[c.key] = true
	}
	var unknown []string
	for key := range prof.Weights {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Weights{}, fmt.Errorf("batcher.yaml profile %q has unknown weights keys: %s", profileName, strings.Join(unknown, ", "))
	}

	var w Weights
	for _, c := range coefficients {
		value, ok := prof.Weights[c.key]
		if !ok {
			return Weights{}, fmt.Errorf("batcher.yaml profile %q is missing weights key %q", profileName, c.key)
		}
		if value < 0 {
			return Weights{}, fmt.Errorf("batcher.yaml profile %q weights key %q is negative: %v", profileName, c.key, value)
		}
		*c.field(&w) = value
	}
	return w, nil
}

// cardLoad is what one card costs to run: the messages it sends and the weight of each entry in its
// read set.
type cardLoad struct {
	messages float64
	readSet  map[string]float64
}

// SegmentCost estimates the cost of running cards in order in one fork:
// the fork's own startup messages, plus each card's messages priced at the startup context and the
// weight of every distinct read-set entry the fork has gathered up to and including that card.
// A card's own estimate is the SegmentCost of the one-card segment.
// A SizeSource error is returned wrapped.
func SegmentCost(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource, w Weights) (float64, error) {
	cost := w.ForkMessages * w.StartupContext
	gathered := map[string]float64{}
	gatheredWeight := 0.0
	for _, card := range cards {
		load, err := loadCard(plan, card, sizes, w)
		if err != nil {
			return 0, fmt.Errorf("estimate card %d: %w", card.Number, err)
		}
		for key, weight := range load.readSet {
			if _, seen := gathered[key]; !seen {
				gathered[key] = weight
				gatheredWeight += weight
			}
		}
		cost += load.messages * (w.StartupContext + gatheredWeight)
	}
	return cost, nil
}

// loadCard derives a card's messages and read set.
// A file entry weighs its lines, and a package entry and a package's tests entry each weigh
// PackageContext; the key prefixes keep the three kinds from colliding.
func loadCard(plan *planparser.Plan, card planparser.Card, sizes SizeSource, w Weights) (cardLoad, error) {
	readSet := map[string]float64{}
	addFile := func(ref string) error {
		file, ok := planparser.RefFile(plan, ref)
		if !ok {
			return nil
		}
		lines, exists, err := sizes.Lines(file)
		if err != nil {
			return err
		}
		if exists {
			readSet["file:"+file] = float64(lines) * w.ContextPerLine
		}
		return nil
	}

	targets := 0
	for _, group := range card.TargetGroups {
		refs := group.Refs
		if group.Type == planparser.CardTypeRename {
			refs = make([]string, 0, len(group.Pairs))
			for _, pair := range group.Pairs {
				refs = append(refs, pair.Old)
			}
		}
		targets += len(refs)
		for _, ref := range refs {
			if err := addFile(ref); err != nil {
				return cardLoad{}, err
			}
		}
	}
	for _, ref := range card.Uses {
		if err := addFile(ref); err != nil {
			return cardLoad{}, err
		}
	}

	testFiles := map[string]bool{}
	for _, targetDir := range planparser.CardTargetDirs(plan, card) {
		readSet["package:"+targetDir.Dir] = w.PackageContext
		readSet["tests:"+targetDir.Dir] = w.PackageContext
		files, err := sizes.TestFiles(targetDir.Dir)
		if err != nil {
			return cardLoad{}, err
		}
		for _, file := range files {
			testFiles[file] = true
		}
	}

	messages := w.TargetMessages*float64(targets) +
		w.TestFileMessages*float64(len(testFiles)) +
		w.UsesMessages*float64(len(card.Uses))
	return cardLoad{messages: messages, readSet: readSet}, nil
}
