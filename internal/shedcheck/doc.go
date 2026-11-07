// Package shedcheck is an authoring-time structural analysis of an assembled shedengine producer graph.
// It walks the OnDone/OnStuck routing the way shedengine.Run would, without calling a producer.
// It reports every structural defect it finds.
// It imports internal/shedengine and the standard library, and nothing else.
//
// # Nothing in production calls Check
//
// Neither shedengine.Run nor loomrecipe.New calls Check.
// Its enforcement point is a go test invariant, not a runtime guard.
// Run cannot assert reachability from entry.
// A legitimately resumed run starts mid-graph from a persisted current_producer.
// Reachability from the entry says nothing about in-flight state.
// A caller assembling a producer list calls Check in its own test suite, before that list reaches a Shed.
//
// # Both endpoints are told, never inferred
//
// Check takes entry and terminals as explicit arguments rather than deriving them from the producer list.
// Shed has no entry field and no terminal field of its own.
// Defaulting either to Producers[0] would give list order a routing meaning it does not have.
// List order is for display and enumeration only.
//
// # The finding kinds
//
// Check reports these kinds, in this fixed order:
//
//   - bad-entry: entry is empty, or names no producer in the list.
//   - no-terminals: terminals is nil or has length zero.
//   - bad-terminal: one per supplied terminal name that is empty or names no producer.
//   - dangling-target: a live row's OnDone or OnStuck names no producer in the list.
//   - unreachable: a row's own list index is never reached walking done and stuck edges from entry.
//   - unexpected-terminal: a live, reachable row whose raw OnDone is empty and whose Name was not told as a terminal.
//   - done-cycle: one finding per member of a cycle among live, reachable rows using done edges only.
//   - blind-gate: a live, reachable row's stuck target has no route back to it, over both edge types.
//
// bad-entry, no-terminals and bad-terminal are the endpoint findings.
// When any fires, Check returns those findings alone and runs no further check.
// Reachability and terminality are meaningless without told endpoints.
//
// # The field-value vs. graph-structure rule
//
// A check that keys on a field value reads the raw field,
// and a check that keys on graph structure reads the post-drop graph, which has every dangling-target edge removed.
// Dangling-target and unexpected-terminal never both fire for the same edge.
// Unexpected-terminal keys on the raw OnDone == "", and a dangling OnDone is never empty.
// A row whose stuck edge is dangling can never be a blind-gate, since the post-drop graph has dropped that edge.
//
// # Deterministic output order
//
// The returned slice concatenates the checks above in the order listed, each in producer list order.
// The exception is bad-terminal, which follows the order the caller supplied terminals in.
// A clean graph returns nil, not an empty non-nil slice.
//
// # Tolerated malformed input
//
// Check never panics.
// A nil or empty producers slice, and a row with an empty Name, are valid input.
// Check degrades them into ordinary findings rather than rejecting them.
//
// A duplicate Name resolves to its first occurrence, as shedengine.findProducer's linear first-match scan does.
// The shadowed later row is therefore reported as unreachable, and Check emits no duplicate-specific kind.
// shedengine.validate() rejects a duplicate Name with its own message,
// so a caller who sees only shedcheck's diagnosis should not read "unreachable" as the row's actual defect.
//
// # One perch mis-wiring Check cannot catch
//
// A Burler round producer that hands back to its Bouncer via OnDone instead of OnStuck is invisible to Check:
// both wirings make the gate reachable from the round producer, so the routing graph is identical either way.
// The difference is the verdict the round producer returns.
// That is behaviour inside Call, not a property of the graph Check inspects.
//
// # What Check never reads or calls
//
// Check never reads ProducerDef.Segment and never calls a producer.
// It reads only Name, OnDone, and OnStuck off each entry.
package shedcheck
