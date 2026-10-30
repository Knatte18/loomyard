//go:build tier3

// smoke_llm_test.go proves creel's delivery into real Claude sessions, which the fake provider of the tmux tier cannot reach: a strand added by hand with its typed lines, and sessions started by shuttle's own launch with creel's hook delivery.

package creelcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
)

// TestSmokeCreelClaudeStrand adds a strand running claude to a hub and drives creel into it.
// The adapter reads the booted session idle, a posted message's carried line and an inject are typed at idle, and the agent's pane shows both.
func TestSmokeCreelClaudeStrand(t *testing.T) {
	claudePath := llmkit.Claude(t, "LYM_REED_CLAUDE")
	// The strand name in the environment is process-global state, set by newSmokeHub.
	smoke := newSmokeHub(t)
	name, guid := smoke.addStrand(t, "smokeclaude", claudePath)
	smoke.startDaemon(t, shuttleengines.New)

	delivery := newHubDelivery(smoke.hub.Path, shuttleengines.New)
	strands, err := delivery.Strands()
	if err != nil {
		t.Fatalf("Strands() = %v", err)
	}
	var strand creelengine.Strand
	for _, candidate := range strands {
		if candidate.GUID == guid {
			strand = candidate
		}
	}
	if strand.GUID != guid || !strand.Live {
		t.Fatalf("Strands() = %+v; want the claude strand %s live", strands, guid)
	}
	// A fresh temp hub shows claude's folder-trust gate, which covers the input box.
	// The pane has no event to wait on, so it is polled on a tick under a real-time deadline until the gate is accepted or the box shows.
	// Accepting the gate adds the temp hub's path to the operator's Claude trust list.
	deadline := time.Now().Add(smokePaneWindow)
	for {
		ops, err := wireShuttle(smoke.hub.Location, shuttleengines.New)
		if err != nil {
			t.Fatalf("wireShuttle() = %v", err)
		}
		accepted, err := ops.runner.AcceptStartupGate(guid)
		if err != nil {
			t.Fatalf("AcceptStartupGate() = %v", err)
		}
		answer, err := delivery.Idle(strand)
		if err != nil {
			t.Fatalf("Idle() = %v", err)
		}
		if accepted || answer.Stall != creelengine.StallNoInputBox {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the claude pane showed no input box and no startup gate; pane:\n%s", smoke.lastPane(t, guid))
		}
		time.Sleep(time.Second)
	}
	// The booted session has no event to wait on, so the idle check is polled on a tick under a real-time deadline.
	awaitIdle := func() {
		t.Helper()
		deadline := time.Now().Add(smokePaneWindow)
		for {
			answer, err := delivery.Idle(strand)
			if err != nil {
				t.Fatalf("Idle() = %v", err)
			}
			if answer.Idle {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the adapter never read the session idle; pane:\n%s", smoke.lastPane(t, guid))
			}
			time.Sleep(time.Second)
		}
	}
	awaitIdle()

	if code, out := smoke.creel("post", name, "creel smoke message"); code != 0 {
		t.Fatalf("lym creel post = %d; output: %s", code, out)
	}
	// The test process posts as EXTERNAL, whose mail is carried in its own line rather than reminded.
	if capture, ok := smoke.awaitPane(t, guid, "[creel] #1 from "); !ok {
		t.Fatalf("the carried line was not typed into the agent's pane; pane:\n%s", capture)
	}
	code, out := smoke.creel("inject", "--wait", "--timeout", "120s", name, "creel smoke inject")
	if code != 0 || !strings.Contains(out, `"outcome":"injected"`) {
		t.Fatalf("lym creel inject --wait = %d, %s; want exit 0 injected", code, out)
	}
	if capture, ok := smoke.awaitPane(t, guid, "creel smoke inject"); !ok {
		t.Fatalf("the agent's pane does not show the injected line; pane:\n%s", capture)
	}

	// Each command either lands or is typed without a confirmed landing; neither busy nor any other error is a result the watcher can act on.
	for _, command := range []creelengine.Command{
		{Name: creelengine.CommandReloadPlugins},
		{Name: creelengine.CommandColor, Argument: "blue"},
		{Name: creelengine.CommandLoadSkills, Argument: "scribe:prose"},
		{Name: creelengine.CommandCompact, Argument: "keep the smoke test"},
		{Name: creelengine.CommandClear},
	} {
		awaitIdle()
		err := delivery.Command(strand, command)
		switch {
		case err == nil:
			t.Logf("command %s landed", command.Name)
		case errors.Is(err, creelengine.ErrNotLanded):
			t.Logf("command %s was typed and not confirmed", command.Name)
		default:
			t.Fatalf("Command(%s) = %v; want nil or ErrNotLanded", command.Name, err)
		}
	}
}

