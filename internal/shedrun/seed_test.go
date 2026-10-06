package shedrun

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestWriteSeed_ReadSeed_RoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		seed Seed
	}{
		{"go driver with params", Seed{Recipe: RecipeBatten, Driver: DriverGo, Params: map[string]string{"slug": "example"}}},
		{"llm driver", Seed{Recipe: RecipeLoom, Driver: DriverLLM}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := syntheticLocation(t)
			if err := WriteSeed(l, "self", tt.seed); err != nil {
				t.Fatalf("WriteSeed() = %v; want nil", err)
			}

			got, found, err := ReadSeed(l, "self")
			if err != nil {
				t.Fatalf("ReadSeed() error = %v; want nil", err)
			}
			if !found {
				t.Fatalf("ReadSeed() found = false; want true")
			}
			if got.Recipe != tt.seed.Recipe || got.Driver != tt.seed.Driver || got.Params["slug"] != tt.seed.Params["slug"] {
				t.Errorf("ReadSeed() = %+v; want %+v", got, tt.seed)
			}
		})
	}
}

func TestReadSeed_Absent(t *testing.T) {
	l := syntheticLocation(t)

	got, found, err := ReadSeed(l, "self")
	if err != nil {
		t.Fatalf("ReadSeed() error = %v; want nil", err)
	}
	if found {
		t.Errorf("ReadSeed() found = true; want false")
	}
	if got.Recipe != "" || got.Driver != "" || len(got.Params) != 0 {
		t.Errorf("ReadSeed() = %+v; want zero value", got)
	}
}

