package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// header is the window's title bar: a centered "Bluetooth" title and a
// settings gear button that opens the settings popover.
type header struct {
	bar  *gtk.HeaderBar
	menu *gtk.MenuButton
}

func newHeader(settings *gtk.Popover) *header {
	h := &header{}

	h.bar = gtk.NewHeaderBar()

	title := gtk.NewLabel("Bluetooth")
	title.AddCSSClass("title")
	h.bar.SetTitleWidget(title)

	h.menu = gtk.NewMenuButton()
	h.menu.SetIconName("open-menu-symbolic")
	h.menu.SetTooltipText("Settings")
	h.menu.SetPopover(settings)
	h.bar.PackEnd(h.menu)

	setA11yLabel(&h.menu.Widget, "Settings")

	return h
}
