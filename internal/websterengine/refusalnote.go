// refusalnote.go writes the friction note a `lyx webster` refusal leaves.

package websterengine

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
)

// RefusalNote describes one refused `lyx webster` invocation.
// Fields holds every envelope key but `ok` and `error`, which is where a refusal carries its class, such as `plan_drifted` or `batch_failed`.
type RefusalNote struct {
	Verb    string
	Args    string
	Message string
	Fields  map[string]any
}

// WriteRefusalNote records a refusal as a friction note named `webster-refusal-<verb>`.
// It is a no-op when frictionDir is empty (Tier 2 off) or when friction.NotePath rejects the id.
// A write failure is returned wrapped with the path; callers only log it.
func WriteRefusalNote(frictionDir string, n RefusalNote) error {
	if frictionDir == "" {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-refusal-"+n.Verb)
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("lyx webster " + n.Verb + " refused\n\n")
	b.WriteString("Arguments: " + n.Args + "\n\n")
	b.WriteString("```\n" + n.Message + "\n```\n")

	keys := make([]string, 0, len(n.Fields))
	for k := range n.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		b.WriteString("\n")
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("%s: %v\n", k, n.Fields[k]))
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: write refusal note %s: %w", path, err)
	}
	return nil
}
