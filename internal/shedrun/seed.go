// seed.go declares the seed.json contract: the Seed struct, the closed driver and recipe
// vocabularies, and ReadSeed/WriteSeed/List, the sole parser and writer of the file anywhere in this
// repository. seed.json is written once at run creation and never mutated mid-run, so ReadSeed reads
// it with no lock. The driver vocabulary is closed at two values; whether a given recipe can honour
// DriverLLM is decided at the seeding sites by that recipe's bootstrap-verb capability, not here.

package shedrun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// ErrDisagreeingSeed is the sentinel WriteSeed wraps its own refusal in when an existing seed at
// runID disagrees with the one being written -- a business judgment a caller can route to a
// recoverable verdict, distinct from a path-resolution or write failure.
var ErrDisagreeingSeed = errors.New("shedrun: disagreeing seed")

// Seed records a run's chosen recipe, driver, and parameters at seed time: the durable, write-once
// artefact this package alone parses and writes as seed.json.
type Seed struct {
	// Recipe is the recipe name this run was seeded to arm: one of RecipeNames().
	Recipe string `json:"recipe"`
	// Driver is the driver this run's child uses: one of DriverGo or DriverLLM.
	Driver string `json:"driver"`
	// Params carries the run's seed-time parameters, keyed by name.
	// It is omitted from the encoded JSON entirely when empty.
	Params map[string]string `json:"params,omitempty"`
}

// The closed driver vocabulary a Seed's Driver field may hold.
const (
	// DriverGo is the shipped driver: a Go-implemented producer arming the run's child directly.
	DriverGo = "go"
	// DriverLLM names an ly-drive session inside the run's own worktree, booted by the recipe's
	// bootstrap verb. Whether a given recipe can honour it is decided at the seeding sites by that
	// recipe's bootstrap-verb capability, not by ValidateDriver.
	DriverLLM = "llm"
)

// ValidateDriver reports whether driver is a legal Seed.Driver value.
// It accepts DriverGo and DriverLLM, and refuses any other value as unknown by naming both legal
// values.
func ValidateDriver(driver string) error {
	switch driver {
	case DriverGo, DriverLLM:
		return nil
	default:
		return fmt.Errorf("shedrun: unknown driver %q; must be %q or %q", driver, DriverGo, DriverLLM)
	}
}

// The closed recipe-name vocabulary a Seed's Recipe field may hold.
// internal/shedrun is the sole declarer of this vocabulary, per the overview's
// shedrun-owns-the-recipe-name-vocabulary Shared Decision; internal/shedcli's recipes map is the sole
// name-to-arming-function table and stays in sync with this vocabulary via a meta-test there.
const (
	// RecipeLoom names the loom recipe.
	RecipeLoom = "loom"
	// RecipeBatten names the batten recipe.
	RecipeBatten = "batten"
)

// RecipeNames returns the closed recipe-name vocabulary as a freshly allocated, sorted slice.
// Callers must not rely on a fixed order beyond "sorted" and must not mutate the vocabulary through
// the returned slice affecting any other caller, since each call allocates its own.
func RecipeNames() []string {
	return []string{RecipeBatten, RecipeLoom}
}

// ValidateRecipe reports whether name is a legal Seed.Recipe value.
// On refusal, the error names the available recipes from RecipeNames().
func ValidateRecipe(name string) error {
	for _, candidate := range RecipeNames() {
		if name == candidate {
			return nil
		}
	}
	return fmt.Errorf("shedrun: unknown recipe %q; available recipes: %v", name, RecipeNames())
}

// ReadSeed reads the seed for runID under l, returning (Seed{}, false, nil) when no seed has been
// written yet.
// It validates runID first, reads SeedFile(l, runID) with no lock -- the seed is write-once and never
// mutated mid-run -- decodes strictly so an unknown key is an error, defaults an absent or empty
// Driver to DriverGo, and validates the decoded Driver and every Params key being non-empty.
func ReadSeed(l *lyxcwd.Location, runID string) (Seed, bool, error) {
	if err := ValidateRunID(runID); err != nil {
		return Seed{}, false, err
	}

	data, err := os.ReadFile(SeedFile(l, runID))
	if err != nil {
		if os.IsNotExist(err) {
			return Seed{}, false, nil
		}
		return Seed{}, false, fmt.Errorf("shedrun: read seed for run %q: %w", runID, err)
	}

	seed, err := decodeSeed(data)
	if err != nil {
		return Seed{}, false, fmt.Errorf("shedrun: decode seed for run %q: %w", runID, err)
	}

	if seed.Driver == "" {
		seed.Driver = DriverGo
	}
	if err := ValidateDriver(seed.Driver); err != nil {
		return Seed{}, false, fmt.Errorf("shedrun: seed for run %q: %w", runID, err)
	}
	for key := range seed.Params {
		if key == "" {
			return Seed{}, false, fmt.Errorf("shedrun: seed for run %q: params key must not be empty", runID)
		}
	}

	return seed, true, nil
}

