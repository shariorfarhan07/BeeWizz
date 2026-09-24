package bluetooth

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestApplyDeviceProps_FullParse(t *testing.T) {
	d := &BluetoothDevice{}
	props := map[string]dbus.Variant{
		"Address":   dbus.MakeVariant("AA:BB:CC:DD:EE:FF"),
		"Name":      dbus.MakeVariant("Some Device"),
		"Alias":     dbus.MakeVariant("My Device"),
		"Icon":      dbus.MakeVariant("audio-headset"),
		"Paired":    dbus.MakeVariant(true),
		"Connected": dbus.MakeVariant(true),
		"Trusted":   dbus.MakeVariant(true),
		"Adapter":   dbus.MakeVariant(dbus.ObjectPath("/org/bluez/hci0")),
	}
	applyDeviceProps(d, props)
	if d.Address != "AA:BB:CC:DD:EE:FF" || d.Name != "Some Device" || d.Alias != "My Device" ||
		d.Icon != "audio-headset" || !d.Paired || !d.Connected || !d.Trusted ||
		d.Adapter != "/org/bluez/hci0" {
		t.Fatalf("unexpected device: %+v", d)
	}
}

func TestApplyDeviceProps_MissingFields(t *testing.T) {
	d := &BluetoothDevice{Name: "existing"}
	applyDeviceProps(d, map[string]dbus.Variant{
		"Address": dbus.MakeVariant("11:22:33:44:55:66"),
	})
	if d.Address != "11:22:33:44:55:66" {
		t.Fatalf("address not applied")
	}
	if d.Name != "existing" {
		t.Fatalf("existing field should be untouched, got %q", d.Name)
	}
	if d.Icon != "" {
		t.Fatalf("icon should stay empty")
	}
}

func TestApplyDeviceProps_WrongVariantTypeIgnored(t *testing.T) {
	d := &BluetoothDevice{Connected: false}
	applyDeviceProps(d, map[string]dbus.Variant{
		"Connected": dbus.MakeVariant("not-a-bool"),
	})
	if d.Connected {
		t.Fatalf("Connected should remain false when variant type mismatches")
	}
}

func TestDisplayName(t *testing.T) {
	cases := []struct {
		name string
		d    BluetoothDevice
		want string
	}{
		{"alias wins and trims", BluetoothDevice{Alias: "  RK N80  ", Name: "Other", Address: "A"}, "RK N80"},
		{"falls back to name", BluetoothDevice{Alias: "  ", Name: " Named Thing ", Address: "A"}, "Named Thing"},
		{"falls back to address", BluetoothDevice{Alias: "", Name: "", Address: "AA:BB"}, "AA:BB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.DisplayName(); got != c.want {
				t.Errorf("DisplayName() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCloneIndependence(t *testing.T) {
	b := 50
	orig := &BluetoothDevice{
		Name:      "X",
		Battery:   &b,
		Batteries: []BatteryLevel{{Label: "", Percent: 50}},
	}
	clone := orig.Clone()
	*clone.Battery = 99
	clone.Batteries[0].Percent = 1
	clone.Name = "Y"

	if *orig.Battery != 50 {
		t.Errorf("mutating clone.Battery affected original: %d", *orig.Battery)
	}
	if orig.Batteries[0].Percent != 50 {
		t.Errorf("mutating clone.Batteries affected original: %+v", orig.Batteries)
	}
	if orig.Name != "X" {
		t.Errorf("mutating clone.Name affected original: %q", orig.Name)
	}
}

func TestEqual(t *testing.T) {
	b1, b2 := 50, 50
	a := &BluetoothDevice{Name: "X", Battery: &b1, Batteries: []BatteryLevel{{Percent: 50}}}
	b := &BluetoothDevice{Name: "X", Battery: &b2, Batteries: []BatteryLevel{{Percent: 50}}}
	if !a.Equal(b) {
		t.Fatalf("expected equal")
	}
	b.Name = "Z"
	if a.Equal(b) {
		t.Fatalf("expected not equal after Name change")
	}
}