func TestReadSeed_AbsentDriverDefaultsToGo(t *testing.T) {
	l := syntheticLocation(t)
	if err := os.MkdirAll(RunDir(l, "self"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(SeedFile(l, "self"), []byte(`{"recipe":"loom"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, found, err := ReadSeed(l, "self")
	if err != nil {
		t.Fatalf("ReadSeed() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("ReadSeed() found = false; want true")
	}
	if got.Driver != DriverGo {
		t.Errorf("ReadSeed() Driver = %q; want %q", got.Driver, DriverGo)
	}
}

func TestValidateDriver(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		driver  string
		wantErr bool
	}{
		{"go", DriverGo, false},
		{"llm", DriverLLM, false},
		{"unknown names both legal values", "rust", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDriver(tt.driver)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateDriver(%q) = %v; want error = %v", tt.driver, err, tt.wantErr)
			}
			if tt.wantErr && (!strings.Contains(err.Error(), DriverGo) || !strings.Contains(err.Error(), DriverLLM)) {
				t.Errorf("ValidateDriver(%q) error = %q; want it to name both %q and %q", tt.driver, err.Error(), DriverGo, DriverLLM)
			}
		})
	}
}

func TestValidateRecipe(t *testing.T) {
	tests := []struct {
		name    string
		recipe  string
		wantErr bool
	}{
		{"loom", RecipeLoom, false},
		{"batten", RecipeBatten, false},
		{"unknown", "someday", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRecipe(tt.recipe)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRecipe(%q) = %v; want error = %v", tt.recipe, err, tt.wantErr)
			}
		})
	}
}

//testtiming:keep pins the sorted order of the recipe vocabulary, which its covering test does not
func TestRecipeNames(t *testing.T) {
	got := RecipeNames()
	want := []string{RecipeBatten, RecipeLoom}
	if len(got) != len(want) {
		t.Fatalf("RecipeNames() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RecipeNames()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// writeLegacySeedFile creates l's self run directory holding raw seed.json bytes.
func writeLegacySeedFile(t *testing.T, l *lyxcwd.Location, body string) {
	t.Helper()
	if err := os.MkdirAll(RunDir(l, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SeedFile(l, "self"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSeed_LegacyParentKey pins that a seed.json carrying the retired parent key still reads, is dropped on a round trip,
// agrees with an identical new seed, and that any other unknown key is still refused.
func TestSeed_LegacyParentKey(t *testing.T) {
	t.Parallel()
	t.Run("decodes and is dropped on a round trip", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		writeLegacySeedFile(t, l, `{"recipe":"loom","driver":"go","parent":"ab:hub"}`)
		got, found, err := ReadSeed(l, "self")
		if err != nil || !found {
			t.Fatalf("ReadSeed() = (found=%v, err=%v); want (true, nil)", found, err)
		}
		want := Seed{Recipe: RecipeLoom, Driver: DriverGo}
		if got.Recipe != want.Recipe || got.Driver != want.Driver || len(got.Params) != 0 {
			t.Errorf("ReadSeed() = %+v; want %+v", got, want)
		}

		l2 := syntheticLocation(t)
		if err := WriteSeed(l2, "self", got); err != nil {
			t.Fatalf("WriteSeed() = %v; want nil", err)
		}
		data, err := os.ReadFile(SeedFile(l2, "self"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "parent") {
			t.Errorf("seed.json = %s; want no parent key after a round trip", data)
		}
	})
	t.Run("an agreeing new seed is a no-op", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		writeLegacySeedFile(t, l, `{"recipe":"loom","driver":"go","parent":"ab:first"}`)
		if err := WriteSeed(l, "self", Seed{Recipe: RecipeLoom, Driver: DriverGo}); err != nil {
			t.Fatalf("WriteSeed() = %v; want a no-op against an agreeing legacy seed", err)
		}
	})
	t.Run("another unknown key is still refused", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		writeLegacySeedFile(t, l, `{"recipe":"loom","driver":"go","parent":"ab:hub","bogus":"x"}`)
		if _, _, err := ReadSeed(l, "self"); err == nil {
			t.Error("ReadSeed() with an unknown key error = nil; want a decode error")
		}
	})
}

func TestWriteSeed_RefusesDisagreeingSeed(t *testing.T) {
	l := syntheticLocation(t)
	first := Seed{Recipe: RecipeLoom, Driver: DriverGo}
	second := Seed{Recipe: RecipeBatten, Driver: DriverGo}

	if err := WriteSeed(l, "self", first); err != nil {
		t.Fatalf("WriteSeed() first call error = %v; want nil", err)
	}

	err := WriteSeed(l, "self", second)
	if err == nil {
		t.Fatalf("WriteSeed() second disagreeing call error = nil; want refusal")
	}
	if !errors.Is(err, ErrDisagreeingSeed) {
		t.Errorf("WriteSeed() error = %v; want it to wrap ErrDisagreeingSeed, so a caller can route it to a recoverable verdict rather than treating it as a mechanism failure", err)
	}
	if !strings.Contains(err.Error(), RecipeLoom) || !strings.Contains(err.Error(), RecipeBatten) {
		t.Errorf("WriteSeed() error = %q; want it to name both the existing (%q) and incoming (%q) recipe", err.Error(), RecipeLoom, RecipeBatten)
	}
	if !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lyx shed status self") {
		t.Errorf("WriteSeed() error = %q; want a way forward naming lyx shed status for the run", err.Error())
	}

	// The way forward taken: driving the existing seed means re-writing it unchanged, which is idempotent.
	if err := WriteSeed(l, "self", first); err != nil {
		t.Errorf("WriteSeed() re-asserting the existing seed = %v; want nil", err)
	}
	if err := WriteSeed(l, "other-run", second); err != nil {
		t.Errorf("WriteSeed() under a different run-id = %v; want nil", err)
	}

	// Params alone disagree: a different count of pairs, then a different value for the same key.
	withSlug := Seed{Recipe: RecipeLoom, Driver: DriverGo, Params: map[string]string{"slug": "a"}}
	if err := WriteSeed(l, "params-run", withSlug); err != nil {
		t.Fatalf("WriteSeed() with params error = %v; want nil", err)
	}
	for name, incoming := range map[string]Seed{
		"fewer params":  {Recipe: RecipeLoom, Driver: DriverGo},
		"changed value": {Recipe: RecipeLoom, Driver: DriverGo, Params: map[string]string{"slug": "b"}},
	} {
		if err := WriteSeed(l, "params-run", incoming); !errors.Is(err, ErrDisagreeingSeed) {
			t.Errorf("WriteSeed() with %s error = %v; want it to wrap ErrDisagreeingSeed", name, err)
		}
	}
}

func TestList(t *testing.T) {
	l := syntheticLocation(t)

	// One directory with a valid, readable seed.json.
	if err := WriteSeed(l, "b-run", Seed{Recipe: RecipeLoom, Driver: DriverGo}); err != nil {
		t.Fatalf("WriteSeed() error = %v", err)
	}
	if err := WriteSeed(l, "a-run", Seed{Recipe: RecipeBatten, Driver: DriverGo}); err != nil {
		t.Fatalf("WriteSeed() error = %v", err)
	}

	// One directory with no seed.json at all.
	noSeedDir := RunDir(l, "no-seed-run")
	if err := os.MkdirAll(noSeedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// One directory whose seed.json exists but is unreadable.
	unreadableDir := RunDir(l, "unreadable-run")
	if err := os.MkdirAll(unreadableDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	unreadableSeed := filepath.Join(unreadableDir, "seed.json")
	if err := os.WriteFile(unreadableSeed, []byte(`{"recipe":"loom"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(unreadableSeed, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(unreadableSeed, 0o644)
	})

	got, err := List(l)
	if err != nil {
		t.Fatalf("List() error = %v; want nil", err)
	}
	want := []string{"a-run", "b-run"}
	if len(got) != len(want) {
		t.Fatalf("List() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestList_MissingParentDirectory(t *testing.T) {
	l := syntheticLocation(t)

	got, err := List(l)
	if err != nil {
		t.Fatalf("List() error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %v; want empty slice", got)
	}
}

func TestMissingSeedMessage(t *testing.T) {
	got := MissingSeedMessage("lyx batten step", "example-run", []string{"b-run", "a-run"}, "seed a run first")
	if !strings.Contains(got, "example-run") {
		t.Errorf("MissingSeedMessage() = %q; want it to name the addressed run-id", got)
	}
	if !strings.Contains(got, "a-run") || !strings.Contains(got, "b-run") {
		t.Errorf("MissingSeedMessage() = %q; want it to list the existing runs", got)
	}
	if strings.Index(got, "a-run") > strings.Index(got, "b-run") {
		t.Errorf("MissingSeedMessage() = %q; want existing runs sorted", got)
	}
	if !strings.Contains(got, "seed a run first") {
		t.Errorf("MissingSeedMessage() = %q; want it to close with the remedy", got)
	}
}

func TestMissingSeedMessage_EmptyListing(t *testing.T) {
	got := MissingSeedMessage("lyx batten step", "example-run", nil, "seed a run first")
	if !strings.Contains(got, "no run is seeded yet") {
		t.Errorf("MissingSeedMessage() = %q; want it to say plainly that no run is seeded yet", got)
	}
}
