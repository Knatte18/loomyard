// export_test.go exposes the unexported poll, read and start machinery to the package's external tests.

package slackengine

import (
	"context"
	"time"
)

// Floor and Ceiling are the shortest and longest wait a relay loop takes between passes.
const (
	Floor   = waitFloor
	Ceiling = waitCeiling
)

// Inbound is the unexported inbound poll.
type Inbound = inbound

// NewInbound builds an inbound poll over the seams.
func NewInbound(geom Geometry, mailbox Mailbox, slack SlackClient, hub Hub, clock Clock) *Inbound {
	return &inbound{geom: geom, mailbox: mailbox, slack: slack, hub: hub, clock: clock}
}

// Poll runs one inbound poll.
func (in *inbound) Poll(ctx context.Context) (bool, error) { return in.poll(ctx) }

// Outbound is the unexported outbound read.
type Outbound = outbound

// NewOutbound builds an outbound read over the seams, with a floor of its own.
func NewOutbound(geom Geometry, mailbox Mailbox, slack SlackClient, clock Clock) *Outbound {
	return newOutbound(geom, mailbox, slack, clock, newSharedFloor())
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
