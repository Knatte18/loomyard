// seam_enforcement_test.go enforces the Shared Decision "provider-seam import rule":
// only internal/shuttleengines and an engine's own directory import a concrete engine package under internal/shuttleengine/.
// The interface (Engine) and its value types live in shuttleengine;
// engines import shuttleengine and implement the interface, never the reverse, and the registry alone wires them in.
// This is what lets a further provider engine be added without touching this package or any caller.

package shuttleengine

import (
	"go/parser"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	// enginePackagePrefix is the import path prefix every concrete engine package carries.
	enginePackagePrefix = "github.com/Knatte18/loomyard/internal/shuttleengine/"
	// engineDirectoryPrefix is enginePackagePrefix as a module-relative directory.
	engineDirectoryPrefix = "internal/shuttleengine/"
	// registryDirectory is the one directory outside an engine's own that may import it.
	registryDirectory = "internal/shuttleengines"
)

// providerSeamViolations returns the production files under base that import a concrete engine package from outside that engine's own directory tree and outside the registry, with the count of files scanned.
// It reads actual import paths through the harness parse, which avoids false positives from string literals in doc comments.
func providerSeamViolations(t *testing.T, base string) ([]string, int) {
	t.Helper()
	var violations []string
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		directory := path.Dir(f.Rel)
		if directory == registryDirectory {
			return
		}
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			imported := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(imported, enginePackagePrefix) {
				continue
			}
			engineDirectory := engineDirectoryPrefix + strings.TrimPrefix(imported, enginePackagePrefix)
			if directory != engineDirectory && !strings.HasPrefix(directory, engineDirectory+"/") {
				violations = append(violations, f.Rel)
			}
		}
	})
	return violations, scanned
}

// startupGateCallers returns the production files under base outside internal/shuttleengine/ that contain a call of AcceptStartupGate, with the count of files scanned.
// The method is the engine-neutral startup-gate acceptance, and a caller of it outside the package is a new decision.
func startupGateCallers(t *testing.T, base string) ([]string, int) {
	t.Helper()
	var callers []string
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		directory := path.Dir(f.Rel)
		if directory == "internal/shuttleengine" || strings.HasPrefix(directory, engineDirectoryPrefix) {
			return
		}
		if strings.Contains(string(f.Data), ".AcceptStartupGate(") {
			callers = append(callers, f.Rel)
		}
	})
	return callers, scanned
}

// TestProviderSeamImportRule verifies that no production file in the module imports a concrete engine package except from that engine's own directory tree or from internal/shuttleengines,
// and that no production file outside internal/shuttleengine/ calls AcceptStartupGate.
// It also verifies that each scan finds a violation when one exists and lets a file inside the package through.
//
//testtiming:keep scans every production file's imports for a concrete engine package outside its own directory and the registry, and its calls for AcceptStartupGate outside this package, which no other test reads
//lyx:guard
func TestProviderSeamImportRule(t *testing.T) {
	t.Parallel()

	const importsClaude = "package x\n\nimport _ \"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine\"\n"
	const callsGate = "package x\n\nfunc f(r interface{ AcceptStartupGate(string) (bool, error) }) { _, _ = r.AcceptStartupGate(\"g\") }\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []string
		// wantCallers is the files the AcceptStartupGate call-site scan flags.
		wantCallers []string
	}{
		{"ImportFromAnotherPackage", map[string]string{"internal/x/x.go": importsClaude}, []string{"internal/x/x.go"}, nil},
		{"ImportFromTheEnginesOwnDirectory", map[string]string{"internal/shuttleengine/claudeengine/a.go": importsClaude}, nil, nil},
		{"ImportFromTheRegistry", map[string]string{"internal/shuttleengines/b.go": importsClaude}, nil, nil},
		{"AcceptStartupGateCalledFromAnotherPackage", map[string]string{"internal/x/x.go": callsGate}, nil, []string{"internal/x/x.go"}},
		{"AcceptStartupGateCalledInsideTheShuttlePackage", map[string]string{"internal/shuttleengine/y.go": callsGate}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			for rel, content := range tt.files {
				abs := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, _ := providerSeamViolations(t, base)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("providerSeamViolations = %v; want %v", got, tt.want)
			}
			if callers, _ := startupGateCallers(t, base); strings.Join(callers, ",") != strings.Join(tt.wantCallers, ",") {
				t.Errorf("startupGateCallers = %v; want %v", callers, tt.wantCallers)
			}
		})
	}

	t.Run("RealModule", func(t *testing.T) {
		t.Parallel()
		got, scanned := providerSeamViolations(t, scankit.Root(t))
		scankit.RequireFloor(t, scanned, 1, "module-wide provider-seam scan")
		if len(got) > 0 {
			t.Errorf("provider-seam import rule violated (Shared Decision \"provider-seam import rule\"): "+
				"only internal/shuttleengines and an engine's own directory import a concrete engine, but found one imported in: %v", got)
		}
		callers, scanned := startupGateCallers(t, scankit.Root(t))
		scankit.RequireFloor(t, scanned, 1, "module-wide startup-gate call-site scan")
		if len(callers) > 0 {
			t.Errorf("AcceptStartupGate is called outside internal/shuttleengine/, which is a new decision: %v", callers)
		}
	})
}
