// pins_test.go covers Pins and ChangedPins over board copies in a t.TempDir().

package stencilstore

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestChangedPins pins the stencils installed when the pins are taken, then changes the board copies.
// It asserts which pinned stencils ChangedPins reports, in name order,
// and that a board copy that cannot be read is an error naming the stencil.
func TestChangedPins(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	registry := newFakeRegistry(map[string][]byte{
		"loom-one":   []byte("shipped one\n"),
		"loom-two":   []byte("shipped two\n"),
		"loom-three": []byte("shipped three\n"),
		"loom-four":  []byte("shipped four\n"),
	})

	tests := []struct {
		name   string
		change func(t *testing.T, dir string)
		want   []string
		// wantErrIn is a substring of the error ChangedPins must return;
		// empty wants success.
		wantErrIn string
	}{
		{"UntouchedCopiesReportNothing", func(*testing.T, string) {}, nil, ""},
		{"EditedBodyIsReported", func(t *testing.T, dir string) {
			writeBoardCopy(t, dir, "loom-two", []byte("body two edited\n"))
		}, []string{"loom-two"}, ""},
		{"EveryEditedBodyIsReportedInNameOrder", func(t *testing.T, dir string) {
			writeBoardCopy(t, dir, "loom-three", []byte("body three edited\n"))
			writeBoardCopy(t, dir, "loom-two", []byte("body two edited\n"))
			writeBoardCopy(t, dir, "loom-one", []byte("body one edited\n"))
		}, []string{"loom-one", "loom-three", "loom-two"}, ""},
		{"RestampedBannerIsNotReported", func(t *testing.T, dir string) {
			writeBoardCopy(t, dir, "loom-one", []byte(StampPrefix+"0000"+StampSuffix+"\nbody one\n"))
		}, nil, ""},
		{"RemovedCopyIsReported", func(t *testing.T, dir string) {
			if err := os.Remove(Path(dir, "loom-one")); err != nil {
				t.Fatal(err)
			}
		}, []string{"loom-one"}, ""},
		{"StencilInstalledAfterThePinsIsNotReported", func(t *testing.T, dir string) {
			writeBoardCopy(t, dir, "loom-four", []byte("body four\n"))
		}, nil, ""},
		{"UnreadableCopyIsAnErrorNamingTheStencil", func(t *testing.T, dir string) {
			// A directory where the board copy belongs exists but cannot be read as a file.
			path := Path(dir, "loom-two")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil, `"loom-two"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeBoardCopy(t, dir, "loom-one", []byte("body one\n"))
			writeBoardCopy(t, dir, "loom-two", []byte("body two\n"))
			writeBoardCopy(t, dir, "loom-three", []byte("body three\n"))

			pins, err := Pins(dir, registry)
			if err != nil {
				t.Fatalf("Pins() error: %v", err)
			}
			wantPins := map[string]string{
				"loom-one":   BodyHash([]byte("body one\n")),
				"loom-two":   BodyHash([]byte("body two\n")),
				"loom-three": BodyHash([]byte("body three\n")),
			}
			if !reflect.DeepEqual(pins, wantPins) {
				t.Fatalf("Pins() = %v; want %v", pins, wantPins)
			}

			tt.change(t, dir)

			got, err := ChangedPins(dir, pins)
			if tt.wantErrIn != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("ChangedPins() error = %v; want one naming %s", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChangedPins() error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangedPins() = %v; want %v", got, tt.want)
			}
		})
	}
}
