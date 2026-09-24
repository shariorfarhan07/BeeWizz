package ui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/shariorfarhan/bluetooth-widget/internal/autostart"
	"github.com/shariorfarhan/bluetooth-widget/internal/config"
	"github.com/shariorfarhan/bluetooth-widget/internal/ui/x11win"
)

// settingsPopover is the gear-button popover with the four preference
// toggles plus a read-only adapter info line.
type settingsPopover struct {
	pop *gtk.Popover

	alwaysOnTop      *gtk.CheckButton
	showBattery      *gtk.CheckButton
	startAtLogin     *gtk.CheckButton
	showDisconnected *gtk.CheckButton
	adapterLabel     *gtk.Label

	updating bool
}

func newSettingsPopover(w *Window) *settingsPopover {
	s := &settingsPopover{}
	s.pop = gtk.NewPopover()

	box := gtk.NewBox(gtk.OrientationVertical, 6)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	s.alwaysOnTop = gtk.NewCheckButtonWithLabel("Always on top")
	if !w.x11 {
		s.alwaysOnTop.SetSensitive(false)
		s.alwaysOnTop.SetTooltipText("Not available on Wayland. Use your desktop's window menu (Alt+Space → Always on Top) instead.")
	}
	s.alwaysOnTop.ConnectToggled(func() {
		if s.updating {
			return
		}
		v := s.alwaysOnTop.Active()
		_ = w.cfg.Update(func(c *config.Config) { c.AlwaysOnTop = v })
		if w.x11 {
			x11win.SetKeepAbove(&w.win.Window, v)
			x11win.SetSkipTaskbar(&w.win.Window, v)
		}
	})
	box.Append(s.alwaysOnTop)

	s.showBattery = gtk.NewCheckButtonWithLabel("Show battery level")
	s.showBattery.ConnectToggled(func() {
		if s.updating {
			return
		}
		v := s.showBattery.Active()
		_ = w.cfg.Update(func(c *config.Config) { c.ShowBattery = v })
		w.rerenderAllRows()
	})
	box.Append(s.showBattery)

	s.startAtLogin = gtk.NewCheckButtonWithLabel("Start at login")
	s.startAtLogin.ConnectToggled(func() {
		if s.updating {
			return
		}
		v := s.startAtLogin.Active()
		var err error
		if v {
			exe, rerr := autostart.ResolveExecutable()
			if rerr != nil {
				err = rerr
			} else {
				err = autostart.Enable(exe)
			}
		} else {
			err = autostart.Disable()
		}
		if err != nil {
			s.updating = true
			s.startAtLogin.SetActive(!v)
			s.updating = false
			s.startAtLogin.SetTooltipText(err.Error())
			return
		}
		_ = w.cfg.Update(func(c *config.Config) { c.StartAtLogin = v })
	})
	box.Append(s.startAtLogin)

	s.showDisconnected = gtk.NewCheckButtonWithLabel("Show disconnected devices")
	s.showDisconnected.ConnectToggled(func() {
		if s.updating {
			return
		}
		v := s.showDisconnected.Active()
		_ = w.cfg.Update(func(c *config.Config) { c.ShowDisconnected = v })
		w.list.InvalidateFilter()
		w.list.InvalidateHeaders()
		w.updatePage()
	})
	box.Append(s.showDisconnected)

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	s.adapterLabel = gtk.NewLabel("")
	s.adapterLabel.AddCSSClass("dim-label")
	s.adapterLabel.SetXAlign(0)
	s.adapterLabel.SetWrap(true)
	box.Append(s.adapterLabel)

	s.pop.SetChild(box)
	return s
}

func (s *settingsPopover) refresh(w *Window) {
	s.updating = true
	c := w.cfg.Get()
	s.alwaysOnTop.SetActive(c.AlwaysOnTop)
	s.showBattery.SetActive(c.ShowBattery)
	s.startAtLogin.SetActive(autostart.IsEnabled())
	s.showDisconnected.SetActive(c.ShowDisconnected)
	s.updating = false

	if w.adapter != nil {
		s.adapterLabel.SetText(fmt.Sprintf("Adapter: %s (%s)", w.adapter.Alias, w.adapter.Address))
	} else {
		s.adapterLabel.SetText("No adapter")
	}
}
