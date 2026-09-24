package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// statusPage is a centered, full-page message with an icon, title,
// subtitle, and optional action button and error label.
type statusPage struct {
	box      *gtk.Box
	title    *gtk.Label
	subtitle *gtk.Label
	action   *gtk.Button
	spinner  *gtk.Spinner
	errLabel *gtk.Label
	pageSpin *gtk.Spinner
}

func newStatusPage(iconName, title, subtitle string) *statusPage {
	p := &statusPage{}

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetVAlign(gtk.AlignCenter)
	box.SetHAlign(gtk.AlignCenter)
	box.SetVExpand(true)
	box.SetHExpand(true)
	p.box = box

	if iconName != "" {
		img := gtk.NewImageFromIconName(iconName)
		img.SetPixelSize(48)
		img.AddCSSClass("dim-label")
		box.Append(img)
	} else {
		sp := gtk.NewSpinner()
		sp.SetSizeRequest(48, 48)
		sp.Start()
		p.pageSpin = sp
		box.Append(sp)
	}

	p.title = gtk.NewLabel(title)
	p.title.AddCSSClass("bw-empty-title")
	box.Append(p.title)

	p.subtitle = gtk.NewLabel(subtitle)
	p.subtitle.AddCSSClass("dim-label")
	p.subtitle.SetWrap(true)
	p.subtitle.SetMaxWidthChars(30)
	p.subtitle.SetJustify(gtk.JustifyCenter)
	p.subtitle.SetVisible(subtitle != "")
	box.Append(p.subtitle)

	p.errLabel = gtk.NewLabel("")
	p.errLabel.AddCSSClass("error")
	p.errLabel.SetWrap(true)
	p.errLabel.SetVisible(false)
	box.Append(p.errLabel)

	return p
}

// addAction adds a suggested-action button with an inline spinner
// (hidden until startBusy is called) below the subtitle.
func (p *statusPage) addAction(label string, onClick func()) {
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)
	row.SetHAlign(gtk.AlignCenter)

	p.action = gtk.NewButtonWithLabel(label)
	p.action.AddCSSClass("suggested-action")
	p.action.ConnectClicked(func() {
		if onClick != nil {
			onClick()
		}
	})
	row.Append(p.action)

	p.spinner = gtk.NewSpinner()
	p.spinner.SetVisible(false)
	row.Append(p.spinner)

	p.box.Append(row)
}

func (p *statusPage) setBusy(busy bool) {
	if p.action != nil {
		p.action.SetSensitive(!busy)
	}
	if p.spinner != nil {
		if busy {
			p.spinner.SetVisible(true)
			p.spinner.Start()
		} else {
			p.spinner.Stop()
			p.spinner.SetVisible(false)
		}
	}
}

func (p *statusPage) setError(text string) {
	p.errLabel.SetText(text)
	p.errLabel.SetVisible(text != "")
}

func (p *statusPage) setTexts(title, subtitle string) {
	p.title.SetText(title)
	p.subtitle.SetText(subtitle)
	p.subtitle.SetVisible(subtitle != "")
}

// statusPages holds all of the stack's non-"devices" pages.
type statusPages struct {
	loading   *statusPage
	empty     *statusPage
	off       *statusPage
	noAdapter *statusPage
	unavail   *statusPage
}

func newStatusPages(stack *gtk.Stack, onPowerOn func()) *statusPages {
	sp := &statusPages{}

	sp.loading = newStatusPage("", "Loading…", "")
	stack.AddNamed(sp.loading.box, "loading")

	sp.empty = newStatusPage("bluetooth-symbolic", "No paired devices", "Pair a Bluetooth device to see it here.")
	stack.AddNamed(sp.empty.box, "empty")

	sp.off = newStatusPage("bluetooth-disabled-symbolic", "Bluetooth is turned off.", "")
	sp.off.addAction("Turn Bluetooth On", onPowerOn)
	stack.AddNamed(sp.off.box, "off")

	sp.noAdapter = newStatusPage("bluetooth-disabled-symbolic", "No Bluetooth adapter detected.",
		"Connect a Bluetooth adapter or check that it is enabled.")
	stack.AddNamed(sp.noAdapter.box, "noadapter")

	sp.unavail = newStatusPage("dialog-error-symbolic", "Bluetooth service unavailable",
		"The BlueZ service (bluetoothd) is not running. The widget will reconnect automatically when it starts.")
	stack.AddNamed(sp.unavail.box, "unavailable")

	return sp
}
