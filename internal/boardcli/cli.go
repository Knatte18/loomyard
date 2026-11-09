// cli.go exposes the cobra command tree for the board module.
//
// Command() returns the root "board" command over one store, board.json, whose entries carry a kind and labels.
// The verbs upsert, upsert-batch, set-status, remove, get, list, list-full, merge and set-deps come from storeVerbs.
// Command itself builds the remaining verbs, such as promote, rename, raddle and sync.
// The intake group (list, import, close) comes from intakeCommand in intake.go, over the hub repository the hubRepo closure opens from the resolved location.
// list and find take --text to print the compact listing from text.go instead of JSON.
// Configuration resolution happens once in a PersistentPreRunE.
// The config file (readme, design_prefix) is loaded from the Bolt checkout's _lym/config/board.yaml, whatever worktree the verb runs in,
// and the board data dir is resolved as fabricengine.BoardDir(layout.HubPath) via lymcwd.Resolve.
// The hidden --board-path persistent flag overrides the data dir for the detached sync child
// process launched by internal/boardengine/spawn.go, bypassing both config and path resolution.

package boardcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubissues"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/spf13/cobra"
)

// exitDeferredSync ends a deferrable board sync that a pause covers.
var exitDeferredSync = func() { os.Exit(0) }

// Command returns the cobra command tree for the board module.
func Command() *cobra.Command {
	// b is populated by PersistentPreRunE and closed over by each subcommand RunE.
	var b *boardengine.Board
	board := func() *boardengine.Board { return b }
	// config is the Config PersistentPreRunE loaded;
	// the labels verb reads its Types and Labels.
	var config boardengine.Config
	// profiles loads the hub's run profiles from the board dir PersistentPreRunE resolved.
	profiles := func() (runprofile.Profiles, error) {
		return runprofile.Load(configengine.Dirs{Board: config.Path})
	}
	// location is the worktree PersistentPreRunE resolved, nil under --board-path.
	var location *lymcwd.Location
	// hubRepo opens the hub's own repository's issues, resolved only when an intake verb asks.
	hubRepo := func() (*hubissues.Repo, error) {
		if location == nil {
			return nil, fmt.Errorf("intake needs the hub's repository, which only a run from a hub worktree resolves: run it from a worktree of the hub")
		}
		url, err := fabricengine.HubRepoURL(location)
		if err != nil {
			return nil, err
		}
		return hubissues.Open(url)
	}

	cmd := &cobra.Command{
		Use:   "board",
		Short: "read and write the task-tracker board's tasks and notes",
		// The dash keeps the legend from reading as a continuation of the Short it follows on the index line.
		Annotations: map[string]string{
			clihelp.IndexNoteAnnotation:         "— " + payloadLegend,
			clihelp.IndexNoteAudienceAnnotation: clihelp.AudienceOperator,
		},
		Long: `board manages the task-tracker board for the current lym worktree.

The board is one store: every entry is a task, a note or a project (its kind) and carries labels. Only
a task can be claimed and depend on other tasks; a note is an idea or observation. A project groups
entries: a task or note joins it through its "project" field, the project lists its members' plan
order in "order", and its "beam" flag, set only by "lym beam create", makes it
a long-lived pair that runs its members. "get" and "list" carry an entry's "parent_beam", the slug of
the beam above it, empty when the prime is its parent. A task carries at least
one type label, a note exactly one, and every label must be configured in board.yaml. Every entry
carries a handle the board assigns, T and a number for a task, P and a number for a project, a
note's type letter and a number for a note, such as T4 or B2. The README renders tasks split into dependency layers and notes grouped by
type, each table sorted by handle. Agents read and write the
board through "lym board", never through the JSON files under ` + fabricengine.BoltDirName + `.

The config file is the hub's <hub>/` + fabricengine.BoltDirName + `/_lym/config/board.yaml, read whatever worktree the verb runs in.
It controls non-geometry settings: readme
and design_prefix filenames and the types and labels lists. The board data dir (<hub>/` + fabricengine.BoltDirName + `) is
derived from the worktree layout via lymcwd and is not config- or
env-overridable. The hidden --board-path flag overrides the data dir for the
detached sync child process. Running "lym board" with no subcommand lists
available subcommands without requiring a git repo.

` + payloadLegend,
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

		// The detached sync child passes --board-path, so it skips config and path resolution.
		if *boardPathFlag != "" {
			cfg = boardengine.Config{Path: *boardPathFlag}
		} else {
			// The working directory locates the worktree layout; config and the data dir follow from it.
			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			layout, rerr := lymcwd.Resolve(cwd)
			if rerr != nil {
				output.Err(cmd.OutOrStdout(), rerr.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			location = layout
			boardDir := fabricengine.BoardDir(layout.HubPath)

			cfg, err = boardengine.LoadConfig(fabricengine.ConfigDirs(layout), "board")
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			cfg.Path = boardDir
			cfg.ParentOf = func(slug string) (string, error) { return hubgeom.ParentOf(layout, slug) }
			cfg.PairExists = func(slug string) bool { return fabricengine.PairWorktreeStands(layout, slug) }
			cfg.ValidateSlug = func(slug string) error {
				fabricConfig, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(layout))
				if err != nil {
					return fmt.Errorf("load the fabric config to judge slug %q: %w", slug, err)
				}
				return fabricengine.ValidatePairSlug(slug, fabricConfig.Dirs())
			}
		}

		cfg = boardengine.ApplySkipEnv(cfg)
		cfg.StrandName = strandNameFromEnv()
		config = cfg
		b = boardengine.New(cfg)
		return nil
	}

	var promoteRecipe string
	promoteCmd := &cobra.Command{
		Use:         "promote {slug|id|handle}",
		Short:       "turn a note into a task once it is ready to be claimed",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Turn a note into a task. A task is returned unchanged, and nothing is written.
The promoted entry loses its priority, which only a note carries, and its note handle: it takes
the lowest free T handle.
--recipe <name> sets the task's recipe in the same write.
A Sketch, or any discussed note whose decision record is stored, becomes a task only with recipe
raddle, which keeps its record and releases the task so its raddle run starts: a promote with no
--recipe gives it raddle, --recipe raddle is accepted, and any other recipe is refused.
Demotion goes through "lym board upsert" with "kind":"note".

Fields (one of):
  "slug"   string  — entry slug
  "id"     integer — entry id
  "handle" string  — entry handle, such as B4

Examples:
  lym board promote '{"slug":"my-note"}'
  lym board promote B4
  lym board promote S3` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			lookup, _, err := resolveLookup([]byte(args[0]), lookupPayload)
			if err != nil {
				return outputError(out, err.Error())
			}
			task, err := b.PromoteTo(lookup, promoteRecipe)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithTask(out, task, "")
		}),
	}
	promoteCmd.Flags().StringVar(&promoteRecipe, "recipe", "", "set the promoted task's recipe in the same write; a Sketch takes raddle when none is given, and no other")

	renameCmd := &cobra.Command{
		Use:         "rename {slug|id|handle, to}",
		Short:       "change an entry's slug in one write, keeping every other field and rewriting depends_on",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Change an entry's slug in one atomic write. The entry keeps its id, handle, position, body, kind, labels,
priority, issues, dates, status and every other field, and every other entry's depends_on names the
new slug in place of the old one. No entry's revised date changes.
Refused, with nothing written, for an absent entry, an entry held by a run, and a target that is
empty, too long or already an entry's slug.
Body and brief text is never rewritten: "mentions" lists the entries whose body or brief still name
the old slug as a whole word, the renamed entry included, and is [] when none does.
A discussed entry's decision record file moves with it. When that step fails after the write, the entry
is renamed and the verb reports the error beside "task" and "mentions": "rename moved the entry but not its decision record" names the
record file left at the old slug's name, which the entry's next Discussion approval stores again under the
new slug; "rename saved the entry, but the orphaned decision record ... could not be removed" names the file
at the new slug's name, to remove by hand.

Fields:
  "slug"   string  — current slug (one of slug, id or handle)
  "id"     integer — numeric entry ID (one of slug, id or handle)
  "handle" string  — entry handle, such as B4 (one of slug, id or handle)
  "to"     string  — the new slug (required)

Output:
  {"ok":true,"task":{...},"mentions":["other-entry"]}

Example:
  lym board rename '{"slug":"old-name","to":"new-name"}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			selector, m, err := resolveLookup([]byte(args[0]), renamePayload)
			if err != nil {
				return outputError(out, err.Error())
			}
			to, ok := m["to"].(string)
			if !ok {
				return outputError(out, "to must be a string")
			}
			result, err := b.Rename(selector, to)
			var recordErr *boardengine.DecisionRecordError
			if errors.As(err, &recordErr) {
				return output.ErrFields(out, err.Error(), map[string]any{"task": result.Task, "mentions": result.Mentions})
			}
			if err != nil {
				return outputError(out, err.Error())
			}
			return output.Ok(out, map[string]any{"task": result.Task, "mentions": result.Mentions})
		}),
	}

	var pruneApply bool
	pruneCmd := &cobra.Command{
		Use:         "prune",
		Short:       "remove the done tasks, clearing finished work off the board",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Without --apply, prune is a read-only dry run: it prints the slugs it would remove and changes nothing.
With --apply it removes every task whose status is done, strips the removed slugs from the remaining
tasks' depends_on, and prints the removed slugs. A done entry still carrying notify_on_land is kept
until its dobby has sent the landing line and cleared the field. Takes no payload. The output carries
"applied": true or false.

Example:
  lym board prune
  lym board prune --apply`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			removed, err := b.Prune(pruneApply)
			if err != nil {
				return outputError(out, err.Error())
			}
			if removed == nil {
				removed = []string{}
			}
			return output.Ok(out, map[string]any{"removed": removed, "applied": pruneApply})
		}),
	}
	pruneCmd.Flags().BoolVar(&pruneApply, "apply", false, "remove the done tasks, keeping one still carrying notify_on_land (default is dry-run/report)")

	var findText bool
	var findLabels []string
	findCmd := &cobra.Command{
		Use:         "find <text>...",
		Short:       "find tasks whose slug, title, brief or body contains the text",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Find tasks whose slug, title, brief or body contains the text, done tasks included.
The arguments are joined with single spaces into one search text. At least one argument is required.
Prints the same tasks array as "lym board list", in an envelope without prime_owner; with --text it prints the compact one-line-per-task
listing instead (errors stay JSON); its columns are handle, kind, priority, age, time since last revision, slug, title, labels and, when set, [status].
--label <name> keeps only entries carrying that label; repeat it to require several (AND).
A label that is in neither the types nor the labels list of board.yaml and that no entry carries is refused.

Examples:
  lym board find retry backoff
  lym board find --text retry
  lym board find --label bug retry`,
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

	raddleCmd := &cobra.Command{
		Use:         "raddle <slug>",
		Short:       "print the approved raddle proposal that filed a Sketch or one of its entries",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Print the approved proposal.yaml of the raddle run that split a Sketch, verbatim, with no envelope.
The slug is the Sketch's own or any slug its run filed, a project's included.
The raddle store outlives prune, so the lookup works after the run's landing pruned the Sketch, and on another
machine once its board pulled the store commit. It reads only.
An error for a slug the store does not hold: "lym report raddle" lists every stored Sketch.

Example:
  lym board raddle my-sketch`,
		Args: cobra.ExactArgs(1),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			proposal, found, err := b.RaddleProposal(args[0])
			if err != nil {
				return outputError(out, err.Error())
			}
			if !found {
				return outputError(out, fmt.Sprintf("the raddle store holds no Sketch or filed entry %q: `lym report raddle` lists every stored Sketch", args[0]))
			}
			if _, err := out.Write(proposal); err != nil {
				return outputError(out, err.Error())
			}
			return 0
		}),
	}

	var retireLegacyApply bool
	retireLegacyCmd := &cobra.Command{
		Use:         "retire-legacy",
		Short:       "delete the legacy tasks.json and notes.json files once no pre-upgrade lym reads them",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Without --apply, retire-legacy is a read-only dry run: it prints the legacy files it would delete
under legacy_files and changes nothing.
With --apply it ends the compatibility window with a pre-upgrade lym: it folds any last done marks
from the legacy files into board.json, then deletes tasks.json and notes.json. Errors when neither
file exists. Takes no payload. The output carries "applied": true or false.

Example:
  lym board retire-legacy
  lym board retire-legacy --apply`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			legacyFiles, err := b.RetireLegacy(retireLegacyApply)
			if err != nil {
				return outputError(out, err.Error())
			}
			return output.Ok(out, map[string]any{"legacy_files": legacyFiles, "applied": retireLegacyApply})
		}),
	}
	retireLegacyCmd.Flags().BoolVar(&retireLegacyApply, "apply", false, "delete the legacy files (default is dry-run/report)")

	rerenderCmd := &cobra.Command{
		Use:         "rerender",
		Short:       "rebuild the README and design docs from board.json",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if err := b.Rerender(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	var deferrableSync bool
	syncCmd := &cobra.Command{
		Use:         "sync",
		Short:       "commit and push pending board changes to the remote",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if deferrableSync {
				if pauserecord.GateCovered() {
					logger.Info("boardcli: a pause covers the board sync; nothing committed or pushed, the change waits for the next sync")
					return outputSuccess(out)
				}
				defer pauserecord.SetGateDeferral(func() {
					logger.Info("boardcli: a pause covers the board sync's push; exiting with the commit left local")
					exitDeferredSync()
				})()
			}
			if err := b.Sync(); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}
	syncCmd.Flags().BoolVar(&deferrableSync, boardengine.DeferrableSyncFlag, false, "internal: exit instead of waiting when a pause covers the sync")
	syncCmd.Flags().MarkHidden(boardengine.DeferrableSyncFlag) //nolint:errcheck

	labelsCmd := &cobra.Command{
		Use:         "labels",
		Short:       "print the configured types and labels with their descriptions, to pick an entry's labels",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Print the type labels and the other labels configured in board.yaml, each list in file order
with its descriptions. An empty list prints as []. Takes no payload.

Output:
  {"ok":true,"types":[{"label":"bug","description":"..."}],"labels":[{"label":"quarry","description":"..."}]}

Example:
  lym board labels`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			return output.Ok(out, map[string]any{
				"types":  labelEntries(config.Types),
				"labels": labelEntries(config.Labels),
			})
		}),
	}

	profilesCmd := &cobra.Command{
		Use:         "profiles",
		Short:       "print the hub's run profiles with their settings and each recipe's default, to pick a task's profile",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Print every run profile in the hub's profiles.yaml, in name order, with its settings:
one block per recipe, each module name to config keys, the shape of a task's run_settings.
Seed-Child writes only the block of the recipe the task runs. An empty list prints as [].
defaults maps a recipe to the profile a task of that recipe takes when it names none. Takes no payload.
Edit the profiles with "lym config profiles".

Output:
  {"ok":true,"defaults":{"tapestry":"light"},"profiles":[{"name":"light","settings":{"tapestry":{"tapestry":{"plan_max_bounces":1}}}}]}

Example:
  lym board profiles`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			loaded, err := profiles()
			if err != nil {
				return outputError(out, err.Error())
			}
			entries := []map[string]any{}
			for _, name := range loaded.Names() {
				entries = append(entries, map[string]any{"name": name, "settings": loaded.ByName[name]})
			}
			return output.Ok(out, map[string]any{"profiles": entries, "defaults": loaded.Defaults})
		}),
	}

	cmd.AddCommand(storeVerbs(board, profiles)...)
	cmd.AddCommand(
		labelsCmd,
		profilesCmd,
		promoteCmd,
		raddleCmd,
		renameCmd,
		pruneCmd,
		findCmd,
		retireLegacyCmd,
		rerenderCmd,
		syncCmd,
		intakeCommand(board, hubRepo),
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

// entrySelector lists the keys, exactly one of which names the entry a payload addresses.
var entrySelector = []string{"slug", "id", "handle"}

// lookupPayload is the payload of the verbs that address one entry by its selector.
var lookupPayload = payloadKeys{exclusive: [][]string{entrySelector}}

// renamePayload is the payload of rename.
var renamePayload = payloadKeys{required: []string{"to"}, exclusive: [][]string{entrySelector}}

// setStatusPayload is the payload of set-status, and merge's set_status object.
var setStatusPayload = payloadKeys{required: []string{"status"}, exclusive: [][]string{entrySelector}}

// setDepsPayload is the payload of set-deps.
var setDepsPayload = payloadKeys{required: []string{"depends_on"}, exclusive: [][]string{entrySelector}}

// upsertPayload is the payload of upsert, and of the task objects that upsert-batch and merge nest.
// The store enforces it on every upsert path.
var upsertPayload = func() payloadKeys {
	required, optional := boardengine.UpsertPayloadKeys()
	return payloadKeys{required: required, optional: optional}
}()

// upsertBatchPayload is the payload of upsert-batch.
var upsertBatchPayload = payloadKeys{required: []string{"tasks"}, nested: map[string]payloadKeys{"tasks": upsertPayload}}

// mergePayload is the payload of merge.
var mergePayload = payloadKeys{
	required: []string{"upsert"},
	optional: []string{"remove_slugs", "set_status"},
	nested:   map[string]payloadKeys{"upsert": upsertPayload, "set_status": setStatusPayload},
}

// storeVerbs builds the store verbs over the one store board returns.
// The upsert paths check a task's profile against the run profiles that profiles returns.
func storeVerbs(board func() *boardengine.Board, profiles func() (runprofile.Profiles, error)) []*cobra.Command {
	// upsert subcommand: create or update a single task.
	var bodyFile string
	upsertCmd := &cobra.Command{
		Use:         "upsert {slug, title?, brief?, body?, depends_on?, isolated?, status?, kind?, labels?, recipe?, priority?, short_name?, issues?, created?, revised?, run_settings?, profile?, pull_request?, notify_on_land?, owner_machine?, project?, order?, beam?, front?, on_land?, discussed?}",
		Short:       "create or update a single task",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Create or update a task identified by its slug.
The "notice" names siblings sharing a new depends_on and a label.

Required field:
  "slug"       string — unique task identifier

Optional fields:
  "title"      string — display title
  "brief"      string — one-line summary
  "body"       string — markdown body
  "depends_on" array  — slugs this task depends on
  "isolated"   bool   — no dependencies by design
  "status"     string — lifecycle status
  "kind"       string — "task", "note" or "project", default "note"
  "labels"     array  — replaces the list; a task needs a type label, a note exactly one
  "recipe"    string — "darn", "tapestry", "arras" or "motif", or default_recipe
  "priority"   string — a note's "high", "normal" or "low"; "normal" clears it
  "short_name" string — display label; falls back to the slug
  "issues"     array  — hub issue numbers
  "created"    string — RFC 3339 creation
  "revised"    string — RFC 3339 scope change
  "run_settings" object — module name to config keys; {} clears it
  "profile"    string — a "lym board profiles" name; "" clears it
  "pull_request" string — "on", "off" or "merge-in" (a true merge, no PR) overrides beam.yaml; "" clears it
  "notify_on_land" bool — tell the orch when the run lands, at once if it has
  "owner_machine" string — the effective owner only; "lym adopt" moves it
  "project"    string — slug of the open project holding the entry; "" leaves none
  "order"      array  — a project's member slugs, in order
  "beam"       bool   — a project's beam flag; only the "lym beam" verbs set it
  "front", "on_land", "discussed" — marks only lym writes: a front task, its land action, an approved Discussion

Flag:
  --body-file <path>  read "body" from the file, or "-" for stdin; not with "body".

Example:
  lym board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary","kind":"task","labels":["enhancement"]}'` + slugLimitNote(),
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
				if err := refuseNoticeOnAbsentEntry(board(), fields); err != nil {
					return outputError(out, err.Error())
				}
				notice, err := prepareRunSettings(board(), fields, strandNameFromEnv(), profiles)
				if err != nil {
					return outputError(out, err.Error())
				}
				task, siblings, err := board().UpsertTaskSiblings(fields)
				if err != nil {
					return outputError(out, err.Error())
				}
				return outputSuccessWithTask(out, task, joinNotices(notice, siblingNotice(siblings), landedNotice(task, fields)))
			})(cmd, args)
		},
	}
	upsertCmd.Flags().StringVar(&bodyFile, "body-file", "", `read the task's "body" from this file, or from stdin when "-"`)

	// upsert-batch subcommand: create or update multiple tasks atomically.
	// A typo'd wrapper (e.g. "taks") errors;
	// an absent or empty tasks array also errors (nothing to upsert is a mistake).
	upsertBatchCmd := &cobra.Command{
		Use:         "upsert-batch {tasks: [<task>]}",
		Short:       "create or update several tasks in one atomic write",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Create or update multiple tasks in one atomic write.
An absent or empty "tasks" array is an error. Each task element uses the same fields as
"lym board upsert" ("slug" required per element).
The "notice" names siblings sharing a new depends_on and a label.

Required wrapper field:
  "tasks" array — one or more task objects (each with "slug" required)

Example:
  lym board upsert-batch '{"tasks":[{"slug":"t1","title":"One","kind":"task","labels":["bug"]},{"slug":"t2","title":"Two","kind":"task","labels":["bug"]}]}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// Decode into a map to detect unknown wrapper keys.
			var raw map[string]any
			if err := json.Unmarshal([]byte(args[0]), &raw); err != nil {
				return outputError(out, fmt.Sprintf("invalid json: %v", err))
			}

			if err := refuseUnknownKey(upsertBatchPayload, raw); err != nil {
				return outputError(out, err.Error())
			}

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
			var notices []string
			for i, v := range tasksArr {
				m, ok := v.(map[string]any)
				if !ok {
					return outputError(out, fmt.Sprintf("tasks[%d] must be an object", i))
				}
				notice, err := prepareRunSettings(board(), m, strandNameFromEnv(), profiles)
				if err != nil {
					return outputError(out, fmt.Sprintf("tasks[%d]: %v", i, err))
				}
				if notice != "" {
					notices = append(notices, notice)
				}
				tasks[i] = m
			}

			siblings, err := board().UpsertTasksBatch(tasks)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithCount(out, len(tasks), joinNotices(append(notices, siblingNotice(siblings))...))
		}),
	}

	// set-status subcommand: set or clear the status field of a task identified by slug or numeric id.
	setStatusCmd := &cobra.Command{
		Use:         "set-status {slug|id|handle, status}",
		Short:       "set the status of a task, or release it with null",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Set the lifecycle status of a task. A new task is "draft"; null releases it, leaving it with no status, and an empty string on a task gives "draft" again.
Null on a project with no status also releases its draft member tasks in the same write, and the notice names them.
Null on a project with a run status or done is refused, naming the release of each draft member.

Fields:
  "slug"   string      — task slug (one of slug, id or handle)
  "id"     integer     — numeric task ID (one of slug, id or handle)
  "handle" string      — task handle, such as T4 (one of slug, id or handle)
  "status" string|null — new status value; null releases a draft task, or a task an older binary left discussed

Examples:
  lym board set-status '{"slug":"my-task","status":"active"}'
  lym board set-status '{"id":96,"status":null}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			// An absent status key is refused while an explicit null clears the status, so a typo cannot silently clear it.
			selector, m, err := resolveLookup([]byte(args[0]), setStatusPayload)
			if err != nil {
				return outputError(out, err.Error())
			}
			var status *string
			if sv := m["status"]; sv != nil {
				s, ok := sv.(string)
				if !ok {
					return outputError(out, "status must be a string or null")
				}
				status = &s
			}

			released, err := board().SetStatusReleasingProject(selector, status)
			if err != nil {
				return outputError(out, err.Error())
			}
			if len(released) == 0 {
				return outputSuccess(out)
			}
			return outputSuccessWithNotice(out, "released the project's draft member tasks with it: "+strings.Join(released, ", "))
		}),
	}

	removeCmd := &cobra.Command{
		Use:         "remove {slug|id|handle}",
		Short:       "remove a task",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Remove a task by slug, numeric ID or handle. Errors if the task is not found.
Its handle's number is free for the next entry of its letter.

Fields:
  "slug"   string  — task slug (one of slug, id or handle)
  "id"     integer — numeric task ID (one of slug, id or handle)
  "handle" string  — task handle, such as T4 (one of slug, id or handle)

Examples:
  lym board remove '{"slug":"my-task"}'
  lym board remove T4` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}
			selector, _, err := resolveLookup([]byte(args[0]), lookupPayload)
			if err != nil {
				return outputError(out, err.Error())
			}
			if err := board().RemoveTask(selector); err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}

	// get writes task:null for a valid payload whose target is absent, which is not an error.
	// A malformed payload is an error.
	var getBody, getRecord bool
	getCmd := &cobra.Command{
		Use:         "get {slug|id|handle}",
		Short:       "fetch a single task, or its body or decision record alone",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Fetch a single task by slug, numeric ID or handle. Returns {"task":null} if not found (not an error).
The result is the envelope {"task": {...}}, which the discussion stencil reads.
A task's "owner_machine" is the machine that owns it: the owner it reads as through its beam, or through root.
--body writes the entry's body alone, verbatim, with no envelope; an empty body writes nothing.
With --body a target that does not exist is an error, since there is no task:null to print.
A task whose run's Discussion was approved carries "discussed": true, and --record writes its stored
decision record alone, verbatim, with no envelope. --record is an error for an absent entry, an entry
that is not discussed, a discussed entry whose record file is missing, and beside --body.
Editing a body as a file: lym board get ... --body > body.md, edit it, then feed it back with
upsert --body-file body.md (merge --body-file takes the same file for a merged entry).

Fields:
  "slug"   string  — task slug (one of slug, id or handle)
  "id"     integer — numeric task ID (one of slug, id or handle)
  "handle" string  — task handle, such as B4 (one of slug, id or handle)

Examples:
  lym board get '{"id":96}'
  lym board get B4
  lym board get '{"slug":"my-task"}' --body
  lym board get '{"slug":"my-task"}' --record` + slugLimitNote(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clihelp.WrapRun(func(out io.Writer, args []string) int {
				if len(args) == 0 {
					return outputError(out, "json payload required")
				}
				if getBody && getRecord {
					return outputError(out, "--body and --record each print one field: pass one of them")
				}
				selector, _, err := resolveLookup([]byte(args[0]), lookupPayload)
				if err != nil {
					return outputError(out, err.Error())
				}
				task, found, err := board().GetTask(selector)
				if err != nil {
					return outputError(out, err.Error())
				}
				if getBody {
					if !found {
						return outputError(out, fmt.Sprintf("no entry %v: check the slug, id or handle with lym board list", selector))
					}
					if _, err := io.WriteString(out, task.Body); err != nil {
						return outputError(out, err.Error())
					}
					return 0
				}
				if getRecord {
					if !found {
						return outputError(out, fmt.Sprintf("no entry %v: check the slug, id or handle with lym board list", selector))
					}
					if !task.Discussed {
						return outputError(out, fmt.Sprintf("entry %q has no decision record: lym stores one when its run's Discussion is approved, or with `lym loom decision store` on a run past that approval", task.Slug))
					}
					record, held, err := board().DecisionRecord(task.Slug)
					if err != nil {
						return outputError(out, err.Error())
					}
					if !held {
						return outputError(out, fmt.Sprintf("entry %q is flagged discussed but has no %s: after a rename the record may remain at the old slug's %s, and the entry's next Discussion approval stores it again", task.Slug, boardengine.DecisionFileName(task.Slug), boardengine.DecisionFileName("<slug>")))
					}
					if _, err := out.Write(record); err != nil {
						return outputError(out, err.Error())
					}
					return 0
				}
				if found {
					if task.OwnerMachine == "" && (task.Kind == boardengine.KindTask || task.Kind == boardengine.KindProject) {
						if task.OwnerMachine, err = board().EffectiveOwner(task.Slug); err != nil {
							return outputError(out, err.Error())
						}
					}
					parentBeam, err := board().ParentBeamOf(task.Slug)
					if err != nil {
						return outputError(out, err.Error())
					}
					return outputGetTask(out, &task, parentBeam)
				}
				return outputGetTask(out, nil, "") // task: null in JSON output
			})(cmd, args)
		},
	}
	getCmd.Flags().BoolVar(&getBody, "body", false, "write the entry's body alone, verbatim, instead of the JSON envelope")
	getCmd.Flags().BoolVar(&getRecord, "record", false, "write the entry's stored decision record alone, verbatim, instead of the JSON envelope")

	var listText bool
	var listLabels []string
	var listSort string
	listCmd := &cobra.Command{
		Use:         "list",
		Short:       "list all tasks with computed fields",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `List all tasks in README order with their computed fields (layer, has_proposal),
beside "prime_owner", the machine that owns the hub's prime ("" when none is recorded).
A task's "owner_machine" is its effective owner, its beam's or root's; only a beam project records one.
With --text, print a "prime owner: <machine>" line ("none" when none is recorded),
then the compact one-line-per-task listing (handle, kind, priority, age, time since last revision, slug, title, labels, [status])
instead of JSON; errors stay JSON.
--label <name> keeps only entries carrying that label; repeat it to require several (AND).
A label that is in neither the types nor the labels list of board.yaml and that no entry carries is refused.
--sort age lists the oldest entries first, --sort revised the longest unrevised first;
an entry without the date goes last, and entries that tie keep README order.

Examples:
  lym board list
  lym board list --text
  lym board list --text --sort age
  lym board list --label bug --label enhancement`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := board().ListTasksBrief(listLabels)
			if err != nil {
				return outputError(out, err.Error())
			}
			if err := sortByDate(tasks, listSort); err != nil {
				return outputError(out, err.Error())
			}
			primeOwner, err := board().PrimeOwner()
			if err != nil {
				return outputError(out, err.Error())
			}
			if !listText {
				return output.Ok(out, map[string]any{"prime_owner": primeOwner, "tasks": tasks})
			}
			if _, err := io.WriteString(out, primeOwnerLine(primeOwner)+RenderCompact(tasks, time.Now())); err != nil {
				return outputError(out, err.Error())
			}
			return 0
		}),
	}
	listCmd.Flags().BoolVar(&listText, "text", false, "print the compact one-line-per-task listing instead of JSON")
	listCmd.Flags().StringVar(&listSort, "sort", "", "order by date instead of README order: age (oldest first) or revised (longest unrevised first)")
	listCmd.Flags().StringArrayVar(&listLabels, "label", nil, "keep only entries carrying this label; repeatable, all must match")

	listFullCmd := &cobra.Command{
		Use:         "list-full",
		Short:       "list all tasks as stored in board.json",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			tasks, err := board().ListTasksFull()
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputListFull(out, tasks)
		}),
	}

	// merge folds entries together: it removes slugs, upserts one task, and sets a status, in one atomic write.
	// The set_status object is checked with the same payload rules as a status change.
	var mergeBodyFile string
	mergeCmd := &cobra.Command{
		Use:         "merge {remove_slugs?, upsert: <task>, set_status?: {slug|id|handle, status}}",
		Short:       "remove tasks, upsert one and set a status in one atomic write, to fold entries together",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Remove tasks, upsert a task, and optionally set status in one atomic write.
The inner "set_status" object takes what the standalone "set-status" command takes.
The "notice" names siblings sharing a new depends_on and a label.

Fields:
  "remove_slugs" array  — slug strings to remove (optional; omit to skip)
  "upsert"       object — task to create or update (required; same fields as "lym board upsert")
  "set_status"   object — status to set after upsert (optional):
    "slug"   string      — task slug (one of slug, id or handle)
    "id"     integer     — numeric task ID (one of slug, id or handle)
    "handle" string      — task handle, such as T4 (one of slug, id or handle)
    "status" string|null — new status; null releases a draft task, or a task an older binary left discussed

A task the merge creates is "draft", and a "set_status" of null releases it.

Flag:
  --body-file <path>  read the upsert's "body" from the file, or from stdin when the path is "-"; every other field still comes from the payload.
                      Refused when the payload's "upsert" also carries "body" (drop one of them),
                      and when the payload argument is itself "-" (stdin can feed only one of them).

Example:
  lym board merge '{"remove_slugs":["old"],"upsert":{"slug":"new","title":"New"},"set_status":{"slug":"new","status":"active"}}'` + slugLimitNote(),
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

				// A stale set_phase errors rather than being silently dropped,
				// which would skip the status step with no feedback.
				if err := refuseUnknownKey(mergePayload, raw); err != nil {
					return outputError(out, err.Error())
				}

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

				// Parse set_status (optional) with the key declaration that set-status uses.
				var setStatusPtr *boardengine.MergeStatusUpdate
				if ssVal, ok := raw["set_status"]; ok && ssVal != nil {
					ssBytes, err := json.Marshal(ssVal)
					if err != nil {
						return outputError(out, fmt.Sprintf("set_status: marshal error: %v", err))
					}
					selector, ssMap, err := resolveLookup(ssBytes, mergePayload.nested["set_status"])
					if err != nil {
						return outputError(out, "set_status: "+err.Error())
					}
					var status *string
					if sv := ssMap["status"]; sv != nil {
						s, ok := sv.(string)
						if !ok {
							return outputError(out, "set_status.status must be a string or null")
						}
						status = &s
					}
					setStatusPtr = &boardengine.MergeStatusUpdate{Selector: selector, Status: status}
				}

				notice, err := prepareRunSettings(board(), upsertFields, strandNameFromEnv(), profiles)
				if err != nil {
					return outputError(out, err.Error())
				}
				task, siblings, err := board().MergeTasks(removeSlugs, upsertFields, setStatusPtr)
				if err != nil {
					return outputError(out, err.Error())
				}
				return outputSuccessWithTask(out, task, joinNotices(notice, siblingNotice(siblings)))
			})(cmd, args)
		},
	}
	mergeCmd.Flags().StringVar(&mergeBodyFile, "body-file", "", `read the upsert's "body" from this file, or from stdin when "-"`)

	// set-deps subcommand: replace the depends_on list for a task.
	// depends_on is required: an absent key is an error, while an explicit [] clears the list.
	// That separates an intentional clear from a typo that would otherwise silently wipe the list.
	setDepsCmd := &cobra.Command{
		Use:         "set-deps {slug|id|handle, depends_on}",
		Short:       "replace a task's depends_on list",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Replace the full depends_on list for a task wholesale.
An absent "depends_on" is an error; an explicit [] clears the list.
The "notice" names siblings sharing a new depends_on and a label.

Fields:
  "slug"       string  — task slug (one of slug, id or handle)
  "id"         integer — numeric task ID (one of slug, id or handle)
  "handle"     string  — task handle, such as T4 (one of slug, id or handle)
  "depends_on" array   — complete list of dependency slug strings; replaces existing list (required)

Example:
  lym board set-deps '{"slug":"my-task","depends_on":["dep-a","dep-b"]}'` + slugLimitNote(),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			if len(args) == 0 {
				return outputError(out, "json payload required")
			}

			// A typo ("depends") is refused as an unknown key instead of silently clearing the list.
			selector, m, err := resolveLookup([]byte(args[0]), setDepsPayload)
			if err != nil {
				return outputError(out, err.Error())
			}

			var dependsOn []string
			if depsVal := m["depends_on"]; depsVal != nil {
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

			siblings, err := board().SetDeps(selector, dependsOn)
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccessWithNotice(out, siblingNotice(siblings))
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

// The orders `list --sort` accepts: oldest creation first, or longest since the last revision first.
const (
	sortByAge     = "age"
	sortByRevised = "revised"
)

// sortByDate reorders tasks stably by the date order names, oldest first, with an unknown date last;
// an empty order leaves tasks as they are, and any other is refused.
func sortByDate(tasks []boardengine.BriefTask, order string) error {
	var date func(boardengine.BriefTask) time.Time
	switch order {
	case "":
		return nil
	case sortByAge:
		date = func(t boardengine.BriefTask) time.Time { return t.Created }
	case sortByRevised:
		date = func(t boardengine.BriefTask) time.Time { return t.Revised }
	default:
		return fmt.Errorf("sort %q is not one of %s, %s: pass one of them, or drop --sort for README order", order, sortByAge, sortByRevised)
	}
	slices.SortStableFunc(tasks, func(a, b boardengine.BriefTask) int {
		da, db := date(a), date(b)
		if da.IsZero() != db.IsZero() {
			if da.IsZero() {
				return 1
			}
			return -1
		}
		return da.Compare(db)
	})
	return nil
}

// writeListing prints tasks as the compact listing when text is set and as the JSON list envelope otherwise.
func writeListing(out io.Writer, tasks []boardengine.BriefTask, text bool) int {
	if !text {
		return outputListBrief(out, tasks)
	}
	if _, err := io.WriteString(out, RenderCompact(tasks, time.Now())); err != nil {
		return outputError(out, err.Error())
	}
	return 0
}

// resolveLookup decodes a payload that addresses one entry by slug, id or handle.
// It validates the payload against keys and returns the selector and the decoded map.
// keys declares the slug|id|handle group and any further keys;
// each of its required keys must be present, and a null value counts as present.
// A raw payload that is a bare handle, such as B4, reads as {"handle":"B4"}.
func resolveLookup(raw []byte, keys payloadKeys) (any, map[string]any, error) {
	var m map[string]any
	if _, _, ok := boardengine.ParseHandle(string(raw)); ok {
		m = map[string]any{"handle": string(raw)}
	} else if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, fmt.Errorf("invalid json: %v", err)
	}
	if err := refuseUnknownKey(keys, m); err != nil {
		return nil, nil, err
	}
	selector, err := lookupSelector(m)
	if err != nil {
		return nil, nil, err
	}
	for _, key := range keys.required {
		if _, ok := m[key]; !ok {
			return nil, nil, fmt.Errorf("missing required field: %s", key)
		}
	}
	return selector, m, nil
}

// lookupSelector returns the selector that m addresses.
// The selector is the slug string, the integral float64 id, or the boardengine.Handle.
// It refuses a payload that has none of the three keys or more than one.
func lookupSelector(m map[string]any) (any, error) {
	given := 0
	for _, key := range entrySelector {
		if _, ok := m[key]; ok {
			given++
		}
	}
	if given == 0 {
		return nil, fmt.Errorf("one of slug, id or handle is required")
	}
	if given > 1 {
		return nil, fmt.Errorf("only one of slug, id or handle may be given")
	}

	if handleVal, hasHandle := m["handle"]; hasHandle {
		handle, ok := handleVal.(string)
		if !ok {
			return nil, fmt.Errorf("handle must be a string")
		}
		if _, _, ok := boardengine.ParseHandle(handle); !ok {
			return nil, fmt.Errorf("handle %q is not a letter followed by a number, such as T4: check it with lym board list --text", handle)
		}
		return boardengine.Handle(handle), nil
	}

	if _, hasSlug := m["slug"]; hasSlug {
		slugStr, ok := m["slug"].(string)
		if !ok || slugStr == "" {
			return nil, fmt.Errorf("slug must be a non-empty string")
		}
		return slugStr, nil
	}

	switch v := m["id"].(type) {
	case float64:
		if v != math.Trunc(v) {
			return nil, fmt.Errorf("id must be an integer")
		}
		return v, nil
	default:
		return nil, fmt.Errorf("id must be a number")
	}
}

// RunCLI runs the board module CLI against the process working directory, writing to out, and returns the exit code.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the board module CLI with cwd as its working directory.
// An empty cwd reads the process working directory through clihelp.Execute,
// and any other value seeds cwd into the execution context through clihelp.ExecuteIn.
// The empty case cannot go through ExecuteIn, because lymcwd.WithCwd panics on an empty directory.
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

// outputSuccessWithNotice writes {"ok":true} with exit code 0, plus a "notice" field when notice is not empty.
func outputSuccessWithNotice(out io.Writer, notice string) int {
	envelope := map[string]any{}
	if notice != "" {
		envelope["notice"] = notice
	}
	return output.Ok(out, envelope)
}

// siblingNotice formats siblings as one line per pair, joined by newlines, and is empty when there are none.
// Each line names the written task, the dependencies it newly waits on, and the sibling that waits on them too.
// The line also names the module labels the two share.
func siblingNotice(siblings []boardengine.Sibling) string {
	lines := make([]string, len(siblings))
	for i, sibling := range siblings {
		lines[i] = fmt.Sprintf(
			`%q now waits on %s, which %q also waits on, and the two share module labels %s: chain one after the other with "lym board set-deps", or leave both if they do not overlap`,
			sibling.Task, strings.Join(sibling.Deps, ", "), sibling.Sibling, strings.Join(sibling.Labels, ", "))
	}
	return strings.Join(lines, "\n")
}

// refuseNoticeOnAbsentEntry refuses a payload that only asks for a landing notice on a slug the board does not hold.
// Such a payload has two fields, slug and notify_on_land set to true.
// Land prunes an entry once its run lands, so such an upsert would file a new entry for a run that already landed.
func refuseNoticeOnAbsentEntry(board *boardengine.Board, fields map[string]any) error {
	slug, _ := fields["slug"].(string)
	if fields["notify_on_land"] != true || len(fields) != 2 || slug == "" {
		return nil
	}
	_, found, err := board.GetTask(slug)
	if err != nil || found {
		return err
	}
	return fmt.Errorf("no entry %q is on the board, and Land prunes an entry once its run lands, so a run of that slug already landed or the slug is wrong: lym batten status %s shows the run; nothing was written", slug, slug)
}

// landedNotice returns the notice for an upsert whose payload sets notify_on_land on a done entry.
// Such an entry's run already landed, so its parent's dobby handles the request at its next tick.
// Any other write gets an empty notice.
func landedNotice(task boardengine.Task, fields map[string]any) string {
	if fields["notify_on_land"] != true || !task.NotifyOnLand || !boardengine.IsDoneStatus(task.Status) {
		return ""
	}
	return fmt.Sprintf("%q is done, so its run already landed: its parent's dobby sends the landing line at its next tick, or only clears notify_on_land when land_notice is off", task.Slug)
}

// joinNotices joins its non-empty notices with newlines.
func joinNotices(notices ...string) string {
	nonEmpty := make([]string, 0, len(notices))
	for _, notice := range notices {
		if notice != "" {
			nonEmpty = append(nonEmpty, notice)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

// outputSuccessWithCount writes {"ok":true,"count":N} with exit code 0, plus a "notice" field when notice is not empty.
func outputSuccessWithCount(out io.Writer, count int, notice string) int {
	envelope := map[string]any{"count": count}
	if notice != "" {
		envelope["notice"] = notice
	}
	return output.Ok(out, envelope)
}

// outputSuccessWithTask writes {"ok":true,"task":{...}} with exit code 0.
// It adds a "notice" field when notice is not empty.
func outputSuccessWithTask(out io.Writer, task boardengine.Task, notice string) int {
	envelope := map[string]any{"task": task}
	if notice != "" {
		envelope["notice"] = notice
	}
	return output.Ok(out, envelope)
}

// outputGetTask writes {"ok":true,"task":{...},"parent_beam":"..."} with task as a pointer.
// A nil task writes null and omits parent_beam.
// parentBeam is the slug of the beam the entry belongs to, empty when the prime is its parent.
// The task carries "discussed": true when its run's Discussion was approved, and no discussed key otherwise.
func outputGetTask(out io.Writer, task *boardengine.Task, parentBeam string) int {
	envelope := map[string]any{"task": task}
	if task != nil {
		envelope["parent_beam"] = parentBeam
	}
	return output.Ok(out, envelope)
}

// outputListBrief writes {"ok":true,"tasks":[...]} with BriefTask objects with exit code 0.
func outputListBrief(out io.Writer, tasks []boardengine.BriefTask) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}

// outputListFull writes {"ok":true,"tasks":[...]} with full Task objects with exit code 0.
func outputListFull(out io.Writer, tasks []boardengine.Task) int {
	return output.Ok(out, map[string]any{"tasks": tasks})
}
