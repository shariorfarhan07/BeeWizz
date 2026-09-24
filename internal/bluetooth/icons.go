package bluetooth

// FallbackIcon is always the last element of IconCandidates' result.
const FallbackIcon = "bluetooth-symbolic"

var iconMap = map[string][]string{
	"audio-headphones":  {"audio-headphones-symbolic", "audio-headset-symbolic"},
	"audio-headset":     {"audio-headset-symbolic", "audio-headphones-symbolic"},
	"audio-card":        {"audio-speakers-symbolic", "audio-card-symbolic"},
	"input-keyboard":    {"input-keyboard-symbolic"},
	"input-mouse":       {"input-mouse-symbolic"},
	"input-gaming":      {"input-gaming-symbolic"},
	"input-tablet":      {"input-tablet-symbolic"},
	"phone":             {"phone-symbolic"},
	"computer":          {"computer-symbolic"},
	"camera-photo":      {"camera-photo-symbolic"},
	"camera-video":      {"camera-video-symbolic", "camera-web-symbolic"},
	"video-display":     {"video-display-symbolic"},
	"printer":           {"printer-symbolic"},
	"scanner":           {"scanner-symbolic"},
	"multimedia-player": {"multimedia-player-symbolic"},
	"modem":             {"modem-symbolic"},
	"network-wireless":  {"network-wireless-symbolic"},
}

// IconCandidates returns an ordered list of freedesktop/GTK icon names
// for the given BlueZ Icon property value. The list always ends with
// FallbackIcon. No brand-specific icons are ever returned.
func IconCandidates(bluezIcon string) []string {
	names := iconMap[bluezIcon]
	out := make([]string, 0, len(names)+1)
	out = append(out, names...)
	out = append(out, FallbackIcon)
	return out
}
