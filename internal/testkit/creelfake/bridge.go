// bridge.go fakes creelengine.MachineBridge, the seam to the Slack bridge of mail across machines.

package creelfake

import (
	"slices"
	"sync"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

var _ creelengine.MachineBridge = (*Bridge)(nil)

// Bridge is a creelengine.MachineBridge whose names and send refusals are scripted.
// With nothing set, Names answers the names it was made with and SendReady answers nil for every orch.
type Bridge struct {
	mu       sync.Mutex
	current  string
	former   []string
	namesErr error
	refused  map[string]error
}

// NewBridge returns a Bridge that answers current as this machine's name and former as its former names.
func NewBridge(current string, former ...string) *Bridge {
	return &Bridge{current: current, former: former, refused: map[string]error{}}
}

// Names implements creelengine.MachineBridge.
// It answers the error FailNames set, if any.
func (b *Bridge) Names() (string, []string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.namesErr != nil {
		return "", nil, b.namesErr
	}
	return b.current, slices.Clone(b.former), nil
}

// SendReady implements creelengine.MachineBridge.
// It answers the error Refuse set for orch, and nil otherwise.
func (b *Bridge) SendReady(orch string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.refused[orch]
}

// Refuse makes SendReady answer err for orch; a nil err removes the refusal.
func (b *Bridge) Refuse(orch string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		delete(b.refused, orch)
		return
	}
	b.refused[orch] = err
}

// FailNames makes Names answer err; a nil err restores the names.
func (b *Bridge) FailNames(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.namesErr = err
}