// smokeTurnWindow bounds how long a Claude scenario waits for a turn, a delivery or a file before it fails.
const smokeTurnWindow = 4 * time.Minute

// startClaudeRun starts a Claude session through a shuttle runner over the hub's reed engine, with claudeengine told the hub's config directories, so its settings carry the creel hooks and its strand the CreelHooks mark exactly as production renders them.
// It stops the run when the test ends and returns the strand's name and guid and the run's events file.
func (s *smokeHub) startClaudeRun(t *testing.T, spec shuttleengine.Spec) (name, guid, eventsPath string) {
	t.Helper()
	ops, err := wireShuttle(s.hub.Location, shuttleengines.New)
	if err != nil {
		t.Fatalf("wireShuttle() = %v", err)
	}
	run, err := ops.runner.Start(spec)
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}
	t.Cleanup(func() { _ = run.Stop() })
	return run.StrandName(), run.StrandGUID(), filepath.Join(run.RunDir(), "events.jsonl")
}

// claudeScenario is one Claude session of the hook delivery smoke test with the files it is told to write.
type claudeScenario struct {
	t      *testing.T
	smoke  *smokeHub
	dir    string
	name   string
	guid   string
	events string
}

// newClaudeScenario starts an interactive Claude session on prompt, in fork mode when fork is set.
// The prompt names files under the scenario's directory, which s.file returns.
func newClaudeScenario(t *testing.T, smoke *smokeHub, prompt string, fork bool) *claudeScenario {
	t.Helper()
	dir := t.TempDir()
	spec := shuttleengine.Spec{
		Prompt:        strings.ReplaceAll(prompt, "{dir}", dir),
		OutputFiles:   []string{filepath.Join(dir, "run-output-never-written")},
		Interactive:   true,
		ForkSubagents: fork,
	}
	name, guid, events := smoke.startClaudeRun(t, spec)
	return &claudeScenario{t: t, smoke: smoke, dir: dir, name: name, guid: guid, events: events}
}

// file returns the path of a file the scenario's agent is told to write.
func (c *claudeScenario) file(name string) string { return filepath.Join(c.dir, name) }

// fail logs the pane and the events file's path and content, which a failed scenario keeps, and ends the test.
func (c *claudeScenario) fail(format string, args ...any) {
	c.t.Helper()
	events, _ := os.ReadFile(c.events)
	c.t.Fatalf("%s\npane:\n%s\nevents file %s:\n%s", fmt.Sprintf(format, args...), c.smoke.lastPane(c.t, c.guid), c.events, events)
}

// await polls cond once a second until it holds or smokeTurnWindow passes.
// A session has no event to wait on, so it polls on a tick under a real-time deadline.
func (c *claudeScenario) await(what string, cond func() bool) {
	c.t.Helper()
	deadline := time.Now().Add(smokeTurnWindow)
	for !cond() {
		if time.Now().After(deadline) {
			c.fail("timed out waiting for %s", what)
		}
		time.Sleep(time.Second)
	}
}

// awaitFile waits until the agent wrote the named file.
func (c *claudeScenario) awaitFile(name string) {
	c.t.Helper()
	c.await("the agent to write "+name, func() bool {
		_, err := os.Stat(c.file(name))
		return err == nil
	})
}

// awaitShown waits until the pane shows text.
func (c *claudeScenario) awaitShown(text string) {
	c.t.Helper()
	if _, ok := c.smoke.awaitPane(c.t, c.guid, text); !ok {
		c.fail("the pane never showed %q", text)
	}
}

// post posts body to the scenario's strand as EXTERNAL.
func (c *claudeScenario) post(args ...string) {
	c.t.Helper()
	if code, out := c.smoke.creel(append([]string{"post"}, args...)...); code != 0 {
		c.fail("lym creel post %v = %d; output: %s", args, code, out)
	}
}

// eventRecords reads the events file as one map per line, skipping a line that is not a JSON object.
func (c *claudeScenario) eventRecords() []map[string]any {
	data, err := os.ReadFile(c.events)
	if err != nil {
		return nil
	}
	var records []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil {
			records = append(records, record)
		}
	}
	return records
}

// payloads counts the hook payloads of the named event the events file holds.
func (c *claudeScenario) payloads(event string) int {
	count := 0
	for _, record := range c.eventRecords() {
		if record["hook_event_name"] == event {
			count++
		}
	}
	return count
}

