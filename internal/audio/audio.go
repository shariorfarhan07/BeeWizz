// Package audio exposes the sound-server side of Bluetooth audio
// devices: which card profile (A2DP / HFP) is active, which output and
// input devices are the defaults, and which A2DP codec is in use.
//
// It talks to PulseAudio (or pipewire-pulse) over the native protocol;
// it never shells out to pactl. Like package bluetooth it has no GTK
// dependency, and the UI only sees the plain types defined here.
package audio

import (
	"context"
	"sort"
	"strings"
)

// Profile is one card profile, e.g. "a2dp_sink" or "handsfree_head_unit".
type Profile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	NumSinks    int    `json:"num_sinks"`
	NumSources  int    `json:"num_sources"`
}

// Kind classifies a profile by what it offers the user.
func (p Profile) Kind() ProfileKind { return ClassifyProfile(p.Name) }

// Codec is one Bluetooth codec the sound server can use for a card.
type Codec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Card is a Bluetooth audio card as seen by the sound server.
type Card struct {
	Index         uint32    `json:"index"`
	Name          string    `json:"name"`
	Address       string    `json:"address"`
	Profiles      []Profile `json:"profiles"`
	ActiveProfile string    `json:"active_profile"`

	// Codecs and CurrentCodec come from the PulseAudio bluez message
	// API. They are empty when the server doesn't support it (e.g.
	// pipewire-pulse, which encodes the codec in the profile instead).
	Codecs       []Codec `json:"codecs,omitempty"`
	CurrentCodec string  `json:"current_codec,omitempty"`
}

// Active returns the active profile, or nil.
func (c *Card) Active() *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].Name == c.ActiveProfile {
			return &c.Profiles[i]
		}
	}
	return nil
}

// CanSwitchCodec reports whether a codec choice is meaningful right
// now: codec switching is only possible in an A2DP profile, and only
// when there is more than one codec to choose from.
func (c *Card) CanSwitchCodec() bool {
	return ClassifyProfile(c.ActiveProfile) == ProfileHighQuality && len(c.Codecs) > 1
}

// BestProfile returns the available profile of the given kind with the
// most endpoints (so a duplex HFP profile beats an output-only one), or
// nil when the card has none.
func (c *Card) BestProfile(kind ProfileKind) *Profile {
	var best *Profile
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if !p.Available || p.Kind() != kind {
			continue
		}
		if best == nil || p.NumSinks+p.NumSources > best.NumSinks+best.NumSources {
			best = p
		}
	}
	return best
}

// Endpoint is a sink (output) or source (input).
type Endpoint struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CardIndex   uint32 `json:"card_index"`
	// Codec is the codec reported on the sink/source itself, if any.
	Codec string `json:"codec,omitempty"`
}

// State is a full snapshot of the sound server.
type State struct {
	Available     bool       `json:"available"`
	Server        string     `json:"server,omitempty"`
	Cards         []Card     `json:"cards"`
	Sinks         []Endpoint `json:"sinks"`
	Sources       []Endpoint `json:"sources"`
	DefaultSink   string     `json:"default_sink"`
	DefaultSource string     `json:"default_source"`
}

// CardForAddress finds the Bluetooth card of the device with the given
// MAC address (case-insensitive), or nil.
func (s *State) CardForAddress(addr string) *Card {
	if s == nil || addr == "" {
		return nil
	}
	for i := range s.Cards {
		if strings.EqualFold(s.Cards[i].Address, addr) {
			return &s.Cards[i]
		}
	}
	return nil
}

// SinksOfCard / SourcesOfCard return the endpoints that belong to card.
func (s *State) SinksOfCard(card uint32) []Endpoint   { return ofCard(s.Sinks, card) }
func (s *State) SourcesOfCard(card uint32) []Endpoint { return ofCard(s.Sources, card) }

func ofCard(eps []Endpoint, card uint32) []Endpoint {
	var out []Endpoint
	for _, e := range eps {
		if e.CardIndex == card {
			out = append(out, e)
		}
	}
	return out
}

// Controller is what the UI uses. Every method may block on IPC and
// must be called off the GTK main thread.
type Controller interface {
	// Start connects to the sound server and keeps the connection alive
	// (reconnecting with backoff) until ctx is cancelled or Close runs.
	Start(ctx context.Context) error
	// Subscribe registers fn to receive every new State snapshot. It is
	// called on a background goroutine.
	Subscribe(fn func(*State)) (unsubscribe func())
	SetProfile(ctx context.Context, card, profile string) error
	SetDefaultSink(ctx context.Context, sink string) error
	SetDefaultSource(ctx context.Context, source string) error
	// UseDeviceMicrophone switches the card to its headset profile and
	// makes the card's microphone the default input.
	UseDeviceMicrophone(ctx context.Context, card string) error
	// UseDeviceOutput switches the card to a profile with an output
	// (high quality preferred) and makes it the default output.
	UseDeviceOutput(ctx context.Context, card string) error
	SwitchCodec(ctx context.Context, card, codec string) error
	Close() error
}

// ProfileKind groups profiles by what they mean for the user.
type ProfileKind int

const (
	ProfileOther       ProfileKind = iota
	ProfileHighQuality             // A2DP: good sound, no microphone
	ProfileHeadset                 // HSP/HFP: microphone, lower quality
	ProfileOff
)

// ClassifyProfile maps PulseAudio and PipeWire profile names to a kind.
func ClassifyProfile(name string) ProfileKind {
	n := strings.ToLower(name)
	switch {
	case n == "off":
		return ProfileOff
	case strings.HasPrefix(n, "a2dp"):
		return ProfileHighQuality
	case strings.HasPrefix(n, "headset"), strings.HasPrefix(n, "handsfree"),
		strings.HasPrefix(n, "hsp"), strings.HasPrefix(n, "hfp"):
		return ProfileHeadset
	}
	return ProfileOther
}

// ProfileLabel is the short, user-facing name for a profile. PipeWire
// puts the codec in the profile description ("... codec AAC"), so it
// is preserved when present.
func ProfileLabel(p Profile) string {
	codec := codecFromDescription(p.Description)
	switch p.Kind() {
	case ProfileHighQuality:
		if codec != "" {
			return "High quality (" + codec + ")"
		}
		return "High quality playback (A2DP)"
	case ProfileHeadset:
		if codec != "" {
			return "Headset with microphone (" + codec + ")"
		}
		return "Headset with microphone (HFP)"
	case ProfileOff:
		return "Off"
	}
	if p.Description != "" {
		return p.Description
	}
	return p.Name
}

// codecFromDescription extracts "AAC" from PipeWire descriptions like
// "High Fidelity Playback (A2DP Sink, codec AAC)".
func codecFromDescription(desc string) string {
	i := strings.Index(strings.ToLower(desc), "codec ")
	if i < 0 {
		return ""
	}
	rest := desc[i+len("codec "):]
	if j := strings.IndexAny(rest, "),"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// SelectableProfiles returns the available profiles in display order:
// high quality, headset, other, off.
func SelectableProfiles(c *Card) []Profile {
	var out []Profile
	for _, p := range c.Profiles {
		if p.Available || p.Name == c.ActiveProfile {
			out = append(out, p)
		}
	}
	rank := func(k ProfileKind) int {
		switch k {
		case ProfileHighQuality:
			return 0
		case ProfileHeadset:
			return 1
		case ProfileOther:
			return 2
		}
		return 3
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i].Kind()) < rank(out[j].Kind()) })
	return out
}
