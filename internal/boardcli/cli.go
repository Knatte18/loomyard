// cli.go exposes the cobra command tree for the board module.
//
// Command() returns the root "board" command over one store, board.json, whose entries carry a kind and labels.
// The verbs upsert, upsert-batch, set-status, remove, get, list, list-full, merge and set-deps come from storeVerbs.
// promote, prune, find, labels and retire-legacy, plus the rerender and sync maintenance verbs, are built in Command itself.
// The intake group (list, import, close) comes from intakeCommand in intake.go.
// list and find take --text to print the compact listing from text.go instead of JSON.
// Configuration resolution happens once in a PersistentPreRunE: the config file (readme,
// design_prefix) is loaded from _lyx/config/board.yaml, and the board data dir is resolved as
// fabricengine.BoardDir(layout.HubPath) via lyxcwd.Resolve.
// The hidden --board-path persistent flag overrides the data dir for the detached sync child
// process launched by spawn.go, bypassing both config and path resolution.

package boardcli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the board module.
func Command() *cobra.Command {
	// b is populated by PersistentPreRunE and closed over by each subcommand RunE.
	var b *boardengine.Board
	board := func() *boardengine.Board { return b }
	// config is the Config PersistentPreRunE loaded;
	// the labels verb reads its Types and Labels.
	var config boardengine.Config

	cmd := &cobra.Command{
		Use:   "board",
		Short: "task-tracker board",
		Long: `board manages the task-tracker board for the current lyx worktree.

The board is one store: every entry is a task or a note (its kind) and carries labels. Only a task
can be claimed and depend on other tasks; a note is an idea or observation. A task carries at least
one type label, a note exactly one, and every label must be configured in board.yaml. The README
renders tasks split into dependency layers and notes grouped by type. Agents read and write the
board through "lyx board", never through the JSON files under _board.

The config file (_lyx/config/board.yaml) controls non-geometry settings: readme
and design_prefix filenames and the types and labels lists. The board data dir (<hub>/_board) is
derived from the worktree layout via lyxcwd and is not config- or
env-overridable. The hidden --board-path flag overrides the data dir for the
detached sync child process. Running "lyx board" with no subcommand lists
available subcommands without requiring a git repo.`,
	}

	boardPathFlag := cmd.PersistentFlags().String("board-path", "", "internal: injected absolute board dir for the detached sync child")
	if err := cmd.PersistentFlags().MarkHidden("board-path"); err != nil {
		panic(fmt.Sprintf("board: MarkHidden board-path: %v", err))
	}

	cmd.RunE = clihelp.GroupRunE

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "board" || cmd.Name() == "intake" {
			return nil
		}

		ctx := cmd.Context()

		var cfg boardengine.Config

		// If --board-path is set, use it directly (internal use for detached sync child).
		if *boardPathFlag != "" {
			cfg = boardengine.Config{Path: *boardPathFlag}
		} else {
			// Resolve configuration from the current working directory.
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			cfg, err = boardengine.LoadConfig(cwd, "board")
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			layout, rerr := lyxcwd.Resolve(cwd)
			if rerr != nil {
				output.Err(cmd.OutOrStdout(), rerr.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			cfg.Path = fabricengine.BoardDir(layout.HubPath)
		}

		cfg = boardengine.ApplySkipEnv(cfg)
		config = cfg
		b = boardengine.New(cfg)
		return nil
	}

	promoteCmd := &cobra.Command{
		Use:   "promote [json-payload]",
		Short: "Turn a note into a task",
		Long: `Turn a note into a task. A task is returned unchanged, and nothing is written. Unknown keys
are rejected. Demotion goes through "lyx board upsert" with "kind":"note".

Fields (one of):
  "slug" string  — entry slug
  "id"   integer — entry id

Example:
  lyx board promote '{"slug":"my-note"}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			lookup, _, err := resolveLookup([]byte(args[0]))
			if err != nil {
				return outputError(out, err.Error())
			}
			task, err := b.Promote(lookup)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithTask(out, task)
		}),
	}

	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove every done task",
		Long: `Remove every task whose status is done, strip the removed slugs from the remaining
tasks' depends_on, and print the removed slugs. Takes no payload.

Example:
  lyx board prune`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			removed, err := b.Prune()
			if err != nil {
				return outputError(out, err.Error())
			}
			if removed == nil {
				removed = []string{}
			}
			return output.Ok(out, map[string]any{"removed": removed})
		}),
	}

	var findText bool
	var findLabels []string
	findCmd := &cobra.Command{
		Use:   "find <text>...",
		Short: "Find tasks whose slug, title, brief or body contains the text",
		Long: `Find tasks whose slug, title, brief or body contains the text, done tasks included.
The arguments are joined with single spaces into one search text. At least one argument is required.
Prints the same JSON as "lyx board list"; with --text it prints the compact one-line-per-task
listing instead (errors stay JSON); its columns are kind, slug, title, labels and, when set, [status].
--label <name> keeps only entries carrying that label; repeat it to require several (AND).
A label that is in neither the types nor the labels list of board.yaml and that no entry carries is refused.

Examples:
  lyx board find retry backoff
  lyx board find --text retry
  lyx board find --label bug retry`,
		Args: cobra.MinimumNArgs(1),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := b.Find(strings.Join(args, " "), findLabels)
			if err != nil {
				return outputError(out, err.Error())
			}
			return writeListing(out, tasks, findText)
		}),
	}
	findCmd.Flags().BoolVar(&findText, "text", false, "print the compact one-line-per-task listing instead of JSON")
	findCmd.Flags().StringArrayVar(&findLabels, "label", nil, "keep only entries carrying this label; repeatable, all must match")

	retireLegacyCmd := &cobra.Command{
		Use:   "retire-legacy",
		Short: "Delete the legacy tasks.json and notes.json files",
		Long: `End the compatibility window with a pre-upgrade lyx: fold any last done marks from the
legacy files into board.json, then delete tasks.json and notes.json. Errors when neither file
exists. Takes no payload.

Example:
  lyx board retire-legacy`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if err := b.RetireLegacy(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	rerenderCmd := &cobra.Command{
		Use:   "rerender",
		Short: "Rebuild the README and design docs from board.json",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if err := b.Rerender(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Commit and push pending board changes to the remote",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if err := b.Sync(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	labelsCmd := &cobra.Command{
		Use:   "labels",
		Short: "Print the configured types and labels with their descriptions",
		Long: `Print the type labels and the other labels configured in board.yaml, each list in file order
with its descriptions. An empty list prints as []. Takes no payload.

Output:
  {"ok":true,"types":[{"label":"bug","description":"..."}],"labels":[{"label":"quarry","description":"..."}]}

Example:
  lyx board labels`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			return output.Ok(out, map[string]any{
				"types":  labelEntries(config.Types),
				"labels": labelEntries(config.Labels),
			})
		}),
	}

	cmd.AddCommand(storeVerbs(board)...)
	cmd.AddCommand(
		labelsCmd,
		promoteCmd,
		pruneCmd,
		findCmd,
		retireLegacyCmd,
		rerenderCmd,
		syncCmd,
		intakeCommand(board),
	)

	return cmd
}

// labelEntries returns labels for JSON output, an empty list rather than null when there are none.
func labelEntries(labels []boardengine.Label) []boardengine.Label {
	if labels == nil {
		return []boardengine.Label{}
	}
	return labels
}

// slugLimitNote is the help paragraph every slug-taking verb ends its Long with.
// It formats the limit from boardengine.MaxSlugLength,
// so help and validation cannot drift.
func slugLimitNote() string {
	return fmt.Sprintf("\n\nA slug is at most %d characters.", boardengine.MaxSlugLength)
}

// storeVerbs builds the nine store verbs over the one store board returns.
func storeVerbs(board func() *boardengine.Board) []*cobra.Command {
	// upsert subcommand: create or update a single task.
	var bodyFile string
	upsertCmd := &cobra.Command{
		Use:   "upsert [json-payload]",
		Short: "Create or update a single task",
		Long: `Create or update a task identified by its slug. Unknown keys are rejected.

Required field:
  "slug"       string — unique task identifier

Optional fields:
  "title"      string — human-readable title
  "brief"      string — one-line summary shown in board listings
  "body"       string — full markdown body (proposal / background)
  "depends_on" array  — list of slug strings this task depends on
  "isolated"   bool   — true if the task has no dependencies by design
  "status"     string — lifecycle status (e.g. "active", "done")
  "kind"       string — "task" or "note", default "note"; only a task can be claimed or depend on tasks
  "labels"     array  — configured labels replacing the whole list; a task needs a type label, a note exactly one
  "recipe"    string — recipe the task's child worktree runs; empty means "loom"
  "short_name" string — short display label; falls back to the slug

Flag:
  --body-file <path>  read "body" from the file, or from stdin when the path is "-"; every other field still comes from the payload.
                      Refused when the payload also carries "body" (drop one of them),
                      and when the payload argument is itself "-" (stdin can feed only one of them).

Example:
  lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary","kind":"task","labels":["enhancement"]}'` + slugLimitNote(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clihelp.WrapRun(func(out io.Writer, args []string) int {
				// cobra strips the "upsert" token; json payload is now args[0].
				if len(args) == 0 {
					return outputError(out, "json payload required")
				}
				fields := map[string]any{}
				decodeErr := json.Unmarshal([]byte(args[0]), &fields)
				if bodyFile != "" {
					// Runs before the decode error is reported,
					// so a "-" payload gets the stdin refusal.
					if fields == nil {
						fields = map[string]any{}
					}
					if err := applyBodyFile(fields, bodyFile, args[0], cmd.InOrStdin()); err != nil {
						return outputError(out, err.Error())
					}
				}
				if decodeErr != nil {
					return outputError(out, fmt.Sprintf("invalid json: %v", decodeErr))
				}
				task, err := board().UpsertTask(fields)
				if err != nil {
					return outputError(out, err.Error())
				}
				return outputSuccessWithTask(out, task)
			})(cmd, args)
		},
	}
	upsertCmd.Flags().StringVar(&bodyFile, "body-file", "", `read the task's "body" from this file, or from stdin when "-"`)

	// upsert-batch subcommand: create or update multiple tasks atomically.
	// Allowed wrapper key: {tasks}. A typo'd wrapper (e.g. "taks") errors;
	// an absent or empty tasks array also errors (nothing to upsert is a mistake).
	upsertBatchCmd := &cobra.Command{
		Use:   "upsert-batch [json-payload]",
		Short: "Create or update multiple tasks atomically",
		Long: `Create or update multiple tasks in one atomic write. Unknown wrapper keys are rejected.
An absent or empty "tasks" array is an error. Each task element uses the same fields as
"lyx board upsert" ("slug" required per element); unknown element keys are also rejected.

Required wrapper field:
  "tasks" array — one or more task objects (each with "slug" required)

Example:
  lyx board upsert-batch '{"tasks":[{"slug":"t1","title":"One","kind":"task","labels":["bug"]},{"slug":"t2","title":"Two","kind":"task","labels":["bug"]}]}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// Decode into a map to detect unknown wrapper keys.
			var raw map[string]any
			if err := json.Unmarshal([]byte(args[0]), &raw); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			// Only "tasks" is permitted at the wrapper level; a typo'd key would
			// decode silently to count:0 with the old typed-struct approach.
			for k := range raw {
				if k != "tasks" {
					return outputError(out, fmt.Sprintf("unknown field: %q", k))
				}
			}

			// tasks is required and must be a non-empty array.
			tasksVal, hasTasksKey := raw["tasks"]
			if !hasTasksKey || tasksVal == nil {
				return outputError(out, "missing required field: tasks")
			}
			tasksArr, ok := tasksVal.([]any)
			if !ok {
				return outputError(out, "tasks must be an array")
			}
			if len(tasksArr) == 0 {
				return outputError(out, "tasks array must not be empty")
			}

			tasks := make([]map[string]any, len(tasksArr))
			for i, v := range tasksArr {
				m, ok := v.(map[string]any)
				if !ok {
					return outputError(out, fmt.Sprintf("tasks[%d] must be an object", i))
				}
				tasks[i] = m
			}

			if err := board().UpsertTasksBatch(tasks); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithCount(out, len(tasks))
		}),
	}

	// set-status subcommand: set or clear the status field of a task identified by
	// slug or numeric id. Allowed keys: {slug, id, status}.
	setStatusCmd := &cobra.Command{
		Use:   "set-status [json-payload]",
		Short: "Set or clear the status of a task",
		Long: `Set or clear the lifecycle status of a task. Unknown keys are rejected.
Exactly one of "slug" or "id" is required. "status" is always required; use null to clear.

Fields:
  "slug"   string      — task slug (mutually exclusive with "id")
  "id"     integer     — numeric task ID (mutually exclusive with "slug")
  "status" string|null — new status value; null clears the current status

Examples:
  lyx board set-status '{"slug":"my-task","status":"active"}'
  lyx board set-status '{"id":96,"status":null}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			// resolveLookup enforces {slug, id, status} allowed keys and exactly-one-of slug/id.
			selector, m, err := resolveLookup([]byte(args[0]), "status")
			if err != nil {
				return outputError(out, err.Error())
			}

			// status key is required: an absent key is an error; an explicit null clears
			// the status. This distinguishes a deliberate clear from a typo that would
			// otherwise silently clear the status value.
			sv, hasStatus := m["status"]
			if !hasStatus {
				return outputError(out, "missing required field: status")
			}
			var status *string
			if sv != nil {
				s, ok := sv.(string)
				if !ok {
					return outputError(out, "status must be a string or null")
				}
				status = &s
			}

			if err := board().SetStatus(selector, status); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// remove subcommand: remove a task by slug or numeric id.
	removeCmd := &cobra.Command{
		Use:   "remove [json-payload]",
		Short: "Remove a task",
		Long: `Remove a task by slug or numeric ID. Unknown keys are rejected.
Exactly one of "slug" or "id" is required. Errors if the task is not found.

Fields:
  "slug" string  — task slug (mutually exclusive with "id")
  "id"   integer — numeric task ID (mutually exclusive with "slug")

Example:
  lyx board remove '{"slug":"my-task"}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			// resolveLookup enforces {slug, id} allowed keys and exactly-one-of.
			selector, _, err := resolveLookup([]byte(args[0]))
			if err != nil {
				return outputError(out, err.Error())
			}
			if err := board().RemoveTask(selector); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// get subcommand: fetch a single task by slug or numeric id; returns task:null
	// for a valid-but-absent target (not an error). Malformed payloads error.
	var getBody bool
	getCmd := &cobra.Command{
		Use:   "get [json-payload]",
		Short: "Fetch a single task",
		Long: `Fetch a single task by slug or numeric ID. Unknown keys are rejected.
Exactly one of "slug" or "id" is required. Returns {"task":null} if not found (not an error).
Malformed payloads (no identifier key, unknown key) are errors.
The result is the envelope {"task": {...}}, which the discussion stencil reads.
--body writes the entry's body alone, verbatim, with no envelope; an empty body writes nothing.
With --body a target that does not exist is an error, since there is no task:null to print.
Editing a body as a file: lyx board get ... --body > body.md, edit it, then feed it back with
upsert --body-file body.md (merge --body-file takes the same file for a merged entry).

Fields:
  "slug" string  — task slug (mutually exclusive with "id")
  "id"   integer — numeric task ID (mutually exclusive with "slug")

Examples:
  lyx board get '{"id":96}'
  lyx board get '{"slug":"my-task"}' --body` + slugLimitNote(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clihelp.WrapRun(func(out io.Writer, args []string) int {
				if len(args) == 0 {
					return outputError(out, "json payload required")
				}
				// resolveLookup enforces {slug, id} allowed keys and exactly-one-of.
				selector, _, err := resolveLookup([]byte(args[0]))
				if err != nil {
					return outputError(out, err.Error())
				}
				task, found, err := board().GetTask(selector)
				if err != nil {
					return outputError(out, err.Error())
				}
				if getBody {
					if !found {
						return outputError(out, fmt.Sprintf("no entry %v: check the slug or id with lyx board list", selector))
					}
					if _, err := io.WriteString(out, task.Body); err != nil {
						return outputError(out, err.Error())
					}
					return 0
				}
				if found {
					return outputGetTask(out, &task)
				}
				return outputGetTask(out, nil) // task: null in JSON output
			})(cmd, args)
		},
	}
	getCmd.Flags().BoolVar(&getBody, "body", false, "write the entry's body alone, verbatim, instead of the JSON envelope")

	// list subcommand: list all tasks with computed fields (layer, has_proposal).
	var listText bool
	var listLabels []string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all tasks with computed fields",
		Long: `List all tasks in README order with their computed fields (layer, has_proposal).
With --text, print the compact one-line-per-task listing (kind, slug, title, labels, [status])
instead of JSON; errors stay JSON.
--label <name> keeps only entries carrying that label; repeat it to require several (AND).
A label that is in neither the types nor the labels list of board.yaml and that no entry carries is refused.

Examples:
  lyx board list
  lyx board list --text
  lyx board list --label bug --label enhancement`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := board().ListTasksBrief(listLabels)
			if err != nil {
				return outputError(out, err.Error())
			}
			return writeListing(out, tasks, listText)
		}),
	}
	listCmd.Flags().BoolVar(&listText, "text", false, "print the compact one-line-per-task listing instead of JSON")
	listCmd.Flags().StringArrayVar(&listLabels, "label", nil, "keep only entries carrying this label; repeatable, all must match")

	// list-full subcommand: list all tasks as stored in board.json.
	listFullCmd := &cobra.Command{
		Use:   "list-full",
		Short: "List all tasks as stored in board.json",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := board().ListTasksFull()
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputListFull(out, tasks)
		}),
	}

	// merge subcommand: remove slugs, upsert one task, and optionally set status — atomically.
	// Allowed top-level keys: {remove_slugs, upsert, set_status}. The inner set_status
	// object is validated identically to the set-status command.
	var mergeBodyFile string
	mergeCmd := &cobra.Command{
		Use:   "merge [json-payload]",
		Short: "Atomically remove, upsert, and set-status",
		Long: `Remove tasks, upsert a task, and optionally set status in one atomic write.
Unknown top-level keys are rejected. The inner "set_status" object is validated
identically to the standalone "set-status" command ({slug|id, status}, exactly-one-of).

Fields:
  "remove_slugs" array  — slug strings to remove (optional; omit to skip)
  "upsert"       object — task to create or update (required; same fields as "lyx board upsert")
  "set_status"   object — status to set after upsert (optional):
    "slug"   string      — task slug (mutually exclusive with "id")
    "id"     integer     — numeric task ID (mutually exclusive with "slug")
    "status" string|null — new status; null clears

Flag:
  --body-file <path>  read the upsert's "body" from the file, or from stdin when the path is "-"; every other field still comes from the payload.
                      Refused when the payload's "upsert" also carries "body" (drop one of them),
                      and when the payload argument is itself "-" (stdin can feed only one of them).

Example:
  lyx board merge '{"remove_slugs":["old"],"upsert":{"slug":"new","title":"New"},"set_status":{"slug":"new","status":"active"}}'` + slugLimitNote(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clihelp.WrapRun(func(out io.Writer, args []string) int {
				if len(args) == 0 {
					return outputError(out, "json payload required")
				}

				// Decode into a map first to detect unknown top-level keys.
				var raw map[string]any
				decodeErr := json.Unmarshal([]byte(args[0]), &raw)
				if decodeErr != nil && mergeBodyFile != "" {
					// Runs before the decode error is reported,
					// so a "-" payload gets the stdin refusal.
					if err := applyBodyFile(map[string]any{}, mergeBodyFile, args[0], cmd.InOrStdin()); err != nil {
						return outputError(out, err.Error())
					}
				}
				if decodeErr != nil {
					return outputError(out, fmt.Sprintf("invalid json: %v", decodeErr))
				}

				// Enforce strict top-level key set;
				// a stale set_phase errors rather than being silently dropped,
				// which would skip the status step with no feedback.
				for k := range raw {
					if k != "remove_slugs" && k != "upsert" && k != "set_status" {
						return outputError(out, fmt.Sprintf("unknown field: %q", k))
					}
				}

				// Parse remove_slugs (optional, default empty).
				var removeSlugs []string
				if rsVal, ok := raw["remove_slugs"]; ok && rsVal != nil {
					rsArr, ok := rsVal.([]any)
					if !ok {
						return outputError(out, "remove_slugs must be an array")
					}
					for _, v := range rsArr {
						s, ok := v.(string)
						if !ok {
							return outputError(out, "remove_slugs elements must be strings")
						}
						removeSlugs = append(removeSlugs, s)
					}
				}

				upsertVal, hasUpsert := raw["upsert"]
				if !hasUpsert || upsertVal == nil {
					return outputError(out, "missing required field: upsert")
				}
				upsertFields, ok := upsertVal.(map[string]any)
				if !ok {
					return outputError(out, "upsert must be an object")
				}
				if mergeBodyFile != "" {
					if err := applyBodyFile(upsertFields, mergeBodyFile, args[0], cmd.InOrStdin()); err != nil {
						return outputError(out, err.Error())
					}
				}

				// Parse set_status (optional):
				// validate using the same resolveLookup logic as the standalone set-status command,
				// with {slug,id,status} allowed,
				// exactly-one-of slug/id, and status key required.
				var setStatusPtr *boardengine.MergeStatusUpdate
				if ssVal, ok := raw["set_status"]; ok && ssVal != nil {
					ssBytes, err := json.Marshal(ssVal)
					if err != nil {
						return outputError(out, fmt.Sprintf("set_status: marshal error: %v", err))
					}
					selector, ssMap, err := resolveLookup(ssBytes, "status")
					if err != nil {
						return outputError(out, "set_status: "+err.Error())
					}
					// status key is required inside set_status, mirroring the standalone command.
					sv, hasStatusKey := ssMap["status"]
					if !hasStatusKey {
						return outputError(out, "set_status: missing required field: status")
					}
					var status *string
					if sv != nil {
						s, ok := sv.(string)
						if !ok {
							return outputError(out, "set_status.status must be a string or null")
						}
						status = &s
					}
					setStatusPtr = &boardengine.MergeStatusUpdate{Selector: selector, Status: status}
				}

				task, err := board().MergeTasks(removeSlugs, upsertFields, setStatusPtr)
				if err != nil {
					return outputError(out, err.Error())
				}
				return outputSuccessWithTask(out, task)
			})(cmd, args)
		},
	}
	mergeCmd.Flags().StringVar(&mergeBodyFile, "body-file", "", `read the upsert's "body" from this file, or from stdin when "-"`)

	// set-deps subcommand: replace the depends_on list for a task.
	// Allowed keys: {slug, depends_on}. depends_on is required (absent errors;
	// explicit [] clears the list, distinguishing intentional clear from a typo
	// that would otherwise silently wipe the task's dependency list).
	setDepsCmd := &cobra.Command{
		Use:   "set-deps [json-payload]",
		Short: "Replace the depends_on list for a task",
		Long: `Replace the full depends_on list for a task wholesale. Unknown keys are rejected.
Both fields are required. An absent "depends_on" is an error; an explicit [] clears the list.

Fields:
  "slug"       string — task slug to update (required)
  "depends_on" array  — complete list of dependency slug strings; replaces existing list (required)

Example:
  lyx board set-deps '{"slug":"my-task","depends_on":["dep-a","dep-b"]}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// Decode into a map to detect unknown keys and key presence.
			var m map[string]any
			if err := json.Unmarshal([]byte(args[0]), &m); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			// Reject unknown keys so a typo ("depends") errors instead of silently
			// clearing the dependency list.
			for k := range m {
				if k != "slug" && k != "depends_on" {
					return outputError(out, fmt.Sprintf("unknown field: %q", k))
				}
			}

			// slug is required to identify the target task.
			slug, ok := m["slug"].(string)
			if !ok || slug == "" {
				return outputError(out, "missing required field: slug")
			}

			// depends_on is required: absent key errors; explicit [] clears the list.
			depsVal, hasDeps := m["depends_on"]
			if !hasDeps {
				return outputError(out, "missing required field: depends_on")
			}

			var dependsOn []string
			if depsVal != nil {
				arr, ok := depsVal.([]any)
				if !ok {
					return outputError(out, "depends_on must be an array")
				}
				dependsOn = make([]string, 0, len(arr))
				for _, v := range arr {
					s, ok := v.(string)
					if !ok {
						return outputError(out, "depends_on elements must be strings")
					}
					dependsOn = append(dependsOn, s)
				}
			} else {
				// Explicit null — treat as empty (clear the list).
				dependsOn = []string{}
			}

			if err := board().SetDeps(slug, dependsOn); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	return []*cobra.Command{
		upsertCmd,
		upsertBatchCmd,
		setStatusCmd,
		removeCmd,
		getCmd,
		listCmd,
		listFullCmd,
		mergeCmd,
		setDepsCmd,
	}
}

// writeListing prints tasks as the compact listing when text is set and as the JSON list envelope otherwise.
func writeListing(out io.Writer, tasks []boardengine.BriefTask, text bool) int {
	if !text {
		return outputListBrief(out, tasks)
	}
	if _, err := io.WriteString(out, RenderCompact(tasks)); err != nil {
		return outputError(out, err.Error())
	}
	return 0
}

// resolveLookup decodes and validates a JSON payload, returning the selector and decoded map.
func resolveLookup(raw []byte, extraKeys ...string) (any, map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, fmt.Errorf("invalid json: %v", err)
	}

	allowed := map[string]bool{"slug": true, "id": true}
	for _, k := range extraKeys {
		allowed[k] = true
	}

	for k := range m {
		if !allowed[k] {
			return nil, nil, fmt.Errorf("unknown field: %q", k)
		}
	}

	_, hasSlug := m["slug"]
	_, hasID := m["id"]

	if !hasSlug && !hasID {
		return nil, nil, fmt.Errorf("one of slug or id is required")
	}
	if hasSlug && hasID {
		return nil, nil, fmt.Errorf("only one of slug or id may be given")
	}

	if hasSlug {
		slugStr, ok := m["slug"].(string)
		if !ok || slugStr == "" {
			return nil, nil, fmt.Errorf("slug must be a non-empty string")
		}
		return slugStr, m, nil
	}

	switch v := m["id"].(type) {
	case float64:
		if v != math.Trunc(v) {
			return nil, nil, fmt.Errorf("id must be an integer")
		}
		return v, m, nil
	default:
		return nil, nil, fmt.Errorf("id must be a number")
	}
}

// RunCLI is the public seam for the board module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
// The branch exists because lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to
// ExecuteIn would panic on every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// outputError writes {"ok":false,"error":"..."} and returns exit code 1.
func outputError(out io.Writer, message string) int {
	return output.Err(out, message)
}

// outputSuccess writes {"ok":true} and returns exit code 0.
func outputSuccess(out io.Writer) int {
	return output.Ok(out, map[string]any{})
}

// outputSuccessWithCount writes {"ok":true,"count":N} with exit code 0.
func outputSuccessWithCount(out io.Writer, count int) int {
	return output.Ok(out, map[string]any{"count": count})
}

// outputSuccessWithTask writes {"ok":true,"task":{...}} with exit code 0.
func outputSuccessWithTask(out io.Writer, task boardengine.Task) int {
	return output.Ok(out, map[string]any{"task": task})
}

// outputGetTask writes {"ok":true,"task":{...}} with task as pointer (nil → null).
func outputGetTask(out io.Writer, task *boardengine.Task) int {
	return output.Ok(out, map[string]any{"task": task})
}

// outputListBrief writes {"ok":true,"tasks":[...]} with BriefTask objects with exit code 0.
func outputListBrief(out io.Writer, tasks []boardengine.BriefTask) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}

// outputListFull writes {"ok":true,"tasks":[...]} with full Task objects with exit code 0.
func outputListFull(out io.Writer, tasks []boardengine.Task) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}
