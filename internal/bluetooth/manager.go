package bluetooth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/shariorfarhan/bluetooth-widget/internal/logging"
)

const (
	connectTimeout    = 40 * time.Second
	disconnectTimeout = 20 * time.Second
	powerTimeout      = 10 * time.Second
	loadTimeout       = 10 * time.Second
)

// BluetoothManager is the UI- and CLI-facing API. It never exposes
// D-Bus object paths or godbus types beyond the opaque DeviceID.
type BluetoothManager interface {
	Start(ctx context.Context) error
	Snapshot() State
	ListPairedDevices() ([]*BluetoothDevice, error)
	ConnectDevice(ctx context.Context, id DeviceID) error
	DisconnectDevice(ctx context.Context, id DeviceID) error
	SetAdapterPowered(ctx context.Context, powered bool) error
	SubscribeToChanges(fn func(Event)) (unsubscribe func())
	Close() error
}

// NewSystemManager returns a BluetoothManager backed by the real system
// D-Bus and BlueZ. The connection is opened lazily inside Start.
func NewSystemManager(logger *slog.Logger) BluetoothManager {
	return newManager(func() (bus, error) { return newSystemBus() }, logger)
}

type systemManager struct {
	newBus func() (bus, error)
	bus    bus
	log    *slog.Logger

	mu sync.RWMutex
	st *store

	subMu   sync.Mutex
	subs    map[int]func(Event)
	nextSub int

	opsMu     sync.Mutex
	inflight  map[DeviceID]string
	powerBusy bool

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	closeOnce sync.Once
}

// newManager builds a manager with an injectable bus constructor, used
// directly by tests to supply a fake bus.
func newManager(newBus func() (bus, error), logger *slog.Logger) *systemManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &systemManager{
		newBus:   newBus,
		log:      logger,
		st:       newStore(),
		subs:     map[int]func(Event){},
		inflight: map[DeviceID]string{},
		done:     make(chan struct{}),
	}
}

func (m *systemManager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	b, err := m.newBus()
	if err != nil {
		m.log.Error("bluetooth bus connect failed", logging.KeyError, err.Error())
		m.mu.Lock()
		m.st.setServiceDown()
		snap := m.st.snapshot()
		m.mu.Unlock()
		m.emit(Event{Kind: EventReset, State: &snap, Adapter: snap.Adapter})
		close(m.done)
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	m.bus = b

	loadCtx, cancel := context.WithTimeout(m.ctx, loadTimeout)
	objs, err := b.GetManagedObjects(loadCtx)
	cancel()

	m.mu.Lock()
	if err != nil {
		m.log.Warn("initial GetManagedObjects failed", logging.KeyError, err.Error())
		m.st.setServiceDown()
	} else {
		m.st.reset(objs)
	}
	snap := m.st.snapshot()
	m.mu.Unlock()
	m.emit(Event{Kind: EventReset, State: &snap, Adapter: snap.Adapter})

	go m.loop()
	return nil
}

func (m *systemManager) loop() {
	defer close(m.done)
	for {
		select {
		case <-m.ctx.Done():
			return
		case sig, ok := <-m.bus.Signals():
			if !ok {
				return
			}
			m.handleSignal(sig)
		}
	}
}

func (m *systemManager) handleSignal(sig *dbus.Signal) {
	switch sig.Name {
	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		if len(sig.Body) < 3 {
			return
		}
		iface, _ := sig.Body[0].(string)
		changed, _ := sig.Body[1].(map[string]dbus.Variant)
		inval, _ := sig.Body[2].([]string)
		if iface != ifaceDevice && iface != ifaceBattery && iface != ifaceAdapter {
			return
		}
		m.mu.Lock()
		events := m.st.propertiesChanged(sig.Path, iface, changed, inval)
		m.mu.Unlock()
		for _, e := range events {
			m.emit(e)
		}

	case "org.freedesktop.DBus.ObjectManager.InterfacesAdded":
		if len(sig.Body) < 2 {
			return
		}
		path, ok := sig.Body[0].(dbus.ObjectPath)
		if !ok {
			return
		}
		ifaces, ok := sig.Body[1].(map[string]map[string]dbus.Variant)
		if !ok {
			return
		}
		m.mu.Lock()
		events := m.st.interfacesAdded(path, ifaces)
		m.mu.Unlock()
		for _, e := range events {
			m.emit(e)
		}

	case "org.freedesktop.DBus.ObjectManager.InterfacesRemoved":
		if len(sig.Body) < 2 {
			return
		}
		path, ok := sig.Body[0].(dbus.ObjectPath)
		if !ok {
			return
		}
		ifaces, ok := sig.Body[1].([]string)
		if !ok {
			return
		}
		m.mu.Lock()
		events := m.st.interfacesRemoved(path, ifaces)
		m.mu.Unlock()
		for _, e := range events {
			m.emit(e)
		}

	case "org.freedesktop.DBus.NameOwnerChanged":
		if len(sig.Body) < 3 {
			return
		}
		name, _ := sig.Body[0].(string)
		if name != bluezService {
			return
		}
		newOwner, _ := sig.Body[2].(string)
		if newOwner == "" {
			m.mu.Lock()
			m.st.setServiceDown()
			snap := m.st.snapshot()
			m.mu.Unlock()
			m.emit(Event{Kind: EventReset, State: &snap, Adapter: snap.Adapter})
			return
		}
		ctx, cancel := context.WithTimeout(m.ctx, loadTimeout)
		objs, err := m.bus.GetManagedObjects(ctx)
		cancel()
		m.mu.Lock()
		if err != nil {
			m.log.Warn("reload after NameOwnerChanged failed", logging.KeyError, err.Error())
			m.st.setServiceDown()
		} else {
			m.st.reset(objs)
		}
		snap := m.st.snapshot()
		m.mu.Unlock()
		m.emit(Event{Kind: EventReset, State: &snap, Adapter: snap.Adapter})

	default:
		// NameAcquired and everything else is ignored.
	}
}

// emit copies the subscriber list under subMu and calls each one
// synchronously and outside of mu. Subscribers must not block.
func (m *systemManager) emit(e Event) {
	m.subMu.Lock()
	fns := make([]func(Event), 0, len(m.subs))
	for _, fn := range m.subs {
		fns = append(fns, fn)
	}
	m.subMu.Unlock()
	for _, fn := range fns {
		fn(e)
	}
}

func (m *systemManager) Snapshot() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.snapshot()
}

