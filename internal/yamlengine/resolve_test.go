// resolve_test.go contains table-driven tests for the Resolve function and its supporting expansion
// logic.

package yamlengine

import (
	"strings"
	"testing"
)

func TestResolve_EmptyInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  []byte
	}{
		{"nil", nil},
		{"empty", []byte("")},
		{"whitespace", []byte("   \n\t  ")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := Resolve(tt.src, map[string]string{})
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			// For nil input, result should be nil
			if tt.src == nil && result != nil {
				t.Errorf("Resolve() = %v; want nil", result)
			}
			// For non-nil input, we just check that it returns something
			if tt.src != nil && result == nil {
				t.Errorf("Resolve() = nil; want non-nil")
			}
		})
	}
}

func TestExpandScalar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		scalar  string
		env     map[string]string
		want    string
		wantErr string
	}{
		{name: "required_present", scalar: "${env:FOO}", env: map[string]string{"FOO": "bar"}, want: "bar"},
		{name: "required_present_empty", scalar: "${env:FOO}", env: map[string]string{"FOO": ""}, want: ""},
		{name: "required_absent", scalar: "${env:FOO}", env: map[string]string{}, wantErr: `unset required env var "FOO"`},
		{
			name:   "required_multiple_in_string",
			scalar: "prefix_${env:FOO}_middle_${env:BAR}_suffix",
			env:    map[string]string{"FOO": "value1", "BAR": "value2"},
			want:   "prefix_value1_middle_value2_suffix",
		},
		{name: "optional_present_nonempty", scalar: "${env:FOO:-default}", env: map[string]string{"FOO": "custom"}, want: "custom"},
		{name: "optional_present_empty", scalar: "${env:FOO:-default}", env: map[string]string{"FOO": ""}, want: "default"},
		{name: "optional_absent", scalar: "${env:FOO:-default}", env: map[string]string{}, want: "default"},
		{name: "optional_empty_default", scalar: "${env:FOO:-}", env: map[string]string{}, want: ""},
		{name: "optional_default_with_spaces", scalar: "${env:FOO:-  spaced  default  }", env: map[string]string{}, want: "  spaced  default  "},
		{name: "optional_default_with_quotes", scalar: "${env:FOO:-'quoted'_value}", env: map[string]string{}, want: "'quoted'_value"},
		{
			name:   "optional_interpolation",
			scalar: "path_${env:LYX_PATH:-../default}/subdir",
			env:    map[string]string{"LYX_PATH": "/custom/path"},
			want:   "path_/custom/path/subdir",
		},
		{
			name:   "optional_interpolation_default",
			scalar: "path_${env:LYX_PATH:-../default}/subdir",
			env:    map[string]string{},
			want:   "path_../default/subdir",
		},
		{name: "resolved_value_contains_marker_syntax", scalar: "${env:FOO}", env: map[string]string{"FOO": "${env:BAR}"}, want: "${env:BAR}"},
		{name: "literal_closing_brace", scalar: "value_with_closing_brace_}", env: map[string]string{}, want: "value_with_closing_brace_}"},
		{name: "literal_string", scalar: "just_a_plain_value", env: map[string]string{}, want: "just_a_plain_value"},
		{name: "literal_empty_string", scalar: "", env: map[string]string{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := expandScalar(tt.scalar, tt.env)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expandScalar() got no error; want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("expandScalar() error = %q; want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("expandScalar() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expandScalar() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestResolve_ExpandsMarkersInNestedMappingsAndSequences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		env  map[string]string
		want []string
	}{
		{"depth_2_leaf", "\nparent:\n  child: ${env:VAL}\n", map[string]string{"VAL": "expanded"}, []string{"child: expanded"}},
		{
			"depth_3_leaf",
			"\nlevel1:\n  level2:\n    level3: ${env:DEEP}\n",
			map[string]string{"DEEP": "nested_value"},
			[]string{"level3: nested_value"},
		},
		{
			"sequence_of_scalars",
			"\nitems:\n  - ${env:ITEM1}\n  - ${env:ITEM2}\n",
			map[string]string{"ITEM1": "value1", "ITEM2": "value2"},
			[]string{"- value1", "- value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Resolve([]byte(tt.src), tt.env)
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(got), want) {
					t.Errorf("Resolve() = %q; want it to contain %q", got, want)
				}
			}
		})
	}
}
