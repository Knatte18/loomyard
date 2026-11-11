// export_test.go exposes the unexported poll, read and start machinery to the package's external tests.

package slackengine

import (
	"context"
	"time"
)

// Floor and Ceiling are the shortest and longest wait a relay loop takes between passes.
// WakeFloor is the shortest time between the starts of two wakes of the wake loop.
const (
	Floor     = waitFloor
	Ceiling   = waitCeiling
	WakeFloor = wakeFloor
)

// Inbound is the unexported inbound read of the hub's channels.
type Inbound = inbound

// NewInbound builds an inbound read over the seams.
func NewInbound(geom Geometry, mailbox Mailbox, slack SlackClient, hub Hub, clock Clock, keeper *ChannelKeeper) *Inbound {
	return newInbound(geom, mailbox, slack, hub, clock, keeper)
}

// Start owes every open channel a read with its thread scan.
func (in *inbound) Start() { in.start() }

// Drain makes the reads the inbox's hints ask for.
func (in *inbound) Drain(ctx context.Context) (bool, error) { return in.drain(ctx) }

// RetryOwed makes each owed read whose time has come.
func (in *inbound) RetryOwed(ctx context.Context) bool { return in.retryOwed(ctx) }

// Outbound is the unexported outbound read.
type Outbound = outbound

// NewOutbound builds an outbound read over the seams, with a floor of its own.
func NewOutbound(geom Geometry, mailbox Mailbox, slack SlackClient, clock Clock, keeper *ChannelKeeper, hub Hub) *Outbound {
	return newOutbound(geom, mailbox, slack, clock, newSharedFloor(), keeper, hub)
}

// Read runs one outbound read and returns the wait before the next.
func (out *outbound) Read(ctx context.Context) (time.Duration, error) { return out.read(ctx) }

// Starter is the unexported starter of the relay.
type Starter = starter

// NewStarter builds a starter over a fake spawn, clock and pid.
func NewStarter(spawn func(geom Geometry, args []string, wait bool) error, clock Clock, pid func() int) Starter {
	return starter{spawn: spawn, clock: clock, pid: pid}
}

// Ensure runs the starter's start.
func (s starter) Ensure(geom Geometry, primeName string, whileAway bool) (StartOutcome, error) {
	return s.ensure(geom, primeName, whileAway)
}

// SpawnRelay runs the starter's relay spawn.
func (s starter) SpawnRelay(geom Geometry, primeName string) error {
	return s.spawnRelay(geom, primeName)
}

// UpdateRelayState applies mutate to the relay state under the record's lock, as a relay or its starter does.
func UpdateRelayState(geom Geometry, mutate func(*RelayState)) error {
	return updateRelayState(geom, mutate)
}

// RelayHealthyLifetime is the lifetime a spawned relay needs for its spawn to count as healthy.
const RelayHealthyLifetime = relayHealthyLifetime

// MaxStartFailures is how many young relays in a row end the starts made while the operator is away.
const MaxStartFailures = maxStartFailures

// The timings of the socket holder, and the name of the hint that asks a relay to read every channel.
const (
	PingInterval     = pingInterval
	PongTimeout      = pongTimeout
	EnvelopeLimit    = envelopeLimit
	ReconnectFloor   = reconnectFloor
	StableConnection = stableConnection
	ResyncHint       = resyncHint
)

// SocketHolder is the unexported holder of the machine's Socket Mode connection.
type SocketHolder = socketHolder

// NewSocketHolder builds a holder whose hint writer is write; a nil write keeps the holder's own.
func NewSocketHolder(geom Geometry, dial Dialer, liveHubs LiveHubs, mailbox Mailbox, slack SlackClient, clock Clock, write func(Geometry, string) error) *SocketHolder {
	holder := newSocketHolder(geom, dial, liveHubs, mailbox, slack, clock)
	if write != nil {
		holder.write = write
	}
	return holder
}

// Run runs the holder until ctx ends.
func (h *socketHolder) Run(ctx context.Context) { h.run(ctx) }

// WriteHint writes the named hint into the hub's inbox as the holder does.
func WriteHint(geom Geometry, name string) error { return writeHint(geom, name) }

// ChannelKeeper is the unexported keeper of a hub's Slack channels.
type ChannelKeeper = channelKeeper

// ErrChannelRefused is the error a refused channel wraps.
var ErrChannelRefused = errChannelRefused

// ErrForwardingOff is the error an ensure wraps for an orch whose slack button is off and that holds no open channel.
var ErrForwardingOff = errForwardingOff

// NewChannelKeeper builds a keeper over the seams.
func NewChannelKeeper(geom Geometry, slack SlackClient, hub Hub, liveHubs LiveHubs, mailbox Mailbox, clock Clock) *ChannelKeeper {
	return newChannelKeeper(geom, slack, hub, liveHubs, mailbox, clock)
}

// Ensure ensures the orch's channel.
func (k *channelKeeper) Ensure(ctx context.Context, orch string) (ChannelEntry, error) {
	return k.ensure(ctx, orch)
}

// Join joins the orch's open channel.
func (k *channelKeeper) Join(ctx context.Context, orch string) error { return k.join(ctx, orch) }

// InviteOperators invites the setup's operator into every open channel that lacks the operator.
func (k *channelKeeper) InviteOperators(ctx context.Context) error { return k.inviteOperators(ctx) }

// Retire retires the channels of the beams the list does not name.
func (k *channelKeeper) Retire(ctx context.Context, beams []string, retryRefused bool) error {
	return k.retire(ctx, beams, retryRefused)
}
