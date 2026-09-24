package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const appCSS = `
.bw-list { background: transparent; }
.bw-row { padding: 6px 8px; border-radius: 8px; }
.bw-name { font-weight: 600; }
.bw-status, .bw-battery { font-size: 0.9em; }
.bw-status.connected { color: #26a269; }
.bw-status.error, label.error { color: #e01b24; }
.bw-battery { opacity: 0.8; }
.bw-section { font-size: 0.75em; font-weight: 700; opacity: 0.6; margin: 8px 12px 2px 12px; }
.bw-empty-title { font-size: 1.15em; font-weight: 700; }
.bw-audio-heading { font-weight: 600; font-size: 0.9em; }
.bw-audio-hint { font-size: 0.85em; }
.bw-banner { background-color: alpha(#e01b24, 0.15); border-radius: 8px; padding: 6px 8px; margin: 6px; }
`

func loadCSS() {
	p := gtk.NewCSSProvider()
	p.LoadFromData(appCSS)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), p, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
