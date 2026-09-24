// Package bluetooth talks to BlueZ over D-Bus. It contains no GTK
// imports so it can be unit tested and reused by both the GUI and the
// CLI.
package bluetooth

// DeviceID is opaque to callers outside this package. Internally it is
// the D-Bus object path of the device.
type DeviceID string

// EventKind identifies the shape of an Event.
type EventKind int

const (
	// EventReset means the full state was replaced (startup, bluetoothd
	// restart, or adapter switch). State is populated.
	EventReset EventKind = iota
	// EventDeviceAdded means a paired device became visible.
	EventDeviceAdded
	// EventDeviceChanged means a visible device's relevant fields changed.
	EventDeviceChanged
	// EventDeviceRemoved means a device was removed or unpaired.
	EventDeviceRemoved
	// EventAdapterChanged means the current adapter's properties changed.
	// Adapter may be nil if there is no longer a current adapter.
	EventAdapterChanged
)

// String returns a stable, lower-case, hyphenated name for the kind,
// suitable for JSON output (used by the -watch CLI mode).
func (k EventKind) String() string {
	switch k {
	case EventReset:
		return "reset"
	case EventDeviceAdded:
		return "device-added"
	case EventDeviceChanged:
		return "device-changed"
	case EventDeviceRemoved:
		return "device-removed"
	case EventAdapterChanged:
		return "adapter-changed"
	default:
		return "unknown"
	}
}

// Event describes a single change in Bluetooth state. Device and
// Adapter fields, when present, are clones safe to keep across
// goroutines.
type Event struct {
	Kind     EventKind
	DeviceID DeviceID
	Device   *BluetoothDevice
	Adapter  *Adapter
	State    *State
}

// State is a full snapshot of the manager's view of the world.
type State struct {
	ServiceAvailable bool               `json:"service_available"`
	Adapter          *Adapter           `json:"adapter"`
	Devices          []*BluetoothDevice `json:"devices"`
}
