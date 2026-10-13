// peers.go implements the `peers` loom subtree: `lyx loom peers`, which lists the other in-flight runs of the hub beside the caller's own, and `lyx loom peers show`, which prints one peer's decision record or plan.
// An agent in one pair uses them to compare its run with the others and with the commits that landed on its parent, without forming another worktree's path itself.
//
// The peer set is every board task whose status is a run status, whatever its state, with the caller's own slug left out.
// Each peer is listed with its title, status, parent branch, the paths it changed since its merge base with that parent, and whether it holds a decision record and a plan.
// A peer whose pair, origin record, branch or records cannot be read is listed with the reason in `unreadable`, never dropped.
// From a task pair the list also carries the commits on the pair's parent branch since the pair forked.
//
// The verbs read only the board, the peers' records and git history through `internal/fabricengine`, and write nothing.
// The group carries its own PersistentPreRunE, and the loom parent's pre-run skips it, for the reason review.go gives.
// The location resolves through lyxcwd, not internal/cliwire, since every loom verb is hub-only.

package loomcli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

const (
	peersGroup = "peers"

	peersRecordDecision = "decision"
	peersRecordPlan     = "plan"

	peersWayList = `"lyx loom peers" lists the in-flight peers`
)

// peersNoBoardMessage is the refusal of both verbs in a hub with no board directory.
const peersNoBoardMessage = "loom: peers: this hub has no board, so no run is in flight beside this one; way forward: nothing to compare, and a standalone run's review stays with its parent"

// peerLine is one in-flight peer as `lyx loom peers` prints it.
type peerLine struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Row is the 1-based position of the status's producer in the loom recipe's rows, 0 when it names none.
	Row          int      `json:"row"`
	ParentBranch string   `json:"parent_branch"`
	ChangedPaths []string `json:"changed_paths"`
	// DecisionRecord and Plan report whether the peer holds that record.
	DecisionRecord bool `json:"decision_record"`
	Plan           bool `json:"plan"`
	// Unreadable lists what could not be read for this peer, one reason per entry.
	Unreadable []string `json:"unreadable"`
}

// peersDeps is every side effect the peers verbs perform, injected so tests supply fakes.
type peersDeps struct {
	// board returns the board's tasks, with false when the hub has no board directory.
	board func() (tasks []boardengine.Task, hasBoard bool, err error)
	// caller returns the caller's own slug, empty from the prime.
	caller func() (string, error)
	// readPeer fills the git and record fields of one peer's line; its identity fields are the verb's.
	readPeer func(slug string) peerLine
	// parentCommits returns the commits on the caller's parent branch since its pair forked, as "<short sha> <subject>" lines.
	parentCommits func() ([]string, error)
	// readRecord returns one peer's record of the given kind, with false when the peer does not hold it.
	readRecord func(slug, record string) (content string, found bool, err error)
	// loomRows is the loom recipe's row names in file order.
	loomRows []string
}

// recipeRow returns the 1-based position of status's producer among rows, and 0 when status is no run status or its producer names no row.
func recipeRow(status *string, rows []string) int {
	producer, ok := boardengine.RunStatusProducer(status)
	if !ok {
		return 0
	}
	if index := slices.Index(rows, producer); index >= 0 {
		return index + 1
	}
	return 0
}

// runStatusTasks returns the tasks of tasks that hold a run status, in board order.
func runStatusTasks(tasks []boardengine.Task) []boardengine.Task {
	var held []boardengine.Task
	for _, task := range tasks {
		if boardengine.IsRunStatus(task.Status) {
			held = append(held, task)
		}
	}
	return held
}

// peersVerb prints the hub's in-flight peers beside the caller, or refuses when the hub has no board.
func peersVerb(out io.Writer, deps peersDeps) int {
	tasks, hasBoard, err := deps.board()
	if err != nil {
		return output.Err(out, fmt.Sprintf("loom: peers: read the board: %s; way forward: fix the board the message names, then re-run \"lyx loom peers\"", err))
	}
	if !hasBoard {
		return output.Err(out, peersNoBoardMessage)
	}
	caller, err := deps.caller()
	if err != nil {
		return output.Err(out, fmt.Sprintf("loom: peers: resolve the caller: %s; way forward: re-run \"lyx loom peers\" from a worktree of this hub", err))
	}

	peers := []peerLine{}
	for _, task := range runStatusTasks(tasks) {
		if task.Slug == caller {
			continue
		}
		line := deps.readPeer(task.Slug)
		line.Slug, line.Title, line.Status = task.Slug, task.Title, *task.Status
		line.Row = recipeRow(task.Status, deps.loomRows)
		if line.ChangedPaths == nil {
			line.ChangedPaths = []string{}
		}
		if line.Unreadable == nil {
			line.Unreadable = []string{}
		}
		peers = append(peers, line)
	}

	fields := map[string]any{"peers": peers}
	if caller != "" {
		ownRow := 0
		for _, task := range tasks {
			if task.Slug == caller {
				ownRow = recipeRow(task.Status, deps.loomRows)
				break
			}
		}
		fields["own_row"] = ownRow
		commits, err := deps.parentCommits()
		if err != nil {
			return output.Err(out, fmt.Sprintf("loom: peers: read the parent's commits: %s; way forward: fix the pair's origin record or its parent branch, then re-run \"lyx loom peers\"", err))
		}
		if commits == nil {
			commits = []string{}
		}
		fields["parent_commits"] = commits
	}
	return output.Ok(out, fields)
}

