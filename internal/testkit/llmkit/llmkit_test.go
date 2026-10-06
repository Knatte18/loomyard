package llmkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const testOverrideEnv = "LLMKIT_TEST_CLAUDE"

// Serial: t.Setenv of PATH and the override variable is process-global state.
func TestClaude(t *testing.T) {
	fakeDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fakeClaude := filepath.Join(fakeDir, name)
	if err := os.WriteFile(fakeClaude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	tests := []struct {
		name        string
		override    string
		path        string
		want        string
		wantSkipped bool
	}{
		{name: "override wins over PATH", override: "/custom/claude", path: fakeDir, want: "/custom/claude"},
		{name: "PATH lookup without override", path: fakeDir, want: fakeClaude},
		{name: "neither skips", path: t.TempDir(), wantSkipped: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(testOverrideEnv, tc.override)
			t.Setenv("PATH", tc.path)

			var got string
			var skipped bool
			t.Run("inner", func(t *testing.T) {
				defer func() { skipped = t.Skipped() }()
				got = Claude(t, testOverrideEnv)
			})

			if skipped != tc.wantSkipped {
				t.Fatalf("skipped = %v, want %v", skipped, tc.wantSkipped)
			}
			if got != tc.want {
				t.Errorf("Claude = %q, want %q", got, tc.want)
			}
		})
	}
}
