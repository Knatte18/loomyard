package shedrecipe

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
)

// fakeShuttle is declared once, in fixture_test.go, and reused here as a non-nil seam value and,
// as a nil *fakeShuttle, a typed-nil interface value.

func TestRequireAbsRoot(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"Empty", ""},
		{"Relative", "relative/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireAbsRoot("MyEntry", "MyField", tt.value)
			if err == nil {
				t.Fatalf("requireAbsRoot() error = nil; want non-nil")
			}
			if !strings.Contains(err.Error(), "MyEntry") || !strings.Contains(err.Error(), "MyField") {
				t.Errorf("requireAbsRoot() error = %v; want it to name entry %q and field %q", err, "MyEntry", "MyField")
			}
		})
	}

	t.Run("Absolute", func(t *testing.T) {
		if err := requireAbsRoot("MyEntry", "MyField", "/abs/path"); err != nil {
			t.Errorf("requireAbsRoot() error = %v; want nil", err)
		}
	})
}

func TestRequireSeam(t *testing.T) {
	t.Run("UntypedNil", func(t *testing.T) {
		err := requireSeam("MyEntry", "MyField", nil)
		if err == nil {
			t.Fatalf("requireSeam() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "MyEntry") || !strings.Contains(err.Error(), "MyField") {
			t.Errorf("requireSeam() error = %v; want it to name entry %q and field %q", err, "MyEntry", "MyField")
		}
	})

	t.Run("NilShuttleInterface", func(t *testing.T) {
		var s shedadapters.Shuttle
		err := requireSeam("MyEntry", "MyField", s)
		if err == nil {
			t.Fatalf("requireSeam() error = nil; want non-nil")
		}
	})

	t.Run("NonNilValue", func(t *testing.T) {
		s := &fakeShuttle{}
		if err := requireSeam("MyEntry", "MyField", s); err != nil {
			t.Errorf("requireSeam() error = %v; want nil", err)
		}
	})

	t.Run("TypedNilConcretePointerStoredInInterface", func(t *testing.T) {
		var s shedadapters.Shuttle = (*fakeShuttle)(nil)
		err := requireSeam("MyEntry", "MyField", s)
		if err == nil {
			t.Fatalf("requireSeam() error = nil; want non-nil -- this is the case a direct nil comparison misses")
		}
		if !strings.Contains(err.Error(), "MyEntry") || !strings.Contains(err.Error(), "MyField") {
			t.Errorf("requireSeam() error = %v; want it to name entry %q and field %q", err, "MyEntry", "MyField")
		}
	})
}
