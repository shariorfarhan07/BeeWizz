package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"
)

type managedObjects = map[dbus.ObjectPath]map[string]map[string]dbus.Variant

// bus is the narrow, mockable D-Bus surface the store and manager need.
// Production code uses systemBus; tests use a fake.
type bus interface {
	GetManagedObjects(ctx context.Context) (managedObjects, error)
	Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error
	SetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string, value any) error
	Signals() <-chan *dbus.Signal
	Close() error
}

type systemBus struct {
	conn *dbus.Conn
	ch   chan *dbus.Signal
}

// newSystemBus opens a private connection to the system bus, subscribes
// to the PropertiesChanged/InterfacesAdded/InterfacesRemoved/
// NameOwnerChanged signals relevant to BlueZ, and starts delivering them
// on the returned bus's Signals channel.
func newSystemBus() (*systemBus, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	matchRules := [][]dbus.MatchOption{
		{
			dbus.WithMatchSender(bluezService),
			dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchPathNamespace("/org/bluez"),
			dbus.WithMatchArg(0, ifaceDevice),
		},
		{
			dbus.WithMatchSender(bluezService),
			dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchPathNamespace("/org/bluez"),
			dbus.WithMatchArg(0, ifaceBattery),
		},
		{
			dbus.WithMatchSender(bluezService),
			dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchPathNamespace("/org/bluez"),
			dbus.WithMatchArg(0, ifaceAdapter),
		},
		{
			dbus.WithMatchSender(bluezService),
			dbus.WithMatchInterface("org.freedesktop.DBus.ObjectManager"),
			dbus.WithMatchMember("InterfacesAdded"),
		},
		{
			dbus.WithMatchSender(bluezService),
			dbus.WithMatchInterface("org.freedesktop.DBus.ObjectManager"),
			dbus.WithMatchMember("InterfacesRemoved"),
		},
		{
			dbus.WithMatchSender("org.freedesktop.DBus"),
			dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, bluezService),
		},
	}
	for _, rule := range matchRules {
		if err := conn.AddMatchSignal(rule...); err != nil {
			conn.Close()
			return nil, err
		}
	}

	ch := make(chan *dbus.Signal, 64)
	conn.Signal(ch)

	return &systemBus{conn: conn, ch: ch}, nil
}

func (b *systemBus) GetManagedObjects(ctx context.Context) (managedObjects, error) {
	var out managedObjects
	err := b.conn.Object(bluezService, dbus.ObjectPath("/")).
		CallWithContext(ctx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).
		Store(&out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (b *systemBus) Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error {
	return b.conn.Object(bluezService, path).CallWithContext(ctx, method, 0, args...).Err
}

func (b *systemBus) SetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string, value any) error {
	return b.conn.Object(bluezService, path).
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, iface, prop, dbus.MakeVariant(value)).Err
}

func (b *systemBus) Signals() <-chan *dbus.Signal { return b.ch }

func (b *systemBus) Close() error {
	b.conn.RemoveSignal(b.ch)
	return b.conn.Close()
}
