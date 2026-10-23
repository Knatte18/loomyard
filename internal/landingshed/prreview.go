// prreview.go implements the PR-Review stub: the seat the human pull-request review engine will take over.
// It acts on nothing and always lets the run pass.

package landingshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// PRReview is the ShedProducer standing in for the pull-request review that is not built yet.
// It opens no pull request, calls no GitHub API, reads no file and takes no lock, so it lets nothing through that its absence would not.
type PRReview struct {
	name    string
	enabled bool
}

var _ shedengine.ShedProducer = (*PRReview)(nil)

// NewPRReview constructs a PRReview that reports under name.
// With enabled set, each call logs once that the PR review is not built; otherwise a call logs nothing.
func NewPRReview(name string, enabled bool) *PRReview {
	return &PRReview{name: name, enabled: enabled}
}

// Call returns Done.
// A cancelled context is an error, as for every landing row.
func (p *PRReview) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if p.enabled {
		logger.Info("landingshed: the PR review is enabled but not built; passing", "row", p.name)
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
