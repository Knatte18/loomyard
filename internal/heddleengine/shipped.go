// shipped.go holds the shipped layer: lym's own instruction entries, embedded in the binary in heddle.json's format, and the layered read that sets them, with any lym override in force, before a repository's entries.

package heddleengine

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// The layers an entry comes from.
const (
	// LayerShipped is the read-only layer lym ships in its binary, changed only by a lym release.
	LayerShipped = "shipped"
	// LayerRepository is the repository's own layer, heddle.json in the Bolt checkout.
	LayerRepository = "repository"
)

// Headings open the two sections of a file target both layers write to, so the repository's text stays apart from lym's.
const (
	// shippedHeading opens the shipped layer's section.
	shippedHeading = "# LoomYard's own instructions"
	// repositoryHeading opens the repository layer's section when a shipped section precedes it.
	repositoryHeading = "# This repository's instructions"
)

//go:embed shipped.json
var shippedJSON []byte

// shippedStore is the shipped layer, decoded and checked once at package load.
var shippedStore = decodeShipped(shippedJSON)

// decodeShipped decodes the embedded shipped layer and panics when it does not decode, an entry fails ValidateEntry or two entries share a slug, since a malformed shipped layer is a defect of the lym build itself.
func decodeShipped(data []byte) Store {
	store, err := decodeBytes(data)
	if err != nil {
		panic(fmt.Sprintf("heddleengine: the shipped layer does not decode: %v", err))
	}
	seen := map[string]bool{}
	for _, entry := range store.Entries {
		if err := ValidateEntry(entry); err != nil {
			panic(fmt.Sprintf("heddleengine: the shipped layer holds an invalid entry: %v", err))
		}
		if seen[entry.Slug] {
			panic(fmt.Sprintf("heddleengine: the shipped layer holds the slug %q twice", entry.Slug))
		}
		seen[entry.Slug] = true
	}
	return store
}

// Layers is what a render reads besides the shipped layer: the repository's store, and the lym overrides in force, none while the hub's lym_override setting is off.
// OverrideWarning is set when the setting is on but the override file did not load, in which case Overrides is empty and the shipped texts stand.
type Layers struct {
	Repository      Store
	Overrides       Store
	OverrideWarning string
}

// LayeredEntry is an entry with the layer it comes from.
// Overridden reports whether its text is a lym override's in place of the shipped one.
type LayeredEntry struct {
	Entry
	Layer      string
	Overridden bool
}

// Ordered returns the shipped layer's entries in order, each carrying the text of its override when Overrides holds one that is not blank, then the repository's entries in order, each marked with its layer.
// This is the render order of every target.
func (l Layers) Ordered() []LayeredEntry {
	var layered []LayeredEntry
	for _, entry := range shippedStore.Ordered() {
		item := LayeredEntry{Entry: entry, Layer: LayerShipped}
		if override, found := l.Overrides.Get(entry.Slug); found && strings.TrimSpace(override.Text) != "" {
			item.Text = override.Text
			item.Overridden = true
		}
		layered = append(layered, item)
	}
	for _, entry := range l.Repository.Ordered() {
		layered = append(layered, LayeredEntry{Entry: entry, Layer: LayerRepository})
	}
	return layered
}

// ReadLayers loads the repository's store from the Bolt checkout boltDir, and the lym overrides when the hub's heddle.yaml sets lym_override.
// An override file that does not load leaves the overrides empty and sets OverrideWarning, since the setting only tunes lym's own text and must not stop the repository's entries from reading.
// It takes no lock; the caller does.
func ReadLayers(boltDir string) (Layers, error) {
	repository, err := Load(boltDir)
	if err != nil {
		return Layers{}, err
	}
	cfg, err := LoadConfig(configengine.Dirs{Board: boltDir})
	if err != nil {
		return Layers{}, err
	}
	layers := Layers{Repository: repository}
	if cfg.LymOverride {
		if layers.Overrides, err = LoadOverrides(boltDir); err != nil {
			layers.Overrides = Store{}
			layers.OverrideWarning = fmt.Sprintf("the lym overrides are ignored and the shipped texts stand: %v", err)
		}
	}
	return layers, nil
}

// refuseShipped refuses a new repository entry under slug when the shipped layer holds it, naming the way forward.
// The caller asks it only for a slug the repository store holds no entry under, so an entry that collides with a shipped slug stays open to change and removal.
func refuseShipped(slug string) error {
	if _, found := shippedStore.Get(slug); found {
		return fmt.Errorf("entry %q ships with lym and is read-only, so no repository entry can be created under it; way forward: give the new repository entry another slug, or tune the shipped text with `lym heddle set %s --layer %s` in a hub whose heddle.yaml sets %s", slug, slug, OverrideLayer, LymOverrideKey)
	}
	return nil
}
