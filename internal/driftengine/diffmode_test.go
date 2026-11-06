// diffmode_test.go — tier 0 unit tests for driftengine.MigrateConfig and driftengine.FlagDiffMode.
//
// Both rewrite in-memory drift.yaml bytes; no hub, no git spawn.

package driftengine

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMigrateConfig pins that the three retired model keys are renamed keeping their values.
// It also pins that a document already holding a new key keeps it and drops the old one,
// and that a document with no retired key is returned unchanged.
func TestMigrateConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		existing  string
		wantKeys  map[string]any
		wantLines []string
		unchanged bool
	}{
		{
			name:      "the three retired keys are renamed keeping their values",
			existing:  "baseline_model: haiku\nverify_model: opus\nverify_interactive: true\nscope: full\n",
			wantKeys:  map[string]any{"expect_model": "haiku", "judge_model": "opus", "judge_interactive": true, "scope": "full"},
			wantLines: []string{"baseline_model: renamed to expect_model", "verify_model: renamed to judge_model", "verify_interactive: renamed to judge_interactive"},
		},
		{
			name:      "a document already holding the new key keeps its value",
			existing:  "verify_model: opus\njudge_model: sonnet\n",
			wantKeys:  map[string]any{"judge_model": "sonnet"},
			wantLines: []string{"verify_model: removed, judge_model already set"},
		},
		{name: "a document with no retired key is unchanged", existing: "expect_model: haiku\n", unchanged: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := MigrateConfig([]byte(tt.existing))
			if err != nil {
				t.Fatalf("MigrateConfig error = %v", err)
			}
			if tt.unchanged {
				if string(got) != tt.existing || len(lines) != 0 {
					t.Errorf("MigrateConfig() = %q, %v; want the document unchanged with no line", got, lines)
				}
				return
			}
			var decoded map[string]any
			if err := yaml.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("migrated document does not parse: %v", err)
			}
			if !reflect.DeepEqual(decoded, tt.wantKeys) {
				t.Errorf("migrated keys = %v; want %v", decoded, tt.wantKeys)
			}
			if !reflect.DeepEqual(lines, tt.wantLines) {
				t.Errorf("MigrateConfig lines = %v; want %v", lines, tt.wantLines)
			}
		})
	}
}

// TestMigrateConfig_SkipFileArgument pins that each form of an argument handing {skip_file} to a variable leaves both commands whole, and that any other use of the placeholder stays.
func TestMigrateConfig_SkipFileArgument(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"BetweenOtherArguments", "run --rm -e NORCE_BASELINE_SKIP_FILE={skip_file} -v {baseline_dir}:/out img", "run --rm -v {baseline_dir}:/out img"},
		{"AtTheEnd", "run {baseline_dir} -e A={skip_file}", "run {baseline_dir}"},
		{"TwoSideBySide", "run -e A={skip_file} -e B={skip_file} {baseline_dir}", "run {baseline_dir}"},
		{"TheLongFlag", "run --env A={skip_file} {baseline_dir}", "run {baseline_dir}"},
		{"TheLongFlagWithAnEqualsSign", "run --env=A={skip_file} {baseline_dir}", "run {baseline_dir}"},
		{"AQuotedAssignment", `run -e "A={skip_file}" {baseline_dir}`, "run {baseline_dir}"},
		{"AnotherVariableStays", "run -e KEEP=1 -e A={skip_file} {baseline_dir}", "run -e KEEP=1 {baseline_dir}"},
		{"ABackslashContinuedCommandKeepsItsContinuations", "run \\\n  -e A={skip_file} \\\n  -v {baseline_dir}:/out img", "run \\\n  \\\n  -v {baseline_dir}:/out img"},
		{"AnEnclosingSingleQuotedShellStringStaysWhole", "sh -c 'docker run -e A={skip_file}'", "sh -c 'docker run -e A={skip_file}'"},
		{"AnEnclosingDoubleQuotedShellStringStaysWhole", `sh -c "docker run -e A={skip_file}"`, `sh -c "docker run -e A={skip_file}"`},
		{"AnArgumentInsideAQuotedShellStringIsRemoved", "sh -c 'docker run -e A={skip_file} img'", "sh -c 'docker run img'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			step := map[string]any{"command": tt.command, "timeout_min": 1}
			existing, err := yaml.Marshal(map[string]any{"capture": step, "compare": step})
			if err != nil {
				t.Fatalf("yaml.Marshal() = %v; want nil", err)
			}

			got, lines, err := MigrateConfig(existing)
			if err != nil {
				t.Fatalf("MigrateConfig error = %v", err)
			}

			var decoded struct {
				Capture stepFile `yaml:"capture"`
				Compare stepFile `yaml:"compare"`
			}
			if err := yaml.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("migrated document does not parse: %v", err)
			}
			if decoded.Capture.Command != tt.want || decoded.Compare.Command != tt.want {
				t.Errorf("commands = %q and %q; want both %q", decoded.Capture.Command, decoded.Compare.Command, tt.want)
			}
			if tt.want == tt.command {
				if len(lines) != 0 {
					t.Errorf("MigrateConfig lines = %v; want none for a command left whole", lines)
				}
				return
			}
			if len(lines) != 2 || !strings.HasPrefix(lines[0], "capture.command") || !strings.HasPrefix(lines[1], "compare.command") {
				t.Errorf("MigrateConfig lines = %v; want one naming each command", lines)
			}
		})
	}

	for _, command := range []string{"cat {skip_file} > {baseline_dir}/out", "run -e A=x{skip_file} {baseline_dir}", "run {baseline_dir}"} {
		t.Run("LeftAlone_"+command, func(t *testing.T) {
			t.Parallel()
			existing := "capture:\n  command: '" + command + "'\n  timeout_min: 1\n"
			got, lines, err := MigrateConfig([]byte(existing))
			if err != nil || string(got) != existing || len(lines) != 0 {
				t.Errorf("MigrateConfig() = %q, %v, %v; want the document unchanged with no line", got, lines, err)
			}
		})
	}
}

