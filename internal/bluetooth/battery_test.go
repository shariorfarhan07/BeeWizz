package bluetooth

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseBatteryPercent(t *testing.T) {
	cases := []struct {
		name    string
		props   map[string]dbus.Variant
		wantOK  bool
		wantPct int
	}{
		{"byte", map[string]dbus.Variant{"Percentage": dbus.MakeVariant(byte(82))}, true, 82},
		{"uint32", map[string]dbus.Variant{"Percentage": dbus.MakeVariant(uint32(70))}, true, 70},
		{"int32", map[string]dbus.Variant{"Percentage": dbus.MakeVariant(int32(65))}, true, 65},
		{"clamp above 100", map[string]dbus.Variant{"Percentage": dbus.MakeVariant(byte(255))}, true, 100},
		{"missing", map[string]dbus.Variant{}, false, 0},
		{"wrong type", map[string]dbus.Variant{"Percentage": dbus.MakeVariant("82")}, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := parseBatteryPercent(c.props)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && p != c.wantPct {
				t.Fatalf("percent = %d, want %d", p, c.wantPct)
			}
		})
	}
}

func TestBatteryLabel(t *testing.T) {
	dp := dbus.ObjectPath("/org/bluez/hci0/dev_AA")
	if l := batteryLabel(dp, dp); l != "" {
		t.Errorf("same path should have empty label, got %q", l)
	}
	if l := batteryLabel(dp, dp+"/left"); l != "left" {
		t.Errorf("expected 'left', got %q", l)
	}
	if l := batteryLabel(dp, "/org/bluez/hci0/dev_BB"); l != "" {
		t.Errorf("unrelated path should return empty, got %q", l)
	}
}

func TestPrimaryBattery(t *testing.T) {
	if primaryBattery(nil) != nil {
		t.Fatalf("empty slice should yield nil")
	}
	p := primaryBattery([]BatteryLevel{{Label: "left", Percent: 84}, {Label: "right", Percent: 80}})
	if p == nil || *p != 80 {
		t.Fatalf("expected min 80 when no primary label, got %v", p)
	}
	p = primaryBattery([]BatteryLevel{{Label: "left", Percent: 84}, {Label: "", Percent: 92}})
	if p == nil || *p != 92 {
		t.Fatalf("expected primary label value 92, got %v", p)
	}
}
