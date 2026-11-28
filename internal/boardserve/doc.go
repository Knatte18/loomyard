// Package boardserve is the HTTP server of one board, bound to the loopback address.
// It serves the viewer page and takes the page's writes.
// It also serves the board's view projection, its live changes and its design and decision files.
//
// # Routes
//
// `GET /` serves the viewer page with a Content-Security-Policy that admits its own inline script and style by hash.
// `GET /api/board` answers the view projection and the board's revision.
// With `after=<revision>` naming the current revision it waits for another one, within a bound.
// It answers `unchanged` when none comes.
// `GET /file/<name>` serves a design or decision file the projection names, as plain text.
// `POST /api/write` applies one write: create, edit, release, promote, draft or undraft.
//
// # Checks
//
// Every request must name the loopback address and port as its Host, which stops DNS rebinding.
// An /api request carries the token as a bearer token.
// A file request carries it in the `t` query parameter, since a link cannot set a header.
// The page itself needs none, as it holds no board data.
// A write must be JSON, come from the server's own origin when it names one, and stay within the body bound.
// No response is cached, and none grants another origin access.
// The token and the loopback address bound the server to the user's own processes.
//
// # Writes
//
// The package writes the board only through boardengine's exported board methods.
// The run lock, the label rules and the task-creation rule therefore apply as they do to any other writer.
// Every write states the revision of the entry the page read, except a create, which expects no entry.
// On top of those rules the server releases no task a run holds, and release, draft and undraft take only a task.
//
// # Lifecycle
//
// At most one process serves per state directory, the one holding its lock, so Live answers from the lock alone.
// The server records its pid, port and token in a state file only its owner reads.
// A launcher page beside it sends a browser to the server with the token in the address fragment.
// One watch loop notices board.json changing, by a file event or a timer, and wakes the long polls,
// so a write by any process reaches the page.
// The same loop ends the server once its binary is replaced or no request for board data has come within the idle bound.
// An exit stops taking connections, releases the lock and then waits for the requests in flight,
// so a successor can start at once and a write under way still completes.
// There is no stop request.
package boardserve
