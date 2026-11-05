// describe.go is the "lym beam describe" sequence: it judges the named beam and has its change description written, stamped in the landing record and committed.

package beamcli

import (
	"fmt"
	"strings"
)

// describe has the beam slug's change description written and returns its path and the verbs that follow.
// It refuses an entry that is no open beam of this machine's pair, a beam whose pawl run is absent or not at Beam-Await, and a beam whose ready record stands, so nothing at or after landing rewrites the description.
func describe(deps landDeps, slug string) (map[string]any, error) {
	const verb = "lym beam describe"
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("%s: read the board's owners: %w", verb, err)
	}
	if err := judgeLandEntry(deps, owners, verb, slug); err != nil {
		return nil, err
	}
	if err := judgeAwaitingRun(deps, verb, slug); err != nil {
		return nil, err
	}
	if err := judgeNotReady(deps, verb, slug); err != nil {
		return nil, err
	}

	path, err := deps.Describe(slug, deps.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "way forward:") {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		return nil, fmt.Errorf("%s: write the description of %s: %w; way forward: fix the cause it names, then run `%s %s` again", verb, slug, err, verb, slug)
	}
	return map[string]any{
		"slug":        slug,
		"description": path,
		"way_forward": fmt.Sprintf("run `lym beam pr open %s` to open the pull request, or `lym beam pr update %s` when one is recorded", slug, slug),
	}, nil
}
