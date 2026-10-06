package shedrecipe

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// shedfake.Shuttle serves here as a non-nil seam value and, as a nil *shedfake.Shuttle, a typed-nil interface value.

// TestRequireHelpers pins the entry-construction guards: each refuses an invalid value with an error naming the entry and the field, and accepts a valid one.
// A typed-nil concrete pointer stored in a seam interface is refused too, which a direct nil comparison misses.
//
//testtiming:keep pins that each entry-construction guard refuses with an error naming the entry and field, including a typed-nil seam, which its covering tests do not
func TestRequireHelpers(t *testing.T) {
	t.Parallel()
	var nilShuttleInterface shedadapters.Shuttle
	var typedNilShuttle shedadapters.Shuttle = (*shedfake.Shuttle)(nil)
	tests := []struct {
		name    string
		call    func() error
		wantErr bool
	}{
		{"requireAbsRoot refuses empty", func() error { return requireAbsRoot("MyEntry", "MyField", "") }, true},
		{"requireAbsRoot refuses relative", func() error { return requireAbsRoot("MyEntry", "MyField", "relative/path") }, true},
		{"requireAbsRoot accepts absolute", func() error { return requireAbsRoot("MyEntry", "MyField", "/abs/path") }, false},
		{"requireNonEmpty refuses empty", func() error { return requireNonEmpty("MyEntry", "MyField", "") }, true},
		{"requireNonEmpty accepts non-empty", func() error { return requireNonEmpty("MyEntry", "MyField", "a-slug") }, false},
		{"requireSeam refuses untyped nil", func() error { return requireSeam("MyEntry", "MyField", nil) }, true},
		{"requireSeam refuses a nil interface", func() error { return requireSeam("MyEntry", "MyField", nilShuttleInterface) }, true},
		{"requireSeam refuses a typed-nil pointer", func() error { return requireSeam("MyEntry", "MyField", typedNilShuttle) }, true},
		{"requireSeam accepts a non-nil value", func() error { return requireSeam("MyEntry", "MyField", &shedfake.Shuttle{}) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.call()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v; want error = %v", err, tt.wantErr)
			}
			if err != nil && (!strings.Contains(err.Error(), "MyEntry") || !strings.Contains(err.Error(), "MyField")) {
				t.Errorf("error = %v; want it to name entry %q and field %q", err, "MyEntry", "MyField")
			}
		})
	}
}