// peersShowVerb prints one peer's record of the given kind under a content key, or refuses with a way forward.
func peersShowVerb(out io.Writer, deps peersDeps, slug, record string) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, "loom: peers show: "+fmt.Sprintf(format, args...))
	}

	tasks, hasBoard, err := deps.board()
	if err != nil {
		return refuse("read the board: %s; way forward: fix the board the message names, then re-run the verb", err)
	}
	if !hasBoard {
		return output.Err(out, peersNoBoardMessage)
	}
	if record != peersRecordDecision && record != peersRecordPlan {
		return refuse("unknown record %q; way forward: pass --record %s or --record %s", record, peersRecordDecision, peersRecordPlan)
	}
	caller, err := deps.caller()
	if err != nil {
		return refuse("resolve the caller: %s; way forward: re-run the verb from a worktree of this hub", err)
	}
	if slug == caller {
		return refuse("%q is this worktree's own run; way forward: read your own records in this worktree", slug)
	}

	var task *boardengine.Task
	for i := range tasks {
		if tasks[i].Slug == slug {
			task = &tasks[i]
		}
	}
	if task == nil {
		return refuse("unknown slug %q; way forward: %s", slug, peersWayList)
	}
	if !boardengine.IsRunStatus(task.Status) {
		return refuse("%q holds no run status, so its run is not in flight; way forward: %s", slug, peersWayList)
	}

	content, found, err := deps.readRecord(slug, record)
	if err != nil {
		return refuse("read the %s of %q: %s; way forward: %s", record, slug, err, peersWayList)
	}
	if !found {
		return refuse("%q has no %s yet; way forward: \"lyx loom peers\" shows which records exist", slug, record)
	}
	return output.Ok(out, map[string]any{"slug": slug, "record": record, "content": content})
}

// callerSlug returns the slug of the caller's own task pair, empty when location is the prime.
func callerSlug(location *lyxcwd.Location) (string, error) {
	prime, err := fabricengine.PrimeName(location)
	if err != nil {
		return "", err
	}
	if location.WorktreeName == prime {
		return "", nil
	}
	return location.WorktreeName, nil
}

// peerRecordPath returns the path of the peer's record of the given kind, assuming record is a known kind.
func peerRecordPath(peer *lyxcwd.Location, record string) string {
	if record == peersRecordDecision {
		return loomengine.DiscussionDecisionRecord(peer)
	}
	return planparser.PlanOverview(peer.AnchorPath())
}

// recordExists reports whether path is a file, and an error for any stat failure other than absence.
func recordExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// hubPeersDeps fills peersDeps for the resolved location from the hub's board, its pairs' origin records, git history and run records.
// The loom rows come from the recipe the running binary embeds; a parse failure there is a defect of the binary, so it panics.
func hubPeersDeps(location *lyxcwd.Location) peersDeps {
	loomRows, err := loomrecipe.RowNames(shedrun.RecipeLoom)
	if err != nil {
		panic(err)
	}
	return peersDeps{
		loomRows: loomRows,
		board: func() ([]boardengine.Task, bool, error) {
			info, err := os.Stat(fabricengine.BoardDir(location.HubPath))
			if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()) {
				return nil, false, nil
			}
			if err != nil {
				return nil, false, err
			}
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return nil, false, err
			}
			tasks, err := board.ListTasksFull()
			return tasks, true, err
		},
		caller: func() (string, error) { return callerSlug(location) },
		readPeer: func(slug string) peerLine {
			return readPeerLine(location, slug)
		},
		parentCommits: func() ([]string, error) {
			caller, err := callerSlug(location)
			if err != nil || caller == "" {
				return nil, err
			}
			parent, err := readRecordedParentBranch(location)
			if err != nil {
				return nil, err
			}
			pair, err := fabricengine.Open(location)
			if err != nil {
				return nil, err
			}
			base, err := pair.MergeBase(parent)
			if err != nil {
				return nil, err
			}
			return pair.CommitLog(base, parent)
		},
		readRecord: func(slug, record string) (string, bool, error) {
			peer, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, slug))
			if err != nil {
				return "", false, err
			}
			content, err := os.ReadFile(peerRecordPath(peer, record))
			if errors.Is(err, fs.ErrNotExist) {
				return "", false, nil
			}
			if err != nil {
				return "", false, err
			}
			return string(content), true, nil
		},
	}
}

