// parse_test.go table-drives Parse against the strict grammar: one table of specs that must be
// accepted (with the exact Spec they must produce),
// and one table of specs that must be rejected (with an exact substring every error must contain,
// naming the offending token or character, plus an optional second substring for cases that also
// pin an appended detail such as a migration hint).

package modelspec

import (
	"strings"
	"testing"
)

func TestParse_Accepts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Spec
	}{
		{
			name: "bare alias",
			in:   "sonnet",
			want: Spec{Alias: "sonnet"},
		},
		{
			name: "alias single param",
			in:   "sonnet[effort=high]",
			want: Spec{Alias: "sonnet", Params: map[string]string{"effort": "high"}},
		},
		{
			name: "alias multiple params",
			in:   "sonnet[effort=high,v=4.5]",
			want: Spec{Alias: "sonnet", Params: map[string]string{"effort": "high", "version": "4.5"}},
		},
		{
			name: "escape form no bracket",
			in:   "claude:claude-sonnet-4-5",
			want: Spec{Engine: "claude", Model: "claude-sonnet-4-5"},
		},
		{
			name: "escape form with bracket",
			in:   "claude:claude-sonnet-4-5[effort=high]",
			want: Spec{Engine: "claude", Model: "claude-sonnet-4-5", Params: map[string]string{"effort": "high"}},
		},
		{
			name: "dotted version value",
			in:   "sonnet[v=4.5]",
			want: Spec{Alias: "sonnet", Params: map[string]string{"version": "4.5"}},
		},
		{
			name: "bare effort shorthand",
			in:   "opus[high]",
			want: Spec{Alias: "opus", Params: map[string]string{"effort": "high"}},
		},
		{
			name: "bare effort shorthand combined with v=",
			in:   "opus[high,v=4.8]",
			want: Spec{Alias: "opus", Params: map[string]string{"effort": "high", "version": "4.8"}},
		},
		{
			name: "v= alone",
			in:   "sonnet[v=4.5]",
			want: Spec{Alias: "sonnet", Params: map[string]string{"version": "4.5"}},
		},
		{
			name: "escape form bare effort shorthand",
			in:   "claude:claude-opus-5[high]",
			want: Spec{Engine: "claude", Model: "claude-opus-5", Params: map[string]string{"effort": "high"}},
		},
		{
			name: "bare token literally spelled effort",
			in:   "sonnet[effort]",
			want: Spec{Alias: "sonnet", Params: map[string]string{"effort": "effort"}},
		},
		{
			name: "bare token that looks like a version number",
			in:   "sonnet[4.5]",
			want: Spec{Alias: "sonnet", Params: map[string]string{"effort": "4.5"}},
		},
		{
			name: "v= and bare effort shorthand, order independent",
			in:   "opus[v=4.8,high]",
			want: Spec{Alias: "opus", Params: map[string]string{"effort": "high", "version": "4.8"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q) returned unexpected error: %v", tt.in, err)
			}
			if got.Alias != tt.want.Alias || got.Engine != tt.want.Engine || got.Model != tt.want.Model {
				t.Errorf("Parse(%q) = %+v; want %+v", tt.in, got, tt.want)
			}
			if len(got.Params) != len(tt.want.Params) {
				t.Errorf("Parse(%q).Params = %v; want %v", tt.in, got.Params, tt.want.Params)
			}
			for k, v := range tt.want.Params {
				if got.Params[k] != v {
					t.Errorf("Parse(%q).Params[%q] = %q; want %q", tt.in, k, got.Params[k], v)
				}
			}
			if tt.want.Params == nil && got.Params != nil {
				t.Errorf("Parse(%q).Params = %v; want nil (no bracket in input)", tt.in, got.Params)
			}
		})
	}
}