// stamps counts the hook-side stamp lines of the named event: the two-key lines that name the event and carry no payload.
func (c *claudeScenario) stamps(event string) int {
	count := 0
	for _, record := range c.eventRecords() {
		if _, isPayload := record["hook_event_name"]; isPayload || len(record) != 2 {
			continue
		}
		for _, value := range record {
			if value == event {
				count++
				break
			}
		}
	}
	return count
}

// promptsContaining counts the UserPromptSubmit payloads whose prompt contains text.
func (c *claudeScenario) promptsContaining(text string) int {
	count := 0
	for _, record := range c.eventRecords() {
		if prompt, _ := record["prompt"].(string); record["hook_event_name"] == "UserPromptSubmit" && strings.Contains(prompt, text) {
			count++
		}
	}
	return count
}

// requireNoPanePath fails when a notice or a reminder was typed as a prompt, which only a pane-mail strand gets.
func (c *claudeScenario) requireNoPanePath() {
	c.t.Helper()
	for _, typed := range []string{"[creel] #", " unread: #"} {
		if got := c.promptsContaining(typed); got != 0 {
			c.fail("%d prompts contain %q; a hook strand is typed no notice or reminder", got, typed)
		}
	}
}

// awaitTurnEnds waits until the events file holds n recorded turn ends.
func (c *claudeScenario) awaitTurnEnds(n int) {
	c.t.Helper()
	c.await(fmt.Sprintf("%d recorded turn ends", n), func() bool { return c.payloads("Stop") >= n })
}

// mainTranscript returns the path of the session's own transcript, the first transcript_path its events hold, or empty before any event carries one.
func (c *claudeScenario) mainTranscript() string {
	for _, record := range c.eventRecords() {
		if path, _ := record["transcript_path"].(string); path != "" {
			return path
		}
	}
	return ""
}

// transcriptCount counts the occurrences of text in the session's main transcript.
func (c *claudeScenario) transcriptCount(text string) int {
	path := c.mainTranscript()
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), text)
}

// requireTranscriptLacks fails when the main transcript holds any of texts, which stand for the hook command text and the error label a block would leave there.
func (c *claudeScenario) requireTranscriptLacks(texts ...string) {
	c.t.Helper()
	for _, text := range texts {
		if got := c.transcriptCount(text); got != 0 {
			c.fail("the transcript holds %q %d times; want none", text, got)
		}
	}
}

// requireWakeShown checks the session's wake-th idle wake once the model has acted on its mail, taking the wakes before it as delivered.
// The token is in the transcript with no turn end recorded since the earlier wake ended, which a turn that ran without the mail would have left.
// After the woken turn ends, the pane shows one `creel mail` row per wake so far and the echo under creelengine.EchoHeading, with the token under it once.
func (c *claudeScenario) requireWakeShown(token string, wake int) {
	c.t.Helper()
	c.await("the transcript to hold "+token, func() bool { return c.transcriptCount(token) > 0 })
	if got := c.payloads("Stop"); got != wake {
		c.fail("events hold %d Stop payloads when the wake of %s reached the transcript; want %d, so no turn ran before the mail", got, token, wake)
	}
	c.awaitTurnEnds(wake + 1)
	c.await("the pane to show the echo of "+token, func() bool { return paneEchoCount(c.smoke.lastPane(c.t, c.guid), token) >= 1 })
	pane := c.smoke.lastPane(c.t, c.guid)
	if got := strings.Count(pane, "creel mail"); got != wake {
		c.fail("the pane shows %d `creel mail` rows; want one per wake, %d", got, wake)
	}
	if got := paneEchoCount(pane, token); got != 1 {
		c.fail("the pane shows the echo of %s %d times; want once", token, got)
	}
}

// paneEchoCount counts the echoes of token in pane: the lines of creelengine.EchoHeading whose next non-empty line holds token.
func paneEchoCount(pane, token string) int {
	lines := strings.Split(pane, "\n")
	count := 0
	for i, line := range lines {
		if !strings.Contains(line, creelengine.EchoHeading) {
			continue
		}
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				continue
			}
			if strings.Contains(next, token) {
				count++
			}
			break
		}
	}
	return count
}

