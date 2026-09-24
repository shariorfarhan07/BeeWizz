package ui

import (
	"context"
	"errors"
	"log/slog"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
	"github.com/shariorfarhan/bluetooth-widget/internal/config"
	"github.com/shariorfarhan/bluetooth-widget/internal/order"
	"github.com/shariorfarhan/bluetooth-widget/internal/ui/x11win"
)

// Window is the widget's single top-level window.
type Window struct {
	app    *gtk.Application
	win    *gtk.ApplicationWindow
	mgr    bluetooth.BluetoothManager
	audio  audio.Controller // nil when audio integration is disabled
	cfg    *config.Store
	sorter order.Sorter
	ctx    context.Context
	cancel context.CancelFunc

	stack    *gtk.Stack
	list     *gtk.ListBox
	rows     map[bluetooth.DeviceID]*deviceRow
	banner   *errorBanner
	header   *header
	settings *settingsPopover
	pages    *statusPages

	audioState *audio.State

	serviceUp bool
	adapter   *bluetooth.Adapter
	loaded    bool
	powerBusy bool
	x11       bool
}

func newWindow(app *gtk.Application, o Options) *Window {
	w := &Window{
		app:   app,
		mgr:   o.Manager,
		audio: o.Audio,
		cfg:   o.Config,
		rows:  map[bluetooth.DeviceID]*deviceRow{},
		x11:   x11win.Supported(),
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.sorter = order.DefaultSorter{IsFavorite: w.cfg.IsFavorite}

	c := w.cfg.Get()

	w.win = gtk.NewApplicationWindow(app)
	w.win.SetTitle("Bluetooth")
	w.win.SetIconName(AppID)

	width, height := c.Window.Width, c.Window.Height
	if width < 280 {
		width = 280
	}
	if height < 200 {
		height = 200
	}
	w.win.SetDefaultSize(width, height)
	w.win.SetSizeRequest(280, 200)

	w.settings = newSettingsPopover(w)
	w.header = newHeader(w.settings.pop)
	w.win.SetTitlebar(w.header.bar)

	w.banner = newErrorBanner()

	w.stack = gtk.NewStack()
	w.pages = newStatusPages(w.stack, w.powerOn)

	w.list = gtk.NewListBox()
	w.list.SetSelectionMode(gtk.SelectionNone)
	w.list.SetActivateOnSingleClick(true)
	w.list.AddCSSClass("navigation-sidebar")
	w.list.AddCSSClass("bw-list")

	w.list.SetSortFunc(func(a, b *gtk.ListBoxRow) int {
		ra := w.rows[bluetooth.DeviceID(a.Name())]
		rb := w.rows[bluetooth.DeviceID(b.Name())]
		if ra == nil || rb == nil {
			return 0
		}
		return w.sorter.Compare(ra.dev, rb.dev)
	})
	w.list.SetFilterFunc(func(row *gtk.ListBoxRow) bool {
		dr := w.rows[bluetooth.DeviceID(row.Name())]
		if dr == nil {
			return false
		}
		return w.cfg.Get().ShowDisconnected || dr.dev.Connected || dr.busy != opNone
	})
	w.list.SetHeaderFunc(func(row, before *gtk.ListBoxRow) {
		dr := w.rows[bluetooth.DeviceID(row.Name())]
		if dr == nil {
			row.SetHeader(nil)
			return
		}
		section := sectionOf(dr.dev)

		var beforeSection string
		var haveBefore bool
		if before != nil {
			if bdr := w.rows[bluetooth.DeviceID(before.Name())]; bdr != nil {
				beforeSection = sectionOf(bdr.dev)
				haveBefore = true
			}
		}
		if !haveBefore || beforeSection != section {
			row.SetHeader(newSectionLabel(section))
		} else {
			row.SetHeader(nil)
		}
	})
	w.list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		dr := w.rows[bluetooth.DeviceID(row.Name())]
		if dr == nil || dr.busy != opNone {
			return
		}
		if dr.dev.Connected {
			w.startOp(dr.id, opDisconnect)
		} else {
			w.startOp(dr.id, opConnect)
		}
	})

	sw := gtk.NewScrolledWindow()
	sw.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	sw.SetVExpand(true)
	sw.SetChild(w.list)
	w.stack.AddNamed(sw, "devices")

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.Append(w.banner.rev)
	content.Append(w.stack)
	w.win.SetChild(content)

	w.stack.SetVisibleChildName("loading")

	w.win.ConnectCloseRequest(func() bool {
		w.saveState()
		return false
	})
	w.win.ConnectMap(func() {
		glib.TimeoutAdd(150, func() {
			w.applyWindowHints()
		})
	})

	unsub := o.Manager.SubscribeToChanges(func(ev bluetooth.Event) {
		glib.IdleAdd(func() { w.handleEvent(ev) })
	})
	_ = unsub // kept alive for the process lifetime; closed via mgr.Close on shutdown

	go func() {
		if err := o.Manager.Start(w.ctx); err != nil {
			slog.Warn("bluetooth manager start", "error", err.Error())
		}
	}()

	if w.audio != nil {
		w.audio.Subscribe(func(st *audio.State) {
			glib.IdleAdd(func() { w.applyAudio(st) })
		})
		go func() {
			if err := w.audio.Start(w.ctx); err != nil {
				slog.Info("sound server unavailable; audio settings hidden until it appears", "error", err.Error())
			}
		}()
	}

	return w
}

