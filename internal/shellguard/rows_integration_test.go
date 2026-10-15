//go:build integration

// rows_integration_test.go runs each row's patterns under `sh` with grep against PreToolUse payloads, pinning which Bash commands each guard denies and which it lets through.

package shellguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// payloadDenied encodes the PreToolUse payload carrying the Bash command and reports whether every pattern of the row matches it under a real grep -Eq run through sh.
func payloadDenied(t *testing.T, row Row, command string) bool {
	t.Helper()

	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": command},
	}); err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	for _, pattern := range row.Patterns {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, "sh", "-c", "grep -Eq '"+pattern+"'")
		cmd.Stdin = bytes.NewReader(payload.Bytes())
		err := cmd.Run()
		cancel()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
			return false
		default:
			t.Fatalf("grep for row %q exited with %v (ctx: %v)", row.Name, err, ctx.Err())
		}
	}
	return true
}

// TestRows_PatternsDenyAndAllow feeds each row's patterns a PreToolUse payload per Bash command and asserts the row denies it or lets it through.
func TestRows_PatternsDenyAndAllow(t *testing.T) {
	t.Parallel()

	rows := map[string]Row{}
	for _, row := range Rows("with a long timeout") {
		rows[row.Name] = row
	}
	tests := []struct {
		row     string
		name    string
		command string
		denied  bool
	}{
		{"raw-go", "test_module_wide", "go test ./...", true},
		{"raw-go", "build_module_wide", "go build ./...", true},
		{"raw-go", "vet_module_wide", "go vet ./...", true},
		{"raw-go", "subtree_pattern", "go test ./internal/x/...", true},
		{"raw-go", "all_pattern", "cd x && go test all", true},
		{"raw-go", "module_wide_after_a_flag", "go test -count=1 ./...", true},
		{"raw-go", "module_wide_after_dash_c", "go test -C backend ./...", true},
		{"raw-go", "path_prefixed_go", "/usr/local/go/bin/go test ./...", true},
		{"raw-go", "second_line", "echo hi\ngo vet ./...", true},
		{"raw-go", "tmux_tier_tags", "go test -tags tmux ./internal/x", true},
		{"raw-go", "llm_tier_in_a_tag_list", "go test -tags=integration,llm ./internal/x", true},
		{"raw-go", "long_tags_flag", "go test --tags tmux ./internal/x", true},
		{"raw-go", "names_the_slot_variable", "LYX_GATE_SLOT=/x lyx gate test ./internal/x", true},
		{"raw-go", "names_the_strand_variable", "echo $LYX_STRAND_NAME", true},
		{"raw-go", "names_the_prebuilt_binary_variable", "LYX_PREBUILT_LYX=/x/lyx go test ./internal/x", true},
		{"raw-go", "package_scoped_test", "go test ./internal/x", false},
		{"raw-go", "integration_tier_with_a_filter", "go test -tags integration ./internal/x -run Foo", false},
		{"raw-go", "gate_route_module_wide", "lyx gate test ./...", false},
		{"raw-go", "gate_route_tmux_tier", "lyx gate test --tags tmux ./internal/x", false},
		{"raw-go", "build_of_one_main_package", "go build -o bin/lyx ./cmd/lyx", false},
		{"raw-go", "go_as_an_argument", "grep -r go ./...", false},
		{"raw-go", "pattern_after_a_separator", "go test ./internal/x && ls ./...", false},
		{"raw-go", "pattern_on_the_next_line", "go test ./internal/x\nls ./...", false},

		{"python", "python3_with_flag", "python3 -c 'print(1)'", true},
		{"python", "after_and_separator", "cd x && python script.py", true},
		{"python", "second_line_starts_python3", "echo hi\npython3 x.py", true},
		{"python", "path_prefixed_with_heredoc", "/usr/bin/python3 - <<EOF\nprint(1)\nEOF", true},
		{"python", "after_xargs", "ls | xargs python3", true},
		{"python", "python_as_argument", "grep python file", false},
		{"python", "python_in_directory_name", "ls python-tools/", false},
		{"python", "behind_bash_dash_c", "bash -c 'python3 x'", false},
		{"python", "through_launcher", "uv run x", false},

		{"in-place-edit", "sed_dash_i", "sed -i 's/a/b/' f", true},
		{"in-place-edit", "sed_dash_i_suffix", "sed -i.bak s/a/b/ f", true},
		{"in-place-edit", "sed_cluster_ni", "sed -ni 3p f", true},
		{"in-place-edit", "sed_long_in_place", "sed --in-place s/a/b/ f", true},
		{"in-place-edit", "sed_long_in_place_suffix", "sed --in-place=.bak s/a/b/ f", true},
		{"in-place-edit", "sed_long_abbreviation", "sed --in s/a/b/ f", true},
		{"in-place-edit", "sed_long_abbreviation_suffix", "sed --in-pl=.bak s/a/b/ f", true},
		{"in-place-edit", "sed_flag_after_script", "sed -n 3p f -i", true},
		{"in-place-edit", "perl_cluster_pi", "perl -pi -e s/a/b/ f", true},
		{"in-place-edit", "path_prefixed_sed", "/usr/bin/sed -i s/a/b/ f", true},
		{"in-place-edit", "after_and_separator", "cd x && sed -i s/a/b/ f", true},
		{"in-place-edit", "after_semicolon", "true; sed -i s/a/b/ f", true},
		{"in-place-edit", "after_pipe", "cat f | sed -i s/a/b/ f", true},
		{"in-place-edit", "sed_print_range", "sed -n 1,5p f", false},
		{"in-place-edit", "sed_substitute_to_stdout", "sed s/a/b/ f", false},
		{"in-place-edit", "sed_long_quiet", "sed --quiet 3p f", false},
		{"in-place-edit", "sed_long_silent", "sed --silent 3p f", false},
		{"in-place-edit", "sed_read_then_grep_dash_i_on_next_line", "sed -n 3p f\ngrep -i x f", false},
		{"in-place-edit", "grep_dash_i", "grep -i x f", false},
		{"in-place-edit", "git_diff", "git diff", false},

		{"computed-rm", "command_substitution", `rm -rf "$(cat x)"`, true},
		{"computed-rm", "variable_path", "rm $DIR/f", true},
		{"computed-rm", "positional_parameter", "rm $1", true},
		{"computed-rm", "braced_variable", "rm ${D}/f", true},
		{"computed-rm", "backtick_substitution", "rm `cat list`", true},
		{"computed-rm", "rm_after_separator", "cd x && rm $DIR/f", true},
		{"computed-rm", "rm_behind_xargs", "ls | xargs rm $F", true},
		{"computed-rm", "literal_path", "rm -f .scratch/a", false},
		{"computed-rm", "two_literal_paths", "rm a b", false},
		{"computed-rm", "process_id_parameter", "rm -f .scratch/a-$$", false},
		{"computed-rm", "exit_status_parameter", "rm -f .scratch/a && echo $?", false},

		{"machine-sleep", "sleep", "systemctl sleep", true},
		{"machine-sleep", "hibernate", "systemctl hibernate", true},
		{"machine-sleep", "suspend", "systemctl suspend", true},
		{"machine-sleep", "hybrid_sleep", "systemctl hybrid-sleep", true},
		{"machine-sleep", "suspend_then_hibernate", "systemctl suspend-then-hibernate", true},
		{"machine-sleep", "flag_before_the_verb", "systemctl --no-wall hibernate", true},
		{"machine-sleep", "path_prefixed_systemctl", "/usr/bin/systemctl hibernate", true},
		{"machine-sleep", "after_sudo", "sudo systemctl suspend", true},
		{"machine-sleep", "after_and_separator", "cd x && systemctl hibernate", true},
		{"machine-sleep", "status", "systemctl status", false},
		{"machine-sleep", "status_of_the_hibernate_target", "systemctl status hibernate.target", false},
		{"machine-sleep", "restart_of_another_unit", "systemctl restart sshd", false},
	}
	for _, tt := range tests {
		t.Run(tt.row+"_"+tt.name, func(t *testing.T) {
			t.Parallel()

			row, ok := rows[tt.row]
			if !ok {
				t.Fatalf("Rows() has no row named %q", tt.row)
			}
			if got := payloadDenied(t, row, tt.command); got != tt.denied {
				t.Errorf("row %q denies %q = %v; want %v", tt.row, strings.TrimSpace(tt.command), got, tt.denied)
			}
		})
	}
}