// requireHeadShownOnce checks the pane's stop-hook summary of a Stop claim of many lines: the first token appears once in the Stop hook feedback lines, there is no second `Stop hook feedback:` row, and no `Stop says:` line carries any token.
// The model's own reply and tool calls may quote a token, so only those summary lines are read.
func (c *claudeScenario) requireHeadShownOnce(first string, tokens []string) {
	c.t.Helper()
	lines := strings.Split(c.smoke.lastPane(c.t, c.guid), "\n")
	rows, inFeedback, headShown := 0, false, 0
	for _, line := range lines {
		if strings.Contains(line, "Stop hook feedback:") {
			rows++
			inFeedback = true
		} else if strings.TrimSpace(line) == "" {
			inFeedback = false
		}
		if inFeedback {
			headShown += strings.Count(line, first)
		}
		if strings.Contains(line, "Stop says:") {
			for _, token := range tokens {
				if strings.Contains(line, token) {
					c.fail("a `Stop says:` line carries %s; the operator is shown the head once, in the feedback row", token)
				}
			}
		}
	}
	if rows != 1 {
		c.fail("the pane shows %d `Stop hook feedback:` rows; want exactly one", rows)
	}
	if headShown != 1 {
		c.fail("the pane's Stop hook feedback lines hold %s %d times; want once", first, headShown)
	}
}

