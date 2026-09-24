package bluetooth

import (
	"strings"

	"github.com/godbus/dbus/v5"
)

// parseBatteryPercent extracts the Percentage property from a Battery1
// property map. BlueZ types it as a D-Bus byte (Go uint8), but other
// integer types are accepted for robustness. The result is clamped to
// 0..100.
func parseBatteryPercent(props map[string]dbus.Variant) (int, bool) {
	v, ok := props["Percentage"]
	if !ok {
		return 0, false
	}
	var p int
	switch n := v.Value().(type) {
	case byte:
		p = int(n)
	case int32:
		p = int(n)
	case uint32:
		p = int(n)
	case int:
		p = n
	default:
		return 0, false
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p, true
}

// batteryLabel derives the label for a Battery1 object attached to
// devicePath. It is "" when the battery lives directly on the device
// path, otherwise the last path element after "devicePath/".
func batteryLabel(devicePath, batteryPath dbus.ObjectPath) string {
	if batteryPath == devicePath {
		return ""
	}
	prefix := string(devicePath) + "/"
	s := string(batteryPath)
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(s, prefix)
	return rest
}

// primaryBattery picks the "primary" percentage to show as
// BluetoothDevice.Battery: the Label=="" level if present, otherwise
// the minimum Percent across all levels. Returns nil for an empty
// slice.
func primaryBattery(levels []BatteryLevel) *int {
	if len(levels) == 0 {
		return nil
	}
	for _, l := range levels {
		if l.Label == "" {
			p := l.Percent
			return &p
		}
	}
	min := levels[0].Percent
	for _, l := range levels[1:] {
		if l.Percent < min {
			min = l.Percent
		}
	}
	return &min
}
