package bluetooth

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func v(x interface{}) dbus.Variant { return dbus.MakeVariant(x) }

func adapterObj(powered bool) map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{
		ifaceAdapter: {
			"Address": v("AA:AA:AA:AA:AA:AA"),
			"Name":    v("hci0"),
			"Alias":   v("hci0"),
			"Powered": v(powered),
		},
	}
}

func deviceObj(adapter dbus.ObjectPath, paired, connected bool) map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{
		ifaceDevice: {
			"Address":   v("11:22:33:44:55:66"),
			"Name":      v("Test Device"),
			"Alias":     v("Test Device"),
			"Paired":    v(paired),
			"Connected": v(connected),
			"Adapter":   v(adapter),
		},
	}
}

func baseFixture() managedObjects {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_11_22_33_44_55_66")
	return managedObjects{
		hci0: adapterObj(true),
		dev:  deviceObj(hci0, true, false),
		dev + "/service0001": {
			"org.bluez.GattService1": {"UUID": v("1801")},
		},
	}
}

func TestStore_ResetKeepsOnlyPairedOnCurrentAdapter(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	paired := dbus.ObjectPath("/org/bluez/hci0/dev_paired")
	unpaired := dbus.ObjectPath("/org/bluez/hci0/dev_unpaired")
	objs := managedObjects{
		hci0:     adapterObj(true),
		paired:   deviceObj(hci0, true, false),
		unpaired: deviceObj(hci0, false, false),
	}
	s := newStore()
	s.reset(objs)
	snap := s.snapshot()
	if len(snap.Devices) != 1 || snap.Devices[0].Path != paired {
		t.Fatalf("expected only the paired device visible, got %+v", snap.Devices)
	}
}

func TestStore_UnpairedToPairedEmitsAdded(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, false, false)})

	events := s.propertiesChanged(dev, ifaceDevice, map[string]dbus.Variant{"Paired": v(true)}, nil)
	if len(events) != 1 || events[0].Kind != EventDeviceAdded {
		t.Fatalf("expected DeviceAdded, got %+v", events)
	}
}

func TestStore_PairedToUnpairedEmitsRemoved(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, false)})

	events := s.propertiesChanged(dev, ifaceDevice, map[string]dbus.Variant{"Paired": v(false)}, nil)
	if len(events) != 1 || events[0].Kind != EventDeviceRemoved {
		t.Fatalf("expected DeviceRemoved, got %+v", events)
	}
}

func TestStore_ConnectedFlipEmitsOneChanged(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, false)})

	events := s.propertiesChanged(dev, ifaceDevice, map[string]dbus.Variant{"Connected": v(true)}, nil)
	if len(events) != 1 || events[0].Kind != EventDeviceChanged {
		t.Fatalf("expected one DeviceChanged, got %+v", events)
	}
	if !events[0].Device.Connected {
		t.Fatalf("expected Connected=true in event")
	}
}

func TestStore_RSSIOnlyChangeEmitsNothing(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, false)})

	// RSSI is not a field applyDeviceProps understands, so this must be a no-op.
	events := s.propertiesChanged(dev, ifaceDevice, map[string]dbus.Variant{"RSSI": v(int16(-40))}, nil)
	if len(events) != 0 {
		t.Fatalf("expected no events for RSSI-only change, got %+v", events)
	}
}

func TestStore_Battery1AddedSetsBatteryAndEmitsChanged(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, true)})

	events := s.interfacesAdded(dev, map[string]map[string]dbus.Variant{
		ifaceBattery: {"Percentage": v(byte(82))},
	})
	if len(events) != 1 || events[0].Kind != EventDeviceChanged {
		t.Fatalf("expected DeviceChanged, got %+v", events)
	}
	if events[0].Device.Battery == nil || *events[0].Device.Battery != 82 {
		t.Fatalf("expected battery 82, got %+v", events[0].Device.Battery)
	}
}

