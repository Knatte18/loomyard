// reinterpret.go carries a hub's creel.yaml across the change of trusted_hubs from shortnames to repository names.

package creelconfig

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// TrustedHubsRepositoryReinterpretation names the change of trusted_hubs from hub shortnames to repository names.
const TrustedHubsRepositoryReinterpretation = "creel-trusted-hubs-repository"

// ErrShortnameUnresolvable is wrapped by a resolver's refusal of a shortname that names no repository on this machine.
// The rewrite drops such an entry; any other resolver error ends the rewrite with that error.
var ErrShortnameUnresolvable = errors.New("creel: shortname names no repository on this machine")

// RewriteShortnamesToRepositories returns the reinterpretation of a hub's creel.yaml.
// resolve answers the repository name, owner/repo, of the hub a shortname names on the machine that runs the rewrite.
// An entry holding a "/" is already a repository name and is kept.
// Every other entry is a shortname.
// It is rewritten to its repository name, or dropped when resolve fails with ErrShortnameUnresolvable,
// and each such entry gets one line naming it.
// Any other resolve error is returned, so a transient failure drops no entry.
// A repository listed twice is kept once.
// The rewrite takes no worktree root or anchored directory, since trusted_hubs names no path.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func RewriteShortnamesToRepositories(resolve func(shortname string) (string, error)) func(existing []byte, worktreeRoot, anchorDir string) (rewritten []byte, rewrote, flagged []string, err error) {
	return func(existing []byte, _, _ string) ([]byte, []string, []string, error) {
		var document yaml.Node
		if err := yaml.Unmarshal(existing, &document); err != nil {
			return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("rewrite trusted hubs: unmarshal yaml: %w", err))
		}
		list := trustedHubsSequence(&document)
		if list == nil {
			return existing, nil, nil, nil
		}
		var (
			kept    []*yaml.Node
			seen    []string
			rewrote []string
		)
		for _, entry := range list.Content {
			value := entry.Value
			if entry.Kind == yaml.ScalarNode && !strings.Contains(value, "/") {
				repository, err := resolve(value)
				if err != nil && !errors.Is(err, ErrShortnameUnresolvable) {
					return nil, nil, nil, fmt.Errorf("rewrite trusted hubs: resolve the shortname %q: %w", value, err)
				}
				if err != nil {
					rewrote = append(rewrote, fmt.Sprintf("%s %q dropped, since it lists repository names now and that shortname names none on this machine (%v); way forward: add the hub's repository as owner/repo with lym config creel from the prime", trustedHubsKey, value, err))
					continue
				}
				rewrote = append(rewrote, fmt.Sprintf("%s %q rewritten to %q, the repository of the hub with that shortname on this machine, since the key lists repository names now", trustedHubsKey, value, repository))
				entry.Value, value = repository, repository
			}
			if entry.Kind == yaml.ScalarNode && containsFold(seen, value) {
				continue
			}
			seen = append(seen, value)
			kept = append(kept, entry)
		}
		if len(rewrote) == 0 && len(kept) == len(list.Content) {
			return existing, nil, nil, nil
		}
		list.Content = kept
		encoded, err := encodeDocument(&document)
		if err != nil {
			return nil, nil, nil, err
		}
		return encoded, rewrote, nil, nil
	}
}

// trustedHubsSequence returns the trusted_hubs sequence of document, or nil when the document holds no such sequence.
func trustedHubsSequence(document *yaml.Node) *yaml.Node {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	mapping := document.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == trustedHubsKey && mapping.Content[i+1].Kind == yaml.SequenceNode {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// containsFold reports whether values holds value, ignoring case as GitHub does for repository names.
func containsFold(values []string, value string) bool {
	for _, held := range values {
		if strings.EqualFold(held, value) {
			return true
		}
	}
	return false
}

// encodeDocument renders a rewritten creel.yaml document.
// An encoder failure is returned as an error marked configengine.ErrInvalid.
func encodeDocument(document *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("rewrite trusted hubs: encode yaml: %w", err))
	}
	if err := encoder.Close(); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("rewrite trusted hubs: encode yaml: %w", err))
	}
	return out.Bytes(), nil
}
