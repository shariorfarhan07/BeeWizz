package bluetooth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestManager(t *testing.T, fb *fakeBus) *systemManager {
	t.Helper()
	m := newManager(func() (bus, error) { return fb, nil }, discardLogger())
	return m
}

func TestManager_StartEmitsResetWithDevices(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)

	var got Event
	ch := make(chan Event, 8)
	m.SubscribeToChanges(func(e Event) { ch <- e })

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	select {
	case got = <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reset event")
	}
	if got.Kind != EventReset {
		t.Fatalf("expected EventReset, got %v", got.Kind)
	}
	if len(got.State.Devices) != 2 {
		t.Fatalf("expected 2 paired devices, got %d: %+v", len(got.State.Devices), got.State.Devices)
	}
}

func TestManager_SignalReachesSubscriber(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	ch := make(chan Event, 8)
	m.SubscribeToChanges(func(e Event) { ch <- e })

	dev := dbus.ObjectPath("/org/bluez/hci0/dev_disconnected")
	fb.sig <- propsChanged(dev, ifaceDevice, map[string]dbus.Variant{"Connected": v(true)})

	select {
	case e := <-ch:
		if e.Kind != EventDeviceChanged {
			t.Fatalf("expected DeviceChanged, got %v", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal-triggered event")
	}
}

func TestManager_ConnectDeviceRecordsCall(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	id := DeviceID("/org/bluez/hci0/dev_disconnected")
	if err := m.ConnectDevice(context.Background(), id); err != nil {
		t.Fatalf("ConnectDevice: %v", err)
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %+v", len(fb.calls), fb.calls)
	}
	if fb.calls[0].Path != dbus.ObjectPath(id) || fb.calls[0].Method != "org.bluez.Device1.Connect" {
		t.Fatalf("unexpected call: %+v", fb.calls[0])
	}
}

func TestManager_ConnectDeviceOutOfRange(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	fb.callErr["org.bluez.Device1.Connect"] = dbus.Error{
		Name: "org.bluez.Error.Failed",
		Body: []interface{}{"br-connection-page-timeout"},
	}
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	id := DeviceID("/org/bluez/hci0/dev_disconnected")
	err := m.ConnectDevice(context.Background(), id)
	var oe *OpError
	if !errors.As(err, &oe) {
		t.Fatalf("expected *OpError, got %v", err)
	}
	if oe.Kind != KindOutOfRange {
		t.Fatalf("expected KindOutOfRange, got %v", oe.Kind)
	}
}

func TestManager_ConcurrentConnectReturnsBusy(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	fb.block = make(chan struct{})
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	id := DeviceID("/org/bluez/hci0/dev_disconnected")
	done := make(chan error, 1)
	go func() { done <- m.ConnectDevice(context.Background(), id) }()

	// Give the first call time to register as in-flight.
	time.Sleep(100 * time.Millisecond)

	err := m.ConnectDevice(context.Background(), id)
	var oe *OpError
	if !errors.As(err, &oe) || oe.Kind != KindBusy {
		t.Fatalf("expected KindBusy for concurrent connect, got %v", err)
	}

	close(fb.block)
	if err := <-done; err != nil {
		t.Fatalf("first connect should succeed once unblocked: %v", err)
	}
}

func TestManager_SetAdapterPoweredRecordsSetProperty(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	if err := m.SetAdapterPowered(context.Background(), true); err != nil {
		t.Fatalf("SetAdapterPowered: %v", err)
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.sets) != 1 {
		t.Fatalf("expected 1 SetProperty call, got %d", len(fb.sets))
	}
	s := fb.sets[0]
	if s.Path != "/org/bluez/hci0" || s.Iface != "org.bluez.Adapter1" || s.Prop != "Powered" || s.Value != true {
		t.Fatalf("unexpected SetProperty call: %+v", s)
	}
}

func TestManager_NameOwnerChanged_DownThenUp(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	ch := make(chan Event, 8)
	m.SubscribeToChanges(func(e Event) { ch <- e })

	fb.sig <- nameOwner(":1.5", "")
	select {
	case e := <-ch:
		if e.Kind != EventReset || e.State.ServiceAvailable {
			t.Fatalf("expected Reset with ServiceAvailable=false, got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-down reset")
	}

	fb.sig <- nameOwner("", ":1.9")
	select {
	case e := <-ch:
		if e.Kind != EventReset || !e.State.ServiceAvailable || len(e.State.Devices) != 2 {
			t.Fatalf("expected Reset with devices restored, got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-up reset")
	}
}

func TestManager_NewBusErrorStartsDown(t *testing.T) {
	m := newManager(func() (bus, error) { return nil, errors.New("boom") }, discardLogger())
	ch := make(chan Event, 8)
	m.SubscribeToChanges(func(e Event) { ch <- e })

	err := m.Start(context.Background())
	if err == nil {
		t.Fatalf("expected Start to return an error")
	}
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("expected ErrServiceUnavailable, got %v", err)
	}
	select {
	case e := <-ch:
		if e.Kind != EventReset || e.State.ServiceAvailable {
			t.Fatalf("expected Reset with service down, got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reset")
	}
}

func TestManager_UnknownIDGivesNoDevice(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	err := m.ConnectDevice(context.Background(), DeviceID("/does/not/exist"))
	var oe *OpError
	if !errors.As(err, &oe) || oe.Kind != KindNoDevice {
		t.Fatalf("expected KindNoDevice, got %v", err)
	}
}

func TestManager_CloseIsIdempotentAndLoopExits(t *testing.T) {
	fb := newFakeBus()
	fb.objs = fixture()
	m := newTestManager(t, fb)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close should be idempotent, got: %v", err)
	}
}
