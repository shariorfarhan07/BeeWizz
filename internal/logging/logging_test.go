package logging

import (
	"log/slog"
	"testing"
)

func TestLevelFromEnv(t *testing.T) {
	cases := []struct {
		name  string
		debug bool
		env   string
		want  slog.Level
	}{
		{"debug flag wins", true, "error", slog.LevelDebug},
		{"env debug", false, "debug", slog.LevelDebug},
		{"env warn", false, "warn", slog.LevelWarn},
		{"env error", false, "error", slog.LevelError},
		{"env info explicit", false, "info", slog.LevelInfo},
		{"default", false, "", slog.LevelInfo},
		{"unknown env", false, "bogus", slog.LevelInfo},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("BLUETOOTH_WIDGET_LOG", c.env)
			if got := LevelFromEnv(c.debug); got != c.want {
				t.Errorf("LevelFromEnv(%v) with env=%q = %v, want %v", c.debug, c.env, got, c.want)
			}
		})
	}
}
