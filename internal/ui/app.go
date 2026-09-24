// Package ui implements the GTK4 front end. It is the only package in
// this module that imports gotk4 / cgo; it talks to the rest of the
// program only through bluetooth.BluetoothManager and config.Store,
// using the opaque bluetooth.DeviceID.
package ui

import (
	"log/slog"
	"os"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
	"github.com/shariorfarhan/bluetooth-widget/internal/autostart"
	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
	"github.com/shariorfarhan/bluetooth-widget/internal/config"
)

// AppID is the application's D-Bus/desktop-file identifier.
const AppID = autostart.AppID

// Options bundles what Run needs from main().
type Options struct {
	Manager bluetooth.BluetoothManager
	// Audio is optional; when nil the per-device audio settings are
	// not offered.
	Audio   audio.Controller
	Config  *config.Store
	Logger  *slog.Logger
	Version string
}

// Run builds and runs the GtkApplication. It blocks until the
// application quits and returns the process exit code.
func Run(opts Options) int {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	app := gtk.NewApplication(AppID, gio.ApplicationFlagsNone)

	var w *Window

	app.ConnectStartup(func() {
		loadCSS()
		setupTheme()
	})

	app.ConnectActivate(func() {
		if w != nil {
			w.win.Present()
			return
		}
		w = newWindow(app, opts)
		w.win.Present()
	})

	quit := gio.NewSimpleAction("quit", nil)
	quit.ConnectActivate(func(_ *glib.Variant) {
		if w != nil {
			w.saveState()
		}
		app.Quit()
	})
	app.AddAction(quit)
	app.SetAccelsForAction("app.quit", []string{"<Control>q"})
	app.SetAccelsForAction("window.close", []string{"<Control>w"})

	app.ConnectShutdown(func() {
		if w != nil {
			w.saveState()
		}
		if opts.Manager != nil {
			_ = opts.Manager.Close()
		}
		if opts.Audio != nil {
			_ = opts.Audio.Close()
		}
	})

	return app.Run([]string{os.Args[0]})
}
