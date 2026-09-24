package bluetooth

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"
)

type fakeCall struct {
	Path   dbus.ObjectPath
	Method string
}

type fakeSet struct {
	Path  dbus.ObjectPath
	Iface string
	Prop  string
	Value any
}

// fakeBus is a minimal in-memory implementation of the `bus` interface
// used by manager_test.go. It never talks to the real system bus.
type fakeBus struct {
	mu      sync.Mutex
	objs    managedObjects
	objsErr error
	calls   []fakeCall
	sets    []fakeSet
	callErr map[string]error
	block   chan struct{}
	sig     chan *dbus.Signal
}

func newFakeBus() *fakeBus {
	return &fakeBus{
		callErr: map[string]error{},
		sig:     make(chan *dbus.Signal, 64),
	}
}

func (b *fakeBus) GetManagedObjects(ctx context.Context) (managedObjects, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.objsErr != nil {
		return nil, b.objsErr
	}
	return b.objs, nil
}

func (b *fakeBus) Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error {
	if b.block != nil {
		select {
		case <-b.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	b.mu.Lock()
	b.calls = append(b.calls, fakeCall{Path: path, Method: method})
	err := b.callErr[method]
	b.mu.Unlock()
	return err
}

func (b *fakeBus) SetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string, value any) error {
	b.mu.Lock()
	b.sets = append(b.sets, fakeSet{Path: path, Iface: iface, Prop: prop, Value: value})
	b.mu.Unlock()
	return nil
}

func (b *fakeBus) Signals() <-chan *dbus.Signal { return b.sig }

func (b *fakeBus) Close() error {
	close(b.sig)
	return nil
}

func propsChanged(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant) *dbus.Signal {
	return &dbus.Signal{
		Name: "org.freedesktop.DBus.Properties.PropertiesChanged",
		Path: path,
		Body: []interface{}{iface, changed, []string{}},
	}
}

func ifAdded(path dbus.ObjectPath, ifaces map[string]map[string]dbus.Variant) *dbus.Signal {
	return &dbus.Signal{
		Name: "org.freedesktop.DBus.ObjectManager.InterfacesAdded",
		Path: "/",
		Body: []interface{}{path, ifaces},
	}
}

func ifRemoved(path dbus.ObjectPath, ifaces []string) *dbus.Signal {
	return &dbus.Signal{
		Name: "org.freedesktop.DBus.ObjectManager.InterfacesRemoved",
		Path: "/",
		Body: []interface{}{path, ifaces},
	}
}

func nameOwner(old, new string) *dbus.Signal {
	return &dbus.Signal{
		Name: "org.freedesktop.DBus.NameOwnerChanged",
		Path: "/org/freedesktop/DBus",
		Body: []interface{}{bluezService, old, new},
	}
}

// fixture returns a managedObjects with one adapter (hci0, powered),
// three devices (paired+connected with a Battery1 reading, paired and
// disconnected with alias "RK N80 " and no Icon, and one unpaired), and
// one GATT child object that must be ignored by the store.
func fixture() managedObjects {
	hci0 := dbus.ObjectPath("/org/bluez/hci0")
	connected := dbus.ObjectPath("/org/bluez/hci0/dev_connected")
	disconnected := dbus.ObjectPath("/org/bluez/hci0/dev_disconnected")
	unpaired := dbus.ObjectPath("/org/bluez/hci0/dev_unpaired")

	return managedObjects{
		hci0: {
			ifaceAdapter: {
				"Address": v("AA:AA:AA:AA:AA:AA"),
				"Name":    v("hci0"),
				"Alias":   v("hci0"),
				"Powered": v(true),
			},
		},
		connected: {
			ifaceDevice: {
				"Address":   v("11:11:11:11:11:11"),
				"Name":      v("Connected Device"),
				"Alias":     v("Connected Device"),
				"Icon":      v("audio-headset"),
				"Paired":    v(true),
				"Connected": v(true),
				"Trusted":   v(true),
				"Adapter":   v(hci0),
			},
			ifaceBattery: {
				"Percentage": v(byte(82)),
			},
		},
		disconnected: {
			ifaceDevice: {
				"Address":   v("22:22:22:22:22:22"),
				"Name":      v("RK N80 "),
				"Alias":     v("RK N80 "),
				"Paired":    v(true),
				"Connected": v(false),
				"Trusted":   v(true),
				"Adapter":   v(hci0),
			},
		},
		unpaired: {
			ifaceDevice: {
				"Address":   v("33:33:33:33:33:33"),
				"Name":      v("Unpaired Device"),
				"Alias":     v("Unpaired Device"),
				"Paired":    v(false),
				"Connected": v(false),
				"Adapter":   v(hci0),
			},
		},
		connected + "/service0001": {
			"org.bluez.GattService1": {"UUID": v("1801")},
		},
	}
}
