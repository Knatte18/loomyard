// landing.go holds a beam's landing record, the tracked record of what the beam's orch has done toward landing it: when it last wrote the description and which pull request it opened.

package pawlengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// LandingRecord is a beam's landing record.
// It names the beam it was written for, since a pair forked from the beam inherits the beam's records.
type LandingRecord struct {
	// Slug is the beam the record was written for.
	Slug string `json:"slug"`
	// DescribedAt is when the description was last written for this beam; zero for never.
	DescribedAt time.Time `json:"described_at"`
	// PullRequest is the beam's recorded pull request; nil while none is recorded.
	PullRequest *LandingPullRequest `json:"pull_request,omitempty"`
}

// LandingPullRequest is the pull request a landing record names.
type LandingPullRequest struct {
	// Number is the pull request's number.
	Number int `json:"number"`
	// URL is the pull request's web address.
	URL string `json:"url"`
	// PushedHead is the beam head the beam branch was last pushed with.
	PushedHead string `json:"pushed_head"`
	// AnnounceHead is the beam head the member-landing notice announces from.
	AnnounceHead string `json:"announce_head"`
}

// errNoPullRequestToDrop aborts DropLandingPullRequest's update, so a record with nothing to drop is not rewritten.
var errNoPullRequestToDrop = errors.New("no pull request to drop")

// ReadLanding reads the landing record at path under the lock at lockPath.
// An absent record, and one naming another slug, read as not found.
func ReadLanding(path, lockPath, slug string) (LandingRecord, bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return LandingRecord{}, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return LandingRecord{}, false, fmt.Errorf("pawlengine: create the landing record's lock directory: %w", err)
	}
	record, found, err := state.ReadJSON[LandingRecord](path, lockPath)
	if err != nil {
		return LandingRecord{}, false, fmt.Errorf("pawlengine: read the landing record: %w", err)
	}
	if !found || record.Slug != slug {
		return LandingRecord{}, false, nil
	}
	return record, true, nil
}

// UpdateLanding applies mutate to the landing record of the beam slug at path, under one hold of the lock at lockPath, and writes the result.
// An absent record, and one naming another slug, start from a record naming slug alone, so a record inherited from another beam is replaced and never removed.
// An error from mutate is returned and nothing is written.
func UpdateLanding(path, lockPath, slug string, mutate func(rec *LandingRecord) error) error {
	err := state.UpdateJSON(path, lockPath, func(cur LandingRecord, found bool) (LandingRecord, error) {
		if !found || cur.Slug != slug {
			cur = LandingRecord{Slug: slug}
		}
		if err := mutate(&cur); err != nil {
			return LandingRecord{}, err
		}
		return cur, nil
	})
	if err != nil {
		return fmt.Errorf("pawlengine: update the landing record: %w", err)
	}
	return nil
}

// DropLandingPullRequest clears the pull request of the landing record of the beam slug at path and reports whether it cleared one.
// An absent record, one naming another slug and one recording no pull request are left as they are.
func DropLandingPullRequest(path, lockPath, slug string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}
	err := state.UpdateJSON(path, lockPath, func(cur LandingRecord, found bool) (LandingRecord, error) {
		if !found || cur.Slug != slug || cur.PullRequest == nil {
			return cur, errNoPullRequestToDrop
		}
		cur.PullRequest = nil
		return cur, nil
	})
	if errors.Is(err, errNoPullRequestToDrop) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("pawlengine: drop the landing record's pull request: %w", err)
	}
	return true, nil
}
