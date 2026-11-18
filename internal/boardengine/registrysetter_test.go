// registrysetter_test.go pins the registry setter's refusals and entry rewrite, and the machine id's one write.

package boardengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRegistryCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		registry Registry
		reg      Registration
		// wantRefusal is the refusal's Error text; empty requires the registration to pass.
		wantRefusal string
		wantMachine string
		wantWay     string
		// wantApplied is the registry apply returns for a passing registration.
		wantApplied Registry
		wantChanged bool
	}{
		{
			name:        "NoID",
			reg:         Registration{Identity: machinename.Identity{Name: "osl46"}},
			wantRefusal: "machine registration refused: the registration carries no machine id; way forward: take the identity from boardengine.EnsureIdentity, which always returns one",
			wantWay:     "take the identity from boardengine.EnsureIdentity, which always returns one",
		},
		{
			name:     "CopiedMachineFile",
			registry: Registry{"id-a": {Name: "osl46"}},
			reg:      Registration{Identity: machinename.Identity{ID: "id-a", Name: "laptop"}},
			wantRefusal: `machine registration refused: the registry holds this machine id as "osl46", ` +
				`which is not this machine's name "laptop" or a name it replaced: the machine file was copied from another machine; ` +
				"way forward: run `lym machine name <new>` on this machine",
			wantMachine: "osl46",
			wantWay:     "run `lym machine name <new>` on this machine",
		},
		{
			name:     "FormerNameIsAnotherMachinesCurrentName",
			registry: Registry{"id-b": {Name: "neve"}},
			reg:      Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve"}}},
			wantRefusal: `machine registration refused: this machine's former name "neve" is the current name of machine "neve"; ` +
				"way forward: run `lym machine name --forget-former neve` on this machine",
			wantMachine: "neve",
			wantWay:     "run `lym machine name --forget-former neve` on this machine",
		},
		{
			name:     "NameIsAnotherMachinesCurrentName",
			registry: Registry{"id-b": {Name: "osl46"}},
			reg:      Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46"}},
			wantRefusal: `machine registration refused: this machine's name "osl46" is a name of machine "osl46"; ` +
				"way forward: run `lym machine name <new>` on this machine",
			wantMachine: "osl46",
			wantWay:     "run `lym machine name <new>` on this machine",
		},
		{
			name:     "NameIsAnotherMachinesFormerName",
			registry: Registry{"id-b": {Name: "box", Former: []string{"osl46"}}},
			reg:      Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46"}},
			wantRefusal: `machine registration refused: this machine's name "osl46" is a name of machine "box"; ` +
				"way forward: run `lym machine name <new>` on this machine",
			wantMachine: "box",
			wantWay:     "run `lym machine name <new>` on this machine",
		},
		{
			name:        "FirstRegistrationCopiesTheFilesFormerNames",
			reg:         Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve"}}},
			wantApplied: Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}},
			wantChanged: true,
		},
		{
			name:        "RenameMovesTheOldNameToFormer",
			registry:    Registry{"id-a": {Name: "neve"}},
			reg:         Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve"}}},
			wantApplied: Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}},
			wantChanged: true,
		},
		{
			name:     "ForgottenFormerNameLeavesTheEntry",
			registry: Registry{"id-a": {Name: "osl46", Former: []string{"neve", "old"}}},
			reg: Registration{
				Identity: machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve", "old"}},
				Forget:   []string{"old"},
			},
			wantApplied: Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}},
			wantChanged: true,
		},
		{
			name:     "CopiedFileWhoseStaleNameWasForgottenPasses",
			registry: Registry{"id-a": {Name: "old"}},
			reg: Registration{
				Identity: machinename.Identity{ID: "id-a", Name: "osl46", Forgotten: []string{"old"}},
				Forget:   []string{"old"},
			},
			wantApplied: Registry{"id-a": {Name: "osl46"}},
			wantChanged: true,
		},
		{
			name:        "RegistrationEqualToTheEntryChangesNothing",
			registry:    Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}},
			reg:         Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve"}}},
			wantApplied: Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.registry.Check(tt.reg)
			if tt.wantRefusal != "" {
				var refusal *RegistrationError
				if !errors.As(err, &refusal) {
					t.Fatalf("Check() error = %v; want a *RegistrationError", err)
				}
				if refusal.Error() != tt.wantRefusal {
					t.Errorf("Check() error = %q; want %q", refusal.Error(), tt.wantRefusal)
				}
				if refusal.Machine != tt.wantMachine || refusal.WayForward != tt.wantWay {
					t.Errorf("Check() machine, way forward = %q, %q; want %q, %q", refusal.Machine, refusal.WayForward, tt.wantMachine, tt.wantWay)
				}
				return
			}
			if err != nil {
				t.Fatalf("Check() error = %v; want nil", err)
			}
			applied, changed := tt.registry.apply(tt.reg)
			if !reflect.DeepEqual(applied, tt.wantApplied) || changed != tt.wantChanged {
				t.Errorf("apply() = %v, %t; want %v, %t", applied, changed, tt.wantApplied, tt.wantChanged)
			}
		})
	}
}

