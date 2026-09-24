package bluetooth

import (
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"
)

// batteryRec is what the store keeps internally per Battery1 object
// path: which device it belongs to, and its last known percentage.
type batteryRec struct {
	device  dbus.ObjectPath
	percent int
}

// store is a pure state machine: it has no locking and does no I/O. All
// mutation methods return the Events that resulted, so the caller
// (systemManager) can emit them after releasing its own lock.
type store struct {
	serviceUp bool
	adapters  map[dbus.ObjectPath]*Adapter
	devices   map[dbus.ObjectPath]*BluetoothDevice
	batteries map[dbus.ObjectPath]batteryRec
	current   dbus.ObjectPath
}

func newStore() *store {
	return &store{
		adapters:  map[dbus.ObjectPath]*Adapter{},
		devices:   map[dbus.ObjectPath]*BluetoothDevice{},
		batteries: map[dbus.ObjectPath]batteryRec{},
	}
}

// reset rebuilds the entire store from a GetManagedObjects result.
func (s *store) reset(objs managedObjects) {
	s.serviceUp = true
	s.adapters = map[dbus.ObjectPath]*Adapter{}
	s.devices = map[dbus.ObjectPath]*BluetoothDevice{}
	s.batteries = map[dbus.ObjectPath]batteryRec{}

	for path, ifaces := range objs {
		if props, ok := ifaces[ifaceAdapter]; ok {
			a := &Adapter{Path: path}
			applyAdapterProps(a, props)
			s.adapters[path] = a
		}
	}
	for path, ifaces := range objs {
		if props, ok := ifaces[ifaceDevice]; ok {
			d := &BluetoothDevice{ID: DeviceID(path), Path: path}
			applyDeviceProps(d, props)
			s.devices[path] = d
		}
	}
	for path, ifaces := range objs {
		if props, ok := ifaces[ifaceBattery]; ok {
			if pct, ok := parseBatteryPercent(props); ok {
				if dp := ownerDevicePath(path, s.devices); dp != "" {
					s.batteries[path] = batteryRec{device: dp, percent: pct}
				}
			}
		}
	}
	for dp := range s.devices {
		s.recomputeBatteries(dp)
	}
	s.current = pickAdapter(s.adapters, s.current)
}

// setServiceDown clears all cached state and marks the service
// unreachable.
func (s *store) setServiceDown() {
	s.serviceUp = false
	s.adapters = map[dbus.ObjectPath]*Adapter{}
	s.devices = map[dbus.ObjectPath]*BluetoothDevice{}
	s.batteries = map[dbus.ObjectPath]batteryRec{}
	s.current = ""
}

// visible reports whether d should be shown: paired, on the current
// adapter, and there is a current adapter at all.
func (s *store) visible(d *BluetoothDevice) bool {
	return d != nil && d.Paired && s.current != "" && d.Adapter == s.current
}

// snapshot returns a full clone of the visible state. Devices are
// sorted by path; the UI re-sorts with its own Sorter.
func (s *store) snapshot() State {
	var devs []*BluetoothDevice
	for _, d := range s.devices {
		if s.visible(d) {
			devs = append(devs, d.Clone())
		}
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].Path < devs[j].Path })

	var ad *Adapter
	if s.current != "" {
		if a, ok := s.adapters[s.current]; ok {
			ad = a.Clone()
		}
	}
	return State{ServiceAvailable: s.serviceUp, Adapter: ad, Devices: devs}
}

// device looks up a visible device by ID and returns a clone.
func (s *store) device(id DeviceID) (*BluetoothDevice, bool) {
	d, ok := s.devices[dbus.ObjectPath(id)]
	if !ok || !s.visible(d) {
		return nil, false
	}
	return d.Clone(), true
}

