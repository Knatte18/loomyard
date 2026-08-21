// batchifier.go implements the Batchifier producer: a fail-fast gate confirming the active
// batchifier resolves cleanly, before Webster spawns any LLM session.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// batchifier is the Batchifier producer. It writes no artifact -- Call returns only (Outcome,
// OutputPointer, error), so there is no channel for a handover to the Webster row in the first
// place.
type batchifier struct {
	name       string
	anchorPath string
}

var _ shedengine.ShedProducer = (*batchifier)(nil)

// NewBatchifier returns a batchifier identified as name, gating batcher.Active(anchorPath). The
// return type is shedengine.ShedProducer, the seam interface, so the internal/shedrecipe registry
// can call this constructor from outside this package while batchifier itself stays unexported.
func NewBatchifier(name, anchorPath string) shedengine.ShedProducer {
	return &batchifier{name: name, anchorPath: anchorPath}
}

// Call implements shedengine.ShedProducer: it calls batcher.Active(b.anchorPath) and maps every
// error to shedengine.Stuck, success to shedengine.Done, in both cases reporting an empty
// shedengine.OutputPointer.
//
// batcher.Active returns a bare error for unknown-name, malformed YAML, and I/O failure alike, with
// no sentinel to discriminate on, so all three conflate onto Stuck here. That conflation is
// accepted: Active already falls back to the embedded batcher.ConfigTemplate() when the config file
// or its directory is absent, so a remaining error is a genuinely broken config far more often than
// an infra fault, and blocked is the right resting state for an operator-fixable fault.
func (b *batchifier) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, b.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if _, err := batcher.Active(b.anchorPath); err != nil {
		if cerr := cancelErr(ctx, b.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{}, nil
}
