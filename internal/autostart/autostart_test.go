package autostart

import (
	"os"
	"strings"
	"testing"
)

func setupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestEnable_WritesCorrectExec(t *testing.T) {
	setupDir(t)
	if err := Enable("/usr/local/bin/bluetooth-widget"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	p, err := FilePath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Exec=/usr/local/bin/bluetooth-widget\n") {
		t.Fatalf("unexpected content: %s", data)
	}
	if !IsEnabled() {
		t.Fatalf("expected IsEnabled() true after Enable")
	}
}

func TestQuoteExec(t *testing.T) {
	if got := QuoteExec("/usr/bin/bluetooth-widget"); got != "/usr/bin/bluetooth-widget" {
		t.Fatalf("plain path should not be quoted, got %q", got)
	}
	if got := QuoteExec("/a b/c"); got != `"/a b/c"` {
		t.Fatalf(`QuoteExec("/a b/c") = %q, want "\"/a b/c\""`, got)
	}
}

func TestDisable_RemovesAndIsIdempotent(t *testing.T) {
	setupDir(t)
	if err := Enable("/usr/bin/bluetooth-widget"); err != nil {
		t.Fatal(err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if IsEnabled() {
		t.Fatalf("expected IsEnabled() false after Disable")
	}
	if err := Disable(); err != nil {
		t.Fatalf("second Disable should be idempotent, got: %v", err)
	}
}

func TestIsEnabled_MissingFile(t *testing.T) {
	setupDir(t)
	if IsEnabled() {
		t.Fatalf("expected false with no file")
	}
}
