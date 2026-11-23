// bodyfile.go supplies an upsert's "body" from a file or stdin, so a long markdown body needs no JSON escaping.

package boardcli

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/output"
)

// stdinPath is the --body-file value that reads stdin.
// It is also the payload argument spelling that the helper refuses to combine with it.
const stdinPath = "-"

// applyBodyFile sets fields["body"] from the file named by bodyFile, or from stdin when bodyFile is "-".
// payloadArg is the raw payload argument, only consulted to refuse stdin feeding both the payload and the body.
// It refuses a payload that already carries "body", and a read failure names the path.
// It refuses a file holding a CLI error envelope line outside a closed fenced code block:
// a failed verb's captured output leaves one where the body belongs.
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
	if envelope, isEnvelope := output.ErrorEnvelopeLine(string(data)); isEnvelope {
		return fmt.Errorf("--body-file %s holds a CLI error envelope, %s, where the body's text belongs: "+
			"the verb whose output fed the file failed, so fix that verb's call and write the file again", bodyFile, envelope)
	}
	fields["body"] = string(data)
	return nil
}
