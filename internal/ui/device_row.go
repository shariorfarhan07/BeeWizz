package ui

import (
	"fmt"
	"strings"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
)

// opKind is the operation currently running for a device row.
type opKind int

const (
	opNone opKind = iota
	opConnect
	opDisconnect
)

type rowCallbacks struct {
	onFavorite func(id bluetooth.DeviceID)
}

// deviceRow is one row in the device list.
type deviceRow struct {
	id       bluetooth.DeviceID
	dev      *bluetooth.BluetoothDevice
	favorite bool
	busy     opKind
	errText  string

	row      *gtk.ListBoxRow
	icon     *gtk.Image
	name     *gtk.Label
	status   *gtk.Label
	battery  *gtk.Label
	battIcon *gtk.Image
	spinner  *gtk.Spinner
	audioBtn *gtk.MenuButton
	audio    *audioPanel // created the first time the device has an audio card
	star     *gtk.Button
}

func newDeviceRow(d *bluetooth.BluetoothDevice, favorite bool, cb rowCallbacks, showBattery bool) *deviceRow {
	r := &deviceRow{id: d.ID, dev: d.Clone(), favorite: favorite}

	r.row = gtk.NewListBoxRow()
	r.row.SetName(string(d.ID))
	r.row.SetActivatable(true)
	r.row.AddCSSClass("bw-row")

	hbox := gtk.NewBox(gtk.OrientationHorizontal, 10)

	r.icon = gtk.NewImage()
	r.icon.SetPixelSize(24)
	hbox.Append(r.icon)

	vbox := gtk.NewBox(gtk.OrientationVertical, 2)
	vbox.SetHExpand(true)

	r.name = gtk.NewLabel("")
	r.name.SetXAlign(0)
	r.name.SetEllipsize(pango.EllipsizeEnd)
	r.name.AddCSSClass("bw-name")
	vbox.Append(r.name)

	line2 := gtk.NewBox(gtk.OrientationHorizontal, 6)

	r.status = gtk.NewLabel("")
	r.status.SetXAlign(0)
	r.status.SetHExpand(true)
	r.status.AddCSSClass("bw-status")
	line2.Append(r.status)

	r.battIcon = gtk.NewImage()
	r.battIcon.SetPixelSize(16)
	line2.Append(r.battIcon)

	r.battery = gtk.NewLabel("")
	r.battery.AddCSSClass("bw-battery")
	line2.Append(r.battery)

	r.spinner = gtk.NewSpinner()
	r.spinner.SetVisible(false)
	line2.Append(r.spinner)

	vbox.Append(line2)
	hbox.Append(vbox)

	r.audioBtn = gtk.NewMenuButton()
	r.audioBtn.SetIconName("audio-volume-high-symbolic")
	r.audioBtn.SetHasFrame(false)
	r.audioBtn.SetVAlign(gtk.AlignCenter)
	r.audioBtn.SetTooltipText("Audio settings")
	setA11yLabel(&r.audioBtn.Widget, "Audio settings")
	r.audioBtn.SetVisible(false)
	hbox.Append(r.audioBtn)

	r.star = gtk.NewButtonFromIconName(starIconName(favorite))
	r.star.SetHasFrame(false)
	r.star.SetVAlign(gtk.AlignCenter)
	r.star.AddCSSClass("flat")
	r.star.ConnectClicked(func() {
		if cb.onFavorite != nil {
			cb.onFavorite(r.id)
		}
	})
	hbox.Append(r.star)

	r.row.SetChild(hbox)

	r.render(showBattery)
	return r
}

func starIconName(favorite bool) string {
	if favorite {
		return "starred-symbolic"
	}
	return "non-starred-symbolic"
}

// update applies a fresh device snapshot (and favorite flag) to the
// row and re-renders it. It reports whether the sort key changed:
// Connected, favorite, or DisplayName.
func (r *deviceRow) update(d *bluetooth.BluetoothDevice, favorite, showBattery bool) bool {
	prevConnected := false
	prevFav := r.favorite
	prevName := ""
	if r.dev != nil {
		prevConnected = r.dev.Connected
		prevName = r.dev.DisplayName()
	}

	r.dev = d.Clone()
	r.favorite = favorite
	r.render(showBattery)

	return prevConnected != d.Connected || prevFav != favorite || prevName != d.DisplayName()
}

func (r *deviceRow) setBusy(op opKind) {
	r.busy = op
	r.battery.SetVisible(false)
	r.battIcon.SetVisible(false)
	r.spinner.SetVisible(true)
	r.spinner.Start()
	if op == opConnect {
		r.status.SetText("Connecting…")
	} else {
		r.status.SetText("Disconnecting…")
	}
	r.status.RemoveCSSClass("connected")
	r.status.RemoveCSSClass("error")
	r.status.AddCSSClass("dim-label")
	r.row.SetTooltipText(fmt.Sprintf("%s (%s) — working…", r.dev.DisplayName(), r.dev.Address))
}

func (r *deviceRow) clearBusy() {
	r.busy = opNone
	r.spinner.Stop()
	r.spinner.SetVisible(false)
	r.render(true)
}

func (r *deviceRow) showError(short string) {
	r.errText = short
	r.render(true)
}

