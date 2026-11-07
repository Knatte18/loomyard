// skillload_test.go pins the skill load: the lookup of a skill's SKILL.md under the skill roots and the load line.
// It also pins the classification from a rollout and the launch that sends the load line first.

package codexengine

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeSkill writes the SKILL.md of a plugin's skill at version under root's marketplace folder and returns its path.
// The file holds skillBody(name).
func writeSkill(t *testing.T, root, marketplace, plugin, version, name string) string {
	t.Helper()
	file := filepath.Join(root, marketplace, plugin, version, skillsDirName, name, skillFileName)
	mkdirAll(t, filepath.Dir(file))
	writeFile(t, file, skillBody(name))
	return file
}

// skillBody returns the content of the SKILL.md writeSkill writes for the skill name.
func skillBody(name string) string {
	return "# " + name + "\n\nThe rules of " + name + ".\n"
}

// skillEngine returns an engine whose codex.yaml names root as its one skill root.
func skillEngine(t *testing.T, root string) *Codex {
	t.Helper()
	repo := newRepoFixture(t)
	anchor, _ := repo.addWorktree(t, "wt")
	repo.writeCodexLayer(t, anchor, "skill_roots: ["+root+"]\n")
	engine, _ := newEngine(anchor)
	return engine
}

// rolloutUserItem returns the rollout line of a user message response item holding text.
func rolloutUserItem(text string) string {
	data, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []map[string]string{{"type": "input_text", "text": text}}}})
	return string(data)
}

// rolloutUserEvent returns the rollout line of a user_message event holding text.
func rolloutUserEvent(text string) string {
	data, _ := json.Marshal(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "user_message", "message": text}})
	return string(data)
}

// rolloutToolCall returns the rollout line of a tool call of payload type kind whose arguments hold command.
func rolloutToolCall(kind, command string) string {
	arguments, _ := json.Marshal(map[string]string{"cmd": command})
	data, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{"type": kind, "name": "exec", "arguments": string(arguments)}})
	return string(data)
}

// rolloutExecCall returns the rollout line of a function_call running command.
func rolloutExecCall(command string) string {
	return rolloutToolCall("function_call", command)
}

// rolloutScriptCall returns the rollout line of a custom_tool_call exec whose input is script.
func rolloutScriptCall(script string) string {
	data, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "name": "exec", "input": script}})
	return string(data)
}

// rolloutStringOutput returns the rollout line of a function_call_output whose output is the string text.
func rolloutStringOutput(text string) string {
	data, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{"type": "function_call_output", "output": text}})
	return string(data)
}

// rolloutBlocksOutput returns the rollout line of a custom_tool_call_output whose output is one text block per text.
func rolloutBlocksOutput(texts ...string) string {
	var blocks []map[string]string
	for _, text := range texts {
		blocks = append(blocks, map[string]string{"type": "input_text", "text": text})
	}
	data, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call_output", "output": blocks}})
	return string(data)
}

// TestSkillPath pins that the lookup returns the newest version under the first root holding the skill.
// It returns none for a skill no root holds or one that names no plugin.
// The newest is the highest number field by field;
// a number orders above text, so a release beats its pre-release and a hash-named folder,
// and a set of versions that would cycle under a mixed comparison still yields one answer.
// A root holding a glob metacharacter resolves as written.
func TestSkillPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	first, second, third := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "root[1]")
	writeSkill(t, first, "market", "scribe", "1.9.0", "prose")
	newest := writeSkill(t, first, "market", "scribe", "1.10.0", "prose")
	writeSkill(t, second, "market", "scribe", "2.0.0", "prose")
	onlySecond := writeSkill(t, second, "market", "ly", "0.1", "board")
	release := writeSkill(t, third, "market", "versions", "1.0.0", "prose")
	writeSkill(t, third, "market", "versions", "1.0.0-beta", "prose")
	writeSkill(t, third, "market", "versions", "abc123", "prose")
	ten := writeSkill(t, third, "market", "cycle", "10", "prose")
	writeSkill(t, third, "market", "cycle", "2", "prose")
	writeSkill(t, third, "market", "cycle", "1a", "prose")
	roots := []string{first, second, third}

	tests := []struct {
		skill string
		want  string
	}{
		{"scribe:prose", newest},
		{"ly:board", onlySecond},
		{"versions:prose", release},
		{"cycle:prose", ten},
		{"scribe:absent", ""},
		{"prose", ""},
	}
	for _, tt := range tests {
		if got := skillPath(tt.skill, roots); got != tt.want {
			t.Errorf("skillPath(%q) = %q; want %q", tt.skill, got, tt.want)
		}
	}
}

