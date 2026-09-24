// Package config loads and atomically saves the widget's local JSON
// preferences file. Only preferences and device MAC addresses are
// stored; Bluetooth pairing/auth data is never touched.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// WindowState is the remembered size/position of the main window.
// Position is only populated (and only used) on X11.
type WindowState struct {
	Width  int  `json:"width"`
	Height int  `json:"height"`
	X      *int `json:"x,omitempty"`
	Y      *int `json:"y,omitempty"`
}

// Config holds all user-editable preferences.
type Config struct {
	Version          int         `json:"version"`
	AlwaysOnTop      bool        `json:"always_on_top"`
	ShowBattery      bool        `json:"show_battery"`
	StartAtLogin     bool        `json:"start_at_login"`
	ShowDisconnected bool        `json:"show_disconnected"`
	Favorites        []string    `json:"favorites"`
	Window           WindowState `json:"window"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		Version:          1,
		AlwaysOnTop:      false,
		ShowBattery:      true,
		StartAtLogin:     false,
		ShowDisconnected: true,
		Favorites:        nil,
		Window:           WindowState{Width: 340, Height: 440},
	}
}

// DefaultPath returns $XDG_CONFIG_HOME/bluetooth-widget/config.json, or
// ~/.config/bluetooth-widget/config.json if XDG_CONFIG_HOME is unset.
func DefaultPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "bluetooth-widget", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "bluetooth-widget", "config.json"), nil
}

// NormalizeMAC upper-cases and trims a MAC address string.
func NormalizeMAC(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func normalizeFavorites(favs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range favs {
		n := NormalizeMAC(f)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Store is a mutex-guarded, disk-backed Config.
type Store struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

// Load always returns a usable *Store populated with defaults merged
// with whatever the file at path contains. The returned error is
// non-nil only when the file existed but could not be read or parsed;
// in that case the store still holds usable defaults and the bad file
// is left untouched until the next Update.
func Load(path string) (*Store, error) {
	cfg := Default()
	s := &Store{path: path, cfg: cfg}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("config file is corrupt; using defaults", "path", path, "error", err.Error())
		return s, fmt.Errorf("parse config: %w", err)
	}
	cfg.Favorites = normalizeFavorites(cfg.Favorites)
	s.cfg = cfg
	return s, nil
}

// Path returns the file path this store was loaded from / saves to.
func (s *Store) Path() string {
	return s.path
}

// Get returns a deep copy of the current configuration.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(s.cfg)
}

// Update applies fn to a copy of the current config under lock,
// normalizes it, saves it atomically, and keeps it as the new current
// config only if the save succeeds.
func (s *Store) Update(fn func(c *Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneConfig(s.cfg)
	fn(&next)
	next.Favorites = normalizeFavorites(next.Favorites)
	if next.Version == 0 {
		next.Version = 1
	}
	if err := Save(s.path, next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

// IsFavorite reports whether addr (any case/spacing) is in favorites.
func (s *Store) IsFavorite(addr string) bool {
	n := NormalizeMAC(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.cfg.Favorites {
		if f == n {
			return true
		}
	}
	return false
}

// SetFavorite adds or removes addr from favorites and saves.
func (s *Store) SetFavorite(addr string, fav bool) error {
	n := NormalizeMAC(addr)
	return s.Update(func(c *Config) {
		if fav {
			c.Favorites = append(c.Favorites, n)
			return
		}
		out := c.Favorites[:0]
		for _, f := range c.Favorites {
			if f != n {
				out = append(out, f)
			}
		}
		c.Favorites = out
	})
}

func cloneConfig(c Config) Config {
	cp := c
	if c.Favorites != nil {
		cp.Favorites = append([]string(nil), c.Favorites...)
	}
	if c.Window.X != nil {
		x := *c.Window.X
		cp.Window.X = &x
	}
	if c.Window.Y != nil {
		y := *c.Window.Y
		cp.Window.Y = &y
	}
	return cp
}

// Save atomically writes c as indented JSON to path: write to a temp
// file in the same directory, fsync, chmod 0600, then rename over the
// destination.
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	f, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmp := f.Name()
	cleanup := func() {
		f.Close()
		os.Remove(tmp)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		cleanup()
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	if _, err := f.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}