// readPeerLine fills the parent branch, changed paths and record flags of one peer's line, naming in Unreadable every part it could not read.
func readPeerLine(location *lyxcwd.Location, slug string) peerLine {
	var line peerLine
	unreadable := func(part string, err error) {
		line.Unreadable = append(line.Unreadable, fmt.Sprintf("%s: %s", part, err))
	}

	root := fabricengine.WorktreePath(location, slug)
	if _, err := os.Stat(root); err != nil {
		unreadable("pair", fmt.Errorf("no worktree at %s", filepath.ToSlash(root)))
		return line
	}
	peer, err := lyxcwd.ResolveWorktree(root)
	if err != nil {
		unreadable("pair", err)
		return line
	}

	for _, record := range []struct {
		kind string
		flag *bool
	}{{peersRecordDecision, &line.DecisionRecord}, {peersRecordPlan, &line.Plan}} {
		exists, err := recordExists(peerRecordPath(peer, record.kind))
		if err != nil {
			unreadable(record.kind, err)
		}
		*record.flag = exists
	}

	origin, found, err := fabricengine.ReadOriginFor(location, slug)
	switch {
	case err != nil:
		unreadable("branch", err)
		return line
	case !found:
		unreadable("branch", errors.New("the pair has no fabric origin record"))
		return line
	}
	line.ParentBranch = origin.ParentBranch

	pair, err := fabricengine.Open(peer)
	if err != nil {
		unreadable("changed_paths", err)
		return line
	}
	base, err := pair.MergeBase(origin.ParentBranch)
	if err != nil {
		unreadable("changed_paths", err)
		return line
	}
	head, err := pair.HeadSHA()
	if err != nil {
		unreadable("changed_paths", err)
		return line
	}
	paths, err := pair.ChangedPaths(base, head)
	if err != nil {
		unreadable("changed_paths", err)
		return line
	}
	line.ChangedPaths = paths
	return line
}

// peersState carries what the peers group's pre-run resolved for its verb.
type peersState struct {
	deps   peersDeps
	record string
}

// resolveDeps resolves the invoking cwd to a location and fills the verbs' deps from it.
func (s *peersState) resolveDeps(cmd *cobra.Command) error {
	cwd, err := lyxcwd.CwdFrom(cmd.Context())
	if err != nil {
		return err
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return err
	}
	s.deps = hubPeersDeps(location)
	return nil
}

// preRun resolves the deps for `show`; the group itself resolves them in its own RunE so that a bare or mistyped group call outside a worktree lists the verbs.
func (s *peersState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == peersGroup {
		return nil
	}
	if err := s.resolveDeps(cmd); err != nil {
		output.Err(cmd.OutOrStdout(), err.Error())
		clihelp.Abort(cmd.Context(), 1)
	}
	return nil
}

// peersCmd builds the `peers` group, which lists the in-flight peers, with `show` beneath it.
func (c *loomCLI) peersCmd() *cobra.Command {
	st := &peersState{}

	group := &cobra.Command{
		Use:   peersGroup,
		Short: "list the other in-flight runs of this hub and the parent's landed commits",
		Long: `peers lists every other run in flight in this hub: each board task holding
a run status, whatever its state, except this worktree's own. Each peer line
carries the slug, title, status, row (the 1-based position of its producer in
the loom recipe, 0 when the recipe has no such row), parent branch, the paths
the peer changed since it forked, whether it holds a decision record and a
plan, and what could not be read. From a task worktree the output also gives
own_row, this run's own position by the same rule, and lists the commits on
this worktree's parent branch since it forked, as short SHA and subject,
newest first. It reads and writes nothing else, and refuses in a hub with no board.

Outside a lyx worktree, a bare call lists the verbs instead.

Example:
  lyx loom peers
  lyx loom peers show <slug> --record decision`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if len(args) > 0 {
				return clihelp.GroupRunE(cmd, args)
			}
			if err := st.resolveDeps(cmd); err != nil {
				return cmd.Help()
			}
			clihelp.SetExit(cmd.Context(), peersVerb(cmd.OutOrStdout(), st.deps))
			return nil
		},
		PersistentPreRunE: st.preRun,
	}

	show := &cobra.Command{
		Use:         "show <slug> --record <value>",
		Short:       "print one peer's decision record or plan",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `show prints the named peer's record under a content key: the decision record
for --record decision, the plan's 00-overview.md for --record plan. It refuses
an unknown slug, a task holding no run status, this worktree's own slug, an
unknown record name and a record the peer does not have yet, each naming its
way forward.

Example:
  lyx loom peers show <slug> --record plan`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), peersShowVerb(cmd.OutOrStdout(), st.deps, args[0], st.record))
			return nil
		},
	}
	show.Flags().StringVar(&st.record, "record", "", "the record to print: decision or plan")
	_ = show.MarkFlagRequired("record")

	group.AddCommand(show)
	return group
}
