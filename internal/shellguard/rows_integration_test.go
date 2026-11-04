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

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
	tiergate.Require(t, 1)
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
		{"raw-go", "steered_package_scoped_build", "go build ./internal/x", false},
		{"raw-go", "steered_package_scoped_vet", "go vet ./internal/x", false},
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

		{"shell-edit", "sed_in_place", "sed -i 's/a/b/' internal/x/f.go", true},
		{"shell-edit", "sed_in_place_with_a_suffix", "sed -i.bak s/a/b/ f", true},
		{"shell-edit", "sed_in_place_in_a_flag_cluster", "sed -Ei 's/a/b/' f", true},
		{"shell-edit", "sed_in_place_after_the_script", `sed "s/a/b/" -i f`, true},
		{"shell-edit", "sed_long_in_place_flag", "sed --in-place s/a/b/ f", true},
		{"shell-edit", "sed_in_place_after_a_separator", "cd x && sed -i s/a/b/ f", true},
		{"shell-edit", "perl_in_place_cluster", "perl -pi -e s/a/b/ f", true},
		{"shell-edit", "perl_in_place_before_the_script", "perl -0777 -i -pe 's/a/b/' f", true},
		{"shell-edit", "redirect_into_a_project_file", "echo x > docs/a.md", true},
		{"shell-edit", "append_redirect_without_a_space", "echo x >>internal/x/f.go", true},
		{"shell-edit", "redirect_on_a_second_line", "cd x\nprintf y > go.mod", true},
		{"shell-edit", "heredoc_after_the_redirect", "cat > internal/x/f.go <<EOF\npackage x\nEOF", true},
		{"shell-edit", "redirect_on_the_heredoc_line", "cat <<'EOF' >> docs/x.md\n## New\nEOF", true},
		{"shell-edit", "tee_into_a_project_file", "cat <<EOF | tee -a docs/a.md\nx\nEOF", true},
		{"shell-edit", "clobber_redirect", "echo x >| f.go", true},
		{"shell-edit", "redirect_into_a_yaml_file", "echo x > f.yaml", true},
		{"shell-edit", "redirect_into_a_yml_file", "echo x > f.yml", true},
		{"shell-edit", "redirect_into_a_toml_file", "echo x > f.toml", true},
		{"shell-edit", "redirect_into_a_shell_file", "echo x > f.sh", true},
		{"shell-edit", "redirect_into_a_powershell_file", "echo x > f.ps1", true},
		{"shell-edit", "redirect_into_a_mod_file", "echo x > go.mod", true},
		{"shell-edit", "redirect_into_a_sum_file", "echo x > go.sum", true},
		{"shell-edit", "redirect_into_a_csharp_file", "echo x > f.cs", true},
		{"shell-edit", "redirect_into_a_csproj_file", "echo x > f.csproj", true},
		{"shell-edit", "redirect_into_a_sln_file", "echo x > f.sln", true},
		{"shell-edit", "redirect_into_a_props_file", "echo x > f.props", true},
		{"shell-edit", "redirect_into_a_targets_file", "echo x > f.targets", true},
		{"shell-edit", "redirect_into_a_quoted_target", `echo x > "f.go"`, false},
		{"shell-edit", "sed_read", "sed -n '1,20p' internal/x/f.go", false},
		{"shell-edit", "sed_to_standard_output", "sed 's/a/b/' f.go", false},
		{"shell-edit", "grep_flag_after_a_pipe_from_sed", "sed -n 1p f | grep -i x", false},
		{"shell-edit", "perl_module_switch", "perl -Mstrict -ne 'print' f.go", false},
		{"shell-edit", "output_captured_into_a_json_file", "lym board list > .scratch/board.json", false},
		{"shell-edit", "redirect_to_dev_null", "go vet ./internal/x > /dev/null 2>&1", false},
		{"shell-edit", "redirect_inside_quotes", `grep -n "x > y.go" f`, false},
		{"shell-edit", "redirect_inside_a_heredoc_body", "cat <<EOF\nx > y.go\nEOF", false},
		{"shell-edit", "sed_in_place_inside_single_quotes", "grep -rn 'sed -i' internal", false},
		{"shell-edit", "tee_into_a_log", "lym gate test ./internal/x | tee .scratch/gate.log", false},
		{"shell-edit", "redirect_into_a_longer_extension", "cp f.go f.go.orig && echo x > f.go.orig", false},

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

		{"headless-claude", "short_flag", "claude -p 'write x'", true},
		{"headless-claude", "print_flag_bundled_after_another_flag", "claude -cp hi", true},
		{"headless-claude", "print_flag_bundled_before_another_flag", "claude -pc hi", true},
		{"headless-claude", "single_dash_word_without_p", "claude -c", false},
		{"headless-claude", "long_flag", "claude --print hi", true},
		{"headless-claude", "long_flag_with_a_value", "claude --print=json hi", true},
		{"headless-claude", "flag_after_other_arguments", "claude --model sonnet -p hi", true},
		{"headless-claude", "path_prefixed", "/usr/local/bin/claude -p hi", true},
		{"headless-claude", "windows_binary", "claude.exe -p hi", true},
		{"headless-claude", "after_a_separator", "cd x && claude -p hi", true},
		{"headless-claude", "interactive_launch", "claude --model sonnet", false},
		{"headless-claude", "claude_as_an_argument", "grep claude -p f", false},
		{"headless-claude", "inside_a_double_quoted_argument", `git commit -m "claude -p is refused"`, false},
		{"headless-claude", "longer_command_word", "claudette -p", false},
		{"headless-claude", "flag_of_a_later_command", "claude --version && ls -p", false},

		{"publish", "git_push", "git push", true},
		{"publish", "git_push_after_dash_c", "git -C x push origin b", true},
		{"publish", "gh_pr_merge", "gh pr merge 3", true},
		{"publish", "gh_pr_close", "gh pr close 3", true},
		{"publish", "gh_pr_create", "gh pr create --fill", true},
		{"publish", "gh_pr_update_branch", "gh pr update-branch 3", true},
		{"publish", "gh_pr_merge_after_a_repo_flag", "gh pr -R o/r merge 3", true},
		{"publish", "gh_pr_merge_after_a_separator", "cd x && gh pr merge 3", true},
		{"publish", "unlisted_gh_command", "gh release create v1", true},
		{"publish", "unlisted_gh_command_sharing_a_prefix", "gh prs", true},
		{"publish", "unlisted_gh_auth_token", "gh auth token", true},
		{"publish", "gh_pr_review_approve", "gh pr review 3 --approve", true},
		{"publish", "gh_pr_review_request_changes", "gh pr review 3 -r -b x", true},
		{"publish", "gh_pr_edit_base", "gh pr edit 3 --base main", true},
		{"publish", "gh_api_post", "gh api -X POST repos/o/r/pulls/3/merge", true},
		{"publish", "gh_api_lowercase_method", "gh api --method=put repos/o/r/pulls/3/merge", true},
		{"publish", "gh_api_input", "gh api repos/o/r/pulls/3 --input body.json", true},
		{"publish", "gh_api_graphql_mutation", "gh api graphql -f query='mutation { mergePullRequest }'", true},
		{"publish", "gh_api_graphql_mutation_on_a_later_query_line", "gh api graphql -f query='query { viewer { login } }\nmutation { mergePullRequest }'", true},
		{"publish", "gh_api_graphql_mutation_in_a_double_quoted_query", `gh api graphql -f query="mutation { mergePullRequest }"`, true},
		{"publish", "quoted_gh_command_word", `gh "pr" merge 3`, true},
		{"publish", "quoted_gh_subcommand_word", `gh pr "merge" 3`, true},
		{"publish", "quoted_unlisted_gh_command", `gh "release" create v1`, true},
		{"publish", "quoted_write_method", `gh api -X "POST" repos/o/r/pulls/3/merge`, true},
		{"publish", "quoted_review_subcommand_with_approval", `gh pr "review" 3 --approve`, true},
		{"publish", "gh_pr_review_request_changes_long_flag", "gh pr review 3 --request-changes", true},
		{"publish", "gh_pr_review_approve_with_a_body_in_a_cluster", "gh pr review 3 -ab x", true},
		{"publish", "gh_pr_review_request_changes_with_a_body_in_a_cluster", "gh pr review 3 -rb x", true},
		{"publish", "gh_pr_edit_base_short_flag", "gh pr edit 3 -B x", true},
		{"publish", "gh_pr_edit_base_with_an_attached_value", "gh pr edit 3 -Bmain", true},
		{"publish", "gh_api_delete_method_with_equals", "gh api --method=DELETE x", true},
		{"publish", "gh_api_method_flag_without_a_space", "gh api -XPOST x", true},
		{"publish", "gh_pr_merge_after_a_repo_flag_before_the_command", "gh --repo o/r pr merge 3", true},
		{"publish", "gh_pr_merge_after_a_repo_flag_with_equals", "gh --repo=o/r pr merge 3", true},
		{"publish", "gh_issue_create", "gh issue create", true},
		{"publish", "gh_issue_list", "gh issue list", true},
		{"publish", "gh_repo_view", "gh repo view", true},
		{"publish", "gh_run_list", "gh run list", true},
		{"publish", "gh_run_watch", "gh run watch 5", true},
		{"publish", "gh_search", "gh search prs --repo o/r x", true},
		{"publish", "gh_status", "gh status", true},
		{"publish", "gh_pr_status", "gh pr status", true},
		{"publish", "git_status", "git status", false},
		{"publish", "gh_as_an_argument", "echo gh", false},
		{"publish", "push_in_a_grep_flag", "git log --grep=push", false},
		{"publish", "longer_word_than_gh", "ghost", false},
		{"publish", "gh_version", "gh --version", false},
		{"publish", "gh_pr_view", "gh pr view 3 --json body,comments", false},
		{"publish", "gh_pr_view_after_a_repo_flag", "gh pr -R o/r view 3", false},
		{"publish", "gh_pr_view_piped", "gh pr view 3 | head", false},
		{"publish", "gh_pr_list", "gh pr list --base main --state merged", false},
		{"publish", "gh_pr_diff", "gh pr diff 3", false},
		{"publish", "gh_pr_checks", "gh pr checks 3", false},
		{"publish", "gh_pr_edit_title_and_body", "gh pr edit 3 --title x --body-file .scratch/body.txt", false},
		{"publish", "gh_pr_comment", `gh pr comment 3 --body "looks good"`, false},
		{"publish", "gh_pr_review_comment", `gh pr review 3 --comment -b "a nit"`, false},
		{"publish", "gh_api_read", "gh api repos/o/r/pulls/3/comments", false},
		{"publish", "gh_api_get_with_fields", "gh api -X GET search/issues -f q=x", false},
		{"publish", "gh_api_graphql_query", "gh api graphql -f query='query { viewer { login } }'", false},
		{"publish", "gh_run_view", "gh run view 5 --log-failed", false},
		{"publish", "quoted_listed_gh_command_word", `gh "pr" view 3`, false},
		{"publish", "quoted_listed_gh_subcommand_word", `gh pr "view" 3`, false},
		{"publish", "quoted_read_method", `gh api -X "GET" search/issues -f q=x`, false},
		{"publish", "gh_pr_review_comment_with_a_short_flag_cluster", `gh pr review 3 -cb "a nit"`, false},
		{"publish", "gh_api_then_mutation_word_on_the_next_command_line", "gh api repos/o/r/pulls/3\ngrep mutation f", false},
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
		{"shell-edit", "x; sed -i s/a/b/ f > g.go"},
		{"headless-claude", "x; claude -p hi"},
		{"publish", "x; gh pr merge 3"},
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
	// Commands no row may take away: the one that reproduces a failing tmux-tier test, and reading a range of a file.
	allowedEverywhere := []struct {
		name    string
		command string
	}{
		{"tmux_tier_reproduce_command", "lym gate test --wait --tier 2 github.com/Knatte18/loomyard/internal/x -- -run '^TestX$'"},
		{"sed_range_read", "sed -n '10,40p' internal/x/f.go"},
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