// decodeSeed strictly decodes data as a Seed, rejecting an unknown JSON key.
func decodeSeed(data []byte) (Seed, error) {
	var seed Seed
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&seed); err != nil {
		return Seed{}, err
	}
	return seed, nil
}

// WriteSeed writes seed for runID under l.
// It validates runID, seed.Recipe, and seed.Driver first, creates RunDir(l, runID), and is idempotent
// against a byte-identical existing seed -- calling WriteSeed twice with the same seed is a no-op the
// second time -- while refusing a disagreeing existing seed with an ErrDisagreeingSeed-wrapped
// message naming both the existing and the incoming values.
func WriteSeed(l *lyxcwd.Location, runID string, seed Seed) error {
	if err := ValidateRunID(runID); err != nil {
		return err
	}
	if err := ValidateRecipe(seed.Recipe); err != nil {
		return err
	}
	if err := ValidateDriver(seed.Driver); err != nil {
		return err
	}

	if err := os.MkdirAll(RunDir(l, runID), 0o755); err != nil {
		return fmt.Errorf("shedrun: create run directory for run %q: %w", runID, err)
	}

	path := SeedFile(l, runID)
	encoded, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		return fmt.Errorf("shedrun: encode seed for run %q: %w", runID, err)
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("shedrun: read existing seed for run %q: %w", runID, err)
		}
	} else {
		existingSeed, decodeErr := decodeSeed(existing)
		if decodeErr != nil {
			return fmt.Errorf("shedrun: decode existing seed for run %q: %w", runID, decodeErr)
		}
		if existingSeed.Driver == "" {
			existingSeed.Driver = DriverGo
		}
		if existingSeed.Recipe == seed.Recipe && existingSeed.Driver == seed.Driver && paramsEqual(existingSeed.Params, seed.Params) {
			return nil
		}
		return fmt.Errorf("%w: run %q is already seeded with %+v; refusing to overwrite with %+v", ErrDisagreeingSeed, runID, existingSeed, seed)
	}

	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("shedrun: write seed for run %q: %w", runID, err)
	}
	return nil
}

// paramsEqual reports whether a and b hold the same set of key-value pairs.
func paramsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		other, ok := b[key]
		if !ok || other != value {
			return false
		}
	}
	return true
}

// List returns every run-id under l that has a readable seed.json, sorted.
// It enumerates directories directly under filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName,
// shedDirName), keeps only those holding a readable seed.json, and returns an empty slice with no
// error when that parent directory does not exist at all.
func List(l *lyxcwd.Location) ([]string, error) {
	parent := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, shedDirName)

	entries, err := os.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("shedrun: list runs: %w", err)
	}

	var runIDs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.ReadFile(filepath.Join(parent, entry.Name(), "seed.json")); err != nil {
			continue
		}
		runIDs = append(runIDs, entry.Name())
	}

	sort.Strings(runIDs)
	return runIDs, nil
}

// MissingSeedMessage renders the one shared "no seed at this run-id" refusal: it names the addressed
// runID, lists existing sorted, says plainly that no run is seeded yet when existing is empty, and
// closes with remedy.
// Both internal/loomcli and internal/battencli call it so the two paths word the refusal identically;
// neither may import the other.
// The rendered text carries no "kind" field and no structure a caller could mistake for one -- the
// step refusal-kind vocabulary is pinned closed at five values, and a missing run is not a sixth.
func MissingSeedMessage(prefix, runID string, existing []string, remedy string) string {
	sorted := append([]string(nil), existing...)
	sort.Strings(sorted)

	if len(sorted) == 0 {
		return fmt.Sprintf("%s: no seed found for run %q; no run is seeded yet. %s", prefix, runID, remedy)
	}
	return fmt.Sprintf("%s: no seed found for run %q; seeded runs: %v. %s", prefix, runID, sorted, remedy)
}
