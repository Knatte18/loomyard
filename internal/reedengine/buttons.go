// buttons.go decodes, validates and fills the status-bar button entries of reed.yaml's and desktop.yaml's buttons blocks, and names the placeholders a click fills.
// The render and the click verb call these pure functions; none reads a file or runs a command.

package reedengine

import (
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
	"gopkg.in/yaml.v3"
)

// ButtonPlace names the kind of session a button row is rendered for.
type ButtonPlace string

const (
	// ButtonPlaceTask is a task pair's session.
	ButtonPlaceTask ButtonPlace = "task"
	// ButtonPlaceBeam is a beam pair's session.
	ButtonPlaceBeam ButtonPlace = "beam"
	// ButtonPlacePrime is the prime worktree's session.
	ButtonPlacePrime ButtonPlace = "prime"
)

// buttonWindowAgents is the window value that limits a button to a strand pane's window and lets its run, confirm and ask text name {strand}.
const buttonWindowAgents = "agents"

// Placeholder names a click fills rather than the render.
const (
	// buttonStrandKey names the clicked agent pane's strand, valid only in an entry with window: agents.
	buttonStrandKey = "strand"
	// buttonAnswerKey names the answer to the entry's ask prompt, valid only in an entry with an ask text.
	buttonAnswerKey = "answer"
)

// placeholder returns the {name} text that name fills in a button's run, confirm and ask text.
func placeholder(name string) string {
	return "{" + name + "}"
}

// buttonEntry is one decoded status-bar button.
type buttonEntry struct {
	// Label is the text drawn on the button.
	Label string `yaml:"label"`
	// Run is the command line the click runs, with its placeholders unfilled.
	Run string `yaml:"run"`
	// Confirm is the prompt asked before Run; empty asks nothing.
	Confirm string `yaml:"confirm"`
	// Ask is the prompt whose answer fills {answer} in Run and Confirm, asked before Confirm; empty asks nothing.
	Ask string `yaml:"ask"`
	// AskDefault fills {answer} when the answer to Ask is empty.
	AskDefault string `yaml:"ask_default"`
	// Window is empty for a button on every window, or buttonWindowAgents for a button on a strand pane's window alone.
	Window string `yaml:"window"`
	// Index is the entry's position within its group.
	Index int `yaml:"-"`
}

// placeButtons returns the valid entries of the group place selects in groups, the buttons node of one config file, in config order.
// The prime takes its own group when that is a non-empty list and the beam group otherwise.
// A zero node or an absent group gives no entries and logs nothing;
// a group that is not a list, an entry that does not decode and an entry buttonEntryProblem refuses are skipped with one Warn naming source, so a bad button costs only itself.
func placeButtons(groups yaml.Node, place ButtonPlace, source string) []buttonEntry {
	groupName := string(place)
	if place == ButtonPlacePrime {
		groupName = string(ButtonPlaceBeam)
		if prime := groupNode(groups, string(ButtonPlacePrime)); prime != nil {
			if prime.Kind != yaml.SequenceNode {
				logger.Warn("reed: skipped a button group that is not a list", "source", source, "group", string(ButtonPlacePrime))
			} else if len(prime.Content) > 0 {
				groupName = string(ButtonPlacePrime)
			}
		}
	}

	group := groupNode(groups, groupName)
	if group == nil {
		return nil
	}
	if group.Kind != yaml.SequenceNode {
		logger.Warn("reed: skipped a button group that is not a list", "source", source, "group", groupName)
		return nil
	}

	var entries []buttonEntry
	for index, node := range group.Content {
		var entry buttonEntry
		if err := node.Decode(&entry); err != nil {
			logger.Warn("reed: skipped a button entry that does not decode", "source", source, "group", groupName, "index", index, "err", err)
			continue
		}
		entry.Index = index
		if problem := buttonEntryProblem(entry); problem != "" {
			logger.Warn("reed: skipped an invalid button entry", "source", source, "group", groupName, "index", index, "problem", problem)
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

// groupNode returns the value of key in the mapping node groups, or nil when groups is not a mapping or holds no such key.
func groupNode(groups yaml.Node, key string) *yaml.Node {
	if groups.Kind == yaml.DocumentNode && len(groups.Content) == 1 {
		groups = *groups.Content[0]
	}
	if groups.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(groups.Content); i += 2 {
		if groups.Content[i].Value == key {
			return groups.Content[i+1]
		}
	}
	return nil
}

// names reports whether the entry's run, confirm or ask text holds the placeholder {name}.
func (entry buttonEntry) names(name string) bool {
	text := placeholder(name)
	return strings.Contains(entry.Run, text) || strings.Contains(entry.Confirm, text) || strings.Contains(entry.Ask, text)
}

// buttonEntryProblem returns the empty string for a valid entry, else one sentence naming its first problem.
// The label, the confirm text and the ask text are drawn inside a tmux format, so none may hold a status sequence start, a strftime field or a control character.
// The ask text may hold no comma either, since tmux splits a prompt at its commas.
func buttonEntryProblem(entry buttonEntry) string {
	if entry.Label == "" {
		return "the label is empty"
	}
	if entry.Run == "" {
		return "the run text is empty"
	}
	for _, field := range []struct{ name, text string }{{"label", entry.Label}, {"confirm", entry.Confirm}, {"ask", entry.Ask}} {
		if strings.ContainsAny(field.text, "#%") || strings.IndexFunc(field.text, unicode.IsControl) >= 0 {
			return "the " + field.name + " holds a '#', a '%' or a control character"
		}
	}
	if strings.Contains(entry.Ask, ",") {
		return "the ask holds a ','"
	}
	if entry.Window != "" && entry.Window != buttonWindowAgents {
		return "the window is neither empty nor " + buttonWindowAgents
	}
	if entry.Window != buttonWindowAgents && entry.names(buttonStrandKey) {
		return placeholder(buttonStrandKey) + " needs window: " + buttonWindowAgents
	}
	if entry.Ask == "" && entry.AskDefault != "" {
		return "the ask_default needs an ask"
	}
	if entry.Ask == "" && entry.names(buttonAnswerKey) {
		return placeholder(buttonAnswerKey) + " needs an ask"
	}
	return ""
}

// fillRunPlaceholders replaces each {name} whose name is a key of values with the value quoted as one word of sh's dialect.
// Brace text naming no key of values stays literal.
func fillRunPlaceholders(sh shell.Shell, run string, values map[string]string) string {
	return placeholderReplacer(values, sh.Quote).Replace(run)
}

// fillPromptPlaceholders replaces each {name} whose name is a key of values with the value, its '#' and '%' doubled so it cannot inject a status sequence or a strftime field into a confirm or ask prompt.
// Brace text naming no key of values stays literal.
func fillPromptPlaceholders(prompt string, values map[string]string) string {
	doubled := strings.NewReplacer("#", "##", "%", "%%")
	return placeholderReplacer(values, doubled.Replace).Replace(prompt)
}

// placeholderReplacer builds one single-pass replacer from {name} to transform(value), so a substituted value is never scanned for placeholders again.
func placeholderReplacer(values map[string]string, transform func(string) string) *strings.Replacer {
	oldNew := make([]string, 0, 2*len(values))
	for name, value := range values {
		oldNew = append(oldNew, placeholder(name), transform(value))
	}
	return strings.NewReplacer(oldNew...)
}