// render is the single place that sets all texts, CSS classes,
// tooltips, and accessible labels from the row's current state.
func (r *deviceRow) render(showBattery bool) {
	d := r.dev
	if d == nil {
		return
	}

	r.name.SetText(d.DisplayName())

	for _, cand := range bluetooth.IconCandidates(d.Icon) {
		theme := gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault())
		if theme != nil && theme.HasIcon(cand) {
			r.icon.SetFromIconName(cand)
			break
		}
	}

	r.status.RemoveCSSClass("connected")
	r.status.RemoveCSSClass("error")
	r.status.RemoveCSSClass("dim-label")

	switch {
	case r.busy == opConnect:
		r.status.SetText("Connecting…")
		r.status.AddCSSClass("dim-label")
	case r.busy == opDisconnect:
		r.status.SetText("Disconnecting…")
		r.status.AddCSSClass("dim-label")
	case r.errText != "":
		r.status.SetText(r.errText)
		r.status.AddCSSClass("error")
	case d.Connected:
		r.status.SetText("● Connected")
		r.status.AddCSSClass("connected")
	default:
		r.status.SetText("○ Click to connect")
		r.status.AddCSSClass("dim-label")
	}

	if r.busy == opNone {
		r.spinner.Stop()
		r.spinner.SetVisible(false)
	}

	r.renderBattery(showBattery)

	r.star.SetIconName(starIconName(r.favorite))
	if r.favorite {
		r.star.SetTooltipText("Remove from favorites")
	} else {
		r.star.SetTooltipText("Add to favorites")
	}
	setA11yLabel(&r.star.Widget, r.star.TooltipText())

	verb := "click to connect"
	if d.Connected {
		verb = "click to disconnect"
	}
	if r.busy != opNone {
		verb = "working…"
	}
	r.row.SetTooltipText(fmt.Sprintf("%s (%s) — %s", d.DisplayName(), d.Address, verb))

	setA11yLabel(&r.row.Widget, r.a11yText())
}

func (r *deviceRow) renderBattery(showBattery bool) {
	d := r.dev
	if !showBattery || len(d.Batteries) == 0 || (r.busy != opNone) {
		r.battery.SetVisible(false)
		r.battIcon.SetVisible(false)
		return
	}

	if len(d.Batteries) == 1 {
		pct := d.Batteries[0].Percent
		r.battery.SetText(fmt.Sprintf("%d%%", pct))
		r.battery.SetVisible(true)
		r.battery.SetTooltipText(fmt.Sprintf("Battery: %d%%", pct))

		level := (pct / 10) * 10
		iconName := fmt.Sprintf("battery-level-%d-symbolic", level)
		theme := gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault())
		if theme != nil && theme.HasIcon(iconName) {
			r.battIcon.SetFromIconName(iconName)
			r.battIcon.SetVisible(true)
		} else {
			r.battIcon.SetVisible(false)
		}
		return
	}

	parts := make([]string, 0, len(d.Batteries))
	for _, b := range d.Batteries {
		label := batteryPartLabel(b.Label)
		parts = append(parts, fmt.Sprintf("%s %d%%", label, b.Percent))
	}
	r.battery.SetText(strings.Join(parts, " · "))
	r.battery.SetVisible(true)
	r.battery.SetTooltipText(strings.Join(parts, ", "))
	r.battIcon.SetVisible(false)
}

func batteryPartLabel(label string) string {
	switch strings.ToLower(label) {
	case "left":
		return "L"
	case "right":
		return "R"
	case "":
		return ""
	default:
		if label == "" {
			return ""
		}
		return strings.ToUpper(label[:1]) + label[1:]
	}
}

func (r *deviceRow) a11yText() string {
	d := r.dev
	parts := []string{d.DisplayName()}
	if r.busy == opConnect {
		parts = append(parts, "Connecting")
	} else if r.busy == opDisconnect {
		parts = append(parts, "Disconnecting")
	} else if d.Connected {
		parts = append(parts, "Connected")
	} else {
		parts = append(parts, "Disconnected")
	}
	if p := d.Battery; p != nil {
		parts = append(parts, fmt.Sprintf("battery %d%%", *p))
	}
	return strings.Join(parts, ", ")
}

// syncAudio shows the audio-settings button while the device is
// connected and the sound server has a card for it, and refreshes the
// popover from st.
func (r *deviceRow) syncAudio(w *Window, st *audio.State) {
	card := st.CardForAddress(r.dev.Address)
	show := card != nil && r.dev.Connected
	if show && r.audio == nil {
		r.audio = newAudioPanel(w, r.dev.Address, r.dev.DisplayName())
		r.audioBtn.SetPopover(r.audio.pop)
	}
	r.audioBtn.SetVisible(show)
	if !show {
		if r.audio != nil {
			r.audio.pop.Popdown()
		}
		return
	}
	r.audio.name = r.dev.DisplayName()
	r.audio.refresh(st)
}

// setA11yLabel sets a widget's ARIA-style accessible label. It is
// non-critical: any failure is silently ignored so it can never crash
// the app.
func setA11yLabel(w *gtk.Widget, text string) {
	defer func() { recover() }()
	val := coreglib.NewValue(text)
	w.UpdateProperty([]gtk.AccessibleProperty{gtk.AccessiblePropertyLabel}, []coreglib.Value{*val})
}
