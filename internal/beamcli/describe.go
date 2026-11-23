// describe.go implements `lym beam describe`: it judges the named beam and records the change description its orch wrote.
// It stamps the description in the landing record, commits it and brings a recorded pull request up to date.
// With --draft it writes a starting point for the orch to edit instead.
// The same sequence records the draft the beam's describer agent writes after a member landing.

package beamcli

import (
	"context"
	"fmt"
	"strings"
)

// describeVerb is the verb the describe sequence's refusals and ways forward name.
const describeVerb = "lym beam describe"

// describe records the beam slug's change description, which its orch wrote.
// It returns the description's path and the verbs that follow.
// When the landing record names a pull request, it then runs `lym beam pr update`'s sequence,
// so the pull request carries the recorded text.
// With draft it instead writes a starting point where no description stands yet and records nothing.
// It refuses what judgeDescribable refuses.
func describe(ctx context.Context, deps landDeps, slug string, draft bool) (map[string]any, error) {
	if err := judgeDescribable(deps, slug); err != nil {
		return nil, err
	}
	if draft {
		path, err := deps.DraftDescription(slug)
		if err != nil {
			return nil, describeFailure(slug, "draft", err)
		}
		return map[string]any{
			"slug":        slug,
			"description": path,
			"drafted":     true,
			"way_forward": fmt.Sprintf("edit the draft into the beam's end result and its member pull requests, call the Merge danger door and blast radius, then run `%s %s` to record it", describeVerb, slug),
		}, nil
	}
	return recordDescription(ctx, deps, slug, "")
}

// rewriteDescription writes the draft at draftPath over the beam slug's change description.
// It then records the description as describe does.
// It refuses what judgeDescribable refuses, and a draft with a finding;
// either refusal leaves the recorded description standing.
func rewriteDescription(ctx context.Context, deps landDeps, slug, draftPath string) (map[string]any, error) {
	if err := judgeDescribable(deps, slug); err != nil {
		return nil, err
	}
	return recordDescription(ctx, deps, slug, draftPath)
}

// judgeDescribable refuses an entry that is no open beam of this machine's pair;
// a beam whose pawl run is absent or not at Beam-Await;
// or a beam whose ready record stands.
// Together these keep anything at or after landing from rewriting the description.
func judgeDescribable(deps landDeps, slug string) error {
	owners, err := deps.ReadOwners()
	if err != nil {
		return fmt.Errorf("%s: read the board's owners: %w", describeVerb, err)
	}
	if err := judgeLandEntry(deps, owners, describeVerb, slug); err != nil {
		return err
	}
	if err := judgeAwaitingRun(deps, describeVerb, slug); err != nil {
		return err
	}
	return judgeNotReady(deps, describeVerb, slug)
}

// recordDescription records the beam slug's change description, from the draft at from when it is set.
// It returns the description's path and the verbs that follow.
// When the landing record names a pull request, it then runs `lym beam pr update`'s sequence,
// so the pull request carries the recorded text.
func recordDescription(ctx context.Context, deps landDeps, slug, from string) (map[string]any, error) {
	path, err := deps.RecordDescription(slug, from, deps.Now().UTC())
	if err != nil {
		return nil, describeFailure(slug, "record", err)
	}
	result := map[string]any{"slug": slug, "description": path}
	record, found, err := deps.Landing(slug)
	if err != nil {
		return nil, afterRecordError{fmt.Errorf("%s: the description of %s is recorded, but the landing record is unreadable: %w; way forward: fix the cause it names, then run `lym beam pr update %s` when a pull request is recorded", describeVerb, slug, err, slug)}
	}
	if !found || record.PullRequest == nil {
		result["way_forward"] = fmt.Sprintf("run `lym beam pr open %s` to open the pull request when the operator says so", slug)
		return result, nil
	}
	updated, err := prUpdate(ctx, deps, slug)
	if err != nil {
		return nil, afterRecordError{fmt.Errorf("%s: the description of %s is recorded, but its pull request is not updated: %w", describeVerb, slug, err)}
	}
	for _, key := range []string{"number", "url", "head", "way_forward"} {
		result[key] = updated[key]
	}
	return result, nil
}

// afterRecordError wraps the failure of a step that follows the record of a description, which stands recorded by then.
type afterRecordError struct{ err error }

func (e afterRecordError) Error() string { return e.err.Error() }

func (e afterRecordError) Unwrap() error { return e.err }

// describeFailure words a failed draft or record step of the description of slug.
// A failure that already names its way forward keeps it;
// any other gets the way forward of fixing the cause and running the verb again.
func describeFailure(slug, step string, err error) error {
	if strings.Contains(err.Error(), "way forward:") {
		return fmt.Errorf("%s: %w", describeVerb, err)
	}
	return fmt.Errorf("%s: %s the description of %s: %w; way forward: fix the cause it names, then run `%s %s` again", describeVerb, step, slug, err, describeVerb, slug)
}