// deviceEvents is the single place that turns a before/after pair for
// one device path into zero or one Event.
func (s *store) deviceEvents(path dbus.ObjectPath, before, after *BluetoothDevice) []Event {
	wasVisible := before != nil && s.visible(before)
	isVisible := after != nil && s.visible(after)
	id := DeviceID(path)

	switch {
	case wasVisible && !isVisible:
		return []Event{{Kind: EventDeviceRemoved, DeviceID: id}}
	case !wasVisible && isVisible:
		return []Event{{Kind: EventDeviceAdded, DeviceID: id, Device: after.Clone()}}
	case wasVisible && isVisible:
		if !before.Equal(after) {
			return []Event{{Kind: EventDeviceChanged, DeviceID: id, Device: after.Clone()}}
		}
	}
	return nil
}

// recomputeBatteries rebuilds BluetoothDevice.Batteries/Battery for one
// device from the current s.batteries map.
func (s *store) recomputeBatteries(dp dbus.ObjectPath) {
	d, ok := s.devices[dp]
	if !ok {
		return
	}
	var levels []BatteryLevel
	for bp, rec := range s.batteries {
		if rec.device != dp {
			continue
		}
		levels = append(levels, BatteryLevel{Label: batteryLabel(dp, bp), Percent: rec.percent, Path: bp})
	}
	sort.Slice(levels, func(i, j int) bool {
		if (levels[i].Label == "") != (levels[j].Label == "") {
			return levels[i].Label == ""
		}
		return levels[i].Label < levels[j].Label
	})
	d.Batteries = levels
	d.Battery = primaryBattery(levels)
}

// ownerDevicePath finds which known device a Battery1 object path
// belongs to: either the device path itself, or a path nested under
// "<devicePath>/".
func ownerDevicePath(batteryPath dbus.ObjectPath, devices map[dbus.ObjectPath]*BluetoothDevice) dbus.ObjectPath {
	if _, ok := devices[batteryPath]; ok {
		return batteryPath
	}
	bp := string(batteryPath)
	for dp := range devices {
		if strings.HasPrefix(bp, string(dp)+"/") {
			return dp
		}
	}
	return ""
}

// interfacesAdded handles org.freedesktop.DBus.ObjectManager.InterfacesAdded.
func (s *store) interfacesAdded(path dbus.ObjectPath, ifaces map[string]map[string]dbus.Variant) []Event {
	if props, ok := ifaces[ifaceAdapter]; ok {
		_, existed := s.adapters[path]
		a := s.adapters[path]
		if a == nil {
			a = &Adapter{Path: path}
			s.adapters[path] = a
		}
		applyAdapterProps(a, props)
		if !existed {
			if nc := pickAdapter(s.adapters, s.current); nc != s.current {
				s.current = nc
				snap := s.snapshot()
				return []Event{{Kind: EventReset, State: &snap, Adapter: snap.Adapter}}
			}
		}
	}

	devProps, hasDevice := ifaces[ifaceDevice]
	battProps, hasBattery := ifaces[ifaceBattery]
	if !hasDevice && !hasBattery {
		return nil
	}

	dp := path
	if !hasDevice {
		owner := ownerDevicePath(path, s.devices)
		if owner == "" {
			return nil // orphan battery signal; no known device yet
		}
		dp = owner
	}

	before := s.devices[dp].Clone()
	d := s.devices[dp]
	if d == nil {
		d = &BluetoothDevice{ID: DeviceID(dp), Path: dp}
		s.devices[dp] = d
	}
	if hasDevice {
		applyDeviceProps(d, devProps)
	}
	if hasBattery {
		if pct, ok := parseBatteryPercent(battProps); ok {
			s.batteries[path] = batteryRec{device: dp, percent: pct}
		}
	}
	s.recomputeBatteries(dp)
	after := d.Clone()
	return s.deviceEvents(dp, before, after)
}

