package bluetooth

import (
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	ifaceAdapter = "org.bluez.Adapter1"
	ifaceDevice  = "org.bluez.Device1"
	ifaceBattery = "org.bluez.Battery1"
	bluezService = "org.bluez"
)

// BatteryLevel is one battery reading attached to a device. Label is ""
// for the device's main/only battery, otherwise a lower-case tag such
// as "left", "right" or "case" derived from the D-Bus object path.
type BatteryLevel struct {
	Label   string          `json:"label"`
	Percent int             `json:"percent"`
	Path    dbus.ObjectPath `json:"-"`
}

// BluetoothDevice is the UI- and CLI-facing model of a single BlueZ
// Device1 object, optionally enriched with Battery1 data. It never
// contains GTK types.
type BluetoothDevice struct {
	ID        DeviceID        `json:"id"`
	Path      dbus.ObjectPath `json:"path"`
	Adapter   dbus.ObjectPath `json:"adapter"`
	Address   string          `json:"address"`
	Name      string          `json:"name"`
	Alias     string          `json:"alias"`
	Icon      string          `json:"icon"`
	Paired    bool            `json:"paired"`
	Connected bool            `json:"connected"`
	Trusted   bool            `json:"trusted"`
	Battery   *int            `json:"battery"`
	Batteries []BatteryLevel  `json:"batteries"`
}

// DisplayName returns the best human-readable name: trimmed Alias,
// falling back to trimmed Name, falling back to the device Address.
func (d *BluetoothDevice) DisplayName() string {
	if a := strings.TrimSpace(d.Alias); a != "" {
		return a
	}
	if n := strings.TrimSpace(d.Name); n != "" {
		return n
	}
	return d.Address
}

// Clone returns a deep copy safe to hand to another goroutine.
func (d *BluetoothDevice) Clone() *BluetoothDevice {
	if d == nil {
		return nil
	}
	c := *d
	if d.Battery != nil {
		b := *d.Battery
		c.Battery = &b
	}
	if d.Batteries != nil {
		c.Batteries = make([]BatteryLevel, len(d.Batteries))
		copy(c.Batteries, d.Batteries)
	}
	return &c
}

// Equal compares all exported fields, including batteries.
func (d *BluetoothDevice) Equal(o *BluetoothDevice) bool {
	if d == nil || o == nil {
		return d == o
	}
	if d.ID != o.ID || d.Path != o.Path || d.Adapter != o.Adapter ||
		d.Address != o.Address || d.Name != o.Name || d.Alias != o.Alias ||
		d.Icon != o.Icon || d.Paired != o.Paired || d.Connected != o.Connected ||
		d.Trusted != o.Trusted {
		return false
	}
	if (d.Battery == nil) != (o.Battery == nil) {
		return false
	}
	if d.Battery != nil && *d.Battery != *o.Battery {
		return false
	}
	if len(d.Batteries) != len(o.Batteries) {
		return false
	}
	for i := range d.Batteries {
		if d.Batteries[i] != o.Batteries[i] {
			return false
		}
	}
	return true
}

// applyDeviceProps sets only the keys present in props, leaving the
// rest of d untouched. Mismatched variant types are ignored via
// comma-ok assertions.
func applyDeviceProps(d *BluetoothDevice, props map[string]dbus.Variant) {
	if v, ok := props["Address"]; ok {
		if s, ok := v.Value().(string); ok {
			d.Address = s
		}
	}
	if v, ok := props["Name"]; ok {
		if s, ok := v.Value().(string); ok {
			d.Name = s
		}
	}
	if v, ok := props["Alias"]; ok {
		if s, ok := v.Value().(string); ok {
			d.Alias = s
		}
	}
	if v, ok := props["Icon"]; ok {
		if s, ok := v.Value().(string); ok {
			d.Icon = s
		}
	}
	if v, ok := props["Paired"]; ok {
		if b, ok := v.Value().(bool); ok {
			d.Paired = b
		}
	}
	if v, ok := props["Connected"]; ok {
		if b, ok := v.Value().(bool); ok {
			d.Connected = b
		}
	}
	if v, ok := props["Trusted"]; ok {
		if b, ok := v.Value().(bool); ok {
			d.Trusted = b
		}
	}
	if v, ok := props["Adapter"]; ok {
		if p, ok := v.Value().(dbus.ObjectPath); ok {
			d.Adapter = p
		}
	}
}

// invalidateDeviceProps clears the fields named by PropertiesChanged's
// "invalidated" list.
func invalidateDeviceProps(d *BluetoothDevice, names []string) {
	for _, n := range names {
		switch n {
		case "Name":
			d.Name = ""
		case "Alias":
			d.Alias = ""
		case "Icon":
			d.Icon = ""
		case "Paired":
			d.Paired = false
		case "Connected":
			d.Connected = false
		case "Trusted":
			d.Trusted = false
		}
	}
}
