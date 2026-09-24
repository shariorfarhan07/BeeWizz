package bluetooth

import "testing"

func TestIconCandidates_Mapping(t *testing.T) {
	cases := map[string][]string{
		"audio-headphones":  {"audio-headphones-symbolic", "audio-headset-symbolic", FallbackIcon},
		"audio-headset":     {"audio-headset-symbolic", "audio-headphones-symbolic", FallbackIcon},
		"audio-card":        {"audio-speakers-symbolic", "audio-card-symbolic", FallbackIcon},
		"input-keyboard":    {"input-keyboard-symbolic", FallbackIcon},
		"input-mouse":       {"input-mouse-symbolic", FallbackIcon},
		"input-gaming":      {"input-gaming-symbolic", FallbackIcon},
		"input-tablet":      {"input-tablet-symbolic", FallbackIcon},
		"phone":             {"phone-symbolic", FallbackIcon},
		"computer":          {"computer-symbolic", FallbackIcon},
		"camera-photo":      {"camera-photo-symbolic", FallbackIcon},
		"camera-video":      {"camera-video-symbolic", "camera-web-symbolic", FallbackIcon},
		"video-display":     {"video-display-symbolic", FallbackIcon},
		"printer":           {"printer-symbolic", FallbackIcon},
		"scanner":           {"scanner-symbolic", FallbackIcon},
		"multimedia-player": {"multimedia-player-symbolic", FallbackIcon},
		"modem":             {"modem-symbolic", FallbackIcon},
		"network-wireless":  {"network-wireless-symbolic", FallbackIcon},
	}
	for icon, want := range cases {
		got := IconCandidates(icon)
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", icon, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", icon, got, want)
			}
		}
	}
}

func TestIconCandidates_UnknownAndEmpty(t *testing.T) {
	for _, icon := range []string{"", "something-totally-unknown"} {
		got := IconCandidates(icon)
		if len(got) != 1 || got[0] != FallbackIcon {
			t.Fatalf("icon %q: got %v, want [%s]", icon, got, FallbackIcon)
		}
	}
}

func TestIconCandidates_AlwaysEndsWithFallback(t *testing.T) {
	for icon := range iconMap {
		got := IconCandidates(icon)
		if got[len(got)-1] != FallbackIcon {
			t.Fatalf("icon %q: last element = %q, want %q", icon, got[len(got)-1], FallbackIcon)
		}
	}
}
