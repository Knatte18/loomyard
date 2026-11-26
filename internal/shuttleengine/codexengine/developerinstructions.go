// developerinstructions.go assembles the standing items a Codex launch passes as `developer_instructions`.
// It joins the items into one text and refuses a text whose setting Codex cannot receive as one argument.

package codexengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// developerInstructionsKey is the Codex setting that carries a session's developer instructions.
const developerInstructionsKey = "developer_instructions"

// maxArgumentBytes is the size Linux caps one argument at (MAX_ARG_STRLEN), counting its terminating NUL.
// The longest argument `execve` takes is therefore maxArgumentBytes-1 bytes.
const maxArgumentBytes = 128 * 1024

// instructionItem is one named part of a launch's developer instructions.
type instructionItem struct {
	// name labels the item in a refusal that names the largest item.
	name string
	// text is the item's text; an empty text adds nothing.
	text string
	// wayForward is the step that shrinks the item.
	wayForward string
}

// creelHeadItem returns the item that opens the developer instructions:
// the deny notice, when there is one, then the creel trust sentence on the same line.
func creelHeadItem(notice string) instructionItem {
	head := shuttleengine.CreelTrustSentence
	if notice != "" {
		head = notice + " " + head
	}
	return instructionItem{
		name:       "the deny notice and creel trust sentence",
		text:       head,
		wayForward: "shorten the hub's " + otherKeyName("gate", "refuse_raw") + " list with lym config gate, since each row adds its notice",
	}
}

// passageItems returns one item per standing passage, named by its heddle slug.
func passageItems(passages []shuttleengine.Passage) []instructionItem {
	items := make([]instructionItem, len(passages))
	for i, passage := range passages {
		items[i] = instructionItem{
			name:       "heddle passage " + passage.Slug,
			text:       passage.Text,
			wayForward: "find the entry with lym heddle show codex-prompt and shorten it with lym heddle set, or with lym heddle set --layer lym for a shipped entry",
		}
	}
	return items
}

// developerInstructionsSetting returns the `-c` setting that carries text as the developer instructions:
// the key and the text as a single-line TOML string.
func developerInstructionsSetting(text string) string {
	return developerInstructionsKey + "=" + tomlString(text)
}

// developerInstructions joins the non-empty items' texts with blank lines.
// It returns "" for no text, which the launch line then omits since Codex refuses a blank value.
// It refuses a text whose setting reaches maxArgumentBytes, since the setting is one argument of the `codex` command.
// The refusal names the setting's size, the cap, the largest item with its size and that item's way forward.
func developerInstructions(items []instructionItem) (string, error) {
	var texts []string
	var largest instructionItem
	for _, item := range items {
		if item.text == "" {
			continue
		}
		texts = append(texts, item.text)
		if len(item.text) > len(largest.text) {
			largest = item
		}
	}
	if len(texts) == 0 {
		return "", nil
	}
	text := strings.Join(texts, "\n\n")
	if size := len(developerInstructionsSetting(text)); size >= maxArgumentBytes {
		return "", fmt.Errorf("the %s setting is %d bytes, but Codex receives it as one argument that Linux caps below %d bytes; the largest item is %s at %d bytes; way forward: %s",
			developerInstructionsKey, size, maxArgumentBytes, largest.name, len(largest.text), largest.wayForward)
	}
	return text, nil
}