func TestStore_Battery1RemovedClearsBattery(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	objs := managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, true)}
	objs[dev][ifaceBattery] = map[string]dbus.Variant{"Percentage": v(byte(82))}
	s.reset(objs)

	events := s.interfacesRemoved(dev, []string{ifaceBattery})
	if len(events) != 1 || events[0].Kind != EventDeviceChanged {
		t.Fatalf("expected DeviceChanged, got %+v", events)
	}
	if events[0].Device.Battery != nil {
		t.Fatalf("expected battery cleared, got %v", *events[0].Device.Battery)
	}
}

func TestStore_Device1RemovedEmitsRemoved(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, false)})

	events := s.interfacesRemoved(dev, []string{ifaceDevice})
	if len(events) != 1 || events[0].Kind != EventDeviceRemoved {
		t.Fatalf("expected DeviceRemoved, got %+v", events)
	}
	if _, ok := s.device(DeviceID(dev)); ok {
		t.Fatalf("device should be gone from store")
	}
}

func TestStore_SecondAdapterDoesNotSwitch(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	hci1 := dbus.ObjectPath("/org/bluez/hci1")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true)})
	if s.current != hci0 {
		t.Fatalf("expected current=hci0, got %s", s.current)
	}

	events := s.interfacesAdded(hci1, adapterObj(true))
	if s.current != hci0 {
		t.Fatalf("current adapter should not switch to hci1, got %s", s.current)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events when a non-current adapter appears, got %+v", events)
	}
}

func TestStore_RemovingCurrentAdapterSwitchesWithReset(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	hci1 := dbus.ObjectPath("/org/bluez/hci1")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), hci1: adapterObj(true)})
	if s.current != hci0 {
		t.Fatalf("expected current=hci0 initially, got %s", s.current)
	}

	events := s.interfacesRemoved(hci0, []string{ifaceAdapter})
	if s.current != hci1 {
		t.Fatalf("expected current to switch to hci1, got %s", s.current)
	}
	if len(events) != 1 || events[0].Kind != EventReset {
		t.Fatalf("expected a Reset event, got %+v", events)
	}
}

func TestStore_DeviceOnOtherAdapterIgnoredWhileHci0Current(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	hci1 := dbus.ObjectPath("/org/bluez/hci1")
	devOnHci1 := dbus.ObjectPath("/org/bluez/hci1/dev_x")
	s := newStore()
	s.reset(managedObjects{
		hci0:      adapterObj(true),
		hci1:      adapterObj(true),
		devOnHci1: deviceObj(hci1, true, false),
	})
	snap := s.snapshot()
	if len(snap.Devices) != 0 {
		t.Fatalf("device on non-current adapter should not be visible: %+v", snap.Devices)
	}
}

func TestStore_AdapterPoweredChangeEmitsAdapterChanged(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(false)})

	events := s.propertiesChanged(hci0, ifaceAdapter, map[string]dbus.Variant{"Powered": v(true)}, nil)
	if len(events) != 1 || events[0].Kind != EventAdapterChanged {
		t.Fatalf("expected AdapterChanged, got %+v", events)
	}
	if !events[0].Adapter.Powered {
		t.Fatalf("expected Powered=true in event")
	}
}

func TestStore_InvalidatedPropsHandled(t *testing.T) {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	dev := dbus.ObjectPath("/org/bluez/hci0/dev_x")
	s := newStore()
	s.reset(managedObjects{hci0: adapterObj(true), dev: deviceObj(hci0, true, false)})

	events := s.propertiesChanged(dev, ifaceDevice, nil, []string{"Name", "Alias"})
	if len(events) != 1 || events[0].Kind != EventDeviceChanged {
		t.Fatalf("expected DeviceChanged, got %+v", events)
	}
	if events[0].Device.Name != "" || events[0].Device.Alias != "" {
		t.Fatalf("expected Name/Alias invalidated, got %+v", events[0].Device)
	}
	// DisplayName falls back to Address once Alias/Name are gone.
	if events[0].Device.DisplayName() != events[0].Device.Address {
		t.Fatalf("expected DisplayName to fall back to Address")
	}
}

func TestStore_GATTChildPathIgnoredOnReset(t *testing.T) {
	s := newStore()
	s.reset(baseFixture())
	snap := s.snapshot()
	if len(snap.Devices) != 1 {
		t.Fatalf("GATT child path should not appear as a device: %+v", snap.Devices)
	}
}
