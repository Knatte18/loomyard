// validate_test.go covers Validate against a bare t.TempDir(), reusing fakeRegistry from
// reconcile_test.go.

package stencilstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{
		"family-one": []byte("{{.kept}}\n{{.dropped}}\n"),
	})

	// on-disk body adds an unknown marker and drops the shipped "dropped" marker.
	onDisk := []byte("{{.kept}}\n{{.unknown}}\n")
	path := Path(baseDir, "family-one")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, onDisk, 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	findings, err := Validate(baseDir, registry)
	if err != nil {
		t.Fatalf("Validate(...) returned error: %v", err)
	}

	want := []Finding{
		{Name: "family-one", Marker: "dropped", Severity: SeverityWarning},
		{Name: "family-one", Marker: "unknown", Severity: SeverityError},
	}
	if len(findings) != len(want) {
		t.Fatalf("Validate(...) = %+v; want %+v", findings, want)
	}
	for i := range want {
		if findings[i] != want[i] {
			t.Errorf("Validate(...)[%d] = %+v; want %+v", i, findings[i], want[i])
		}
	}
}
