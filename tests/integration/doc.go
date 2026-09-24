// Package integration holds opt-in tests that talk to the real BlueZ
// over the system D-Bus. They are read-only: they never call Connect,
// Disconnect or SetAdapterPowered. Build with -tags integration.
package integration
