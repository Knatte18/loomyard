// estimate.go — the card-size estimator behind the cost-model batchifier.
//
// Declares SizeSource, the file-size and test-file reads the estimator needs, and DiskSizes, its on-disk implementation;
// Weights, the coefficients of the cost model, and ProfileWeights, which reads them from a batcher.yaml profile;
// and PeakContext, the estimated context a fork holds after running a contiguous run of cards.

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

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// SizeSource reads the worktree facts the estimator weighs.
// Every path is worktree-relative and slash-separated.
type SizeSource interface {
	// Lines reports the line count of the file at path, and false when the file does not exist.
	Lines(path string) (n int, exists bool, err error)

	// TestFiles lists the *_test.go files directly in dir;
	// an absent dir has none.
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

// StartBase is the part of the orchestrating session's start context computed from what it loads.
// The zero value adds nothing.
type StartBase struct {
	// Lines is the line count of the text the session loads at start.
	Lines int

	// Fixed is the context, in tokens, of the system prompt and tools.
	Fixed float64
}

// context returns the base's context under w: Lines at ContextPerLine plus Fixed.
func (b StartBase) context(w Weights) float64 {
	return float64(b.Lines)*w.ContextPerLine + b.Fixed
}

// Weights are the cost model's coefficients, one per key of a batcher.yaml profile's weights: map.
type Weights struct {
	// Orientation is the context the orchestrating session gains between loading its start base and spawning its first fork: the codebase and plan it reads to orient itself.
	Orientation float64 `json:"orientation"`

	// BatchGrowth is the context the orchestrating session gains per finished batch, so a fork at position k starts the start base + Orientation + (k−1) × BatchGrowth deep.
	BatchGrowth float64 `json:"batch_growth"`

	// RetiredMasterBase is read only: it lets a breakdown recorded before the start base was computed decode.
	// It is outside coefficients and nothing writes it.
	RetiredMasterBase float64 `json:"master_base,omitempty"`

	// RetiredStartupContext is read only: it lets a breakdown recorded before the start model grew with batch position decode.
	// It is outside coefficients and nothing writes it.
	RetiredStartupContext float64 `json:"startup_context,omitempty"`

	// ForkMessages is the messages spent starting a fork, before any card.
	ForkMessages float64 `json:"fork_messages"`

	// MessageContext is the context one message adds: its tool call and the tool's output.
	MessageContext float64 `json:"message_context"`

	// TargetMessages is the messages per target a card edits.
	TargetMessages float64 `json:"target_messages"`

	// TestFileMessages is the messages per test file in a card's target directories.
	TestFileMessages float64 `json:"test_file_messages"`

	// UsesMessages is the messages per entry in a card's Uses list.
	UsesMessages float64 `json:"uses_messages"`

	// ContextPerLine is the context a read file, or the card's own text, adds per line.
	ContextPerLine float64 `json:"context_per_line"`

	// PackageContext is the context a target directory adds, once for its package and once for its tests.
	PackageContext float64 `json:"package_context"`

	// WritePerCardLine is the context a card's output adds per line of its card text, which carries the code the fork writes.
	WritePerCardLine float64 `json:"write_per_card_line"`
}

// coefficients maps each weights: key to the Weights field it fills;
// it is the one declaration of the key set.
var coefficients = []struct {
	key   string
	field func(*Weights) *float64
}{
	{"orientation", func(w *Weights) *float64 { return &w.Orientation }},
	{"batch_growth", func(w *Weights) *float64 { return &w.BatchGrowth }},
	{"fork_messages", func(w *Weights) *float64 { return &w.ForkMessages }},
	{"message_context", func(w *Weights) *float64 { return &w.MessageContext }},
	{"target_messages", func(w *Weights) *float64 { return &w.TargetMessages }},
	{"test_file_messages", func(w *Weights) *float64 { return &w.TestFileMessages }},
	{"uses_messages", func(w *Weights) *float64 { return &w.UsesMessages }},
	{"context_per_line", func(w *Weights) *float64 { return &w.ContextPerLine }},
	{"package_context", func(w *Weights) *float64 { return &w.PackageContext }},
	{"write_per_card_line", func(w *Weights) *float64 { return &w.WritePerCardLine }},
}

// ProfileWeights loads batcher.yaml under baseDir and returns the named profile's weights.
// It errors naming batcher.yaml when the profile is absent, a coefficient is missing or negative, or a weights: key is not a coefficient.
func ProfileWeights(baseDir, profileName string) (Weights, error) {
	cfg, err := loadConfig(baseDir)
	if err != nil {
		return Weights{}, err
	}
	prof, ok := cfg.Profiles[profileName]
	if !ok {
		return Weights{}, fmt.Errorf("batcher.yaml has no profile %q", profileName)
	}
	return profileWeights(profileName, prof)
}

// profileWeights reads prof's weights: map into Weights, erroring naming batcher.yaml when a coefficient is missing or negative, a key is not a coefficient or a key is retired.
// Every error is marked configengine.ErrInvalid, and a retired key's also wraps ErrRetiredKey.
func profileWeights(profileName string, prof profile) (Weights, error) {
	for _, retired := range []string{"master_base", "startup_context"} {
		if _, ok := prof.Weights[retired]; ok {
			return Weights{}, retiredKeyError(profileName, "weights key "+retired)
		}
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
		return Weights{}, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q has unknown weights keys: %s", profileName, strings.Join(unknown, ", ")))
	}

	var w Weights
	for _, c := range coefficients {
		value, ok := prof.Weights[c.key]
		if !ok {
			return Weights{}, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q is missing weights key %q", profileName, c.key))
		}
		if value < 0 {
			return Weights{}, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q weights key %q is negative: %v", profileName, c.key, value))
		}
		*c.field(&w) = value
	}
	return w, nil
}

// cardLoad is what one card costs to run: the messages it sends and the weight of each entry in its read set.
type cardLoad struct {
	messages float64
	// written is the context the card's own output adds beyond its messages: the code and prose it writes.
	written float64
	readSet map[string]float64
}

// PeakContext estimates the context a fork holds after running cards in one session, which is its largest:
// the start context of the fork's 1-based batch position, the base's context + Orientation + (position−1) × BatchGrowth, plus the weight of every distinct read-set entry the cards gather, plus the fork's own startup messages and every card's messages at MessageContext each.
// Context only grows within a fork, so a segment's peak is the context after its last card, and adding a card never lowers it.
// A card's own estimate is the PeakContext of the one-card segment.
// A SizeSource error is returned wrapped.
func PeakContext(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource, w Weights, base StartBase, position int) (float64, error) {
	loads := make([]cardLoad, len(cards))
	for i, card := range cards {
		load, err := loadCard(plan, card, sizes, w)
		if err != nil {
			return 0, fmt.Errorf("estimate card %d: %w", card.Number, err)
		}
		loads[i] = load
	}
	var peak peakAccumulator
	peak.start(w, base, position)
	for _, load := range loads {
		peak.add(load, w)
	}
	return peak.value, nil
}

// Breakdown records how a batch's PeakContext was formed, so a recorded run can be fitted against the peak its fork measured.
// The peak is Startup plus ReadUnion plus every card's Written.
type Breakdown struct {
	// Weights are the coefficients the estimate used.
	Weights Weights `json:"weights"`

	// Position is the 1-based batch position the estimate priced the fork at.
	Position int `json:"position"`

	// Startup is the fork's start context at Position plus its startup messages.
	Startup float64 `json:"startup"`

	// ReadUnion is the weight of the distinct read-set entries of all the batch's cards, card texts included.
	ReadUnion float64 `json:"read_union"`

	// Cards are the per-card components, in card order.
	Cards []CardBreakdown `json:"cards"`
}

// CardBreakdown is one card's components before the batch's read sets are merged.
type CardBreakdown struct {
	Card int `json:"card"`

	// CardText is the weight of the card's own text.
	CardText float64 `json:"card_text"`

	// Reads are the card's other read-set entries, file:, package: and tests: keys with their weights.
	Reads map[string]float64 `json:"reads"`

	// Messages is the card's estimated message count.
	Messages float64 `json:"messages"`

	// Written is the card's write allowance: its messages at MessageContext plus its card text at WritePerCardLine.
	Written float64 `json:"written"`
}

// breakdownOf builds the Breakdown, at the told 1-based position and start base, of the segment whose cards and loads are told, which must be the same length.
func breakdownOf(cards []planparser.Card, loads []cardLoad, w Weights, base StartBase, position int) Breakdown {
	b := Breakdown{Weights: w, Position: position, Startup: startContext(w, base, position) + w.ForkMessages*w.MessageContext}
	seen := map[string]bool{}
	for i, load := range loads {
		cb := CardBreakdown{Card: cards[i].Number, Reads: map[string]float64{}, Messages: load.messages, Written: load.messages*w.MessageContext + load.written}
		for key, weight := range load.readSet {
			if strings.HasPrefix(key, cardKeyPrefix) {
				cb.CardText = weight
			} else {
				cb.Reads[key] = weight
			}
			if !seen[key] {
				seen[key] = true
				b.ReadUnion += weight
			}
		}
		b.Cards = append(b.Cards, cb)
	}
	return b
}

// cardKeyPrefix opens the read-set key of a card's own text.
const cardKeyPrefix = "card:"

// peakAccumulator builds a segment's PeakContext one card at a time, in either direction, since the peak of a set of cards does not depend on their order.
type peakAccumulator struct {
	gathered map[string]bool
	value    float64
}

// startContext is the context a fork at the 1-based batch position starts with.
func startContext(w Weights, base StartBase, position int) float64 {
	return base.context(w) + w.Orientation + float64(position-1)*w.BatchGrowth
}

// start resets the accumulator to an empty fork at the 1-based batch position: its start context and startup messages.
func (p *peakAccumulator) start(w Weights, base StartBase, position int) {
	p.gathered = map[string]bool{}
	p.value = startContext(w, base, position) + w.ForkMessages*w.MessageContext
}

// add takes one card into the segment: its messages, and the read-set entries no earlier card gathered.
func (p *peakAccumulator) add(load cardLoad, w Weights) {
	p.value += load.messages*w.MessageContext + load.written
	for key, weight := range load.readSet {
		if !p.gathered[key] {
			p.gathered[key] = true
			p.value += weight
		}
	}
}

// loadCard derives a card's messages and read set.
// A file entry weighs its lines, and a package entry and a package's tests entry each weigh PackageContext;
// the key prefixes keep the three kinds from colliding.
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

	// The card's own text is in the fork's context, and the code it carries is roughly what the fork writes back out.
	cardLines := 0
	if card.SourcePath != "" {
		lines, exists, err := sizes.Lines(card.SourcePath)
		if err != nil {
			return cardLoad{}, err
		}
		if exists {
			cardLines = lines
			readSet[cardKeyPrefix+card.SourcePath] = float64(lines) * w.ContextPerLine
		}
	}

	messages := w.TargetMessages*float64(targets) +
		w.TestFileMessages*float64(len(testFiles)) +
		w.UsesMessages*float64(len(card.Uses))
	return cardLoad{messages: messages, written: float64(cardLines) * w.WritePerCardLine, readSet: readSet}, nil
}
