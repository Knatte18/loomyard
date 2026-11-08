// binary_test.go pins that the lym binary lookup never hands out the running test binary.

package selfexec_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLymBinary_RefusesTheRunningTestBinary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path, err := selfexec.LymBinary()

	if path != "" || !errors.Is(err, selfexec.ErrTestBinary) {
		t.Errorf("LymBinary() = %q, %v; want \"\", ErrTestBinary", path, err)
	}
}

func TestResolveLymBinary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	lookupErr := errors.New("no executable")
	tests := []struct {
		name       string
		executable func() (string, error)
		wantPath   string
		wantErr    error
	}{
		{"StandInPath", func() (string, error) { return "/opt/lym/bin/lym", nil }, "/opt/lym/bin/lym", nil},
		{"LookupError", func() (string, error) { return "", lookupErr }, "", lookupErr},
		{"TestBinaryPath", os.Executable, "", selfexec.ErrTestBinary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path, err := selfexec.ResolveLymBinary(tt.executable)

			if path != tt.wantPath || !errors.Is(err, tt.wantErr) {
				t.Errorf("ResolveLymBinary() = %q, %v; want %q, %v", path, err, tt.wantPath, tt.wantErr)
			}
		})
	}
}
