// Package stopclass holds the stop classes of a run and the one settlement table that decides who settles each.
// Settle takes a class, an autonomy level, the halt count at the stop's row and lym's settles of the class there.
// It answers lym, the orch or the operator, with lym's bound for the class.
//
// The verdict is read in a fixed order:
//
//  1. The operator, when the level is "true" or "unattended" and the class's operator case holds:
//     always, or at the third halt at the row and after.
//  2. lym, while lym's settles of the class at the row are within its bound.
//  3. The orch otherwise.
//
// At level "false" the operator case never applies, and a stop lym does not settle goes to the orch.
// lym's bound applies at every level.
//
// A bound of one grants lym one settle of its own, such as a fresh start or a re-step, per class at the row.
// An unlimited bound marks a wait lym always settles, such as a pause that lifts.
// A bound of zero grants lym no settle from the table:
// either the class has no lym settle,
// or another settler already applies its own rule first and hands on only what it cannot settle.
// Settle then answers the verdict of the stop that settler handed on.
//
// Some cases of the operator column rest on facts no count shows.
// Examples are a driver still dead after the chase and an agent question showing a run cannot finish within its record.
// Settle answers the counted cases alone.
//
// The package imports only the standard library and does no I/O,
// so every reader of a stop's verdict reads the same table.
package stopclass
