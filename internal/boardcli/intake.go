// intake.go holds the `lyx board intake` verb group: list, import and close against the inbox repository.
//
// It does the mechanics of intake and never triages: the caller decides what each issue becomes.
// boardengine owns the store side and selfreportengine the GitHub side; this file only wires the two.
// The board is written before GitHub is touched, and GitHub is touched after the board lock is released.

package boardcli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/spf13/cobra"
)

// intakeImportPayload is the payload of `intake import`.
var intakeImportPayload = payloadKeys{
	required:  []string{"issue"},
	optional:  []string{"title", "brief", "labels"},
	exclusive: [][2]string{{"slug", "into"}},
}

// intakeClosePayload is the payload of `intake close`.
var intakeClosePayload = payloadKeys{required: []string{"issue", "reason"}, optional: []string{"completed"}}

// intakeCommand builds the `intake` group over the board that board returns.
func intakeCommand(board func() *boardengine.Board) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "intake",
		Short: "bring GitHub inbox issues onto the board",
		Long: `intake moves issues from the inbox repository onto the board. It does the mechanics and never
triages: list shows the open issues no entry records yet, import records one as a note or folds it
into an entry and closes it with a pointer, and close closes noise with a stated reason.`,
		RunE: clihelp.GroupRunE,
	}

	listCmd := &cobra.Command{
		Use:         "list",
		Short:       "list open inbox issues no board entry records, to triage them",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `List the open issues of the inbox repository that no board entry records, pull requests excluded.
Takes no payload.

Prints {"issues":[...]}, each with number, title, body, labels, url and created_at.

Example:
  lyx board intake list`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			return intakeList(out, board())
		}),
	}

	importCmd := &cobra.Command{
		Use:         "import {issue, slug|into, title?, brief?, labels?}",
		Short:       "import an inbox issue as a note or fold it into an entry, then close it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Record an open inbox issue on the board, then comment on it and close it as completed.

Fields:
  "issue"  integer — issue number (required)
  "slug"   string  — import as a new note with this slug
  "title"  string  — note title; defaults to the issue title (with slug only)
  "brief"  string  — note brief (with slug only)
  "labels" array   — note labels replacing the carried-over ones (with slug only)
  "into"   string  — fold the issue into this existing entry instead

A new note carries the issue's configured labels and needs exactly one type label. A fold appends the
issue to the entry's body and issues list. An issue an entry already records is a no-op that prints the
recording entries and touches nothing on GitHub. The board is written before GitHub is touched; when
the comment or close then fails, the error carries the written entry and the "close" payload to run.

Examples:
  lyx board intake import '{"issue":12,"slug":"retry-backoff"}'
  lyx board intake import '{"issue":13,"into":"retry-backoff"}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			return intakeImport(out, board(), args[0])
		}),
	}

	closeCmd := &cobra.Command{
		Use:         "close {issue, reason, completed?}",
		Short:       "close an inbox issue that is noise, with a stated reason",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Comment on an open inbox issue with the reason and close it.
It writes nothing to the board and never edits the issue's content. A pull request and an
already closed issue are refused. It also closes an issue left open by a failed import close.

Fields:
  "issue"     integer — issue number (required)
  "reason"    string  — the comment posted before closing (required, non-empty)
  "completed" bool    — close as completed instead of not planned (optional)

Example:
  lyx board intake close '{"issue":14,"reason":"Duplicate of #12."}'`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			return intakeClose(out, args[0])
		}),
	}

	cmd.AddCommand(listCmd, importCmd, closeCmd)
	return cmd
}

// intakeIssueView is the view of an inbox issue that list prints.
type intakeIssueView struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Labels    []string  `json:"labels"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// intakeList prints the open inbox issues that no board entry records.
func intakeList(out io.Writer, b *boardengine.Board) int {
	issues, err := selfreportengine.ListOpenIssues()
	if err != nil {
		return outputError(out, err.Error())
	}
	recorded, err := b.RecordedIssues()
	if err != nil {
		return outputError(out, err.Error())
	}
	views := []intakeIssueView{}
	for _, issue := range issues {
		if _, done := recorded[issue.Number]; done {
			continue
		}
		views = append(views, intakeIssueView{
			Number:    issue.Number,
			Title:     issue.Title,
			Body:      issue.Body,
			Labels:    issue.Labels,
			URL:       issue.URL,
			CreatedAt: issue.CreatedAt,
		})
	}
	return output.Ok(out, map[string]any{"issues": views})
}

// decodeIntakePayload decodes payload into a map, refusing any key keys does not declare.
func decodeIntakePayload(payload string, keys payloadKeys) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return nil, fmt.Errorf("invalid json: %v", err)
	}
	if err := refuseUnknownKey(keys, fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// decodeIntakeField decodes the value of key into target; an absent key leaves target untouched.
func decodeIntakeField(fields map[string]json.RawMessage, key string, target any) error {
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid %q: %v", key, err)
	}
	return nil
}

