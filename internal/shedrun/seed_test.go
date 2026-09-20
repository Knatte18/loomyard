package shedrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSeed_ReadSeed_RoundTrip(t *testing.T) {
	l := syntheticLocation(t)
	seed := Seed{Recipe: RecipeBatten, Driver: DriverGo, Params: map[string]string{"slug": "example"}}

	if err := WriteSeed(l, "self", seed); err != nil {
		t.Fatalf("WriteSeed() = %v; want nil", err)
	}

	got, found, err := ReadSeed(l, "self")
	if err != nil {
		t.Fatalf("ReadSeed() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("ReadSeed() found = false; want true")
	}
	if got.Recipe != seed.Recipe || got.Driver != seed.Driver || got.Params["slug"] != "example" {
		t.Errorf("ReadSeed() = %+v; want %+v", got, seed)
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

func TestWriteSeed_ReadSeed_RoundTrip_LLMDriver(t *testing.T) {
	l := syntheticLocation(t)
	seed := Seed{Recipe: RecipeLoom, Driver: DriverLLM}

	if err := WriteSeed(l, "self", seed); err != nil {
		t.Fatalf("WriteSeed() = %v; want nil", err)
	}

	got, found, err := ReadSeed(l, "self")
	if err != nil {
		t.Fatalf("ReadSeed() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("ReadSeed() found = false; want true")
	}
	if got.Driver != DriverLLM {
		t.Errorf("ReadSeed() Driver = %q; want %q", got.Driver, DriverLLM)
	}
}

func TestValidateDriver(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		wantErr bool
	}{
		{"go", DriverGo, false},
		{"llm", DriverLLM, false},
		{"unknown", "rust", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDriver(tt.driver)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDriver(%q) = %v; want error = %v", tt.driver, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDriver_UnknownNamesBothLegalValues(t *testing.T) {
	err := ValidateDriver("rust")
	if err == nil {
		t.Fatalf("ValidateDriver(%q) error = nil; want refusal naming both legal values", "rust")
	}
	if !strings.Contains(err.Error(), DriverGo) || !strings.Contains(err.Error(), DriverLLM) {
		t.Errorf("ValidateDriver(%q) error = %q; want it to name both %q and %q", "rust", err.Error(), DriverGo, DriverLLM)
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

func TestWriteSeed_IdempotentAgainstIdenticalSeed(t *testing.T) {
	l := syntheticLocation(t)
	seed := Seed{Recipe: RecipeLoom, Driver: DriverGo}

	if err := WriteSeed(l, "self", seed); err != nil {
		t.Fatalf("WriteSeed() first call error = %v; want nil", err)
	}
	if err := WriteSeed(l, "self", seed); err != nil {
		t.Errorf("WriteSeed() second identical call error = %v; want nil (idempotent)", err)
	}
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
	if !strings.Contains(err.Error(), RecipeLoom) || !strings.Contains(err.Error(), RecipeBatten) {
		t.Errorf("WriteSeed() error = %q; want it to name both the existing (%q) and incoming (%q) recipe", err.Error(), RecipeLoom, RecipeBatten)
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