// TestFlagDiffMode pins each flag, a base that needs none, and that the document is never rewritten.
func TestFlagDiffMode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		existing string
		// wantFlags are the texts each flagged line holds, one slice per flagged line.
		wantFlags [][]string
	}{
		{
			name:      "a compare with no diff set",
			existing:  "compare: {command: \"cmp {baseline_dir}\", timeout_min: 5}\n",
			wantFlags: [][]string{{"compare is set", "diff: command", "lym config drift --set diff=command"}},
		},
		{
			name:     "a compare with diff set",
			existing: "diff: lines\ncompare: {command: \"cmp {baseline_dir}\", timeout_min: 5}\n",
		},
		{
			name:      "a skip file in the capture and the compare",
			existing:  "diff: lines\ncapture: {command: \"p {baseline_dir} {skip_file}\", timeout_min: 1}\ncompare: {command: \"c {baseline_dir} {skip_file}\", timeout_min: 1}\n",
			wantFlags: [][]string{{"capture.command", "{skip_file}", "lym config drift --set 'capture="}, {"compare.command", "{skip_file}", "lym config drift --set 'compare="}},
		},
		{
			name:      "scope partial",
			existing:  "diff: lines\nscope: partial\nskip: [slow]\n",
			wantFlags: [][]string{{"scope: partial", "lym config drift --set scope=full", "scope=off", "lym config reconcile"}},
		},
		{name: "a base with none of them", existing: ConfigTemplate()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := FlagDiffMode([]byte(tt.existing), "/root", "/root/anchor")
			if err != nil {
				t.Fatalf("FlagDiffMode error = %v", err)
			}
			if string(got) != tt.existing || len(rewrote) != 0 {
				t.Errorf("FlagDiffMode() rewrote %q with lines %v; want the document untouched", got, rewrote)
			}
			if len(flagged) != len(tt.wantFlags) {
				t.Fatalf("FlagDiffMode() flagged %v; want %d lines", flagged, len(tt.wantFlags))
			}
			for i, texts := range tt.wantFlags {
				for _, want := range texts {
					if !strings.Contains(flagged[i], want) {
						t.Errorf("flagged line %d = %q; want it to hold %q", i, flagged[i], want)
					}
				}
			}
		})
	}

	t.Run("a document that does not parse is refused", func(t *testing.T) {
		t.Parallel()
		if _, _, _, err := FlagDiffMode([]byte("capture: [unclosed\n"), "/root", "/root/anchor"); !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("FlagDiffMode error = %v; want ErrInvalid", err)
		}
	})
}
