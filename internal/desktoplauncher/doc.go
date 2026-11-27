// Package desktoplauncher writes the desktop launchers that run `lym machine start`:
// one per hub, which starts that hub, and one for every hub, which starts every live hub of this machine.
// It alone names where the launchers live and what they hold;
// they sit on the machine's desktop, outside every hub, and no tracked record names them.
//
// Locate finds this machine's desktop directory and the form its launchers take:
//
//   - On Linux, a freedesktop `.desktop` file in the desktop directory `user-dirs.dirs` names, else `$HOME/Desktop`.
//   - Under WSL, a `.cmd` file on the Windows desktop, which runs the verb in this distribution through `wsl.exe`.
//   - On native Windows, a `.cmd` file on the Windows desktop that runs the verb directly.
//
// XDG_DESKTOP_DIR, when set, names the desktop directory on every platform,
// so a test process points it at a directory of its own.
// A desktop directory that does not exist, or a platform with no launcher form, is ErrNoDesktop:
// a machine with no desktop gets no launcher.
//
// A hub's launcher is named by the hub's shortname, `lym-hub-<shortname>` with the form's extension,
// and the all-hubs launcher is `lym-hubs`.
// A `.desktop` launcher shows its `Name=` key.
// That key is `lym: <shortname>` for a hub and `lym: start every hub` for the all-hubs launcher.
// A `.cmd` launcher shows its file name and carries the same title on its `rem` line.
// Remove removes the all-hubs launcher once no hub's launcher stands.
// Add and Remove hold one lock per desktop, kept in the per-user state directory,
// so concurrent processes never leave a hub launcher standing without the all-hubs one.
package desktoplauncher
