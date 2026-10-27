//go:build tier1

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

// payloadDenied encodes the PreToolUse payload carrying the Bash command, and the description field when it is non-empty, and reports whether every pattern of the row matches it under a real grep -Eq run through sh.
func payloadDenied(t *testing.T, row Row, command, description string) bool {
	t.Helper()

	toolInput := map[string]any{"command": command}
	if description != "" {
		toolInput["description"] = description
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      toolInput,
	}); err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	for _, pattern := range row.Patterns {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, "sh", "-c", "grep -Eq '"+strings.ReplaceAll(pattern, "'", `'\''`)+"'")
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
	for _, row := range Rows("with a long timeout", []string{"go test", "dotnet test"}) {
		rows[row.Name] = row
	}
	if steer := rows["raw-go"].Steer; !strings.Contains(steer, "--tier") || strings.Contains(steer, "--tags") {
		t.Errorf("raw-go steer = %q; want it to name --tier and not --tags", steer)
	}
	for _, row := range Rows("with a long timeout", nil) {
		if row.Name == "refuse-raw" {
			t.Errorf("Rows() with an empty refuse list renders a %q row; want none", row.Name)
		}
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
		{"raw-go", "tier3_in_a_tag_list", "go test -tags=tier1,tier3 ./internal/x", true},
		{"raw-go", "long_tags_flag", "go test --tags tier2 ./internal/x", true},
		{"raw-go", "tier2_tag", "go test -tags tier2 ./internal/x", true},
		{"raw-go", "tier3_tag_with_equals", "go test -tags=tier3 ./internal/x", true},
		{"raw-go", "tier1_tag_package_scoped", "go test -tags tier1 ./internal/x", false},
		{"raw-go", "names_the_slot_variable", "LYM_GATE_SLOT=/x lym gate test ./internal/x", true},
		{"raw-go", "names_the_strand_variable", "echo $LYM_STRAND_NAME", true},
		{"raw-go", "names_the_prebuilt_binary_variable", "LYM_PREBUILT_LYM=/x/lym go test ./internal/x", true},
		{"raw-go", "package_scoped_test", "go test ./internal/x", false},
		{"raw-go", "tier1_with_a_filter", "go test -tags tier1 ./internal/x -run Foo", false},
		{"raw-go", "gate_route_module_wide", "lym gate test ./...", false},
		{"raw-go", "gate_route_tier2", "lym gate test --tags tier2 ./internal/x", false},
		{"raw-go", "build_of_one_main_package", "go build -o bin/lym ./cmd/lym", false},
		{"raw-go", "go_as_an_argument", "grep -r go ./...", false},
		{"raw-go", "pattern_after_a_separator", "go test ./internal/x && ls ./...", false},
		{"raw-go", "pattern_on_the_next_line", "go test ./internal/x\nls ./...", false},
		{"raw-go", "subshell", "(cd x && go test ./...)", true},
		{"raw-go", "after_a_closed_single_quote", "echo 'a' && go test ./...", true},
		{"raw-go", "after_a_closed_double_quote", `echo "a"; go vet ./...`, true},
		{"raw-go", "after_a_heredoc_operator_on_its_line", "cat <<EOF | go test ./...\nx\nEOF", true},
		{"raw-go", "double_quoted_expansion_of_the_strand_variable", `echo "$LYM_STRAND_NAME"`, true},
		{"raw-go", "braced_expansion_of_the_slot_variable", `echo "a ${LYM_GATE_SLOT}"`, true},
		{"raw-go", "grep_for_the_tier2_run_in_single_quotes", "grep -rn 'go test -tags tier2' internal", false},
		{"raw-go", "grep_alternation_in_double_quotes", `grep -rnE "(go test ./...|go vet all)" internal`, false},
		{"raw-go", "commit_message_naming_a_module_wide_run", `git commit -m "fix; go test ./... passes"`, false},
		{"raw-go", "heredoc_body_drafting_test_runs", "cat > .scratch/t.go <<EOF\n  go test -tags tier2 ./x\n(go test ./...)\nEOF", false},
		{"raw-go", "grep_for_the_slot_variable_in_double_quotes", `grep -rn "LYM_GATE_SLOT" internal`, false},
		{"raw-go", "single_quoted_strand_variable", "echo '$LYM_STRAND_NAME'", false},
		{"raw-go", "heredoc_body_naming_the_prebuilt_binary_variable", "cat <<EOF\nLYM_PREBUILT_LYM=/x\nEOF", false},

		{"refuse-raw", "go_test_package", "go test ./internal/x", true},
		{"refuse-raw", "go_test_after_a_separator", "cd x && go test ./...", true},
		{"refuse-raw", "go_test_after_env", "env go test ./x", true},
		{"refuse-raw", "second_listed_prefix", "dotnet test X.sln", true},
		{"refuse-raw", "gate_route", "lym gate test --tier 0", false},
		{"refuse-raw", "prefix_in_single_quotes", "grep -rn 'go test' internal", false},
		{"refuse-raw", "flag_between_the_words", "go -C dir test ./x", false},
		{"refuse-raw", "assignment_prefix", "GOFLAGS=x go test ./x", false},
		{"refuse-raw", "longer_word", "go testing", false},

		{"python", "python3_with_flag", "python3 -c 'print(1)'", true},
		{"python", "after_and_separator", "cd x && python script.py", true},
		{"python", "second_line_starts_python3", "echo hi\npython3 x.py", true},
		{"python", "path_prefixed_with_heredoc", "/usr/bin/python3 - <<EOF\nprint(1)\nEOF", true},
		{"python", "after_xargs", "ls | xargs python3", true},
		{"python", "python_as_argument", "grep python file", false},
		{"python", "python_in_directory_name", "ls python-tools/", false},
		{"python", "behind_bash_dash_c", "bash -c 'python3 x'", false},
		{"python", "through_launcher", "uv run x", false},
		{"python", "separator_inside_a_double_quoted_argument", `grep -n "x; python3" f`, false},
		{"python", "after_a_backtick", "echo `python3 x`", true},
		{"python", "after_a_command_substitution", "echo $(python3 x)", true},
		{"python", "after_a_subshell_paren", "(python3 x)", true},

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
		{"computed-rm", "rm_inside_a_double_quoted_argument", `grep -n "x; rm $d" f`, false},

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
		{"machine-sleep", "inside_a_heredoc_body", "cat > .scratch/a <<EOF\nsystemctl hibernate\nEOF", false},
	}
	// Each description names a command the row would deny as a command, in the payload's description field, beside a command that is harmless.
	descriptionTests := []struct {
		row         string
		description string
	}{
		{"raw-go", "go test ./... and LYM_GATE_SLOT"},
		{"python", "x; python3 x.py"},
		{"computed-rm", "x; rm $d"},
		{"machine-sleep", "x; systemctl hibernate"},
	}
	for _, tt := range tests {
		t.Run(tt.row+"_"+tt.name, func(t *testing.T) {
			t.Parallel()

			row, ok := rows[tt.row]
			if !ok {
				t.Fatalf("Rows() has no row named %q", tt.row)
			}
			if got := payloadDenied(t, row, tt.command, ""); got != tt.denied {
				t.Errorf("row %q denies %q = %v; want %v", tt.row, strings.TrimSpace(tt.command), got, tt.denied)
			}
		})
	}
	for _, tt := range descriptionTests {
		t.Run(tt.row+"_command_word_in_the_description_field", func(t *testing.T) {
			t.Parallel()

			row, ok := rows[tt.row]
			if !ok {
				t.Fatalf("Rows() has no row named %q", tt.row)
			}
			if payloadDenied(t, row, "ls", tt.description) {
				t.Errorf("row %q denies description %q beside command ls; want allowed", tt.row, tt.description)
			}
		})
	}
	// Commands no row may take away: the one that reproduces a failing tmux-tier test, and scripted edits of a project file.
	allowedEverywhere := []struct {
		name    string
		command string
	}{
		{"tmux_tier_reproduce_command", "lym gate test --wait --tier 2 github.com/Knatte18/loomyard/internal/x -- -run '^TestX$'"},
		{"sed_in_place_edit", "sed -i 's/a/b/' internal/x/f.go"},
		{"perl_in_place_edit", "perl -pi -e s/a/b/ f"},
		{"redirect_into_a_project_file", "echo x > docs/a.md"},
		{"heredoc_appended_to_a_project_file", "cat >> docs/x.md <<EOF\n## New\nEOF"},
	}
	for name, row := range rows {
		for _, tt := range allowedEverywhere {
			t.Run(name+"_"+tt.name, func(t *testing.T) {
				t.Parallel()

				if payloadDenied(t, row, tt.command, "") {
					t.Errorf("row %q denies %q; want allowed", name, tt.command)
				}
			})
		}
	}
}
