// Package cli implements the non-GUI command-line modes of the widget
// (-dump, -watch, -connect, -disconnect, -power, -autostart). It never
// imports GTK.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
	"github.com/shariorfarhan/bluetooth-widget/internal/autostart"
	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
	"github.com/shariorfarhan/bluetooth-widget/internal/config"
)

// Dump starts the manager, prints one JSON snapshot of its state, and
// exits.
func Dump(ctx context.Context, m bluetooth.BluetoothManager, w io.Writer) int {
	if err := m.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
	}
	defer m.Close()

	snap := m.Snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		return 1
	}
	fmt.Fprintln(w, string(data))
	return 0
}

type watchLine struct {
	Time     string                     `json:"time"`
	Kind     string                     `json:"kind"`
	DeviceID string                     `json:"device_id,omitempty"`
	Device   *bluetooth.BluetoothDevice `json:"device,omitempty"`
	Adapter  *bluetooth.Adapter         `json:"adapter,omitempty"`
	State    *bluetooth.State           `json:"state,omitempty"`
}

// Watch subscribes to change events, printing one JSON line per event,
// and blocks until ctx is done (typically via SIGINT/SIGTERM).
func Watch(ctx context.Context, m bluetooth.BluetoothManager, w io.Writer) int {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	unsub := m.SubscribeToChanges(func(ev bluetooth.Event) {
		line := watchLine{
			Time:     time.Now().Format(time.RFC3339Nano),
			Kind:     ev.Kind.String(),
			DeviceID: string(ev.DeviceID),
			Device:   ev.Device,
			Adapter:  ev.Adapter,
			State:    ev.State,
		}
		data, err := json.Marshal(line)
		if err != nil {
			return
		}
		fmt.Fprintln(w, string(data))
	})
	defer unsub()

	if err := m.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
	}
	defer m.Close()

	<-ctx.Done()
	return 0
}

// ConnectByAddress starts the manager, finds the paired device with the
// given address, and connects or disconnects it.
func ConnectByAddress(ctx context.Context, m bluetooth.BluetoothManager, addr string, connect bool, w io.Writer) int {
	if err := m.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		return 1
	}
	defer m.Close()

	target := config.NormalizeMAC(addr)
	snap := m.Snapshot()
	var id bluetooth.DeviceID
	var found *bluetooth.BluetoothDevice
	for _, d := range snap.Devices {
		if config.NormalizeMAC(d.Address) == target {
			id = d.ID
			found = d
			break
		}
	}
	if found == nil {
		fmt.Fprintf(os.Stderr, "no paired device with address %s\n", addr)
		return 1
	}

	var err error
	verb := "Connecting"
	if connect {
		err = m.ConnectDevice(ctx, id)
	} else {
		verb = "Disconnecting"
		err = m.DisconnectDevice(ctx, id)
	}
	if err != nil {
		fmt.Fprintf(w, "%s %s failed: %s\n", verb, found.DisplayName(), friendlyOf(err))
		return 1
	}
	fmt.Fprintf(w, "%s %s: ok\n", verb, found.DisplayName())
	return 0
}

// SetPower sets the current adapter's Powered property.
func SetPower(ctx context.Context, m bluetooth.BluetoothManager, on bool, w io.Writer) int {
	if err := m.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		return 1
	}
	defer m.Close()

	if err := m.SetAdapterPowered(ctx, on); err != nil {
		fmt.Fprintf(w, "set power failed: %s\n", friendlyOf(err))
		return 1
	}
	fmt.Fprintln(w, "ok")
	return 0
}

// SetAutostart enables or disables the login autostart entry and keeps
// the config's StartAtLogin flag in sync.
func SetAutostart(on bool, cfg *config.Store, w io.Writer) int {
	if on {
		exe, err := autostart.ResolveExecutable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "autostart:", err)
			return 1
		}
		if err := autostart.Enable(exe); err != nil {
			fmt.Fprintln(os.Stderr, "autostart:", err)
			return 1
		}
	} else {
		if err := autostart.Disable(); err != nil {
			fmt.Fprintln(os.Stderr, "autostart:", err)
			return 1
		}
	}
	if cfg != nil {
		_ = cfg.Update(func(c *config.Config) { c.StartAtLogin = on })
	}
	fmt.Fprintf(w, "autostart: %v\n", on)
	return 0
}

func friendlyOf(err error) string {
	var oe *bluetooth.OpError
	if e, ok := err.(*bluetooth.OpError); ok {
		oe = e
		if f := oe.Friendly(); f != "" {
			return f
		}
		return oe.Short()
	}
	return err.Error()
}

// AudioDump prints one JSON snapshot of the sound server's Bluetooth
// audio state (cards, profiles, codecs, outputs, inputs) and exits.
func AudioDump(ctx context.Context, c *audio.PulseController, w io.Writer) int {
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "sound server:", err)
		return 1
	}
	snap, err := c.Snapshot(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "snapshot:", err)
		return 1
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		return 1
	}
	fmt.Fprintln(w, string(data))
	return 0
}
