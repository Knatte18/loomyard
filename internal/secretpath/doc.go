// Package secretpath decides whether a repository-relative path names a secret file.
// No lym commit path may add such a file, for example a `.env` file or a private key.
// lym ships a fixed pattern list; a hub adds its own patterns, which never lift a shipped one.
// It imports the standard library only.
package secretpath
