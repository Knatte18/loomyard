// cli.go defines the cobra command tree for the selfreport module.

// Package selfreportcli provides the cobra command tree for filing LoomYard bugs and enhancements as GitHub issues.
// The tree is reachable as `lym selfreport create`.
// The create subcommand delegates the GitHub API call to `selfreportengine`.
// It takes the issue body as text through --body or from a file through --body-file.
// It refuses --body text that is one line naming an existing file.
// A path meant for --body-file is therefore never filed as the body itself.
// --attach appends a file's contents to the body under the file's base name.
// Evidence from a run directory therefore outlives the worktree that held it.
package selfreportcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/spf13/cobra"
)

// stdinArgument is the --body or --body-file value that reads the body from stdin.
const stdinArgument = "-"

// bodyFlag is the name of the flag that gives the issue body as text.
const bodyFlag = "body"

// bodyFileFlag is the name of the flag that gives the issue body as a file path.
const bodyFileFlag = "body-file"

// attachFlag is the name of the flag that appends a file's contents to the issue body.
const attachFlag = "attach"

// maxIssueBodyRunes is the most characters GitHub accepts in an issue body.
const maxIssueBodyRunes = 65536

// stdin is the source that runCreate reads body content from when --body or --body-file is "-".
// Tests replace it with a strings.Reader so the stdin path runs without blocking on OS input.
var stdin io.Reader = os.Stdin