func (m *systemManager) ListPairedDevices() ([]*BluetoothDevice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.st.serviceUp {
		return nil, ErrServiceUnavailable
	}
	snap := m.st.snapshot()
	return snap.Devices, nil
}

func (m *systemManager) SubscribeToChanges(fn func(Event)) func() {
	m.subMu.Lock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = fn
	m.subMu.Unlock()
	return func() {
		m.subMu.Lock()
		delete(m.subs, id)
		m.subMu.Unlock()
	}
}

func (m *systemManager) beginOp(id DeviceID, op string) bool {
	m.opsMu.Lock()
	defer m.opsMu.Unlock()
	if _, busy := m.inflight[id]; busy {
		return false
	}
	m.inflight[id] = op
	return true
}

func (m *systemManager) endOp(id DeviceID) {
	m.opsMu.Lock()
	delete(m.inflight, id)
	m.opsMu.Unlock()
}

func (m *systemManager) ConnectDevice(ctx context.Context, id DeviceID) error {
	return m.doOp(ctx, id, "connect", connectTimeout, "org.bluez.Device1.Connect")
}

func (m *systemManager) DisconnectDevice(ctx context.Context, id DeviceID) error {
	return m.doOp(ctx, id, "disconnect", disconnectTimeout, "org.bluez.Device1.Disconnect")
}

func (m *systemManager) doOp(ctx context.Context, id DeviceID, op string, timeout time.Duration, method string) error {
	m.mu.RLock()
	dev, ok := m.st.device(id)
	m.mu.RUnlock()
	if !ok {
		return ConvertError(op, "", fmt.Errorf("%w: %s", ErrUnknownDevice, id))
	}
	name := dev.DisplayName()

	if !m.beginOp(id, op) {
		return &OpError{Op: op, Device: name, Kind: KindBusy, Err: ErrBusy}
	}
	defer m.endOp(id)

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m.log.Info(op, logging.KeyOp, op, logging.KeyDevice, name, logging.KeyAddress, dev.Address, logging.KeyPath, string(dev.Path))

	err := m.bus.Call(opCtx, dev.Path, method)
	cerr := ConvertError(op, name, err)
	if cerr != nil {
		var oe *OpError
		if asOpError(cerr, &oe) {
			m.log.Error(op, logging.KeyOp, op, logging.KeyDevice, name, logging.KeyError, oe.Error(),
				logging.KeyDBusError, oe.DBusName, logging.KeyDBusMessage, oe.DBusMessage)
		} else {
			m.log.Error(op, logging.KeyOp, op, logging.KeyDevice, name, logging.KeyError, cerr.Error())
		}
	} else {
		m.log.Info(op, logging.KeyOp, op, logging.KeyDevice, name, "result", "ok")
	}
	return cerr
}

func asOpError(err error, target **OpError) bool {
	if oe, ok := err.(*OpError); ok {
		*target = oe
		return true
	}
	return false
}

func (m *systemManager) SetAdapterPowered(ctx context.Context, powered bool) error {
	m.mu.RLock()
	current := m.st.current
	m.mu.RUnlock()
	if current == "" {
		return ConvertError("set-powered", "", ErrNoAdapter)
	}

	m.opsMu.Lock()
	if m.powerBusy {
		m.opsMu.Unlock()
		return &OpError{Op: "set-powered", Kind: KindBusy, Err: ErrBusy}
	}
	m.powerBusy = true
	m.opsMu.Unlock()
	defer func() {
		m.opsMu.Lock()
		m.powerBusy = false
		m.opsMu.Unlock()
	}()

	opCtx, cancel := context.WithTimeout(ctx, powerTimeout)
	defer cancel()

	err := m.bus.SetProperty(opCtx, current, ifaceAdapter, "Powered", powered)
	return ConvertError("set-powered", "", err)
}

func (m *systemManager) Close() error {
	var err error
	m.closeOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		if m.bus != nil {
			err = m.bus.Close()
		}
		select {
		case <-m.done:
		case <-time.After(2 * time.Second):
		}
	})
	return err
}
