package ui

import (
	"context"
	"fmt"
	"log/slog"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
)

// invalidListPosition is GTK_INVALID_LIST_POSITION.
const invalidListPosition = uint(0xFFFFFFFF)

// choice is one dropdown entry: the label shown and what selecting it
// does.
type choice struct {
	label string
	apply func(ctx context.Context, c audio.Controller) error
}

// selector is a labelled GtkDropDown whose entries are rebuilt from
// each audio snapshot without triggering its own change handler.
type selector struct {
	box     *gtk.Box
	dd      *gtk.DropDown
	hint    *gtk.Label
	choices []choice
	handler coreglib.SignalHandle
}

func newSelector(title string, onPick func(choice)) *selector {
	s := &selector{}
	s.box = gtk.NewBox(gtk.OrientationVertical, 4)

	l := gtk.NewLabel(title)
	l.SetXAlign(0)
	l.AddCSSClass("bw-audio-heading")
	s.box.Append(l)

	s.dd = gtk.NewDropDownFromStrings(nil)
	s.dd.SetHExpand(true)
	setA11yLabel(&s.dd.Widget, title)
	s.handler = s.dd.NotifyProperty("selected", func() {
		i := s.dd.Selected()
		if i != invalidListPosition && int(i) < len(s.choices) && s.choices[i].apply != nil {
			onPick(s.choices[i])
		}
	})
	s.box.Append(s.dd)

	s.hint = gtk.NewLabel("")
	s.hint.SetXAlign(0)
	s.hint.SetWrap(true)
	s.hint.SetWrapMode(pango.WrapWordChar)
	s.hint.SetMaxWidthChars(34)
	s.hint.AddCSSClass("dim-label")
	s.hint.AddCSSClass("bw-audio-hint")
	s.hint.SetVisible(false)
	s.box.Append(s.hint)
	return s
}

// set replaces the entries and selection. The notify handler is
// blocked so that reflecting server state never re-applies it.
func (s *selector) set(choices []choice, selected int) {
	s.choices = choices
	labels := make([]string, len(choices))
	for i, c := range choices {
		labels[i] = c.label
	}
	s.dd.HandlerBlock(s.handler)
	s.dd.SetModel(gtk.NewStringList(labels))
	if selected >= 0 {
		s.dd.SetSelected(uint(selected))
	} else {
		s.dd.SetSelected(invalidListPosition)
	}
	s.dd.HandlerUnblock(s.handler)
}

func (s *selector) setHint(text string) {
	s.hint.SetText(text)
	s.hint.SetVisible(text != "")
}

// audioPanel is the popover behind a connected audio device's
// "Audio settings" button: audio mode (profile), output device, input
// device, and codec.
type audioPanel struct {
	w       *Window
	address string
	name    string
	pop     *gtk.Popover

	mode, output, input, codec *selector

	spinner *gtk.Spinner
	errLbl  *gtk.Label
	busy    bool
	last    *audio.State
}

func newAudioPanel(w *Window, address, name string) *audioPanel {
	p := &audioPanel{w: w, address: address, name: name}
	p.pop = gtk.NewPopover()

	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)
	box.SetSizeRequest(260, -1)

	pick := func(what string) func(choice) {
		return func(c choice) { p.run(what, c.apply) }
	}
	p.mode = newSelector("Audio mode", pick("change the audio mode"))
	p.output = newSelector("Output device", pick("change the output device"))
	p.input = newSelector("Input device", pick("change the input device"))
	p.codec = newSelector("Codec", pick("switch the codec"))
	box.Append(p.mode.box)
	box.Append(p.output.box)
	box.Append(p.input.box)
	box.Append(p.codec.box)

	status := gtk.NewBox(gtk.OrientationHorizontal, 6)
	p.spinner = gtk.NewSpinner()
	p.spinner.SetVisible(false)
	status.Append(p.spinner)
	p.errLbl = gtk.NewLabel("")
	p.errLbl.SetXAlign(0)
	p.errLbl.SetWrap(true)
	p.errLbl.SetMaxWidthChars(34)
	p.errLbl.AddCSSClass("error")
	p.errLbl.SetVisible(false)
	status.Append(p.errLbl)
	box.Append(status)

	p.pop.SetChild(box)
	return p
}

// run performs a controller call off the GTK thread and re-enables the
// panel when it finishes. The next snapshot refreshes the selections;
// on failure the last snapshot is re-applied so the dropdowns snap back.
func (p *audioPanel) run(what string, fn func(ctx context.Context, c audio.Controller) error) {
	if p.busy || p.w.audio == nil {
		return
	}
	p.setBusy(true)
	p.errLbl.SetVisible(false)
	ctrl := p.w.audio
	ctx := p.w.ctx
	go func() {
		err := fn(ctx, ctrl)
		glib.IdleAdd(func() {
			p.setBusy(false)
			if err != nil {
				slog.Warn("audio operation failed", "device", p.name, "address", p.address, "error", err.Error())
				p.errLbl.SetText(fmt.Sprintf("Couldn't %s.", what))
				p.errLbl.SetVisible(true)
				p.refresh(p.last)
			}
		})
	}()
}

