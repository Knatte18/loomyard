// bridge.go fakes creelengine.MachineBridge, the seam to the Slack bridge of mail across machines and the hub's relay.

package creelfake

import (
	"slices"
	"sync"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

var _ creelengine.MachineBridge = (*Bridge)(nil)

// Bridge is a creelengine.MachineBridge whose names, refusals and relay state are scripted.
// With nothing set, Names answers the names it was made with.
// With nothing set, SendReady answers nil for every orch.
// With nothing set, RelayPostReady answers the relay as running for every sender.
type Bridge struct {
	mu        sync.Mutex
	current   string
	former    []string
	namesErr  error
	refused   map[string]error
	relayDown bool
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

// RelayPostReady implements creelengine.MachineBridge.
// It answers the error Refuse set for sender, and otherwise whether SetRelayLive left the relay running.
func (b *Bridge) RelayPostReady(sender string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.refused[sender]; err != nil {
		return false, err
	}
	return !b.relayDown, nil
}

// Refuse makes SendReady and RelayPostReady answer err for the named sender; a nil err removes the refusal.
func (b *Bridge) Refuse(name string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		delete(b.refused, name)
		return
	}
	b.refused[name] = err
}

// SetRelayLive sets whether RelayPostReady answers the hub's relay as running.
func (b *Bridge) SetRelayLive(live bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.relayDown = !live
}

// FailNames makes Names answer err; a nil err restores the names.
func (b *Bridge) FailNames(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.namesErr = err
}