func sectionOf(d *bluetooth.BluetoothDevice) string {
	if d.Connected {
		return "CONNECTED"
	}
	return "PAIRED"
}

func newSectionLabel(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.AddCSSClass("bw-section")
	l.SetXAlign(0)
	return l
}

func (w *Window) handleEvent(ev bluetooth.Event) {
	switch ev.Kind {
	case bluetooth.EventReset:
		w.applyReset(ev.State)
	case bluetooth.EventDeviceAdded, bluetooth.EventDeviceChanged:
		w.upsertDevice(ev.Device)
	case bluetooth.EventDeviceRemoved:
		w.removeDevice(ev.DeviceID)
	case bluetooth.EventAdapterChanged:
		w.adapter = ev.Adapter
		w.settings.refresh(w)
	}
	w.updatePage()
}

// applyAudio stores a new sound-server snapshot and updates every
// row's audio button and popover.
func (w *Window) applyAudio(st *audio.State) {
	w.audioState = st
	for _, r := range w.rows {
		r.syncAudio(w, st)
	}
}

func (w *Window) applyReset(st *bluetooth.State) {
	if st == nil {
		return
	}
	for _, r := range w.rows {
		w.list.Remove(r.row)
	}
	w.rows = map[bluetooth.DeviceID]*deviceRow{}

	w.serviceUp = st.ServiceAvailable
	w.adapter = st.Adapter

	c := w.cfg.Get()
	for _, d := range st.Devices {
		r := newDeviceRow(d, w.cfg.IsFavorite(d.Address), rowCallbacks{onFavorite: w.toggleFavorite}, c.ShowBattery)
		w.rows[d.ID] = r
		w.list.Append(r.row)
		r.syncAudio(w, w.audioState)
	}

	w.settings.refresh(w)
	w.loaded = true
}

func (w *Window) upsertDevice(d *bluetooth.BluetoothDevice) {
	if d == nil {
		return
	}
	c := w.cfg.Get()
	fav := w.cfg.IsFavorite(d.Address)

	r, ok := w.rows[d.ID]
	if ok {
		changed := r.update(d, fav, c.ShowBattery)
		if changed {
			r.row.Changed()
			w.list.InvalidateHeaders()
		}
	} else {
		r = newDeviceRow(d, fav, rowCallbacks{onFavorite: w.toggleFavorite}, c.ShowBattery)
		w.rows[d.ID] = r
		w.list.Append(r.row)
	}
	r.syncAudio(w, w.audioState)

	if d.Connected && w.banner.refersTo(d.ID) {
		w.banner.hide()
	}
}

func (w *Window) removeDevice(id bluetooth.DeviceID) {
	r, ok := w.rows[id]
	if !ok {
		return
	}
	w.list.Remove(r.row)
	delete(w.rows, id)
	if w.banner.refersTo(id) {
		w.banner.hide()
	}
}

func (w *Window) visibleCount() int {
	c := w.cfg.Get()
	n := 0
	for _, r := range w.rows {
		if c.ShowDisconnected || r.dev.Connected || r.busy != opNone {
			n++
		}
	}
	return n
}

