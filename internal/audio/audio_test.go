package audio

import "testing"

func TestClassifyProfile(t *testing.T) {
	cases := map[string]ProfileKind{
		"a2dp_sink":              ProfileHighQuality,
		"a2dp-sink-aac":          ProfileHighQuality, // PipeWire
		"handsfree_head_unit":    ProfileHeadset,
		"headset_head_unit":      ProfileHeadset,
		"headset-head-unit-msbc": ProfileHeadset, // PipeWire
		"off":                    ProfileOff,
		"output:analog-stereo":   ProfileOther,
	}
	for name, want := range cases {
		if got := ClassifyProfile(name); got != want {
			t.Errorf("ClassifyProfile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestProfileLabel(t *testing.T) {
	cases := []struct {
		p    Profile
		want string
	}{
		{Profile{Name: "a2dp_sink", Description: "High Fidelity Playback (A2DP Sink)"}, "High quality playback (A2DP)"},
		{Profile{Name: "a2dp-sink-aac", Description: "High Fidelity Playback (A2DP Sink, codec AAC)"}, "High quality (AAC)"},
		{Profile{Name: "headset-head-unit-msbc", Description: "Headset Head Unit (HSP/HFP, codec mSBC)"}, "Headset with microphone (mSBC)"},
		{Profile{Name: "handsfree_head_unit", Description: "Handsfree Head Unit (HFP)"}, "Headset with microphone (HFP)"},
		{Profile{Name: "off", Description: "Off"}, "Off"},
		{Profile{Name: "weird", Description: "Something"}, "Something"},
	}
	for _, c := range cases {
		if got := ProfileLabel(c.p); got != c.want {
			t.Errorf("ProfileLabel(%q) = %q, want %q", c.p.Name, got, c.want)
		}
	}
}

func TestSelectableProfilesOrderAndAvailability(t *testing.T) {
	c := &Card{
		ActiveProfile: "off",
		Profiles: []Profile{
			{Name: "off", Available: true},
			{Name: "handsfree_head_unit", Available: true},
			{Name: "headset_head_unit", Available: false},
			{Name: "a2dp_sink", Available: true},
		},
	}
	got := SelectableProfiles(c)
	want := []string{"a2dp_sink", "handsfree_head_unit", "off"}
	if len(got) != len(want) {
		t.Fatalf("got %d profiles, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("profile[%d] = %q, want %q", i, got[i].Name, want[i])
		}
	}
}

func TestBestProfilePrefersDuplex(t *testing.T) {
	c := &Card{Profiles: []Profile{
		{Name: "headset_audio_gateway", Available: true, NumSinks: 1},
		{Name: "handsfree_head_unit", Available: true, NumSinks: 1, NumSources: 1},
	}}
	if p := c.BestProfile(ProfileHeadset); p == nil || p.Name != "handsfree_head_unit" {
		t.Fatalf("BestProfile = %+v", p)
	}
	if p := c.BestProfile(ProfileHighQuality); p != nil {
		t.Fatalf("want nil, got %+v", p)
	}
}

func TestCanSwitchCodec(t *testing.T) {
	two := []Codec{{Name: "sbc"}, {Name: "sbc_xq_552"}}
	if !(&Card{ActiveProfile: "a2dp_sink", Codecs: two}).CanSwitchCodec() {
		t.Error("A2DP with two codecs should allow switching")
	}
	if (&Card{ActiveProfile: "handsfree_head_unit", Codecs: two}).CanSwitchCodec() {
		t.Error("HFP must not allow codec switching")
	}
	if (&Card{ActiveProfile: "a2dp_sink", Codecs: two[:1]}).CanSwitchCodec() {
		t.Error("a single codec is not a choice")
	}
}

func TestParseCodecResponses(t *testing.T) {
	codecs, err := ParseCodecList(`[{"name":"sbc","description":"SBC"},{"name":"aptx"},{"description":"nameless"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(codecs) != 2 || codecs[1].Description != "APTX" {
		t.Fatalf("codecs = %+v", codecs)
	}
	if _, err := ParseCodecList("not json"); err == nil {
		t.Error("want error for invalid JSON")
	}
	if got := ParseCurrentCodec(`"sbc_xq_552"`); got != "sbc_xq_552" {
		t.Errorf("current = %q", got)
	}
	if got := ParseCurrentCodec("null"); got != "" {
		t.Errorf("null current = %q", got)
	}
}
