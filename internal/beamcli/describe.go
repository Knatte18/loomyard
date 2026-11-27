// describe.go implements `lym beam describe`: it judges the named beam and records the change description its orch wrote.
// It stamps the description in the landing record, commits it and brings a recorded pull request up to date.
// With --draft it writes a starting point for the orch to edit instead.

package beamcli

import (
	"context"
	"fmt"
	"strings"
)

// describe records the beam slug's change description, which its orch wrote.
// It returns the description's path and the verbs that follow.
// When the landing record names a pull request, it then runs `lym beam pr update`'s sequence,
// so the pull request carries the recorded text.
// With draft it instead writes a starting point where no description stands yet and records nothing.
// It refuses an entry that is no open beam of this machine's pair;
// a beam whose pawl run is absent or not at Beam-Await;
// or a beam whose ready record stands.
// Together these keep anything at or after landing from rewriting the description.
func describe(ctx context.Context, deps landDeps, slug string, draft bool) (map[string]any, error) {
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

	if draft {
		path, err := deps.DraftDescription(slug)
		if err != nil {
			return nil, describeFailure(verb, slug, "draft", err)
		}
		return map[string]any{
			"slug":        slug,
			"description": path,
			"drafted":     true,
			"way_forward": fmt.Sprintf("edit the draft into the beam's end result and its member pull requests, then run `%s %s` to record it", verb, slug),
		}, nil
	}

	path, err := deps.RecordDescription(slug, deps.Now().UTC())
	if err != nil {
		return nil, describeFailure(verb, slug, "record", err)
	}
	result := map[string]any{"slug": slug, "description": path}
	record, found, err := deps.Landing(slug)
	if err != nil {
		return nil, fmt.Errorf("%s: the description of %s is recorded, but the landing record is unreadable: %w; way forward: fix the cause it names, then run `lym beam pr update %s` when a pull request is recorded", verb, slug, err, slug)
	}
	if !found || record.PullRequest == nil {
		result["way_forward"] = fmt.Sprintf("run `lym beam pr open %s` to open the pull request when the operator says so", slug)
		return result, nil
	}
	updated, err := prUpdate(ctx, deps, slug)
	if err != nil {
		return nil, fmt.Errorf("%s: the description of %s is recorded, but its pull request is not updated: %w", verb, slug, err)
	}
	for _, key := range []string{"number", "url", "head", "way_forward"} {
		result[key] = updated[key]
	}
	return result, nil
}

// describeFailure words a failed draft or record step of the description of slug.
// A failure that already names its way forward keeps it;
// any other gets the way forward of fixing the cause and running the verb again.
func describeFailure(verb, slug, step string, err error) error {
	if strings.Contains(err.Error(), "way forward:") {
		return fmt.Errorf("%s: %w", verb, err)
	}
	return fmt.Errorf("%s: %s the description of %s: %w; way forward: fix the cause it names, then run `%s %s` again", verb, step, slug, err, verb, slug)
}
