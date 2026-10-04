// bodyfile.go supplies an upsert's "body" from a file or stdin, so a long markdown body needs no JSON escaping.

package boardcli

import (
	"fmt"
	"io"
	"os"
)

// stdinPath is the --body-file value that reads stdin, and the payload argument spelling the helper refuses to share it with.
const stdinPath = "-"

// applyBodyFile sets fields["body"] from the file named by bodyFile, or from stdin when bodyFile is "-".
// payloadArg is the raw payload argument, only consulted to refuse stdin feeding both the payload and the body.
// It refuses a payload that already carries "body", and a read failure names the path.
func applyBodyFile(fields map[string]any, bodyFile, payloadArg string, stdin io.Reader) error {
	if bodyFile == stdinPath && payloadArg == stdinPath {
		return fmt.Errorf(`stdin cannot supply both the payload and the body: pass the payload as an argument, or give --body-file a path`)
	}
	if _, has := fields["body"]; has {
		return fmt.Errorf(`"body" is given both in the payload and by --body-file %s: drop "body" from the payload, or drop the flag`, bodyFile)
	}
	var data []byte
	var err error
	if bodyFile == stdinPath {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(bodyFile)
	}
	if err != nil {
		return fmt.Errorf("read --body-file %s: %w", bodyFile, err)
	}
	fields["body"] = string(data)
	return nil
}
