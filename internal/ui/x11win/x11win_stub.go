//go:build nox11

// Package x11win provides best-effort X11 window hints. This build
// (tag "nox11") is used when GTK4's X11-cgo headers are unavailable;
// every function is a no-op.
package x11win

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

func Supported() bool { return false }

func SetKeepAbove(w *gtk.Window, on bool) bool { return false }

func SetSkipTaskbar(w *gtk.Window, on bool) bool { return false }

func Position(w *gtk.Window) (x, y int, ok bool) { return 0, 0, false }

func Move(w *gtk.Window, x, y int) bool { return false }