func (p *audioPanel) setBusy(b bool) {
	p.busy = b
	p.spinner.SetVisible(b)
	if b {
		p.spinner.Start()
	} else {
		p.spinner.Stop()
	}
	for _, s := range []*selector{p.mode, p.output, p.input} {
		s.dd.SetSensitive(!b)
	}
	p.codec.dd.SetSensitive(!b && p.codecSwitchable())
}

func (p *audioPanel) codecSwitchable() bool {
	card := p.last.CardForAddress(p.address)
	return card != nil && card.CanSwitchCodec()
}

// refresh rebuilds every selector from st.
func (p *audioPanel) refresh(st *audio.State) {
	p.last = st
	card := st.CardForAddress(p.address)
	if card == nil {
		return
	}
	cardName := card.Name

	// Audio mode.
	var modes []choice
	sel := -1
	for _, pr := range audio.SelectableProfiles(card) {
		profile := pr.Name
		if profile == card.ActiveProfile {
			sel = len(modes)
		}
		modes = append(modes, choice{
			label: audio.ProfileLabel(pr),
			apply: func(ctx context.Context, c audio.Controller) error { return c.SetProfile(ctx, cardName, profile) },
		})
	}
	p.mode.set(modes, sel)
	switch audio.ClassifyProfile(card.ActiveProfile) {
	case audio.ProfileHeadset:
		p.mode.setHint("Microphone on; sound quality is reduced.")
	case audio.ProfileHighQuality:
		p.mode.setHint("Best sound quality; the microphone is off.")
	default:
		p.mode.setHint("")
	}

	// Output device: every sink, plus this device when it has none yet.
	var outs []choice
	sel = -1
	for _, e := range st.Sinks {
		sink := e.Name
		if sink == st.DefaultSink {
			sel = len(outs)
		}
		outs = append(outs, choice{
			label: e.Description,
			apply: func(ctx context.Context, c audio.Controller) error { return c.SetDefaultSink(ctx, sink) },
		})
	}
	if len(st.SinksOfCard(card.Index)) == 0 && (card.BestProfile(audio.ProfileHighQuality) != nil || card.BestProfile(audio.ProfileHeadset) != nil) {
		outs = append(outs, choice{
			label: p.name + " (turn on audio)",
			apply: func(ctx context.Context, c audio.Controller) error { return c.UseDeviceOutput(ctx, cardName) },
		})
	}
	p.output.set(outs, sel)

	// Input device: every source, plus this device's microphone when
	// it needs a switch to headset mode first.
	var ins []choice
	sel = -1
	for _, e := range st.Sources {
		source := e.Name
		if source == st.DefaultSource {
			sel = len(ins)
		}
		ins = append(ins, choice{
			label: e.Description,
			apply: func(ctx context.Context, c audio.Controller) error { return c.SetDefaultSource(ctx, source) },
		})
	}
	micHint := ""
	if len(st.SourcesOfCard(card.Index)) == 0 {
		if hp := card.BestProfile(audio.ProfileHeadset); hp != nil && hp.NumSources > 0 {
			ins = append(ins, choice{
				label: p.name + " microphone",
				apply: func(ctx context.Context, c audio.Controller) error { return c.UseDeviceMicrophone(ctx, cardName) },
			})
			micHint = "Using this device's microphone switches it to headset mode."
		}
	}
	p.input.set(ins, sel)
	p.input.setHint(micHint)

	// Codec.
	p.codec.box.SetVisible(true)
	switch {
	case len(card.Codecs) > 0:
		var codecs []choice
		sel = -1
		for _, cd := range card.Codecs {
			codec := cd.Name
			if codec == card.CurrentCodec {
				sel = len(codecs)
			}
			codecs = append(codecs, choice{
				label: cd.Description,
				apply: func(ctx context.Context, c audio.Controller) error { return c.SwitchCodec(ctx, cardName, codec) },
			})
		}
		p.codec.set(codecs, sel)
		switch {
		case audio.ClassifyProfile(card.ActiveProfile) != audio.ProfileHighQuality:
			p.codec.setHint("Codecs can only be changed in high quality mode.")
		case len(card.Codecs) < 2:
			p.codec.setHint("This device supports only one codec.")
		default:
			p.codec.setHint("")
		}
	case currentSinkCodec(st, card) != "":
		// No codec API (e.g. PipeWire): show the codec read-only; the
		// audio mode list carries the codec choice there.
		p.codec.set([]choice{{label: currentSinkCodec(st, card)}}, 0)
		p.codec.setHint("Pick a codec through the audio mode list.")
	default:
		p.codec.box.SetVisible(false)
	}

	p.setBusy(p.busy)
}

func currentSinkCodec(st *audio.State, card *audio.Card) string {
	for _, e := range st.SinksOfCard(card.Index) {
		if e.Codec != "" {
			return e.Codec
		}
	}
	for _, e := range st.SourcesOfCard(card.Index) {
		if e.Codec != "" {
			return e.Codec
		}
	}
	return ""
}
