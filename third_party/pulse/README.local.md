Vendored copy of the `proto` package of github.com/jfreymuth/pulse v0.1.3 (MIT, see LICENSE).

Local changes:
- `proto/objectmessage.go`: adds PA_COMMAND_SEND_OBJECT_MESSAGE (op 104), used to list/switch Bluetooth codecs.
- `proto/client.go`: no longer prints unknown opcodes to stdout.

Wired in via the `replace` directive in the top-level go.mod.
