// envsource_test.go covers .env parsing, OS-environment overlay precedence, and the DotEnv path
// constructor that pins envsource as the single declarer of the ".env" filename token.

package envsource

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// TestDotEnv verifies that DotEnv joins baseDir with the ".env" filename — moved here from hubgeometry's own unit test now that envsource is the single declarer of the ".env" token.
//
//testtiming:keep pins the ".env" filename token itself, which the covering Build rows only read back through DotEnv
func TestDotEnv(t *testing.T) {
	t.Parallel()

	baseDir := "/home/user/project"
	got := DotEnv(baseDir)
	want := filepath.Join(baseDir, ".env")

	if got != want {
		t.Errorf("DotEnv(%q) = %q; want %q", baseDir, got, want)
	}
}

func TestBuild_DotEnvParsing(t *testing.T) {
	t.Parallel()
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
		{
			name:    "TrailingSpaceAndEmptyValueArePreserved",
			content: "VAR_WITH_SPACE= exact value \nVAR_EMPTY=",
			want: map[string]string{
				"VAR_WITH_SPACE": " exact value ",
				"VAR_EMPTY":      "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			dotEnvPath := filepath.Join(tmpDir, ".env")
			if err := os.WriteFile(dotEnvPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			got, err := readDotEnv(dotEnvPath)
			if err != nil {
				t.Fatalf("readDotEnv() = %v; want nil", err)
			}

			if !maps.Equal(got, tt.want) {
				t.Errorf("readDotEnv() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestBuild pins the merged environment: .env entries and OS variables both appear, the OS value wins a shared key, and an absent or empty .env leaves just the OS variables.
// It serializes its rows because they set process environment variables.
func TestBuild(t *testing.T) {
	tests := []struct {
		name       string
		dotEnvBody string
		osEnvVars  map[string]string
		want       map[string]string
	}{
		{
			name:      "AbsentDotEnv",
			osEnvVars: map[string]string{"TEST_VAR": "test_value"},
			want:      map[string]string{"TEST_VAR": "test_value"},
		},
		{
			name:      "EmptyDotEnv",
			osEnvVars: map[string]string{"VAR_A": "value_a"},
			want:      map[string]string{"VAR_A": "value_a"},
		},
		{
			name:       "DotEnvOnly",
			dotEnvBody: "KEY1=val1\nKEY2=val2",
			want:       map[string]string{"KEY1": "val1", "KEY2": "val2"},
		},
		{
			name:       "OSWinsSharedKeyAndBothSidesKeepTheirOwn",
			dotEnvBody: "SHARED_KEY=dotenv_value\nDOTENV_ONLY=from_dotenv",
			osEnvVars:  map[string]string{"SHARED_KEY": "os_value", "OS_ONLY": "from_os"},
			want:       map[string]string{"SHARED_KEY": "os_value", "DOTENV_ONLY": "from_dotenv", "OS_ONLY": "from_os"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if tt.dotEnvBody != "" {
				if err := os.WriteFile(DotEnv(tmpDir), []byte(tt.dotEnvBody), 0o644); err != nil {
					t.Fatalf("write .env: %v", err)
				}
			}
			for key, val := range tt.osEnvVars {
				t.Setenv(key, val)
			}

			got, err := Build(tmpDir)
			if err != nil {
				t.Fatalf("Build() = %v; want nil", err)
			}

			for key, want := range tt.want {
				if val, ok := got[key]; !ok || val != want {
					t.Errorf("Build()[%s] = %q (present %v); want %q", key, val, ok, want)
				}
			}
		})
	}
}
