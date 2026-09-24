//go:build !nox11

// Package x11win provides best-effort X11 window hints (keep-above,
// skip-taskbar, position) that GTK4 has no portable API for. It is a
// no-op on Wayland (Supported() returns false).
package x11win

/*
#cgo pkg-config: gtk4 x11
#include <stdint.h>
#include <string.h>
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>
#include <X11/Xlib.h>
#include <X11/Xatom.h>

static int bw_is_x11(void) {
    GdkDisplay *d = gdk_display_get_default();
    return d != NULL && GDK_IS_X11_DISPLAY(d);
}
static GdkSurface *bw_surface(uintptr_t w) {
    GtkNative *n = gtk_widget_get_native(GTK_WIDGET((gpointer)w));
    if (n == NULL) return NULL;
    GdkSurface *s = gtk_native_get_surface(n);
    if (s == NULL || !GDK_IS_X11_SURFACE(s)) return NULL;
    return s;
}
static int bw_wm_state(uintptr_t w, int add, const char *atom_name) {
    GdkSurface *s = bw_surface(w); if (!s) return 0;
    Display *dpy = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
    Window xid = gdk_x11_surface_get_xid(s);
    XEvent ev; memset(&ev, 0, sizeof ev);
    ev.xclient.type = ClientMessage;
    ev.xclient.window = xid;
    ev.xclient.message_type = XInternAtom(dpy, "_NET_WM_STATE", False);
    ev.xclient.format = 32;
    ev.xclient.data.l[0] = add ? 1 : 0;
    ev.xclient.data.l[1] = XInternAtom(dpy, atom_name, False);
    ev.xclient.data.l[2] = 0;
    ev.xclient.data.l[3] = 1;
    XSendEvent(dpy, DefaultRootWindow(dpy), False, SubstructureRedirectMask | SubstructureNotifyMask, &ev);
    XFlush(dpy);
    return 1;
}
static int bw_set_above(uintptr_t w, int on) { return bw_wm_state(w, on, "_NET_WM_STATE_ABOVE"); }
static int bw_set_skip_taskbar(uintptr_t w, int on) {
    int a = bw_wm_state(w, on, "_NET_WM_STATE_SKIP_TASKBAR");
    int b = bw_wm_state(w, on, "_NET_WM_STATE_SKIP_PAGER");
    return a && b;
}
static int bw_get_pos(uintptr_t w, int *x, int *y) {
    GdkSurface *s = bw_surface(w); if (!s) return 0;
    Display *dpy = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
    Window child;
    return XTranslateCoordinates(dpy, gdk_x11_surface_get_xid(s), DefaultRootWindow(dpy), 0, 0, x, y, &child) ? 1 : 0;
}
static int bw_move(uintptr_t w, int x, int y) {
    GdkSurface *s = bw_surface(w); if (!s) return 0;
    Display *dpy = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
    XMoveWindow(dpy, gdk_x11_surface_get_xid(s), x, y);
    XFlush(dpy);
    return 1;
}
*/
import "C"

import (
	"runtime"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func ptr(w *gtk.Window) C.uintptr_t {
	return C.uintptr_t(coreglib.InternObject(w).Native())
}

// Supported reports whether the current GDK backend is X11.
func Supported() bool {
	return C.bw_is_x11() != 0
}

// SetKeepAbove toggles the _NET_WM_STATE_ABOVE hint on w.
func SetKeepAbove(w *gtk.Window, on bool) bool {
	onC := 0
	if on {
		onC = 1
	}
	ok := C.bw_set_above(ptr(w), C.int(onC)) != 0
	runtime.KeepAlive(w)
	return ok
}

// SetSkipTaskbar toggles the _NET_WM_STATE_SKIP_TASKBAR/SKIP_PAGER
// hints on w.
func SetSkipTaskbar(w *gtk.Window, on bool) bool {
	onC := 0
	if on {
		onC = 1
	}
	ok := C.bw_set_skip_taskbar(ptr(w), C.int(onC)) != 0
	runtime.KeepAlive(w)
	return ok
}

// Position returns w's root-relative X11 position.
func Position(w *gtk.Window) (x, y int, ok bool) {
	var cx, cy C.int
	res := C.bw_get_pos(ptr(w), &cx, &cy)
	runtime.KeepAlive(w)
	if res == 0 {
		return 0, 0, false
	}
	return int(cx), int(cy), true
}

// Move sets w's root-relative X11 position.
func Move(w *gtk.Window, x, y int) bool {
	ok := C.bw_move(ptr(w), C.int(x), C.int(y)) != 0
	runtime.KeepAlive(w)
	return ok
}
