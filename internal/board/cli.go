// cli.go exposes the cobra command tree for the board module.
//
// Command() returns the root "board" command with 11 subcommands, each
// wrapping the existing handler bodies from the legacy switch. Configuration
// resolution happens once in a PersistentPreRunE so that "lyx board" with no
// subcommand lists available subcommands without requiring a git repo or board
// config. The hidden --board-path persistent flag bypasses cwd resolution for
// the detached sync child process launched by spawn.go.

package board

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the board module.
//
// The parent "board" command carries a PersistentPreRunE that resolves config
// and constructs the Board instance once, before any subcommand runs. The
// resolved Board is shared via a closure variable closed over by all 11 RunEs.
// When the parent is invoked with no subcommand, cobra lists available
// subcommands without invoking the PreRunE, so no board config is needed.
func Command() *cobra.Command {
	// b is populated by PersistentPreRunE and closed over by each subcommand RunE.
	var b *Board

	cmd := &cobra.Command{
		Use:   "board",
		Short: "task-tracker board",
		Long: `board manages the task-tracker wiki board for the current lyx worktree.

Configuration is resolved from the current working directory via _lyx/config/board.yaml.
The --board-path flag (hidden; injected by the detached sync child) bypasses this resolution.
Running "lyx board" with no subcommand lists available subcommands without requiring a git repo.`,
	}

	// --board-path is an internal persistent flag injected by spawnSync so that
	// the detached sync child can bypass cwd resolution and use the absolute board
	// path directly. It must be hidden so it does not appear in help output.
	boardPathFlag := cmd.PersistentFlags().String("board-path", "", "internal: injected absolute board dir for the detached sync child")
	if err := cmd.PersistentFlags().MarkHidden("board-path"); err != nil {
		// MarkHidden only errors when the flag name does not exist, which cannot
		// happen here since we just registered it above.
		panic(fmt.Sprintf("board: MarkHidden board-path: %v", err))
	}

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		var cfg Config

		// If --board-path is set, use it directly (internal use for detached sync child).
		if *boardPathFlag != "" {
			cfg = Config{Path: *boardPathFlag}
		} else {
			// Resolve configuration from the current working directory.
			cwd, err := paths.Getwd()
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			cfg, err = LoadConfig(cwd, "board")
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
		}

		// Fold BOARD_SKIP_* env into cfg at the single production entry point.
		cfg = applySkipEnv(cfg)
		b = New(cfg)
		return nil
	}

	// upsert subcommand: create or update a single task.
	upsertCmd := &cobra.Command{
		Use:   "upsert [json-payload]",
		Short: "Create or update a single task",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0 // abort was signalled; do nothing
			}
			// cobra strips the "upsert" token; json payload is now args[0].
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var fields map[string]any
			if err := json.Unmarshal([]byte(args[0]), &fields); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			task, err := b.UpsertTask(fields)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithTask(out, task)
		}),
	}

	// upsert-batch subcommand: create or update multiple tasks atomically.
	upsertBatchCmd := &cobra.Command{
		Use:   "upsert-batch [json-payload]",
		Short: "Create or update multiple tasks atomically",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var payload struct {
				Tasks []map[string]any `json:"tasks"`
			}
			if err := json.Unmarshal([]byte(args[0]), &payload); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			if err := b.UpsertTasksBatch(payload.Tasks); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithCount(out, len(payload.Tasks))
		}),
	}

	// set-phase subcommand: set or clear the status field of a task.
	setPhaseCmd := &cobra.Command{
		Use:   "set-phase [json-payload]",
		Short: "Set or clear the phase of a task",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var payload struct {
				IDOrSlug any     `json:"id_or_slug"`
				Phase    *string `json:"phase"` // pointer: JSON null → Go nil → status cleared
			}
			if err := json.Unmarshal([]byte(args[0]), &payload); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			if err := b.SetPhase(payload.IDOrSlug, payload.Phase); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// remove subcommand: remove a task by id or slug.
	removeCmd := &cobra.Command{
		Use:   "remove [json-payload]",
		Short: "Remove a task",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var payload struct {
				IDOrSlug any `json:"id_or_slug"`
			}
			if err := json.Unmarshal([]byte(args[0]), &payload); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			if err := b.RemoveTask(payload.IDOrSlug); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// get subcommand: fetch a single task; returns task:null if not found (not an error).
	getCmd := &cobra.Command{
		Use:   "get [json-payload]",
		Short: "Fetch a single task",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var payload struct {
				IDOrSlug any `json:"id_or_slug"`
			}
			if err := json.Unmarshal([]byte(args[0]), &payload); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			task, found, err := b.GetTask(payload.IDOrSlug)
			if err != nil {
				return outputError(out, err.Error())
			}
			if found {
				return outputGetTask(out, &task)
			}
			return outputGetTask(out, nil) // task: null in JSON output
		}),
	}

	// list subcommand: list all tasks with computed fields (layer, has_proposal).
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all tasks with computed fields",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			tasks, err := b.ListTasksBrief()
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputListBrief(out, tasks)
		}),
	}

	// list-full subcommand: list all tasks as stored in tasks.json.
	listFullCmd := &cobra.Command{
		Use:   "list-full",
		Short: "List all tasks as stored in tasks.json",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			tasks, err := b.ListTasksFull()
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputListFull(out, tasks)
		}),
	}

	// merge subcommand: remove + upsert + set_phase atomically.
	mergeCmd := &cobra.Command{
		Use:   "merge [json-payload]",
		Short: "Atomically remove, upsert, and set-phase",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var payload struct {
				RemoveSlugs []string       `json:"remove_slugs"`
				Upsert      map[string]any `json:"upsert"`
				SetPhase    []any          `json:"set_phase"` // two elements: [id_or_slug, phase]
			}
			if err := json.Unmarshal([]byte(args[0]), &payload); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			// set_phase must be exactly two elements if provided.
			var setPhasePtr *[2]any
			if len(payload.SetPhase) > 0 {
				if len(payload.SetPhase) != 2 {
					return outputError(out, "set_phase must be array of length 2")
				}
				setPhasePtr = &[2]any{payload.SetPhase[0], payload.SetPhase[1]}
			}

			task, err := b.MergeTasks(payload.RemoveSlugs, payload.Upsert, setPhasePtr)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithTask(out, task)
		}),
	}

	// set-deps subcommand: replace the depends_on list for a task.
	setDepsCmd := &cobra.Command{
		Use:   "set-deps [json-payload]",
		Short: "Replace the depends_on list for a task",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			var payload struct {
				Slug      string   `json:"slug"`
				DependsOn []string `json:"depends_on"`
			}
			if err := json.Unmarshal([]byte(args[0]), &payload); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}
			if err := b.SetDeps(payload.Slug, payload.DependsOn); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// rerender subcommand: rebuild Home.md and _Sidebar.md from the current tasks.json.
	rerenderCmd := &cobra.Command{
		Use:   "rerender",
		Short: "Rebuild Home.md and _Sidebar.md from tasks.json",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if err := b.Rerender(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// sync subcommand: commit and push pending changes to the remote.
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Commit and push pending board changes to the remote",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if b == nil {
				return 0
			}
			if err := b.Sync(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	cmd.AddCommand(
		upsertCmd,
		upsertBatchCmd,
		setPhaseCmd,
		removeCmd,
		getCmd,
		listCmd,
		listFullCmd,
		mergeCmd,
		setDepsCmd,
		rerenderCmd,
		syncCmd,
	)

	return cmd
}

// RunCLI is the public seam for the board module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// outputError writes {"ok":false,"error":"..."} and returns exit code 1.
func outputError(out io.Writer, message string) int {
	return output.Err(out, message)
}

// outputSuccess writes {"ok":true} and returns exit code 0.
func outputSuccess(out io.Writer) int {
	return output.Ok(out, map[string]any{})
}

// outputSuccessWithCount writes {"ok":true,"count":N} and returns exit code 0.
func outputSuccessWithCount(out io.Writer, count int) int {
	return output.Ok(out, map[string]any{"count": count})
}

// outputSuccessWithTask writes {"ok":true,"task":{...}} and returns exit code 0.
func outputSuccessWithTask(out io.Writer, task Task) int {
	return output.Ok(out, map[string]any{"task": task})
}

// outputGetTask writes {"ok":true,"task":{...}} or {"ok":true,"task":null} and returns exit code 0.
// task is a pointer: nil produces task:null in JSON (task not found, but not an error).
func outputGetTask(out io.Writer, task *Task) int {
	return output.Ok(out, map[string]any{"task": task})
}

// outputListBrief writes {"ok":true,"tasks":[...]} with BriefTask objects and returns exit code 0.
func outputListBrief(out io.Writer, tasks []BriefTask) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}

// outputListFull writes {"ok":true,"tasks":[...]} with full Task objects and returns exit code 0.
func outputListFull(out io.Writer, tasks []Task) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}

// applySkipEnv folds BOARD_SKIP_GIT and BOARD_SKIP_PUSH environment variables into cfg.
// This is the single production env read; all other consumption sites use the config fields.
func applySkipEnv(cfg Config) Config {
	if os.Getenv("BOARD_SKIP_GIT") == "1" {
		cfg.SkipGit = true
	}
	if os.Getenv("BOARD_SKIP_PUSH") == "1" {
		cfg.SkipPush = true
	}
	return cfg
}
