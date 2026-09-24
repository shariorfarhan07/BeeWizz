//go:build integration

package integration

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
)

// TestRealBlueZ_ReadOnly exercises NewSystemManager against the real
// system D-Bus and BlueZ, read-only. It never calls Connect,
// Disconnect or SetAdapterPowered, and it never modifies the user's
// Bluetooth state.
func TestRealBlueZ_ReadOnly(t *testing.T) {
	m := bluetooth.NewSystemManager(slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		if errors.Is(err, bluetooth.ErrServiceUnavailable) {
			t.Skip("bluetoothd is not available on this machine")
		}
		t.Fatalf("Start: %v", err)
	}
	defer m.Close()

	snap := m.Snapshot()
	if snap.Adapter != nil {
		t.Logf("adapter: %+v", snap.Adapter)
	} else {
		t.Log("no adapter present")
	}
	for _, d := range snap.Devices {
		t.Logf("device: %s (%s) paired=%v connected=%v", d.DisplayName(), d.Address, d.Paired, d.Connected)
		if !d.Paired {
			t.Errorf("device %s should be paired (only paired devices are visible)", d.Address)
		}
		if d.Address == "" {
			t.Errorf("device at %s has empty address", d.Path)
		}
	}

	listed, err := m.ListPairedDevices()
	if err != nil {
		t.Fatalf("ListPairedDevices: %v", err)
	}
	if len(listed) != len(snap.Devices) {
		t.Errorf("ListPairedDevices returned %d devices, snapshot has %d", len(listed), len(snap.Devices))
	}

	unsub := m.SubscribeToChanges(func(bluetooth.Event) {})
	time.Sleep(500 * time.Millisecond)
	unsub()

	done := make(chan struct{})
	go func() {
		if err := m.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s")
	}
}