// Command builds the cobra command tree for the selfreport module.
// The parent command has no shared setup, so all flags and logic live on the create subcommand.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "selfreport",
		Short: "self-report a LoomYard bug or enhancement to lym's own repo on GitHub",
		// ArbitraryArgs lets GroupRunE handle unknown subcommands when this group is the cobra root, as under clihelp.Execute.
		Args: cobra.ArbitraryArgs,
		RunE: clihelp.GroupRunE,
	}

	// The RunE closure passes createCmd to runCreate, which reads flags through it.
	// The variable is declared before its literal so that the closure can refer to it.
	var createCmd *cobra.Command
	createCmd = &cobra.Command{
		Use:         "create <title>",
		Short:       "file a self-report issue on the LoomYard repository via the GitHub API",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `create files a new issue on the Knatte18/loomyard GitHub repository via the GitHub REST API.
A GitHub token must be resolvable before running this command: set GH_TOKEN or
GITHUB_TOKEN, or have the gh CLI installed and authenticated as a fallback token
source -- the gh CLI is not the transport itself and is only ever shelled out to
for a token when both environment variables are unset.

Examples:

  lym selfreport create "Crash on empty board" --body-file report.md
      Create a bug issue whose body is the contents of report.md.

  lym selfreport create "Crash on empty board" -b -
      Create a bug issue and read the issue body from stdin; "-" means read from stdin.

  lym selfreport create "Board hangs" -b "The board hangs on an empty hub."
      Create a bug issue whose body is the given text.
      Text that is one line naming an existing file is refused: pass that file with --body-file.

  lym selfreport create "Idle reading wrong" --body-file report.md --attach startup-capture.txt
      Append the capture to the body, under its base name in a code fence.
      A run directory is deleted with its worktree, so evidence a report rests on goes in the
      issue this way or quoted in the body, never as a path alone.
      A body the attachments push past GitHub's issue body limit is refused: attach an excerpt.

  lym selfreport create "Add dark mode" --label enhancement
      Create an enhancement issue; --label replaces the default "bug" label.

  lym selfreport create "Panic on nil pointer"
      Create a bug issue with no body; the default "bug" label is applied automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			return runCreate(out, args, createCmd)
		}),
	}
	createCmd.Flags().StringP(bodyFlag, "b", "", `issue body text; use "-" to read from stdin`)
	createCmd.Flags().String(bodyFileFlag, "", `file whose contents are the issue body; use "-" to read from stdin`)
	createCmd.MarkFlagsMutuallyExclusive(bodyFlag, bodyFileFlag)
	createCmd.Flags().StringArray(attachFlag, []string{}, `file whose contents are appended to the body (repeatable)`)
	createCmd.Flags().StringArray("label", []string{}, `label to apply (repeatable); defaults to "bug" when omitted`)
	cmd.AddCommand(createCmd)

	return cmd
}

// runCreate runs the create subcommand for title and writes the result to out.
// It resolves the body from --body or --body-file, reading stdin when either is "-".
// It applies the default "bug" label when no --label is given.
// The GitHub call is made by `selfreportengine.CreateIssue`.
func runCreate(out io.Writer, args []string, cmd *cobra.Command) int {
	title := args[0]

	body, err := resolveBody(cmd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	attachments, _ := cmd.Flags().GetStringArray(attachFlag)
	body, err = appendAttachments(body, attachments)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Default to the "bug" label when no --label flag is given.
	// Defect reports against LoomYard are the primary use of this command.
	labels, _ := cmd.Flags().GetStringArray("label")
	if len(labels) == 0 {
		labels = selfreportengine.DefaultLabels()
	}

	url, number, err := selfreportengine.CreateIssue(title, body, labels)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// The success envelope always carries url.
	// It carries number only when CreateIssue returned a non-zero issue number.
	fields := map[string]any{"url": url}
	if number != 0 {
		fields["number"] = number
	}
	return output.Ok(out, fields)
}

// resolveBody returns the issue body that cmd's --body or --body-file flag gives.
// It returns nil when neither flag is set, so CreateIssue omits the "body" field and GitHub applies its own default;
// --body "" gives an empty body rather than none.
// It refuses --body text that is one line naming an existing regular file.
// Filing that text would file the path and lose the report the file holds.
func resolveBody(cmd *cobra.Command) (*string, error) {
	switch {
	case cmd.Flags().Changed(bodyFlag):
		bodyText, _ := cmd.Flags().GetString(bodyFlag)
		if bodyText == stdinArgument {
			return readBodySource(stdinArgument)
		}
		if path, ok := existingFileNamedBy(bodyText); ok {
			return nil, fmt.Errorf(
				"--body text %q names an existing file, so the issue body would be its path; "+
					"pass --body-file %q to file the file's contents", bodyText, path)
		}
		return &bodyText, nil
	case cmd.Flags().Changed(bodyFileFlag):
		bodyFile, _ := cmd.Flags().GetString(bodyFileFlag)
		return readBodySource(bodyFile)
	}
	return nil, nil
}

// readBodySource returns all of stdin when source is "-", and otherwise the contents of the file source names.
func readBodySource(source string) (*string, error) {
	var content []byte
	var err error
	if source == stdinArgument {
		// Read through the stdin variable that tests replace.
		content, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
	} else {
		content, err = os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("failed to read --body-file: %w", err)
		}
	}
	bodyText := string(content)
	return &bodyText, nil
}

// appendAttachments returns body followed by one section per file in paths, in order.
// It returns body unchanged when paths is empty.
// Each section names the file by its base name.
// That keeps a local directory such as a home path out of the public issue.
// It refuses a body the sections push past GitHub's limit, since GitHub would reject the whole issue.
// It refuses a file that is not valid UTF-8, since the JSON the issue travels in would replace its invalid bytes.
func appendAttachments(body *string, paths []string) (*string, error) {
	if len(paths) == 0 {
		return body, nil
	}
	var sections []string
	if body != nil && *body != "" {
		sections = append(sections, strings.TrimRight(*body, "\n"))
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read --%s %q: %w", attachFlag, path, err)
		}
		if !utf8.Valid(content) {
			return nil, fmt.Errorf("--%s %q is not valid UTF-8 text, so the issue would hold altered bytes: "+
				"attach a UTF-8 text excerpt of it", attachFlag, path)
		}
		sections = append(sections, attachmentSection(filepath.Base(path), string(content)))
	}
	text := strings.Join(sections, "\n\n")
	if count := utf8.RuneCountInString(text); count > maxIssueBodyRunes {
		return nil, fmt.Errorf("the issue body with its attachments is %d characters, over GitHub's limit of %d: "+
			"attach an excerpt holding the lines the report rests on", count, maxIssueBodyRunes)
	}
	return &text, nil
}

// attachmentSection renders one attached file as a heading naming it and its contents in a code fence.
// The fence is longer than any backtick run in the contents, so the contents cannot close it early.
// The name is a code span delimited by more backticks than any run in the name, so the name cannot close it early.
func attachmentSection(name, content string) string {
	fence := strings.Repeat("`", max(3, longestBacktickRun(content)+1))
	return fmt.Sprintf("## Attached %s\n\n%s\n%s\n%s", codeSpan(name), fence, strings.TrimRight(content, "\n"), fence)
}

// longestBacktickRun returns the length of the longest run of consecutive backticks in text.
func longestBacktickRun(text string) int {
	longestRun, run := 0, 0
	for _, r := range text {
		if r != '`' {
			run = 0
			continue
		}
		run++
		longestRun = max(longestRun, run)
	}
	return longestRun
}

// codeSpan renders text as a markdown code span whose delimiter is longer than any backtick run in text.
// It pads text with a space on each side when text starts or ends with a backtick, which a delimiter would merge with.
func codeSpan(text string) string {
	delimiter := strings.Repeat("`", longestBacktickRun(text)+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return delimiter + text + delimiter
}

// existingFileNamedBy reports whether bodyText is one line naming an existing regular file.
// It returns that line as the path.
func existingFileNamedBy(bodyText string) (path string, ok bool) {
	path = strings.TrimSpace(bodyText)
	if path == "" || strings.Contains(path, "\n") {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return path, true
}