func TestEnsureIdentityWith(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const hostName = "myhost"
	hostname := func() (string, error) { return hostName, nil }
	locateIn := func(dir string) machineFileLocator {
		return func() (string, string, bool, error) {
			return filepath.Join(dir, "machine.json"), filepath.Join(dir, "machine.lock"), true, nil
		}
	}
	readRaw := func(t *testing.T, dir string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "machine.json"))
		if err != nil {
			t.Fatalf("read machine file: %v", err)
		}
		return string(data)
	}

	t.Run("NoDirectoryIsRefusedNamingTheVariableAndTheRestart", func(t *testing.T) {
		t.Parallel()
		variable := "XDG_CONFIG_HOME or HOME"
		if runtime.GOOS == "windows" {
			variable = "LOCALAPPDATA"
		}
		unresolved := func() (string, string, bool, error) { return "", "", false, nil }
		_, err := ensureIdentityWith(unresolved, hostname)
		var refusal *RegistrationError
		if !errors.As(err, &refusal) {
			t.Fatalf("ensureIdentityWith() error = %v; want a *RegistrationError", err)
		}
		if !strings.Contains(refusal.Reason, variable) || !strings.Contains(refusal.WayForward, "set "+variable+" to an absolute path") ||
			!strings.Contains(refusal.WayForward, "`lym beam stop` and `lym beam start`") {
			t.Errorf("ensureIdentityWith() refusal = %v; want it to name %s and the restart", refusal, variable)
		}
		if got := identityRefusal(unresolved); got == nil || got.Error() != refusal.Error() {
			t.Errorf("identityRefusal() = %v; want %v", got, refusal)
		}
		if got := identityRefusal(locateIn(t.TempDir())); got != nil {
			t.Errorf("identityRefusal() with a directory = %v; want nil", got)
		}
	})

	t.Run("NoFileIsCreatedWithAnIDAndTheHostnameName", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		got, err := ensureIdentityWith(locateIn(dir), hostname)
		if err != nil {
			t.Fatalf("ensureIdentityWith() error = %v; want nil", err)
		}
		if len(got.ID) != 32 || got.Name != hostName || len(got.Former) != 0 {
			t.Errorf("ensureIdentityWith() = %+v; want a 32-character id and the name %q with no former name", got, hostName)
		}
		file, found, err := machinename.ReadFile(filepath.Join(dir, "machine.json"))
		if err != nil || !found {
			t.Fatalf("ReadFile() = %+v, %t, %v; want the created file", file, found, err)
		}
		if file.ID != got.ID || file.Name != hostName || !reflect.DeepEqual(file.Names, []string{hostName}) {
			t.Errorf("machine file = %+v; want id %q, name and names %q", file, got.ID, hostName)
		}
	})

	t.Run("FileWithoutAnIDGetsOneAndKeepsItsOtherKeys", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "machine.json"), []byte(`{"name":"osl46","names":["neve","osl46"],"extra":7}`), 0o644); err != nil {
			t.Fatalf("seed machine file: %v", err)
		}
		got, err := ensureIdentityWith(locateIn(dir), hostname)
		if err != nil {
			t.Fatalf("ensureIdentityWith() error = %v; want nil", err)
		}
		if len(got.ID) != 32 || got.Name != "osl46" || !reflect.DeepEqual(got.Former, []string{"neve"}) {
			t.Errorf("ensureIdentityWith() = %+v; want a 32-character id, name osl46 and former [neve]", got)
		}
		raw := readRaw(t, dir)
		for _, want := range []string{`"extra": 7`, `"neve"`, `"id": "` + got.ID + `"`} {
			if !strings.Contains(raw, want) {
				t.Errorf("machine file = %s; want it to contain %s", raw, want)
			}
		}
	})

	t.Run("FileWithAnIDIsReturnedUnchangedTwice", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seed := `{"name":"osl46","names":["osl46"],"id":"abc","forgotten":["old"]}`
		if err := os.WriteFile(filepath.Join(dir, "machine.json"), []byte(seed), 0o644); err != nil {
			t.Fatalf("seed machine file: %v", err)
		}
		want := machinename.Identity{ID: "abc", Name: "osl46", Forgotten: []string{"old"}}
		for range 2 {
			got, err := ensureIdentityWith(locateIn(dir), hostname)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("ensureIdentityWith() = %+v, %v; want %+v", got, err, want)
			}
		}
		if raw := readRaw(t, dir); raw != seed {
			t.Errorf("machine file = %s; want it untouched: %s", raw, seed)
		}
	})

	t.Run("FileHoldingAnInvalidNameIsAnErrorAndStaysUnchanged", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seed := `{"name":"Bad_Name","names":["Bad_Name"]}`
		if err := os.WriteFile(filepath.Join(dir, "machine.json"), []byte(seed), 0o644); err != nil {
			t.Fatalf("seed machine file: %v", err)
		}
		_, err := ensureIdentityWith(locateIn(dir), hostname)
		if err == nil || !strings.Contains(err.Error(), "holds no valid name") {
			t.Fatalf("ensureIdentityWith() error = %v; want one saying the file holds no valid name", err)
		}
		if raw := readRaw(t, dir); raw != seed {
			t.Errorf("machine file = %s; want it untouched: %s", raw, seed)
		}
	})
}