// TestSkillLoadMessage pins the load line: each found skill's path in order and an unfound one left out.
// When no skill is found, the line asks for no read.
func TestSkillLoadMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	prose := writeSkill(t, root, "market", "scribe", "1.0.0", "prose")
	testingSkill := writeSkill(t, root, "market", "scribe", "1.0.0", "testing")
	engine := skillEngine(t, root)

	got := engine.SkillLoadMessage([]string{"scribe:testing", "scribe:absent", "scribe:prose"})
	if !strings.Contains(got, ": "+testingSkill+", "+prose+". ") || strings.Contains(got, "absent") || strings.Contains(got, "\n") {
		t.Errorf("SkillLoadMessage() = %q; want one line naming %s then %s and no absent skill", got, testingSkill, prose)
	}
	if got := engine.SkillLoadMessage([]string{"scribe:absent"}); strings.Contains(got, skillFileName) {
		t.Errorf("SkillLoadMessage() of no found skill = %q; want a line naming no file", got)
	}
}

// TestSkillLoadMessage_ExpandsAHomeRoot pins that a skill root starting with ~/ resolves under the user's home.
// It sets the home environment, so it does not run in parallel.
func TestSkillLoadMessage_ExpandsAHomeRoot(t *testing.T) {
	tiergate.Require(t, 0)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	prose := writeSkill(t, filepath.Join(home, "cache"), "market", "scribe", "1.0.0", "prose")
	engine := skillEngine(t, "~/cache")

	if got := engine.SkillLoadMessage([]string{"scribe:prose"}); !strings.Contains(got, ": "+prose+". ") {
		t.Errorf("SkillLoadMessage() = %q; want it to name %s under the expanded root", got, prose)
	}
}

// TestSkillLoadMessage_WarnsOnceWhenCodexYamlDoesNotLoad pins that a codex.yaml that does not load resolves no skill.
// It logs one Warn.
func TestSkillLoadMessage_WarnsOnceWhenCodexYamlDoesNotLoad(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	repo := newRepoFixture(t)
	anchor, _ := repo.addWorktree(t, "wt")
	repo.writeCodexLayer(t, anchor, "skill_roots: [\n")
	engine, warnings := newEngine(anchor)

	if got := engine.SkillLoadMessage([]string{"scribe:prose"}); strings.Contains(got, skillFileName) {
		t.Errorf("SkillLoadMessage() = %q; want a line naming no file", got)
	}
	if got := warnings.String(); strings.Count(got, "\n") != 0 || !strings.Contains(got, "did not load") {
		t.Errorf("warnings = %q; want exactly one naming that codex.yaml did not load", got)
	}
}