// decodeIssueNumber reads the required positive "issue" number.
func decodeIssueNumber(fields map[string]json.RawMessage) (int, error) {
	var number int
	if err := decodeIntakeField(fields, "issue", &number); err != nil {
		return 0, err
	}
	if number <= 0 {
		return 0, fmt.Errorf(`missing required field: issue, the number of an inbox issue (a positive integer)`)
	}
	return number, nil
}

// inboxIssue converts a fetched issue to the boardengine view of it.
func inboxIssue(issue selfreportengine.Issue) boardengine.InboxIssue {
	return boardengine.InboxIssue{
		Number:      issue.Number,
		Title:       issue.Title,
		Body:        issue.Body,
		URL:         issue.URL,
		Labels:      issue.Labels,
		Open:        issue.State == "open",
		PullRequest: issue.PullRequest,
	}
}

// intakeImport records the issue named by payload on the board, then comments on it and closes it as completed.
func intakeImport(out io.Writer, b *boardengine.Board, payload string) int {
	fields, err := decodeIntakePayload(payload, intakeImportPayload)
	if err != nil {
		return outputError(out, err.Error())
	}
	number, err := decodeIssueNumber(fields)
	if err != nil {
		return outputError(out, err.Error())
	}
	req := boardengine.ImportRequest{}
	for key, target := range map[string]any{
		"slug": &req.Slug, "title": &req.Title, "brief": &req.Brief, "labels": &req.Labels, "into": &req.Into,
	} {
		if err := decodeIntakeField(fields, key, target); err != nil {
			return outputError(out, err.Error())
		}
	}

	issue, err := selfreportengine.GetIssue(number)
	if err != nil {
		return outputError(out, err.Error())
	}
	req.Issue = inboxIssue(issue)

	result, err := b.ImportIssue(req)
	if err != nil {
		return outputError(out, err.Error())
	}
	if len(result.Recorded) > 0 {
		return output.Ok(out, map[string]any{"recorded": result.Recorded})
	}

	dropped := result.Dropped
	if dropped == nil {
		dropped = []string{}
	}
	comment := fmt.Sprintf("Imported into the board as `%s`.", result.Entry.Slug)
	if err := selfreportengine.CommentAndClose(number, comment, true); err != nil {
		return output.ErrFields(out, err.Error(), map[string]any{
			"entry":   result.Entry,
			"dropped": dropped,
			"close":   map[string]any{"issue": number, "reason": comment, "completed": true},
		})
	}
	return output.Ok(out, map[string]any{"entry": result.Entry, "dropped": dropped})
}

// intakeClose comments on the open issue named by payload with the reason and closes it.
func intakeClose(out io.Writer, payload string) int {
	fields, err := decodeIntakePayload(payload, intakeClosePayload)
	if err != nil {
		return outputError(out, err.Error())
	}
	number, err := decodeIssueNumber(fields)
	if err != nil {
		return outputError(out, err.Error())
	}
	var reason string
	var completed bool
	if err := decodeIntakeField(fields, "reason", &reason); err != nil {
		return outputError(out, err.Error())
	}
	if err := decodeIntakeField(fields, "completed", &completed); err != nil {
		return outputError(out, err.Error())
	}
	if reason == "" {
		return outputError(out, "missing required field: reason, the comment to post before closing: say why the issue is closed")
	}

	issue, err := selfreportengine.GetIssue(number)
	if err != nil {
		return outputError(out, err.Error())
	}
	if issue.PullRequest {
		return outputError(out, fmt.Sprintf("#%d is a pull request, and only an issue can be closed here: pick an issue number", number))
	}
	if issue.State != "open" {
		return outputError(out, fmt.Sprintf("issue #%d is already closed (state %q, reason %q): nothing to close", number, issue.State, issue.StateReason))
	}
	if err := selfreportengine.CommentAndClose(number, reason, completed); err != nil {
		return outputError(out, err.Error())
	}
	return output.Ok(out, map[string]any{"issue": number, "completed": completed})
}