// TestSmokeCreelHookDelivery proves creel's hook delivery into real Claude sessions started by shuttle's own launch, so their settings and launch line are the ones production renders.
// The test process posts as EXTERNAL.
// It needs a `lym` built from this tree on PATH, since the sessions' hooks run `lym creel hook-read` and `lym creel hook-wait`.
func TestSmokeCreelHookDelivery(t *testing.T) {
	llmkit.Claude(t, "LYM_REED_CLAUDE")
	if _, err := exec.LookPath("lym"); err != nil {
		t.Skip("lym is not on PATH, and the sessions' hooks run it")
	}
	// The strand name in the environment is process-global state, set by newSmokeHub.
	smoke := newSmokeHub(t)
	smoke.startDaemon(t, shuttleengines.New)

	t.Run("mid-turn post is delivered at the next tool call", func(t *testing.T) {
		c := newClaudeScenario(t, smoke, "Run two Bash calls one after the other: `sleep 25`, then `echo second`. "+
			"If a message starting with [creel] arrives, do what it says at once, then finish the two calls and reply with the word finished.", false)
		c.awaitShown("sleep 25")
		c.post(c.name, "creel-midturn-token: write the word ok into the file "+c.file("midturn.txt"))
		c.awaitFile("midturn.txt")
		c.awaitShown("creel-midturn-token")
		c.awaitTurnEnds(1)
		c.requireNoPanePath()
		c.requireTranscriptLacks("lym creel hook-read", "hook blocking error")
	})

	t.Run("a post after the last tool call is delivered at the Stop as additional context and the turn goes on", func(t *testing.T) {
		c := newClaudeScenario(t, smoke, "Run one Bash call, `echo only-command`. After it, write out the lines item-1 to item-200, one per line, and end your turn. "+
			"If a message starting with [creel] arrives, do what it says before you end your turn.", false)
		c.awaitShown("item-20")
		c.post(c.name, "creel-stop-token: write the word ok into the file "+c.file("stop.txt"))
		c.awaitFile("stop.txt")
		c.awaitTurnEnds(1)
		c.awaitShown("creel-stop-token")
		if got := c.payloads("Stop"); got != 1 {
			c.fail("events hold %d Stop payloads; want exactly the real end's, none for the blocked stop", got)
		}
		if got := c.stamps("Stop"); got != 1 {
			c.fail("events hold %d Stop stamps; want exactly the real end's, none for the blocked stop", got)
		}
		c.requireNoPanePath()
		c.requireTranscriptLacks("lym creel hook-read", "hook blocking error")
	})

	t.Run("a Stop claim of more than five lines shows the operator its head once", func(t *testing.T) {
		c := newClaudeScenario(t, smoke, "Run one Bash call, `echo only-command`. After it, write out the lines item-1 to item-200, one per line, and end your turn. "+
			"If messages starting with [creel] arrive, act on the last one before you end your turn.", false)
		c.awaitShown("item-20")
		var tokens []string
		for i := 1; i <= 7; i++ {
			token := fmt.Sprintf("creel-head-%d", i)
			tokens = append(tokens, token)
			body := token + ": nothing to do for this one"
			if i == 7 {
				body = token + ": write the word ok into the file " + c.file("head.txt")
			}
			c.post(c.name, body)
		}
		c.awaitFile("head.txt")
		c.awaitTurnEnds(1)
		c.requireHeadShownOnce(tokens[0], tokens)
		c.requireNoPanePath()
	})

	t.Run("an idle session with a draft is woken and keeps its draft", func(t *testing.T) {
		const draft = "creel-draft-must-stay"
		c := newClaudeScenario(t, smoke, "Reply with the single word ready and end your turn. "+
			"If a message starting with [creel] arrives later, do what it says.", false)
		c.awaitTurnEnds(1)
		if err := smoke.reed.SendText(c.guid, draft, false); err != nil {
			c.fail("typing the draft: %v", err)
		}
		c.awaitShown(draft)
		c.post(c.name, "creel-wake-token: write the word ok into the file "+c.file("wake.txt"))
		c.awaitFile("wake.txt")
		c.requireWakeShown("creel-wake-token", 1)
		if !strings.Contains(smoke.lastPane(t, c.guid), draft) {
			c.fail("the draft %q is gone from the input box", draft)
		}
		if got := c.promptsContaining(creelengine.BellLine); got != 0 {
			c.fail("%d prompts contain the bell line; the waiter was to wake the session without it", got)
		}
		c.requireNoPanePath()
	})

	t.Run("successive wakes are each delivered through the waiter", func(t *testing.T) {
		c := newClaudeScenario(t, smoke, "Reply with the single word ready and end your turn. "+
			"Each time a message starting with [creel] arrives, do what it says.", false)
		c.awaitTurnEnds(1)
		for i := 1; i <= 3; i++ {
			token := fmt.Sprintf("creel-successive-%d", i)
			file := fmt.Sprintf("successive-%d.txt", i)
			c.post(c.name, token+": write the word ok into the file "+c.file(file))
			c.awaitFile(file)
			c.requireWakeShown(token, i)
		}
		if got := c.promptsContaining(creelengine.BellLine); got != 0 {
			c.fail("%d prompts contain the bell line; each wake was to come from the waiter", got)
		}
		c.requireNoPanePath()
	})

	t.Run("a fork takes no mail and the main thread's next boundary does", func(t *testing.T) {
		c := newClaudeScenario(t, smoke, "Use the Agent tool with subagent_type fork to have a fork run `sleep 40` as a Bash call and then answer fork-done. "+
			"Wait for the fork, then run `echo main-after` as your own Bash call and reply with the word finished. "+
			"If a message starting with [creel] arrives for you, do what it says.", true)
		c.awaitShown("sleep 40")
		c.post(c.name, "creel-fork-token: write the word ok into the file "+c.file("fork.txt"))
		c.awaitFile("fork.txt")
		c.awaitShown("creel-fork-token")
		c.awaitTurnEnds(1)
		transcripts := forkTranscripts(c)
		if len(transcripts) == 0 {
			c.fail("no fork transcript found beside the session's own")
		}
		for _, path := range transcripts {
			data, err := os.ReadFile(path)
			if err != nil {
				c.fail("reading the fork transcript %s: %v", path, err)
			}
			if strings.Contains(string(data), "creel-fork-token") {
				c.fail("the fork transcript %s holds the creel message; only the main thread takes mail", path)
			}
		}
		c.requireNoPanePath()
	})

	t.Run("post --interrupt stops the first turn and the bell wakes the session once", func(t *testing.T) {
		c := newClaudeScenario(t, smoke, "Run one Bash call, `sleep 300`, then reply with the word finished. "+
			"If a message starting with [creel] arrives, do what it says.", false)
		c.awaitShown("sleep 300")
		code, out := smoke.creel("post", "--interrupt", c.name, "creel-interrupt-token: write the word ok into the file "+c.file("interrupt.txt"))
		if code != 0 || !strings.Contains(out, `"interrupted":true`) {
			c.fail("lym creel post --interrupt = %d, %s; want exit 0 and interrupted", code, out)
		}
		c.awaitFile("interrupt.txt")
		c.awaitShown("creel-interrupt-token")
		c.awaitTurnEnds(1)
		if got := c.promptsContaining(creelengine.BellLine); got != 1 {
			c.fail("%d prompts contain the bell line; want exactly one, the bell that started the turn", got)
		}
		// A second bell for the same set would arrive within a few probe ceilings, so the count is watched on a tick for a short while.
		watchUntil := time.Now().Add(20 * time.Second)
		for time.Now().Before(watchUntil) {
			if got := c.promptsContaining(creelengine.BellLine); got != 1 {
				c.fail("%d prompts contain the bell line; want no second bell for the same set", got)
			}
			time.Sleep(time.Second)
		}
		c.requireNoPanePath()
	})
}

// forkTranscripts returns the transcripts of the session's forks: the files under the directory named for the session's own transcript, and when that directory does not exist the other transcripts of its project directory.
func forkTranscripts(c *claudeScenario) []string {
	main := c.mainTranscript()
	if main == "" {
		return nil
	}
	root := strings.TrimSuffix(main, ".jsonl")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		root = filepath.Dir(main)
	}
	var found []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".jsonl") && path != main {
			found = append(found, path)
		}
		return nil
	})
	return found
}