// TestClassifySkillLoad pins the classification from a rollout.
// A tool call of any call payload type after the load message naming a skill's tail loads it,
// and a call before the message does not.
// A tool call output after the message holding the whole SKILL.md loads it too, as a read through a built path leaves;
// one holding part of the file, or one before the message, does not.
// A repeated load message restarts the count, and a truncated line is skipped.
// A skill no root holds is unknown, and an unreadable rollout or one without the load message is unverified.
func TestClassifySkillLoad(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	prose := writeSkill(t, root, "market", "scribe", "1.0.0", "prose")
	testingSkill := writeSkill(t, root, "market", "scribe", "1.0.0", "testing")
	engine := skillEngine(t, root)
	skills := []string{"scribe:prose", "scribe:testing", "scribe:absent"}
	message := engine.SkillLoadMessage(skills)

	tests := []struct {
		name  string
		lines []string
		want  shuttleengine.SkillLoadReport
	}{
		{
			name:  "ReadAfterTheMessage",
			lines: []string{rolloutUserItem(message), rolloutExecCall("cat " + prose), rolloutExecCall("sed -n 1,200p " + testingSkill)},
			want:  shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:prose", "scribe:testing"}, Unknown: []string{"scribe:absent"}},
		},
		{
			name:  "ReadBeforeTheMessage",
			lines: []string{rolloutExecCall("cat " + testingSkill), rolloutStringOutput(skillBody("testing")), rolloutUserEvent(message), rolloutExecCall("cat " + prose)},
			want:  shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:prose"}, Unknown: []string{"scribe:absent"}, Missing: []string{"scribe:testing"}},
		},
		{
			name: "ReadThroughABuiltPath",
			lines: []string{
				rolloutUserItem(message),
				rolloutScriptCall("for (const name of [\"prose\",\"testing\"]) { await tools.exec_command({cmd: `cat " + root + "/market/scribe/1.0.0/skills/${name}/SKILL.md`}); }"),
				rolloutBlocksOutput("Script completed\nOutput:\n# pro", "se\n\nThe rules of prose.\n"),
				rolloutStringOutput(skillBody("testing")),
			},
			want: shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:prose", "scribe:testing"}, Unknown: []string{"scribe:absent"}},
		},
		{
			name:  "PartialOutputDoesNotLoad",
			lines: []string{rolloutUserItem(message), rolloutStringOutput("# testing\n")},
			want:  shuttleengine.SkillLoadReport{Verified: true, Unknown: []string{"scribe:absent"}, Missing: []string{"scribe:prose", "scribe:testing"}},
		},
		{
			name:  "CustomToolAndLocalShellCalls",
			lines: []string{rolloutUserItem(message), rolloutToolCall("custom_tool_call", "cat "+prose), rolloutToolCall("local_shell_call", "cat "+testingSkill)},
			want:  shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:prose", "scribe:testing"}, Unknown: []string{"scribe:absent"}},
		},
		{
			name:  "RepeatedLoadMessageRestartsTheCount",
			lines: []string{rolloutUserItem(message), rolloutExecCall("cat " + prose), rolloutStringOutput(skillBody("prose")), rolloutUserEvent(message), rolloutExecCall("cat " + testingSkill)},
			want:  shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:testing"}, Unknown: []string{"scribe:absent"}, Missing: []string{"scribe:prose"}},
		},
		{
			name:  "TruncatedLineIsSkipped",
			lines: []string{rolloutUserItem(message), `{"type":"response_item","payload":{"type":"function_c`, rolloutExecCall("cat " + prose)},
			want:  shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:prose"}, Unknown: []string{"scribe:absent"}, Missing: []string{"scribe:testing"}},
		},
		{
			name:  "NoLoadMessage",
			lines: []string{rolloutUserItem("something else"), rolloutExecCall("cat " + prose)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rollout := writeRollout(t, tt.lines...)
			got := engine.ClassifySkillLoad(shuttleengine.Event{Raw: []byte(`{"hook_event_name":"Stop","transcript_path":"` + rollout + `"}`)}, skills)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ClassifySkillLoad() = %+v; want %+v", got, tt.want)
			}
		})
	}
	if got := engine.ClassifySkillLoad(shuttleengine.Event{Raw: []byte(`{"transcript_path":"/absent/rollout.jsonl"}`)}, skills); got.Verified {
		t.Errorf("ClassifySkillLoad() over an absent rollout = %+v; want unverified", got)
	}
}

// TestClassifySkillLoad_NewerVersionAfterTheLine pins that a newer skill version leaves the report verified.
// The version is written after the load line was sent.
// The line is recognised by its fixed text and not by the paths it named.
func TestClassifySkillLoad_NewerVersionAfterTheLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	sent := writeSkill(t, root, "market", "scribe", "1.0.0", "prose")
	engine := skillEngine(t, root)
	message := engine.SkillLoadMessage([]string{"scribe:prose"})
	writeSkill(t, root, "market", "scribe", "2.0.0", "prose")

	rollout := writeRollout(t, rolloutUserItem(message), rolloutExecCall("cat "+sent))
	got := engine.ClassifySkillLoad(shuttleengine.Event{Raw: []byte(`{"transcript_path":"` + rollout + `"}`)}, []string{"scribe:prose"})
	if want := (shuttleengine.SkillLoadReport{Verified: true, Loaded: []string{"scribe:prose"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("ClassifySkillLoad() = %+v; want %+v", got, want)
	}
}

// TestPrepare_SkillsLoadFirst pins that a spec naming skills launches with the load line as its first turn.
// It returns the prompt pointer as the prompt line.
func TestPrepare_SkillsLoadFirst(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	writeSkill(t, root, "market", "scribe", "1.0.0", "prose")
	engine := skillEngine(t, root)
	runDir := t.TempDir()

	launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "p", Skills: []string{"scribe:prose"}}, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	pointer := launchPointer(filepath.Join(runDir, "prompt.md"))
	if launch.PromptLine != pointer {
		t.Errorf("Launch.PromptLine = %q; want the pointer %q", launch.PromptLine, pointer)
	}
	loadLine := engine.SkillLoadMessage([]string{"scribe:prose"})
	if !strings.Contains(launch.Cmd, "--dangerously-bypass-hook-trust '"+loadLine+"'; ") || strings.Contains(launch.Cmd, pointer) {
		t.Errorf("Launch.Cmd = %q; want the load line %q as its first turn and no pointer", launch.Cmd, loadLine)
	}
}
