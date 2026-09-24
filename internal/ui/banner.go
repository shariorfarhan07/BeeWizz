package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
)

// errorBanner is a dismissible, retryable banner shown above the
// device list for operation failures.
type errorBanner struct {
	rev      *gtk.Revealer
	label    *gtk.Label
	retry    *gtk.Button
	closeBtn *gtk.Button

	deviceID bluetooth.DeviceID
	onRetry  func()
}

func newErrorBanner() *errorBanner {
	b := &errorBanner{}

	box := gtk.NewBox(gtk.OrientationHorizontal, 8)
	box.AddCSSClass("bw-banner")

	icon := gtk.NewImageFromIconName("dialog-error-symbolic")
	box.Append(icon)

	b.label = gtk.NewLabel("")
	b.label.SetWrap(true)
	b.label.SetHExpand(true)
	b.label.SetXAlign(0)
	box.Append(b.label)

	b.retry = gtk.NewButtonWithLabel("Retry")
	b.retry.ConnectClicked(func() {
		onRetry := b.onRetry
		b.hide()
		if onRetry != nil {
			onRetry()
		}
	})
	box.Append(b.retry)

	b.closeBtn = gtk.NewButtonFromIconName("window-close-symbolic")
	b.closeBtn.AddCSSClass("flat")
	b.closeBtn.SetTooltipText("Dismiss")
	b.closeBtn.ConnectClicked(func() { b.hide() })
	box.Append(b.closeBtn)

	b.rev = gtk.NewRevealer()
	b.rev.SetTransitionType(gtk.RevealerTransitionTypeSlideDown)
	b.rev.SetChild(box)
	b.rev.SetRevealChild(false)

	return b
}

func (b *errorBanner) show(text string, retry func(), id bluetooth.DeviceID) {
	if text == "" {
		return
	}
	b.label.SetText(text)
	b.onRetry = retry
	b.deviceID = id
	b.retry.SetVisible(retry != nil)
	b.rev.SetRevealChild(true)
}

func (b *errorBanner) hide() {
	b.rev.SetRevealChild(false)
	b.onRetry = nil
	b.deviceID = ""
}

func (b *errorBanner) refersTo(id bluetooth.DeviceID) bool {
	return b.deviceID != "" && b.deviceID == id
}