// interfacesRemoved handles org.freedesktop.DBus.ObjectManager.InterfacesRemoved.
func (s *store) interfacesRemoved(path dbus.ObjectPath, ifaces []string) []Event {
	var hasAdapter, hasDevice, hasBattery bool
	for _, i := range ifaces {
		switch i {
		case ifaceAdapter:
			hasAdapter = true
		case ifaceDevice:
			hasDevice = true
		case ifaceBattery:
			hasBattery = true
		}
	}

	if hasAdapter {
		if _, ok := s.adapters[path]; ok {
			delete(s.adapters, path)
			if nc := pickAdapter(s.adapters, s.current); nc != s.current {
				s.current = nc
				snap := s.snapshot()
				return []Event{{Kind: EventReset, State: &snap, Adapter: snap.Adapter}}
			}
		}
	}

	if hasDevice {
		if before, ok := s.devices[path]; ok {
			bClone := before.Clone()
			delete(s.devices, path)
			for bp, rec := range s.batteries {
				if rec.device == path {
					delete(s.batteries, bp)
				}
			}
			return s.deviceEvents(path, bClone, nil)
		}
		return nil
	}

	if hasBattery {
		if rec, ok := s.batteries[path]; ok {
			dp := rec.device
			delete(s.batteries, path)
			if d, ok2 := s.devices[dp]; ok2 {
				before := d.Clone()
				s.recomputeBatteries(dp)
				after := d.Clone()
				return s.deviceEvents(dp, before, after)
			}
		}
	}

	return nil
}

// propertiesChanged handles org.freedesktop.DBus.Properties.PropertiesChanged
// for one of Device1, Battery1 or Adapter1.
func (s *store) propertiesChanged(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant, invalidated []string) []Event {
	switch iface {
	case ifaceAdapter:
		a, ok := s.adapters[path]
		if !ok {
			return nil
		}
		before := a.Clone()
		applyAdapterProps(a, changed)
		if path != s.current {
			return nil
		}
		after := a.Clone()
		if !adapterEqual(before, after) {
			return []Event{{Kind: EventAdapterChanged, Adapter: after}}
		}
		return nil

	case ifaceDevice:
		d, ok := s.devices[path]
		if !ok {
			return nil
		}
		before := d.Clone()
		applyDeviceProps(d, changed)
		invalidateDeviceProps(d, invalidated)
		after := d.Clone()
		return s.deviceEvents(path, before, after)

	case ifaceBattery:
		rec, ok := s.batteries[path]
		if !ok {
			if pct, ok2 := parseBatteryPercent(changed); ok2 {
				dp := ownerDevicePath(path, s.devices)
				if dp == "" {
					return nil
				}
				s.batteries[path] = batteryRec{device: dp, percent: pct}
				if d, ok3 := s.devices[dp]; ok3 {
					before := d.Clone()
					s.recomputeBatteries(dp)
					after := d.Clone()
					return s.deviceEvents(dp, before, after)
				}
			}
			return nil
		}
		dp := rec.device
		d, ok2 := s.devices[dp]
		if !ok2 {
			return nil
		}
		before := d.Clone()
		if pct, ok3 := parseBatteryPercent(changed); ok3 {
			rec.percent = pct
			s.batteries[path] = rec
		}
		for _, n := range invalidated {
			if n == "Percentage" {
				delete(s.batteries, path)
			}
		}
		s.recomputeBatteries(dp)
		after := d.Clone()
		return s.deviceEvents(dp, before, after)
	}
	return nil
}

func adapterEqual(a, b *Adapter) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Path == b.Path && a.Address == b.Address && a.Name == b.Name &&
		a.Alias == b.Alias && a.Powered == b.Powered &&
		a.Discoverable == b.Discoverable && a.Pairable == b.Pairable
}

// pickAdapter keeps current if it still exists, else picks the
// lexicographically smallest path (hci0 before hci1); "" if none.
func pickAdapter(adapters map[dbus.ObjectPath]*Adapter, current dbus.ObjectPath) dbus.ObjectPath {
	if current != "" {
		if _, ok := adapters[current]; ok {
			return current
		}
	}
	var best dbus.ObjectPath
	first := true
	for p := range adapters {
		if first || p < best {
			best = p
			first = false
		}
	}
	return best
}
