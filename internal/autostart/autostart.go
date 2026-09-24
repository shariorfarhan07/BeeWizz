// Package autostart manages the ~/.config/autostart XDG desktop file
// used to launch the widget at login, with no system-wide install.
package autostart

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// AppID is the application's D-Bus/desktop-file identifier, shared with
// internal/ui.
const AppID = "com.example.BluetoothWidget"

// Dir returns $XDG_CONFIG_HOME/autostart, or ~/.config/autostart.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "autostart"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart"), nil
}

// FilePath returns Dir()/<AppID>.desktop.
func FilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppID+".desktop"), nil
}

// IsEnabled reports whether the autostart file exists and does not
// disable itself via Hidden=true or X-GNOME-Autostart-enabled=false.
func IsEnabled() bool {
	p, err := FilePath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	text := string(data)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "Hidden=true" || line == "X-GNOME-Autostart-enabled=false" {
			return false
		}
	}
	return true
}

// Content renders the desktop file text for execPath.
func Content(execPath string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Bluetooth Widget
Comment=Connect and disconnect paired Bluetooth devices with one click
Exec=%s
Icon=%s
Terminal=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
`, QuoteExec(execPath), AppID)
}

// QuoteExec quotes p per the Desktop Entry Specification's Exec key
// rules if it contains characters needing quoting.
func QuoteExec(p string) string {
	const special = " \t\n\"'\\><~|&;$*?#()`"
	needsQuote := false
	for _, r := range p {
		if strings.ContainsRune(special, r) {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return p
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range p {
		switch r {
		case '"', '`', '$', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// Enable writes the autostart desktop file for execPath atomically.
func Enable(execPath string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir autostart dir: %w", err)
	}
	path, err := FilePath()
	if err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".autostart-*.desktop")
	if err != nil {
		return fmt.Errorf("create temp autostart file: %w", err)
	}
	tmp := f.Name()
	cleanup := func() {
		f.Close()
		os.Remove(tmp)
	}

	if _, err := f.WriteString(Content(execPath)); err != nil {
		cleanup()
		return fmt.Errorf("write autostart file: %w", err)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync autostart file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close autostart file: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod autostart file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename autostart file: %w", err)
	}
	return nil
}

// Disable removes the autostart file. Missing file is not an error.
func Disable() error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// ResolveExecutable returns the absolute, symlink-resolved path to the
// currently running binary. It refuses to resolve a `go build`/`go run`
// temp directory so autostart always points at an installed binary.
func ResolveExecutable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	if strings.Contains(resolved, "/go-build") {
		return "", fmt.Errorf("run the installed binary to enable autostart")
	}
	return resolved, nil
}
