package ui

import (
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// keepSettings prevents the gio.Settings object from being garbage
// collected while its ConnectChanged handler is still needed.
var keepSettings *gio.Settings

// setupTheme makes the app follow org.gnome.desktop.interface's
// color-scheme, when that schema is available. g_settings_new aborts
// the process if the schema is missing, so the schema is always
// checked first.
func setupTheme() {
	src := gio.SettingsSchemaSourceGetDefault()
	if src == nil {
		return
	}
	schema := src.Lookup("org.gnome.desktop.interface", true)
	if schema == nil || !schema.HasKey("color-scheme") {
		return
	}
	s := gio.NewSettings("org.gnome.desktop.interface")
	apply := func() {
		prefersDark := s.String("color-scheme") == "prefer-dark"
		gtk.SettingsGetDefault().SetObjectProperty("gtk-application-prefer-dark-theme", prefersDark)
	}
	apply()
	s.ConnectChanged(func(key string) {
		if key == "color-scheme" {
			apply()
		}
	})
	keepSettings = s
}