func (w *Window) updatePage() {
	name := w.pageName()
	if w.stack.VisibleChildName() != name {
		w.stack.SetVisibleChildName(name)
	}
	if name == "empty" {
		if len(w.rows) == 0 {
			w.pages.empty.setTexts("No paired devices", "Pair a Bluetooth device to see it here.")
		} else {
			w.pages.empty.setTexts("No connected devices", `Turn on "Show disconnected devices" in settings to see all paired devices.`)
		}
	}
}

func (w *Window) pageName() string {
	switch {
	case !w.loaded:
		return "loading"
	case !w.serviceUp:
		return "unavailable"
	case w.adapter == nil:
		return "noadapter"
	case !w.adapter.Powered:
		return "off"
	case w.visibleCount() == 0:
		return "empty"
	default:
		return "devices"
	}
}

func (w *Window) startOp(id bluetooth.DeviceID, op opKind) {
	r := w.rows[id]
	if r == nil || r.busy != opNone {
		return
	}
	r.setBusy(op)

	go func() {
		var err error
		if op == opConnect {
			err = w.mgr.ConnectDevice(w.ctx, id)
		} else {
			err = w.mgr.DisconnectDevice(w.ctx, id)
		}
		glib.IdleAdd(func() { w.finishOp(id, op, err) })
	}()
}

func (w *Window) finishOp(id bluetooth.DeviceID, op opKind, err error) {
	r := w.rows[id]
	if r == nil {
		return
	}
	r.clearBusy()

	if err == nil {
		return
	}

	var oe *bluetooth.OpError
	if errors.As(err, &oe) {
		if oe.Kind == bluetooth.KindInProgress || oe.Kind == bluetooth.KindBusy {
			return
		}
		r.showError(oe.Short())
		if friendly := oe.Friendly(); friendly != "" {
			w.banner.show(friendly, func() { w.startOp(id, op) }, id)
		}
		return
	}

	r.showError("Unable to connect")
	w.banner.show("Unable to connect.", func() { w.startOp(id, op) }, id)
}

func (w *Window) powerOn() {
	w.pages.off.setBusy(true)
	go func() {
		err := w.mgr.SetAdapterPowered(w.ctx, true)
		glib.IdleAdd(func() {
			w.pages.off.setBusy(false)
			if err != nil {
				var oe *bluetooth.OpError
				if errors.As(err, &oe) {
					w.pages.off.setError(oe.Friendly())
				} else {
					w.pages.off.setError("Could not turn Bluetooth on.")
				}
			} else {
				w.pages.off.setError("")
			}
		})
	}()
}

func (w *Window) toggleFavorite(id bluetooth.DeviceID) {
	r := w.rows[id]
	if r == nil {
		return
	}
	addr := r.dev.Address
	newFav := !w.cfg.IsFavorite(addr)
	_ = w.cfg.SetFavorite(addr, newFav)

	c := w.cfg.Get()
	for _, row := range w.rows {
		fav := w.cfg.IsFavorite(row.dev.Address)
		changed := row.update(row.dev, fav, c.ShowBattery)
		if changed {
			row.row.Changed()
		}
	}
	w.list.InvalidateSort()
	w.list.InvalidateHeaders()
}

func (w *Window) rerenderAllRows() {
	c := w.cfg.Get()
	for _, r := range w.rows {
		r.render(c.ShowBattery)
	}
}

func (w *Window) applyWindowHints() {
	if !w.x11 {
		return
	}
	c := w.cfg.Get()
	if c.Window.X != nil && c.Window.Y != nil && *c.Window.X >= 0 && *c.Window.Y >= 0 {
		x11win.Move(&w.win.Window, *c.Window.X, *c.Window.Y)
	}
	if c.AlwaysOnTop {
		x11win.SetKeepAbove(&w.win.Window, true)
		x11win.SetSkipTaskbar(&w.win.Window, true)
	}
}

func (w *Window) saveState() {
	width, height := w.win.DefaultSize()
	if width <= 0 {
		width = w.win.Width()
	}
	if height <= 0 {
		height = w.win.Height()
	}
	maximized := w.win.IsMaximized()

	var xPtr, yPtr *int
	if w.x11 {
		if x, y, ok := x11win.Position(&w.win.Window); ok {
			xPtr, yPtr = &x, &y
		}
	}

	_ = w.cfg.Update(func(c *config.Config) {
		if !maximized {
			c.Window.Width = width
			c.Window.Height = height
		}
		if xPtr != nil && yPtr != nil {
			c.Window.X = xPtr
			c.Window.Y = yPtr
		}
	})
}
