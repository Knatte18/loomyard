package envsource

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_DotEnvParsing(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name: "SkipCommentLines",
			content: `# This is a comment
VAR1=value1`,
			want: map[string]string{
				"VAR1": "value1",
			},
		},
		{
			name: "SkipBlankLines",
			content: `VAR1=value1

VAR2=value2`,
			want: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
		},
		{
			name: "SkipLinesWithoutEquals",
			content: `INVALID_LINE
VAR1=value1
ANOTHER_INVALID`,
			want: map[string]string{
				"VAR1": "value1",
			},
		},
		{
			name: "EqualsInValue",
			content: `EXPR=a=b
VAR1=value=with=multiple=equals`,
			want: map[string]string{
				"EXPR": "a=b",
				"VAR1": "value=with=multiple=equals",
			},
		},
		{
			name: "DoNotTrimValues",
			content: `VAR1= value with spaces
VAR2=  leading  space`,
			want: map[string]string{
				"VAR1": " value with spaces",
				"VAR2": "  leading  space",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dotEnvPath := filepath.Join(tmpDir, ".env")
			if err := os.WriteFile(dotEnvPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			got, err := readDotEnv(dotEnvPath)
			if err != nil {
				t.Fatalf("readDotEnv() = %v; want nil", err)
			}

			if !mapsEqual(got, tt.want) {
				t.Errorf("readDotEnv() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestBuild_AbsentDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	// Set a test environment variable
	t.Setenv("TEST_VAR", "test_value")

	got, err := Build(tmpDir)
	if err != nil {
		t.Fatalf("Build() = %v; want nil", err)
	}

	// Absent .env should still return OS vars
	if val, ok := got["TEST_VAR"]; !ok {
		t.Errorf("Build() missing TEST_VAR; want test_value")
	} else if val != "test_value" {
		t.Errorf("Build()[TEST_VAR] = %q; want %q", val, "test_value")
	}
}

func TestBuild_OSOverlay(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a .env file with a variable
	dotEnvContent := `SHARED_KEY=dotenv_value
DOTENV_ONLY=from_dotenv`
	dotEnvPath := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(dotEnvPath, []byte(dotEnvContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// Set OS environment variable that overlaps
	t.Setenv("SHARED_KEY", "os_value")
	t.Setenv("OS_ONLY", "from_os")

	got, err := Build(tmpDir)
	if err != nil {
		t.Fatalf("Build() = %v; want nil", err)
	}

	// OS should win over .env for the shared key
	if val, ok := got["SHARED_KEY"]; !ok {
		t.Errorf("Build() missing SHARED_KEY")
	} else if val != "os_value" {
		t.Errorf("Build()[SHARED_KEY] = %q; want %q", val, "os_value")
	}

	// .env-only key should survive
	if val, ok := got["DOTENV_ONLY"]; !ok {
		t.Errorf("Build() missing DOTENV_ONLY")
	} else if val != "from_dotenv" {
		t.Errorf("Build()[DOTENV_ONLY] = %q; want %q", val, "from_dotenv")
	}

	// OS-only key should be present
	if val, ok := got["OS_ONLY"]; !ok {
		t.Errorf("Build() missing OS_ONLY")
	} else if val != "from_os" {
		t.Errorf("Build()[OS_ONLY] = %q; want %q", val, "from_os")
	}
}

func TestBuild_MultipleScenarios(t *testing.T) {
	tests := []struct {
		name       string
		dotEnvBody string
		osEnvVars  map[string]string
		check      func(t *testing.T, got map[string]string)
	}{
		{
			name:       "EmptyDotEnv",
			dotEnvBody: "",
			osEnvVars: map[string]string{
				"VAR_A": "value_a",
			},
			check: func(t *testing.T, got map[string]string) {
				if val, ok := got["VAR_A"]; !ok || val != "value_a" {
					t.Errorf("EmptyDotEnv: missing or wrong VAR_A")
				}
			},
		},
		{
			name: "DotEnvOnly",
			dotEnvBody: `KEY1=val1
KEY2=val2`,
			osEnvVars: map[string]string{},
			check: func(t *testing.T, got map[string]string) {
				if val, ok := got["KEY1"]; !ok || val != "val1" {
					t.Errorf("DotEnvOnly: missing or wrong KEY1")
				}
				if val, ok := got["KEY2"]; !ok || val != "val2" {
					t.Errorf("DotEnvOnly: missing or wrong KEY2")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if tt.dotEnvBody != "" {
				dotEnvPath := filepath.Join(tmpDir, ".env")
				if err := os.WriteFile(dotEnvPath, []byte(tt.dotEnvBody), 0o644); err != nil {
					t.Fatalf("write .env: %v", err)
				}
			}

			// Set OS env vars
			for key, val := range tt.osEnvVars {
				t.Setenv(key, val)
			}

			got, err := Build(tmpDir)
			if err != nil {
				t.Fatalf("Build() = %v; want nil", err)
			}

			tt.check(t, got)
		})
	}
}

// mapsEqual reports whether two string maps are equal.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || bv != v {
			return false
		}
	}
	return true
}

// TestBuild_PrecisionValuePreservation verifies that values are not trimmed and
// special characters are preserved exactly as written.
func TestBuild_PrecisionValuePreservation(t *testing.T) {
	tmpDir := t.TempDir()
	dotEnvContent := fmt.Sprintf("VAR_WITH_SPACE= exact value \nVAR_EMPTY=%s", "")
	dotEnvPath := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(dotEnvPath, []byte(dotEnvContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	got, err := readDotEnv(dotEnvPath)
	if err != nil {
		t.Fatalf("readDotEnv() = %v; want nil", err)
	}

	if val, ok := got["VAR_WITH_SPACE"]; !ok {
		t.Error("readDotEnv() missing VAR_WITH_SPACE")
	} else if val != " exact value " {
		t.Errorf("readDotEnv()[VAR_WITH_SPACE] = %q; want %q", val, " exact value ")
	}

	if val, ok := got["VAR_EMPTY"]; !ok {
		t.Error("readDotEnv() missing VAR_EMPTY")
	} else if val != "" {
		t.Errorf("readDotEnv()[VAR_EMPTY] = %q; want %q", val, "")
	}
}
