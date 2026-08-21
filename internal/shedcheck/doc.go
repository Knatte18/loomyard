// Package shedcheck is an authoring-time structural analysis of an assembled shedengine producer
// graph: it walks the OnDone/OnStuck routing the same way shedengine.Run would, without ever
// calling a producer, and reports every structural defect it finds.
// It imports internal/shedengine and the standard library, and nothing else.
//
// # Nothing in production calls Check
//
// Neither shedengine.Run nor loomshed.New calls Check.
// Its enforcement point is a go test invariant, not a runtime guard, because reachability-from-
// entry is not a property Run is in a position to assert: a legitimately resumed run starts
// mid-graph from a persisted current_producer, so "is every row reachable from the entry" would
// be the wrong question for Run to ask about its own in-flight state.
// A caller assembling a producer list is expected to call Check in its own test suite, at
// authoring time, before that list ever reaches a Shed.
//
// # Both endpoints are told, never inferred
//
// Check takes entry and terminals as explicit arguments rather than deriving them from the
// producer list. Shed has no entry field and no terminal field of its own -- defaulting either to
// Producers[0] would re-introduce the positional routing meaning internal/shedengine/doc.go
// explicitly disclaims: list order is display and enumeration order only, and carries no routing
// meaning of its own.
//
// # The eight finding kinds
//
// Check reports exactly eight kinds, in this fixed order:
//
//   - bad-entry: entry is empty, or names no producer in the list.
//   - no-terminals: terminals is nil or has length zero.
//   - bad-terminal: one per supplied terminal name that is empty or names no producer.
//   - dangling-target: a live row's OnDone or OnStuck names no producer in the list.
//   - unreachable: a row's own list index is never reached walking done and stuck edges from
//     entry.
//   - unexpected-terminal: a live, reachable row whose raw OnDone is empty and whose Name was not
//     told as a terminal.
//   - done-cycle: one finding per member of a cycle among live, reachable rows using done edges
//     only.
//   - blind-gate: a live, reachable row's stuck target has no route back to it, over both edge
//     types.
//
// The first three are endpoint findings: when any fires, Check returns those findings alone and
// runs no further check, because reachability and terminality are meaningless without told
// endpoints.
//
// # The field-value vs. graph-structure rule
//
// A check that keys on a field value reads the raw field, and a check that keys on graph structure
// reads the post-drop graph -- the graph with every dangling-target edge already removed. Two
// consequences follow: dangling-target and unexpected-terminal never both fire for the same edge,
// because unexpected-terminal keys on the raw OnDone == "" and a dangling OnDone is never empty;
// and a row whose stuck edge is dangling can never be a blind-gate, because the post-drop graph
// has already dropped that edge entirely.
//
// # Deterministic output order
//
// The returned slice is the concatenation of the checks above in exactly the order listed, and
// within each check in producer list order -- except bad-terminal, which follows the order the
// caller supplied terminals in. A clean graph returns nil, not an empty non-nil slice.
//
// # Tolerated malformed input
//
// Check never panics. A nil or empty producers slice, and a row with an empty Name, are all valid
// input that Check degrades into ordinary findings rather than rejecting.
//
// A duplicate Name is one case worth stating outright: it resolves to its first occurrence, exactly
// as shedengine.findProducer's linear first-match scan would, so the shadowed later row is reported
// as unreachable rather than as a duplicate of any kind -- Check emits no duplicate-flavoured kind
// at all. shedengine.validate() already rejects a duplicate Name with its own distinct message, so
// a caller who only ever sees shedcheck's diagnosis on a duplicate should not read "unreachable" as
// the row's actual defect.
//
// # One perch mis-wiring Check cannot catch
//
// A Burler round producer that hands back to its Bouncer via OnDone instead of OnStuck is invisible
// to Check: both wirings make the gate reachable from the round producer, so the routing graph is
// identical either way. The difference is which verdict the round producer returns, which is
// behaviour inside Call and not a property of the graph Check inspects.
//
// # What Check never reads or calls
//
// Check never reads ProducerDef.Segment, and it never calls a producer -- it reads only Name,
// OnDone, and OnStuck off each entry.
package shedcheck