func TestParse_Rejects(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantSubstr string
		// alsoSubstr, when non-empty, is a second substring the error must
		// also contain — used for cases pinning both the base rejection and
		// an appended migration hint or detail.
		alsoSubstr string
	}{
		{
			name:       "empty input",
			in:         "",
			wantSubstr: "empty spec string",
		},
		{
			name:       "whitespace in middle",
			in:         "sonnet effort",
			wantSubstr: "whitespace character",
		},
		{
			name:       "leading whitespace",
			in:         " sonnet",
			wantSubstr: "whitespace character",
		},
		{
			name:       "uppercase alias",
			in:         "Sonnet",
			wantSubstr: "invalid character",
		},
		{
			name:       "uppercase engine",
			in:         "Claude:sonnet",
			wantSubstr: "invalid character",
		},
		{
			name:       "uppercase param key",
			in:         "sonnet[Effort=high]",
			wantSubstr: "invalid character",
		},
		{
			name:       "escape form model-id bad charset",
			in:         "claude:claude/sonnet",
			wantSubstr: "invalid character",
		},
		{
			name:       "param value bad charset",
			in:         "sonnet[effort=high!]",
			wantSubstr: "invalid character",
		},
		{
			name:       "empty bracket",
			in:         "sonnet[]",
			wantSubstr: "empty bracket",
		},
		{
			name:       "trailing text after bracket",
			in:         "sonnet[effort=high]x",
			wantSubstr: "must end with ']'",
		},
		{
			name:       "unterminated bracket",
			in:         "sonnet[effort=high",
			wantSubstr: "must end with ']'",
		},
		{
			name:       "two colons",
			in:         "claude:sonnet:extra",
			wantSubstr: "exactly one ':'",
		},
		{
			name:       "empty engine",
			in:         ":sonnet",
			wantSubstr: "empty engine",
		},
		{
			name:       "empty model-id",
			in:         "claude:",
			wantSubstr: "empty model-id",
		},
		{
			name:       "duplicate param key",
			in:         "sonnet[effort=high,effort=low]",
			wantSubstr: "duplicate param key",
		},
		{
			name:       "empty param key",
			in:         "sonnet[=high]",
			wantSubstr: "empty param key",
		},
		{
			name:       "empty param value",
			in:         "sonnet[effort=]",
			wantSubstr: "empty value for param key",
		},
		{
			name:       "unknown param key",
			in:         "sonnet[speed=fast]",
			wantSubstr: "unknown param key",
		},
		{
			name:       "unknown escape-form engine",
			in:         "gemini:gemini-pro",
			wantSubstr: "unknown engine",
		},
		{
			name:       "unknown param key version, migration hint",
			in:         "sonnet[version=4.5]",
			wantSubstr: "unknown param key",
			alsoSubstr: `the bracket version param is spelled "v"`,
		},
		{
			name:       "duplicate param key, bare effort vs key=value effort",
			in:         "opus[high,effort=max]",
			wantSubstr: "duplicate param key",
			alsoSubstr: `(segments "high" and "effort=max" both set it)`,
		},
		{
			name:       "duplicate param key, two bare effort tokens",
			in:         "opus[high,max]",
			wantSubstr: "duplicate param key",
		},
		{
			name:       "duplicate param key, two v= segments",
			in:         "opus[v=4.8,v=4.5]",
			wantSubstr: "duplicate param key",
			alsoSubstr: `(segments "v=4.8" and "v=4.5" both set it)`,
		},
		{
			name:       "unknown key check fires before duplicate check",
			in:         "opus[v=4.8,version=4.8]",
			wantSubstr: "unknown param key",
		},
		{
			name:       "empty param, trailing comma",
			in:         "opus[high,]",
			wantSubstr: "empty param in spec",
		},
		{
			name:       "empty param, leading comma",
			in:         "opus[,high]",
			wantSubstr: "empty param in spec",
		},
		{
			name:       "empty param, middle comma run",
			in:         "opus[a,,b]",
			wantSubstr: "empty param in spec",
		},
		{
			name:       "bare token bad charset",
			in:         "opus[high!]",
			wantSubstr: "invalid character",
		},
		{
			name:       "whitespace inside bracket not softened by shorthand",
			in:         "opus[high, v=4.8]",
			wantSubstr: "whitespace character",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.in)
			if err == nil {
				t.Fatalf("Parse(%q) returned nil error; want error containing %q", tt.in, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("Parse(%q) error = %q; want substring %q", tt.in, err.Error(), tt.wantSubstr)
			}
			if tt.alsoSubstr != "" && !strings.Contains(err.Error(), tt.alsoSubstr) {
				t.Errorf("Parse(%q) error = %q; want substring %q", tt.in, err.Error(), tt.alsoSubstr)
			}
			if !strings.HasPrefix(err.Error(), "modelspec: ") {
				t.Errorf("Parse(%q) error = %q; want prefix \"modelspec: \"", tt.in, err.Error())
			}
		})
	}
}
