// Package logging configures the application's structured logger.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Attribute key constants used across the bluetooth manager and CLI so
// log lines stay consistent, e.g.:
//
//	INFO connect device=CMF-Buds path=/org/bluez/hci0/dev_...
//	ERROR connect device=CMF-Buds error=org.bluez.Error.Failed
const (
	KeyOp          = "op"
	KeyDevice      = "device"
	KeyAddress     = "address"
	KeyPath        = "path"
	KeyError       = "error"
	KeyDBusError   = "dbus_error"
	KeyDBusMessage = "dbus_message"
)

// LevelFromEnv resolves the log level. The debug flag wins over
// everything; otherwise the BLUETOOTH_WIDGET_LOG environment variable is
// consulted ("debug", "info", "warn", "error"); the default is Info.
func LevelFromEnv(debug bool) slog.Level {
	if debug {
		return slog.LevelDebug
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BLUETOOTH_WIDGET_LOG"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

// Setup builds a text-handler slog.Logger writing to w at the given
// level, sets it as the process default, and returns it.
func Setup(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}
