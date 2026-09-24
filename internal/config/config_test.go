package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	return p
}

func TestLoad_MissingFileGivesDefaults(t *testing.T) {
	p := tempConfigPath(t)
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := s.Get()
	if !c.ShowBattery || !c.ShowDisconnected {
		t.Fatalf("expected ShowBattery and ShowDisconnected true by default: %+v", c)
	}
	if c.Window.Width != 340 || c.Window.Height != 440 {
		t.Fatalf("expected default window 340x440, got %dx%d", c.Window.Width, c.Window.Height)
	}
}

func TestRoundTrip(t *testing.T) {
	p := tempConfigPath(t)
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Update(func(c *Config) {
		c.AlwaysOnTop = true
		c.ShowBattery = false
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	s2, err := Load(p)
	if err != nil {
		t.Fatalf("Load reload: %v", err)
	}
	c2 := s2.Get()
	if !c2.AlwaysOnTop || c2.ShowBattery {
		t.Fatalf("round trip mismatch: %+v", c2)
	}
}

func TestLoad_PartialFileKeepsDefaults(t *testing.T) {
	p := tempConfigPath(t)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"always_on_top": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := s.Get()
	if !c.AlwaysOnTop {
		t.Fatalf("expected always_on_top true from file")
	}
	if !c.ShowBattery || !c.ShowDisconnected {
		t.Fatalf("expected missing keys to keep defaults: %+v", c)
	}
}

func TestLoad_CorruptFileKeepsDefaultsAndLeavesFileUntouched(t *testing.T) {
	p := tempConfigPath(t)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{not valid json`)
	if err := os.WriteFile(p, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(p)
	if err == nil {
		t.Fatalf("expected non-nil error for corrupt file")
	}
	c := s.Get()
	if !c.ShowBattery {
		t.Fatalf("expected defaults on corrupt file: %+v", c)
	}
	data, rerr := os.ReadFile(p)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(data) != string(bad) {
		t.Fatalf("corrupt file should be left untouched")
	}
}

func TestSave_NoTempFilesLeftAndMode0600(t *testing.T) {
	p := tempConfigPath(t)
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Update(func(c *Config) { c.ShowBattery = false }); err != nil {
		t.Fatalf("Update: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Base(e.Name()) != filepath.Base(p) {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %v", info.Mode().Perm())
	}
}

func TestFavorites_NormalizedUppercasedDeduped(t *testing.T) {
	p := tempConfigPath(t)
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Update(func(c *Config) {
		c.Favorites = []string{" aa:bb:cc:dd:ee:ff ", "AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"}
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	c := s.Get()
	if len(c.Favorites) != 2 {
		t.Fatalf("expected 2 deduped favorites, got %v", c.Favorites)
	}
	for _, f := range c.Favorites {
		if f != NormalizeMAC(f) {
			t.Fatalf("favorite %q not normalized", f)
		}
	}
}

func TestSetFavorite(t *testing.T) {
	p := tempConfigPath(t)
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.SetFavorite("aa:bb:cc:dd:ee:ff", true); err != nil {
		t.Fatal(err)
	}
	if !s.IsFavorite("AA:BB:CC:DD:EE:FF") {
		t.Fatalf("expected favorite to be set")
	}
	if err := s.SetFavorite("AA:BB:CC:DD:EE:FF", false); err != nil {
		t.Fatal(err)
	}
	if s.IsFavorite("aa:bb:cc:dd:ee:ff") {
		t.Fatalf("expected favorite to be cleared")
	}
}

func TestMarshalIsIndentedJSON(t *testing.T) {
	c := Default()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty json")
	}
}
