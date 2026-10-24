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
		{"raw-go", "names_the_slot_variable", "LYX_GATE_SLOT=/x lyx gate test ./internal/x", true},
		{"raw-go", "names_the_strand_variable", "echo $LYX_STRAND_NAME", true},
		{"raw-go", "names_the_prebuilt_binary_variable", "LYX_PREBUILT_LYX=/x/lyx go test ./internal/x", true},
		{"raw-go", "package_scoped_test", "go test ./internal/x", false},
		{"raw-go", "tier1_with_a_filter", "go test -tags tier1 ./internal/x -run Foo", false},
		{"raw-go", "gate_route_module_wide", "lyx gate test ./...", false},
		{"raw-go", "gate_route_tier2", "lyx gate test --tags tier2 ./internal/x", false},
		{"raw-go", "build_of_one_main_package", "go build -o bin/lyx ./cmd/lyx", false},
		{"raw-go", "go_as_an_argument", "grep -r go ./...", false},
		{"raw-go", "pattern_after_a_separator", "go test ./internal/x && ls ./...", false},
		{"raw-go", "pattern_on_the_next_line", "go test ./internal/x\nls ./...", false},
		{"raw-go", "subshell", "(cd x && go test ./...)", true},
		{"raw-go", "after_a_closed_single_quote", "echo 'a' && go test ./...", true},
		{"raw-go", "after_a_closed_double_quote", `echo "a"; go vet ./...`, true},
		{"raw-go", "after_a_heredoc_operator_on_its_line", "cat <<EOF | go test ./...\nx\nEOF", true},
		{"raw-go", "double_quoted_expansion_of_the_strand_variable", `echo "$LYX_STRAND_NAME"`, true},
		{"raw-go", "braced_expansion_of_the_slot_variable", `echo "a ${LYX_GATE_SLOT}"`, true},
		{"raw-go", "grep_for_the_tier2_run_in_single_quotes", "grep -rn 'go test -tags tier2' internal", false},
		{"raw-go", "grep_alternation_in_double_quotes", `grep -rnE "(go test ./...|go vet all)" internal`, false},
		{"raw-go", "commit_message_naming_a_module_wide_run", `git commit -m "fix; go test ./... passes"`, false},
		{"raw-go", "heredoc_body_drafting_test_runs", "cat > .scratch/t.go <<EOF\n  go test -tags tier2 ./x\n(go test ./...)\nEOF", false},
		{"raw-go", "grep_for_the_slot_variable_in_double_quotes", `grep -rn "LYX_GATE_SLOT" internal`, false},
		{"raw-go", "single_quoted_strand_variable", "echo '$LYX_STRAND_NAME'", false},
		{"raw-go", "heredoc_body_naming_the_prebuilt_binary_variable", "cat <<EOF\nLYX_PREBUILT_LYX=/x\nEOF", false},

		{"refuse-raw", "go_test_package", "go test ./internal/x", true},
		{"refuse-raw", "go_test_after_a_separator", "cd x && go test ./...", true},
		{"refuse-raw", "go_test_after_env", "env go test ./x", true},
		{"refuse-raw", "second_listed_prefix", "dotnet test X.sln", true},
		{"refuse-raw", "gate_route", "lyx gate test --tier 0", false},
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
		{"in-place-edit", "grep_alternation_in_single_quotes", "grep -rnE '(sed -i|perl -pi)' .", false},

		{"redirect-write", "heredoc_appended_to_a_doc", "cat >> docs/x.md <<EOF\n## New\nEOF", true},
		{"redirect-write", "redirect_after_the_heredoc_operator", "cat <<EOF > docs/x.md\nbody\nEOF", true},
		{"redirect-write", "echo_to_a_file", "echo x > file", true},
		{"redirect-write", "append_to_an_absolute_path", "printf x >> /home/u/repo/file", true},
		{"redirect-write", "both_streams", "make &> out.log", true},
		{"redirect-write", "clobber", "echo x >| f", true},
		{"redirect-write", "dot_slash_path", "echo x > ./f", true},
		{"redirect-write", "dotfile", "echo x > .gitignore", true},
		{"redirect-write", "parent_directory", "echo x > ../f", true},
		{"redirect-write", "scratch_lookalike", "echo x > .scratchpad/f", true},
		{"redirect-write", "tmp_lookalike", "echo x > /tmpfile", true},
		{"redirect-write", "double_quoted_target", `echo x > "docs/a b.md"`, true},
		{"redirect-write", "single_quoted_target", "echo x > 'docs/f'", true},
		{"redirect-write", "dev_lookalike", "echo x > /devx", true},
		{"redirect-write", "after_and_separator", "cd x && echo y > f", true},
		{"redirect-write", "after_a_quoted_greater_than", `echo "a>b" > f`, true},
		{"redirect-write", "scratch_relative", "echo x > .scratch/a", false},
		{"redirect-write", "scratch_dot_slash", "echo x >> ./.scratch/a", false},
		{"redirect-write", "dev_null_and_duplication", "go build ./cmd/x > /dev/null 2>&1", false},
		{"redirect-write", "tmp", "ls > /tmp/x", false},
		{"redirect-write", "duplication_to_stderr", "echo x >&2", false},
		{"redirect-write", "greater_than_in_double_quotes", `grep "a > b" f`, false},
		{"redirect-write", "greater_than_in_single_quotes", "git log --format='%h -> %s'", false},
		{"redirect-write", "greater_than_in_a_heredoc_body", "cat <<EOF | lyx x\na > b\nEOF", false},
		{"redirect-write", "heredoc_into_scratch", "cat > .scratch/a.md <<EOF\n# x > y\nEOF", false},
		{"redirect-write", "variable_target", `echo x > "$f"`, false},
		{"redirect-write", "input_redirect", "wc -l < file", false},
		{"redirect-write", "process_substitution", "diff <(ls a) <(ls b)", false},

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
		{"raw-go", "go test ./... and LYX_GATE_SLOT"},
		{"python", "x; python3 x.py"},
		{"in-place-edit", "x; sed -i s/a/b/ f"},
		{"redirect-write", "x; echo y > f"},
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
	// The command that reproduces a failing tmux-tier test; no row may take it away.
	const tmuxTierReproduce = "lyx gate test --wait --tier 2 github.com/Knatte18/loomyard/internal/x -- -run '^TestX$'"
	for name, row := range rows {
		t.Run(name+"_tmux_tier_reproduce_command", func(t *testing.T) {
			t.Parallel()

			if payloadDenied(t, row, tmuxTierReproduce, "") {
				t.Errorf("row %q denies %q; want allowed", name, tmuxTierReproduce)
			}
		})
	}
}
